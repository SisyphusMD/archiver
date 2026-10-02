package runlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquireAndFormat(t *testing.T) {
	dir := t.TempDir()
	path, flag := filepath.Join(dir, "main.lock"), filepath.Join(dir, "stop")
	os.WriteFile(flag, nil, 0o644)
	l, stale, err := Acquire(path, flag, "duplicacy", "pre-backup")
	if err != nil || stale {
		t.Fatalf("Acquire: %v stale=%v", err, stale)
	}
	if l.StopRequested() {
		t.Error("a stop flag left by an earlier run must not stop this one")
	}
	// A second holder in this process is refused by the kernel lock alone.
	if _, _, err := Acquire(path, flag, "duplicacy", "pre-backup"); !errors.As(err, new(*Busy)) {
		t.Fatalf("second Acquire: %v", err)
	}
	l.SetStage("service:/srv/app", "backup")
	l.Record("paused")
	l.Record("running")
	l.Record("completed")
	b, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if lines[0] != fmt.Sprintf("%d service:/srv/app backup", os.Getpid()) || len(lines) != 5 || !strings.HasSuffix(lines[1], " running") {
		t.Fatalf("lock file in bash's format:\n%s", b)
	}
	if s := l.State(); s.Stage != "backup" || len(s.Events) != 4 {
		t.Fatalf("State = %+v", s)
	}
	if s := Summarize(l.State()); s.EndState != "completed" || s.Paused != 0 {
		t.Fatalf("Summarize = %+v", s)
	}
	os.WriteFile(flag, nil, 0o644)
	if !l.StopRequested() {
		t.Error("stop flag not seen")
	}
	l.Release()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("lock file left behind")
	}
	if _, err := os.Stat(flag); !os.IsNotExist(err) {
		t.Error("stop flag left behind")
	}
	l2, _, err := Acquire(path, flag, "duplicacy", "pre-backup")
	if err != nil {
		t.Fatalf("Acquire after Release: %v", err)
	}
	l2.Release()
}

// A bash run holds no kernel lock; its live PID in the lock file must still refuse a Go
// run, and a dead one is stale.
func TestBashLockFile(t *testing.T) {
	dir := t.TempDir()
	path, flag := filepath.Join(dir, "main.lock"), filepath.Join(dir, "stop")
	os.WriteFile(path, []byte(fmt.Sprintf("%d duplicacy copy\n1700000000 running\n", os.Getppid())), 0o644)
	_, _, err := Acquire(path, flag, "duplicacy", "pre-backup")
	var busy *Busy
	if !errors.As(err, &busy) || busy.Holder.Stage != "copy" {
		t.Fatalf("live bash lock: %v", err)
	}
	os.WriteFile(path, []byte("999999999 duplicacy copy\n1700000000 running\n"), 0o644)
	l, stale, err := Acquire(path, flag, "duplicacy", "pre-backup")
	if err != nil || !stale {
		t.Fatalf("dead bash lock: %v stale=%v", err, stale)
	}
	l.Release()
}
