package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// The test binary stands in for archiver when started as the guard.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == GuardCommand {
		os.Exit(Guard(os.Args[2:]))
	}
	os.Exit(m.Run())
}

func guard(t *testing.T, lock *os.File, script string) *exec.Cmd {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := Guarded(self, lock, "sh", "-c", script)
	cmd.SysProcAttr = nil // the test, not a dying parent, signals it
	cmd.Dir = t.TempDir()
	return cmd
}

func waitFile(t *testing.T, path string) string {
	t.Helper()
	for i := 0; i < 100; i++ {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return strings.TrimSpace(string(b))
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", path)
	return ""
}

func running(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	s := string(b)
	f := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
	return len(f) > 0 && f[0] != "Z"
}

func TestGuardPassesTheHooksExitCode(t *testing.T) {
	cmd := guard(t, nil, "exit 3")
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 3 {
		t.Fatalf("exit = %v, want 3", err)
	}
}

func TestGuardHoldsTheLockUntilTheHooksForegroundIsGone(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "restore.lock")
	lock, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	fg, daemon := filepath.Join(dir, "fg"), filepath.Join(dir, "daemon")
	// A foreground child that ignores SIGTERM, and a daemon in a session of its own.
	cmd := guard(t, lock, `setsid sh -c 'echo $$ > `+daemon+`; exec sleep 30' &
sh -c 'trap "" TERM; echo $$ > `+fg+`; while :; do sleep 0.1; done'`)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	lock.Close() // now only the guard holds it, as when the restore has died
	fgPID, _ := strconv.Atoi(waitFile(t, fg))
	daemonPID, _ := strconv.Atoi(waitFile(t, daemon))
	defer syscall.Kill(daemonPID, syscall.SIGKILL) //nolint:errcheck // cleanup
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	probe, err := os.OpenFile(lockPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	for {
		select {
		case err := <-done:
			if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 128+int(syscall.SIGTERM) {
				t.Fatalf("guard exit = %v, want %d", err, 128+int(syscall.SIGTERM))
			}
			if running(fgPID) {
				t.Fatal("the guard returned while the hook's foreground child still ran")
			}
			if !running(daemonPID) {
				t.Fatal("the guard ended a daemon the hook started in its own session")
			}
			if err := syscall.Flock(int(probe.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
				t.Fatalf("the lock was still held after the guard exited: %v", err)
			}
			return
		case <-time.After(50 * time.Millisecond):
			if running(fgPID) {
				if err := syscall.Flock(int(probe.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
					t.Fatal("the lock was free while the hook's foreground child still ran")
				}
			}
		}
	}
}

func TestGuardKeepsTheLockFromTheHook(t *testing.T) {
	dir := t.TempDir()
	lock, err := os.Create(filepath.Join(dir, "restore.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	out := filepath.Join(dir, "fd3")
	if err := guard(t, lock, "readlink /proc/$$/fd/3 > "+out+" || echo none > "+out).Run(); err != nil {
		t.Fatal(err)
	}
	if got := waitFile(t, out); strings.Contains(got, "restore.lock") {
		t.Fatalf("the hook inherited the restore lock: fd 3 = %s", got)
	}
}

// pty opens a new pseudo-terminal, returning its master and slave.
func pty(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skip("no pseudo-terminals:", err)
	}
	var unlock int32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); errno != 0 {
		t.Fatal(errno)
	}
	var n uint32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n))); errno != 0 {
		t.Fatal(errno)
	}
	slave, err := os.OpenFile("/dev/pts/"+strconv.Itoa(int(n)), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	return master, slave
}

func TestGuardGivesTheHookTheTerminalWhenStdinIsPiped(t *testing.T) {
	master, slave := pty(t)
	defer master.Close()
	defer slave.Close()
	out := filepath.Join(t.TempDir(), "answer")
	cmd := guard(t, nil, "read answer </dev/tty; echo \"$answer\" > "+out)
	// The guard as a restore with a terminal of its own and piped answers on stdin.
	cmd.Stdout, cmd.Stderr = slave, slave
	cmd.ExtraFiles = []*os.File{slave}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 3}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	time.Sleep(300 * time.Millisecond)
	master.Write([]byte("typed\n")) //nolint:errcheck // checked by the answer
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		cmd.Process.Kill() //nolint:errcheck // cleanup
		t.Fatal("the hook never read the terminal (stopped in the background?)")
	}
	if got := waitFile(t, out); got != "typed" {
		t.Fatalf("answer = %q", got)
	}
}

func TestGuardContinuesAHookStoppedFromTheTerminal(t *testing.T) {
	master, slave := pty(t)
	defer master.Close()
	defer slave.Close()
	out := filepath.Join(t.TempDir(), "resumed")
	// The guard leads its session here, an orphaned group whose own stop the kernel ignores:
	// it must still hand the terminal back and continue the hook, not leave it stopped.
	cmd := guard(t, nil, "kill -TSTP $$; echo resumed > "+out)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.ExtraFiles = []*os.File{slave}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 3}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		cmd.Process.Kill() //nolint:errcheck // cleanup
		t.Fatal("the stopped hook was never continued")
	}
	if got := waitFile(t, out); got != "resumed" {
		t.Fatalf("hook wrote %q", got)
	}
}
