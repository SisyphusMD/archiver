package restore

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

func TestDrillHook(t *testing.T) {
	for _, tc := range []struct {
		name, script, limit, want string
		mode                      os.FileMode
	}{
		{"pass", "exit 0", "", "", 0o700},
		{"fail", "exit 3", "", "exit 3", 0o700},
		{"timeout", "sleep 30 & wait", "20ms", "did not finish", 0o700},
		{"unsafe", "exit 0", "", "changed by users other than its owner", 0o770},
		{"environment", `test "$PWD" = "$ARCHIVER_DRILL_DIR" && test "$ARCHIVER_RESTORE_REVISION" = 7 && test "$ARCHIVER_RESTORE_STORAGE" = local && test "$ARCHIVER_SERVICE" = app && test "$ARCHIVER_SNAPSHOT_ID" = h-app && test -n "$ARCHIVER_SERVICE_DIR" && test -z "${STORAGE_PASSWORD+x}${RSA_PASSPHRASE+x}${DUPLICACY_LOCAL_PASSWORD+x}${STORAGE_TARGET_1_S3_SECRET+x}"`, "", "", 0o700},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, work := hookFixture(t, tc.script, tc.mode, tc.limit)
			got := d.hook(config.Target{Name: "local"}, "app", "h-app", 7, work)
			if tc.want == "" && got != "" || tc.want != "" && !strings.Contains(got, tc.want) {
				t.Fatalf("hook: %q, want %q", got, tc.want)
			}
		})
	}
}

func hookFixture(t *testing.T, script string, mode os.FileMode, limit string) (*drillRun, string) {
	t.Helper()
	root := t.TempDir()
	service, work := filepath.Join(root, "app"), filepath.Join(root, "scratch")
	for _, dir := range []string{service, work} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(service, "drill"), []byte("#!/bin/sh\n"+script+"\n"), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(service, "drill"), mode); err != nil {
		t.Fatal(err)
	}
	lock, _, err := runlock.Acquire(filepath.Join(root, "lock"), filepath.Join(root, "stop"), "drill", "hook")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lock.Release() })
	return &drillRun{e: &Env{cfg: &config.Config{DrillHookLimit: limit}, Environ: []string{"PATH=" + os.Getenv("PATH"), "STORAGE_PASSWORD=secret", "RSA_PASSPHRASE=secret", "DUPLICACY_LOCAL_PASSWORD=secret", "STORAGE_TARGET_1_S3_SECRET=secret", "ARCHIVER_DRILL_DIR=wrong"}}, log: &logging.Log{Dir: root, Basename: "drill"}, lock: lock, dirs: map[string]string{"app": service}}, work
}

func TestDrillHookStop(t *testing.T) {
	d, work := hookFixture(t, "touch started\nsleep 30 & wait", 0o700, "10s")
	done := make(chan string, 1)
	go func() { done <- d.hook(config.Target{Name: "local"}, "app", "h-app", 7, work) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(work, "started")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("hook did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := os.WriteFile(d.lock.StopFlag, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got != "stop requested" {
			t.Fatalf("got %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stop did not end hook")
	}
}

func TestDrillHookBeforeRun(t *testing.T) {
	d, work := hookFixture(t, "touch ran", 0o700, "")
	d.e.beforeRun = func() {
		if err := os.WriteFile(d.lock.StopFlag, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := d.hook(config.Target{Name: "local"}, "app", "h-app", 7, work); got != "stop requested" {
		t.Fatalf("got %q", got)
	}
	if _, err := os.Stat(filepath.Join(work, "ran")); !os.IsNotExist(err) {
		t.Fatal("hook started after stop in pause gate")
	}
}

func TestDrillHookTimeoutEndsChildren(t *testing.T) {
	d, work := hookFixture(t, "(sleep 0.3; touch escaped) & wait", 0o700, "30ms")
	if got := d.hook(config.Target{Name: "local"}, "app", "h-app", 7, work); !strings.Contains(got, "did not finish") {
		t.Fatalf("got %q", got)
	}
	time.Sleep(500 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(work, "escaped")); !os.IsNotExist(err) {
		t.Fatal("hook child survived timeout")
	}
}

func TestDrillHookRelativeService(t *testing.T) {
	d, work := hookFixture(t, "exit 0", 0o700, "")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	d.dirs["app"], err = filepath.Rel(cwd, d.dirs["app"])
	if err != nil {
		t.Fatal(err)
	}
	if got := d.hook(config.Target{Name: "local"}, "app", "h-app", 7, work); got != "" {
		t.Fatal(got)
	}
}

func TestDrillHookExitEndsChildren(t *testing.T) {
	for _, tc := range []struct {
		name, script, want string
	}{
		{"pass", "sleep 300 >/dev/null 2>&1 &\nexit 0", ""},
		{"fail", "sleep 300 >/dev/null 2>&1 &\nexit 3", "exit 3"},
		{"open-output", "sleep 300 &\nexit 0", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, work := hookFixture(t, "echo $$ > group\n"+tc.script, 0o700, "10s")
			got := d.hook(config.Target{Name: "local"}, "app", "h-app", 7, work)
			group := hookPID(t, filepath.Join(work, "group"))
			t.Cleanup(func() { _ = syscall.Kill(-group, syscall.SIGKILL) })
			if tc.want == "" && got != "" || tc.want != "" && !strings.Contains(got, tc.want) {
				t.Fatalf("hook: %q, want %q", got, tc.want)
			}
			deadline := time.Now().Add(time.Second)
			for hookGroupAlive(t, group) {
				if time.Now().After(deadline) {
					t.Fatal("a hook process outlived its leader")
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

func TestDrillHookPauseDoesNotUseLimit(t *testing.T) {
	d, work := hookFixture(t, "echo $$ > group\nwhile [ ! -f finish ]; do sleep 0.01; done\nsleep 0.15", 0o700, "500ms")
	done, ended := make(chan string, 1), make(chan struct{})
	go func() {
		done <- d.hook(config.Target{Name: "local"}, "app", "h-app", 7, work)
		close(ended)
	}()
	t.Cleanup(func() {
		_ = os.WriteFile(d.lock.StopFlag, nil, 0o600)
		select {
		case <-ended:
		case <-time.After(3 * time.Second):
			t.Error("hook did not end")
		}
	})
	group := hookPID(t, filepath.Join(work, "group"))
	t.Cleanup(func() { _ = syscall.Kill(-group, syscall.SIGKILL) })
	if err := d.lock.Record("paused"); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(-group, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		t.Fatalf("hook ended while paused: %q", got)
	case <-time.After(750 * time.Millisecond):
	}
	if err := os.WriteFile(filepath.Join(work, "finish"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(-group, syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	if err := d.lock.Record("running"); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got != "" {
			t.Fatalf("hook failed after resuming: %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("hook did not finish after resuming")
	}
}

func hookPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("hook did not record its PID")
	return 0
}

func hookGroupAlive(t *testing.T, group int) bool {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		end := strings.LastIndexByte(string(b), ')')
		if end < 0 {
			t.Fatalf("unreadable process state: %q", b)
		}
		fields := strings.Fields(string(b)[end+1:])
		if len(fields) < 3 {
			t.Fatalf("unreadable process state: %q", b)
		}
		if fields[2] == strconv.Itoa(group) && fields[0] != "Z" && fields[0] != "X" {
			return true
		}
	}
	return false
}
