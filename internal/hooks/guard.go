package hooks

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// GuardCommand is the hidden archiver command a restore hook runs under (see Guard).
const GuardCommand = "hook-guard"

// How long the guard waits for the kernel to finish the hook's killed processes.
const guardKill = 20 * time.Second

// Guarded returns the command that runs prog under self's hook guard, handing it lock (the
// restore lock, or nil) as file descriptor 3. The guard gets SIGTERM if the thread that
// starts it dies, so the caller keeps that goroutine on one OS thread while it runs.
func Guarded(self string, lock *os.File, prog string, args ...string) *exec.Cmd {
	argv := []string{GuardCommand}
	if lock != nil {
		argv = append(argv, "--lock")
	}
	cmd := exec.Command(self, append(append(argv, "--", prog), args...)...)
	if lock != nil {
		cmd.ExtraFiles = []*os.File{lock}
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	return cmd
}

// Guard runs a restore hook in its own process group and returns its exit code. When the
// restore that started it dies first (the guard's SIGTERM, from Guarded), it kills the
// hook's group (SIGKILL: a hook cannot ignore it) and returns only once none of it runs:
// with --lock it holds the restore lock on descriptor 3 until then, so no backup starts while what the hook ran
// in the foreground still writes to the service directory. What the hook started in a
// session of its own (a daemon) is left alone, as it would be had the hook finished.
func Guard(args []string) int {
	lock := false
	for len(args) > 0 && args[0] != "--" {
		if args[0] != "--lock" {
			fmt.Fprintf(os.Stderr, "%s: unknown option %s\n", GuardCommand, args[0])
			return 2
		}
		lock = true
		args = args[1:]
	}
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: archiver %s [--lock] -- PROGRAM [ARG...]\n", GuardCommand)
		return 2
	}
	args = args[1:]
	if lock {
		syscall.CloseOnExec(3) // the hook must not hold the lock: a service it starts would keep it
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec // G702: the restore hook Archiver chose and checked (hooks.Safe)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	attr := &syscall.SysProcAttr{Setpgid: true}
	// A hook in a group of its own is in the background of the terminal (an interactive
	// restore, or one whose answers are piped while /dev/tty stays reachable), and would stop
	// on SIGTTIN reading it; it gets the terminal while it runs.
	tty := foreground()
	if tty != nil {
		defer tty.Close()
		attr.Foreground, attr.Ctty = true, int(tty.Fd())
	}
	cmd.SysProcAttr = attr
	if err := cmd.Start(); err != nil {
		reclaim(tty) // the child may have taken the terminal before its exec failed
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return 127
		}
		return 126
	}
	pgid := cmd.Process.Pid
	done := make(chan syscall.WaitStatus, 1)
	go func() { done <- wait(pgid, tty) }()
	select {
	case ws := <-done:
		// The restore may have died as the hook exited: then what is left of its group
		// goes too, before the lock is let go.
		select {
		case <-sigs:
			end(pgid)
		default:
		}
		reclaim(tty)
		return exitCode(ws)
	case <-sigs:
		end(pgid)
		<-done
		reclaim(tty)
		return 128 + int(syscall.SIGTERM)
	}
}

// wait waits for the hook to end. A hook stopped from the terminal (Ctrl-Z) stops the
// restore's job with it, as when they shared a group: the shell sees the job stop, and on
// fg the hook gets the terminal back and continues.
func wait(pid int, tty *os.File) syscall.WaitStatus {
	for {
		var ws syscall.WaitStatus
		_, err := syscall.Wait4(pid, &ws, syscall.WUNTRACED, nil)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil || !ws.Stopped() {
			return ws
		}
		if tty == nil {
			continue
		}
		reclaim(tty)
		syscall.Kill(0, syscall.SIGTSTP) //nolint:errcheck // this process stops here until the job continues
		give(tty, pid)
		syscall.Kill(-pid, syscall.SIGCONT) //nolint:errcheck // gone already is fine
	}
}

// end kills group pgid and waits for it to be gone.
func end(pgid int) {
	syscall.Kill(-pgid, syscall.SIGKILL) //nolint:errcheck // gone already is fine
	waitGroup(pgid, guardKill)
}

// waitGroup waits up to limit for group pgid to have no running process.
func waitGroup(pgid int, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for groupRunning(pgid) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
	return true
}

// groupRunning reports whether a process of group pgid is still running; a zombie, waiting
// for whoever reaps it, writes nothing and does not count.
func groupRunning(pgid int) bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return syscall.Kill(-pgid, 0) == nil
	}
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		// pid (comm) state ppid pgrp ...; comm may hold spaces and parentheses.
		s := string(b)
		f := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
		if len(f) >= 3 && f[2] == strconv.Itoa(pgid) && f[0] != "Z" && f[0] != "X" {
			return true
		}
	}
	return false
}

// foreground opens the controlling terminal when this process's group is in its
// foreground, and returns nil otherwise (no terminal, or already in the background).
func foreground() *os.File {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	var pgrp int32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, tty.Fd(), syscall.TIOCGPGRP, uintptr(unsafe.Pointer(&pgrp))); errno != 0 || int(pgrp) != syscall.Getpgrp() {
		tty.Close()
		return nil
	}
	return tty
}

// reclaim makes this process's group the terminal's foreground group again.
func reclaim(tty *os.File) {
	give(tty, syscall.Getpgrp())
}

// give makes pgrp the terminal's foreground group.
func give(tty *os.File, pgrp int) {
	if tty == nil {
		return
	}
	// Asking for the terminal from the background is stopped by SIGTTOU otherwise.
	signal.Ignore(syscall.SIGTTOU)
	p := int32(pgrp) //nolint:gosec // G115: a process group ID is a pid_t

	syscall.Syscall(syscall.SYS_IOCTL, tty.Fd(), syscall.TIOCSPGRP, uintptr(unsafe.Pointer(&p))) //nolint:errcheck // best effort
}

// exitCode is the hook's exit code, or 128 plus the signal that ended it, as a shell reports.
func exitCode(ws syscall.WaitStatus) int {
	if ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ws.ExitStatus()
}
