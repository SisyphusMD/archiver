package proc

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// TerminateGroupGrace lets a process group clean up before killing its survivors.
// The leader exiting does not mean its background children have finished cleanup.
func (p *Proc) TerminateGroupGrace(grace time.Duration) {
	// A paused hook must run again before TERM so its cleanup can release resources.
	p.Signal(syscall.SIGCONT)
	p.Signal(syscall.SIGTERM)
	timer := time.NewTimer(grace)
	defer timer.Stop()
	poll := time.NewTicker(20 * time.Millisecond)
	defer poll.Stop()
	for groupRunning(p.PID()) {
		select {
		case <-timer.C:
			p.Terminate(true)
			<-p.Done()
			return
		case <-poll.C:
		}
	}
	<-p.Done()
}

func groupRunning(group int) bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return syscall.Kill(-group, 0) == nil
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			continue
		}
		end := strings.LastIndexByte(string(b), ')')
		if end < 0 {
			continue
		}
		fields := strings.Fields(string(b)[end+1:])
		// Zombies no longer execute and cannot handle signals; only their parent can reap them.
		if len(fields) >= 3 && fields[2] == strconv.Itoa(group) && fields[0] != "Z" && fields[0] != "X" {
			return true
		}
	}
	return false
}
