package deepcheck

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/inuse"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/notify"
	"github.com/SisyphusMD/archiver/internal/resume"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

func TestProgressAndIncidents(t *testing.T) {
	for _, kind := range []string{"pass", "fail", "empty", "timeout", "backup", "signal", "stop", "kill"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			l := layout.Layout{Root: root, Lock: filepath.Join(root, "locks")}
			for _, dir := range []string{l.LogDir(), l.Lock} {
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			target := config.Target{Name: "local", Type: "local", Values: config.Values{"LOCAL_PATH": "/unused"}}
			cfg := &config.Config{Targets: []config.Target{target}, DeepCheckTimeout: "5s", Threads: "1"}
			if kind == "timeout" {
				cfg.DeepCheckTimeout = "200ms"
			}
			repo := RepoDir(l.LogDir(), target, target)
			cache := filepath.Join(repo, ".duplicacy", "cache", "local")
			if err := os.MkdirAll(cache, 0o700); err != nil {
				t.Fatal(err)
			}
			verified := filepath.Join(cache, "verified_chunks")
			keep := filepath.Join(cache, "unrelated")
			if err := os.WriteFile(keep, []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(root, "duplicacy")
			script := `#!/bin/sh
case "$*" in *-no-script*check*-all*-chunks*-fossils*-persist*) ;; *) exit 9;; esac
mkdir -p .duplicacy/cache/local
printf '%s' $$ > .duplicacy/cache/local/pid
printf '{}' > .duplicacy/cache/local/verified_chunks
sleep 0.1
`
			switch kind {
			case "fail":
				script += "echo 'Failed to decrypt the chunk " + strings.Repeat("a", 64) + ": damaged'\nexit 1\n"
			case "empty":
				script += "echo 'Chunk " + strings.Repeat("b", 64) + " has a size of 0'\nexit 1\n"
			case "timeout", "backup", "signal", "stop", "kill":
				script += "trap 'exit 130' INT\nwhile :; do sleep 0.05; done\n"
			}
			if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			sigs := make(chan os.Signal, 1)
			run := &Run{Layout: l, Duplicacy: bin, Environ: os.Environ(), Stdout: io.Discard, Stderr: io.Discard, Signals: sigs}
			n := &notify.Notifier{Incidents: l.Incidents()}
			if kind != "empty" {
				report(n, "local", Result{Message: "previous failure"})
			}
			done := make(chan bool, 1)
			go func() { done <- run.one(cfg, target) }()
			deadline := time.Now().Add(3 * time.Second)
			for {
				if _, err := os.Stat(verified); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("check did not start")
				}
				time.Sleep(10 * time.Millisecond)
			}
			switch kind {
			case "backup":
				lock, _, err := runlock.Acquire(l.BackupLock(), filepath.Join(l.Lock, "stop"), "backup", "starting")
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Release()
			case "kill":
				b, err := os.ReadFile(filepath.Join(cache, "pid"))
				if err != nil {
					t.Fatal(err)
				}
				pid, err := strconv.Atoi(string(b))
				if err != nil {
					t.Fatal(err)
				}
				if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
					t.Fatal(err)
				}
			case "signal":
				sigs <- os.Interrupt
			case "stop":
				if err := os.WriteFile(l.DeepCheckStopFlag("local"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var ok bool
			select {
			case ok = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("check did not end")
			}
			if ok != (kind == "pass") {
				t.Fatalf("result %v", ok)
			}
			_, err := os.Stat(verified)
			completed := kind == "pass" || kind == "fail" || kind == "empty"
			if completed && !os.IsNotExist(err) {
				t.Fatal("completed check kept verified chunks")
			}
			if !completed && err != nil {
				t.Fatal("interrupted check lost verified chunks")
			}
			if _, err := os.Stat(keep); err != nil {
				t.Fatal("cleared unrelated cache")
			}
			result := Read(l)["local"]
			if result.OK != ok || result.At == 0 || result.Seconds <= 0 || (ok && result.LastSuccess == 0) {
				t.Fatalf("bad state: %+v", result)
			}
			_, open := notify.OpenIncidents(l.Incidents())["deep-check:local"]
			if kind == "empty" {
				b, err := os.ReadFile(l.Incidents())
				if err != nil {
					t.Fatal(err)
				}
				var incidents map[string]struct {
					Message string `json:"message"`
				}
				if err := json.Unmarshal(b, &incidents); err != nil {
					t.Fatal(err)
				}
				if msg := incidents["deep-check:local"].Message; !strings.Contains(msg, strings.Repeat("b", 64)) {
					t.Fatalf("empty chunk missing from incident: %s", msg)
				}
			}
			if open == ok {
				t.Fatal("incident was not raised/cleared")
			}
			_, pending := resume.Read(l.RunRecord("deep-check-local"))
			if pending == ok {
				t.Fatal("due record was not retained/cleared")
			}
		})
	}
}

func TestDiagnosticsBounded(t *testing.T) {
	var d diagnostics
	id := strings.Repeat("a", 64)
	for range 100 {
		_, _ = d.Write([]byte("Verified chunk " + id + " (1/100)\n"))
	}
	_, _ = d.Write([]byte("Chunk " + id + " referenced by snapshot host-app at revision 3 does not exist\nSome chunks referenced by snapshot host-app at revision 3 are missing\n"))
	for range 100 {
		_, _ = d.Write([]byte("Failed to decrypt the chunk " + id + ": damage\n"))
	}
	if len(d.lines) != 2 || !strings.Contains(d.describe(), id) || !strings.Contains(d.describe(), "revision 3") {
		t.Fatalf("bad diagnostics: %s", d.describe())
	}
}

func TestRepoIdentity(t *testing.T) {
	a := config.Target{Name: "offsite", Type: "local", Values: config.Values{"LOCAL_PATH": "/a"}}
	b := config.Target{Name: "offsite", Type: "local", Values: config.Values{"LOCAL_PATH": "/b"}}
	if RepoDir("/logs", a, a) == RepoDir("/logs", a, b) {
		t.Fatal("repointed storage inherited cache")
	}
}

func TestDiagnosticsKeepLateRevisions(t *testing.T) {
	var d diagnostics
	for i := range 8 {
		id := strings.Repeat(strconv.Itoa(i), 64)
		_, _ = d.Write([]byte("Failed to decrypt the chunk " + id + ": damaged\n"))
	}
	for i := range 8 {
		_, _ = d.Write([]byte("Some chunks referenced by snapshot host-app at revision " + strconv.Itoa(i+1) + " are missing\n"))
	}
	if d.chunks != 3 || d.revisions != 3 || !strings.Contains(d.describe(), "revision 1") {
		t.Fatalf("unbounded or lost revisions: %+v", d)
	}
}

func TestHoldSurvivesControllerDeath(t *testing.T) {
	if root := os.Getenv("ARCHIVER_DEEP_CHECK_HELPER"); root != "" {
		l := layout.Layout{Root: root, Lock: filepath.Join(root, "locks")}
		target := config.Target{Name: "local", Type: "local", Values: config.Values{"LOCAL_PATH": "/unused"}}
		cfg := &config.Config{Targets: []config.Target{target}, DeepCheckTimeout: "1m", Threads: "1"}
		r := &Run{Layout: l, Duplicacy: filepath.Join(root, "duplicacy"), Environ: os.Environ(), Stdout: io.Discard, Stderr: io.Discard}
		if !r.one(cfg, target) {
			os.Exit(1)
		}
		return
	}
	root := t.TempDir()
	l := layout.Layout{Root: root, Lock: filepath.Join(root, "locks")}
	for _, dir := range []string{l.Lock, l.LogDir()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	script := "#!/bin/sh\nprintf '%s' $$ > '" + filepath.Join(root, "pid") + "'\nexec sleep 60\n"
	if err := os.WriteFile(filepath.Join(root, "duplicacy"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	controller := exec.Command(bin, "-test.run=^TestHoldSurvivesControllerDeath$")
	controller.Env = append(os.Environ(), "ARCHIVER_DEEP_CHECK_HELPER="+root)
	if err := controller.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Process.Kill(); _ = controller.Wait() }()
	deadline := time.Now().Add(5 * time.Second)
	var pid int
	for {
		b, err := os.ReadFile(filepath.Join(root, "pid"))
		if err == nil {
			pid, _ = strconv.Atoi(string(b))
			if pid > 0 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("duplicacy child did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	if err := controller.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = controller.Wait()
	held, err := inuse.Read(l.InUseDir(), "local")
	if err != nil {
		t.Fatal(err)
	}
	if !held.Has("any-snapshot", 1) || !held.Has("new-snapshot", 100) {
		t.Fatal("prune protection lost while duplicacy child is still running")
	}
	lock, acquired, err := runlock.Hold(l.DeepCheckLock("local") + ".flock")
	if err != nil {
		t.Fatal(err)
	}
	if acquired {
		lock.Close()
		t.Fatal("deep-check run lock lost while duplicacy child is still running")
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for {
		lock, acquired, err = runlock.Hold(l.DeepCheckLock("local") + ".flock")
		if err != nil {
			t.Fatal(err)
		}
		if acquired {
			lock.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("deep-check run lock not released after child exit")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
