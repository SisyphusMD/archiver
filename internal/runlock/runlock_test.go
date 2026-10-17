package runlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
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
	// The pause lasts however long the two Record calls are apart, whole seconds.
	if s := Summarize(l.State()); s.EndState != "completed" || s.Paused < 0 || s.Active != s.End-s.Start-s.Paused {
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

// A lock still held by a run that already removed its lock file is waited for, not refused.
func TestAcquireWaitsForRelease(t *testing.T) {
	dir := t.TempDir()
	path, flag := filepath.Join(dir, "main.lock"), filepath.Join(dir, "stop")
	f, _ := os.OpenFile(path+".flock", os.O_RDWR|os.O_CREATE, 0o644)
	syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	go func() { time.Sleep(300 * time.Millisecond); f.Close() }()
	l, _, err := Acquire(path, flag, "duplicacy", "pre-backup")
	if err != nil {
		t.Fatalf("a releasing holder made Acquire fail: %v", err)
	}
	l.Release()
}

func TestPauseSerializesCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drill.lock")
	l, _, err := Acquire(path, path+".stop", "drill", "hook")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	signaling, finish := make(chan struct{}), make(chan struct{})
	paused := make(chan error, 1)
	go func() {
		_, err := Pause(path, time.Now(), func() {
			close(signaling)
			<-finish
		})
		paused <- err
	}()
	<-signaling
	cleaned := make(chan error, 1)
	go func() { cleaned <- l.Record("cleanup") }()
	select {
	case err := <-cleaned:
		t.Fatalf("cleanup overtook pause signals: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(finish)
	if err := <-paused; err != nil {
		t.Fatal(err)
	}
	if err := <-cleaned; err != nil {
		t.Fatal(err)
	}
	resumed, err := Resume(path, time.Now(), func() { t.Error("cleanup was signaled to resume") })
	if err != nil || resumed {
		t.Fatalf("cleanup resume: resumed=%v err=%v", resumed, err)
	}
	ok, err := Pause(path, time.Now(), func() { t.Error("cleanup was signaled to pause") })
	if err != nil || ok || l.State().Paused() {
		t.Fatalf("cleanup pause: paused=%v err=%v state=%+v", ok, err, l.State())
	}
	if err := l.Record("running"); err != nil {
		t.Fatal(err)
	}
	ok, err = Pause(path, time.Now(), func() {})
	if err != nil || !ok || !l.State().Paused() {
		t.Fatalf("pause after cleanup: paused=%v err=%v", ok, err)
	}
}

// A program start and a pause never interleave: a pause waits for a start in progress (so
// its signal finds the program), and once recorded no start happens.
func TestUnpausedExcludesPause(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup.lock")
	l, _, err := Acquire(path, path+".stop", "duplicacy", "backup")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	starting, finish := make(chan struct{}), make(chan struct{})
	started := make(chan bool, 1)
	go func() {
		ok, err := l.Unpaused(func() {
			close(starting)
			<-finish
		})
		if err != nil {
			t.Error(err)
		}
		started <- ok
	}()
	<-starting
	paused := make(chan bool, 1)
	go func() {
		ok, err := Pause(path, time.Now(), func() {})
		if err != nil {
			t.Error(err)
		}
		paused <- ok
	}()
	select {
	case <-paused:
		t.Fatal("pause was recorded while a program was starting")
	case <-time.After(50 * time.Millisecond):
	}
	close(finish)
	if !<-started || !<-paused {
		t.Fatal("the start and then the pause should both have happened")
	}
	ran := false
	ok, err := l.Unpaused(func() { ran = true })
	if err != nil || ok || ran {
		t.Fatalf("a start after the pause ran (ok %v, ran %v, err %v)", ok, ran, err)
	}
}
