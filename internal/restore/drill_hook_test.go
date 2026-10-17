package restore

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/runctl"
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
	l := layout.Layout{Lock: root}
	lock, _, err := runlock.Acquire(l.DrillLock(), l.DrillStopFlag(), "drill", "hook")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lock.Release() })
	return &drillRun{e: &Env{Layout: l, cfg: &config.Config{DrillHookLimit: limit}, Environ: []string{"PATH=" + os.Getenv("PATH"), "STORAGE_PASSWORD=secret", "RSA_PASSPHRASE=secret", "DUPLICACY_LOCAL_PASSWORD=secret", "STORAGE_TARGET_1_S3_SECRET=secret", "ARCHIVER_DRILL_DIR=wrong"}}, log: &logging.Log{Dir: root, Basename: "drill"}, lock: lock, dirs: map[string]string{"app": service}}, work
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

func TestDrillHookStopWhilePaused(t *testing.T) {
	d, work := hookFixture(t, "touch ran", 0o700, "")
	if err := d.lock.Record("paused"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.lock.StopFlag, nil, 0o600); err != nil {
		t.Fatal(err)
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

func TestDrillHookTermCleanup(t *testing.T) {
	for _, stop := range []bool{false, true} {
		t.Run(strconv.FormatBool(stop), func(t *testing.T) {
			d, work := hookFixture(t, "trap 'sleep 0.15; test -d \"$ARCHIVER_DRILL_DIR\" && touch cleaned; exit 0' TERM\necho $$ > group\nsleep 30 & wait", 0o700, "200ms")
			done := make(chan string, 1)
			go func() { done <- d.hook(config.Target{Name: "local"}, "app", "h-app", 7, work) }()
			group := hookPID(t, filepath.Join(work, "group"))
			t.Cleanup(func() { _ = syscall.Kill(-group, syscall.SIGKILL) })
			if stop {
				if err := os.WriteFile(d.lock.StopFlag, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case got := <-done:
				want := "did not finish"
				if stop {
					want = "stop requested"
				}
				if !strings.Contains(got, want) {
					t.Fatalf("got %q, want %q", got, want)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("hook did not stop")
			}
			if _, err := os.Stat(filepath.Join(work, "cleaned")); err != nil {
				t.Fatalf("TERM cleanup did not finish before hook returned: %v", err)
			}
		})
	}
}

func TestDrillHookGraceKillsSurvivors(t *testing.T) {
	for _, leaderExits := range []bool{false, true} {
		t.Run(strconv.FormatBool(leaderExits), func(t *testing.T) {
			trap := "trap '' TERM"
			if leaderExits {
				trap = "trap 'exit 0' TERM"
			}
			d, work := hookFixture(t, trap+"\n(sh -c 'trap \"\" TERM; echo ready > child-ready; while :; do sleep 1; done') >/dev/null 2>&1 &\nwhile [ ! -f child-ready ]; do sleep 0.01; done\necho $$ > group\nwait", 0o700, "150ms")
			d.e.cfg.DrillHookGrace = "150ms"
			started := time.Now()
			got := d.hook(config.Target{Name: "local"}, "app", "h-app", 7, work)
			group := hookPID(t, filepath.Join(work, "group"))
			t.Cleanup(func() { _ = syscall.Kill(-group, syscall.SIGKILL) })
			if !strings.Contains(got, "did not finish") {
				t.Fatalf("got %q", got)
			}
			elapsed := time.Since(started)
			if elapsed < 300*time.Millisecond || elapsed > 3*time.Second {
				t.Fatalf("cleanup did not respect grace: %s", elapsed)
			}
			deadline := time.Now().Add(time.Second)
			for hookGroupAlive(t, group) {
				if time.Now().After(deadline) {
					t.Fatal("TERM-ignoring descendant survived KILL")
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

func TestDrillHookCleanupCannotBePaused(t *testing.T) {
	for _, stopPaused := range []bool{false, true} {
		t.Run(strconv.FormatBool(stopPaused), func(t *testing.T) {
			limit := "200ms"
			if stopPaused {
				limit = "5s"
			}
			d, work := hookFixture(t, "trap 'touch cleaning; sleep 0.6; touch cleaned; exit 0' TERM\necho $$ > group\nsleep 30 & wait", 0o700, limit)
			d.e.cfg.DrillHookGrace = "2s"
			done := make(chan string, 1)
			go func() { done <- d.hook(config.Target{Name: "local"}, "app", "h-app", 7, work) }()
			group := hookPID(t, filepath.Join(work, "group"))
			t.Cleanup(func() { _ = syscall.Kill(-group, syscall.SIGKILL) })
			e := runctl.Env{Layout: d.e.Layout, Out: io.Discard, Log: d.log, Now: time.Now,
				Workers: func(string) bool { return false }, Notify: func(string, string) {}}
			if stopPaused {
				if runctl.Pause(e) != 0 || !d.lock.State().Paused() {
					t.Fatal("drill was not paused")
				}
				if runctl.Stop(e, "drill", false) != 0 {
					t.Fatal("stop failed")
				}
			}
			deadline := time.Now().Add(3 * time.Second)
			for {
				if _, err := os.Stat(filepath.Join(work, "cleaning")); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("TERM cleanup did not start")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !stopPaused {
				for range 2 {
					if runctl.Pause(e) != 0 || d.lock.State().Paused() {
						t.Fatal("pause froze hook cleanup")
					}
				}
			}
			select {
			case got := <-done:
				want := "did not finish"
				if stopPaused {
					want = "stop requested"
				}
				if !strings.Contains(got, want) {
					t.Fatalf("got %q, want %q", got, want)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("hook did not stop")
			}
			if _, err := os.Stat(filepath.Join(work, "cleaned")); err != nil {
				t.Fatalf("TERM cleanup did not finish: %v", err)
			}
		})
	}
}

// A pause that lands while the hook is about to start must hold it back: the start and the
// pause share the lock file's edit lock, so the hook cannot slip in after the pause's
// signals went out.
func TestDrillHookHeldBackByAPauseAtItsStart(t *testing.T) {
	d, work := hookFixture(t, "touch ran", 0o700, "")
	edit, err := os.OpenFile(d.lock.Path+".edit", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer edit.Close()
	if err := syscall.Flock(int(edit.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	done := make(chan string, 1)
	go func() { done <- d.hook(config.Target{Name: "local"}, "app", "h-app", 7, work) }()
	time.Sleep(500 * time.Millisecond)
	// What `archiver pause` does under the edit lock: record the pause (its signals find no
	// hook, so nothing may start after this).
	f, err := os.OpenFile(d.lock.Path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(strconv.FormatInt(time.Now().Unix(), 10) + " paused\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := syscall.Flock(int(edit.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	if _, err := os.Stat(filepath.Join(work, "ran")); !os.IsNotExist(err) {
		t.Fatal("the hook started although the drill was paused")
	}
	if err := d.lock.Record("running"); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got != "" {
			t.Fatalf("hook: %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the hook did not run after resume")
	}
	if _, err := os.Stat(filepath.Join(work, "ran")); err != nil {
		t.Fatal("the hook did not run after resume")
	}
}
