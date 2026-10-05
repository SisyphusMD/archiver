// Package runctl implements `archiver stop`, `pause` and `resume` (ADR 10): acting on the
// running backup, the running maintenance, and the copy workers (ADR 15).
//
// A Go pipeline watches its own lock: it ends gracefully when the stop flag appears and
// starts no program while the run is recorded paused, so for it these commands record the
// request and signal what is already running. A bash pipeline (a deployment still on legacy
// hooks) does neither, so it is stopped from outside as bash's own stop did.
package runctl

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

// Env is what the commands act through.
type Env struct {
	Layout layout.Layout
	Out    io.Writer
	Log    *logging.Log
	// Notify sends a notification when a notifier is configured.
	Notify func(title, msg string)
	// Workers sends one command to the daemon's copy workers; false means none run.
	Workers func(cmd string) bool
	Now     func() time.Time
}

func (e Env) stopFlag() string { return filepath.Join(e.Layout.Lock, "archiver-stop-requested") }
func (e Env) maintenanceStopFlag() string {
	return filepath.Join(e.Layout.Lock, "archiver-maintenance-stop-requested")
}

// backup reads the backup lock; ok is false when no backup runs.
func (e Env) backup() (lockstate.Lock, bool) {
	l, held, err := lockstate.ReadLock(e.Layout.BackupLock())
	if err != nil || !held || !l.Alive() {
		return lockstate.Lock{}, false
	}
	return l, true
}

// Pause pauses the copy workers and the running backup.
func Pause(e Env) int {
	if e.Workers("pause") {
		fmt.Fprintln(e.Out, "Copies to the secondary storages paused.")
	}
	l, ok := e.backup()
	if !ok {
		fmt.Fprintln(e.Out, "No running backup found.")
		return 0
	}
	if exists(e.stopFlag()) {
		fmt.Fprintln(e.Out, "Cannot pause: stop has been requested.")
		return 1
	}
	if l.Paused() {
		fmt.Fprintln(e.Out, "Backup is already paused. Use 'archiver resume' to resume.")
		return 0
	}
	fmt.Fprintln(e.Out, "Pausing backup...")
	e.Log.Message(logging.Info, "", fmt.Sprintf("Pausing backup process (PID: %d).", l.PID))
	// Recorded first: a Go pipeline starts no program once it reads "paused", and what it
	// already started is stopped below.
	if err := runlock.Append(e.Layout.BackupLock(), "paused", e.Now()); err != nil {
		fmt.Fprintln(e.Out, "Could not record the pause:", err)
		return 1
	}
	signalTree(l.PID, syscall.SIGSTOP)
	// A program the pipeline started just before it read the pause is caught here.
	time.Sleep(300 * time.Millisecond)
	signalTree(l.PID, syscall.SIGSTOP)
	active := logging.Duration(runlock.Summarize(e.reread(l)).Active)
	fmt.Fprintf(e.Out, "Backup paused. Active runtime: %s.\n", active)
	e.Log.Message(logging.Info, "", "Backup paused. Active runtime: "+active+".")
	e.Notify("Backup Paused", "Paused after "+active+" of active runtime.")
	return 0
}

// Resume resumes the copy workers and a paused backup.
func Resume(e Env) int {
	if e.Workers("resume") {
		fmt.Fprintln(e.Out, "Copies to the secondary storages resumed.")
	}
	l, ok := e.backup()
	if !ok {
		fmt.Fprintln(e.Out, "No paused backup found.")
		return 0
	}
	if !l.Paused() {
		fmt.Fprintln(e.Out, "Backup is not paused. Nothing to resume.")
		return 0
	}
	paused := logging.Duration(e.Now().Unix() - l.Events[len(l.Events)-1].At)
	fmt.Fprintln(e.Out, "Resuming backup...")
	e.Log.Message(logging.Info, "", fmt.Sprintf("Resuming backup process (PID: %d).", l.PID))
	e.resume(l)
	fmt.Fprintf(e.Out, "Backup resumed. Was paused for: %s.\n", paused)
	e.Log.Message(logging.Info, "", "Backup resumed. Was paused for: "+paused+".")
	e.Notify("Backup Resumed", "Resuming after "+paused+" of pause time.")
	return 0
}

func (e Env) resume(l lockstate.Lock) {
	signalTree(l.PID, syscall.SIGCONT)
	runlock.Append(e.Layout.BackupLock(), "running", e.Now())
}

func (e Env) reread(l lockstate.Lock) lockstate.Lock {
	if r, ok, _ := lockstate.ReadLock(e.Layout.BackupLock()); ok {
		return r
	}
	return l
}

// StopWait is how long `archiver stop` waits for a Go backup to end.
var StopWait = 2 * time.Minute

// waitForEnd waits until the backup lock is no longer held by pid, or limit passes.
func (e Env) waitForEnd(pid int, limit time.Duration) bool {
	deadline := e.Now().Add(limit)
	for e.Now().Before(deadline) {
		if l, ok := e.backup(); !ok || l.PID != pid {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

// Stop stops the running maintenance and/or backup (target backup, maintenance or all) and
// the copy workers. immediate ends them now instead of letting them finish cleanly.
func Stop(e Env, target string, immediate bool) int {
	if target == "maintenance" || target == "all" {
		e.stopMaintenance(target, immediate)
		if target == "maintenance" {
			return 0
		}
	}
	// Sent again at the end: a stopping backup may hand its copies off (waking the workers)
	// between the first stop and its own.
	if e.Workers("stop") {
		fmt.Fprintln(e.Out, "Copies to the secondary storages stopped.")
	}
	defer e.Workers("stop")

	l, ok := e.backup()
	if !ok {
		fmt.Fprintln(e.Out, "No running backup found.")
		return 0
	}
	fmt.Fprintf(e.Out, "Stopping backup (PID: %d, context: %s, stage: %s)...\n", l.PID, l.Context, l.Stage)
	e.Log.Message(logging.Info, "", fmt.Sprintf("Stop requested (PID: %d, context: %s, stage: %s).", l.PID, l.Context, l.Stage))

	if runlock.HeldByGo(e.Layout.BackupLock()) {
		touch(e.stopFlag())
		if l.Paused() {
			e.resume(l)
		}
		if immediate {
			// Its SIGTERM handling is the graceful-stop contract: post-backup hooks still run.
			signalTree(l.PID, syscall.SIGTERM)
			syscall.Kill(l.PID, syscall.SIGTERM)
		}
		fmt.Fprintln(e.Out, "Stop requested. The backup ends its current step, runs its post-backup hooks, and reports the stop.")
		// The final stop of the copy workers (deferred) must come after the backup can no
		// longer hand them its copies.
		if e.waitForEnd(l.PID, StopWait) {
			fmt.Fprintln(e.Out, "Backup stopped.")
		} else {
			fmt.Fprintf(e.Out, "The backup is still stopping after %s (its post-backup hooks may still be running).\n", StopWait)
		}
		return 0
	}

	// A bash pipeline: between services it reads the stop flag itself; inside duplicacy it
	// has to be ended from outside.
	if immediate || l.Context == "duplicacy" {
		runlock.Append(e.Layout.BackupLock(), "stopped", e.Now())
		took := logging.Duration(e.Now().Unix() - l.StartedAt())
		msg := "Stopped after " + took + "."
		fmt.Fprintln(e.Out, "Backup stopped. "+msg)
		e.Log.Message(logging.Info, "", "Backup stopped. "+msg)
		e.Notify("Backup Stopped", msg)
		sig := syscall.SIGTERM
		if l.Paused() {
			sig = syscall.SIGKILL // a stopped process cannot act on TERM
		}
		signalTree(l.PID, sig)
		syscall.Kill(l.PID, sig)
		return 0
	}
	if strings.HasPrefix(l.Context, "service:") {
		e.Log.Message(logging.Info, "", "Setting stop flag for service cleanup.")
		touch(e.stopFlag())
		if l.Paused() {
			e.resume(l)
		}
		fmt.Fprintln(e.Out, "Stop flag set. Service will complete cleanup and terminate.")
	}
	return 0
}

// stopMaintenance stops a running maintenance: it reads its stop flag every second and ends
// its current duplicacy command itself; immediate also ends its programs now.
func (e Env) stopMaintenance(target string, immediate bool) {
	m, held, err := lockstate.ReadLock(e.Layout.MaintenanceLock())
	if err != nil || !held || !m.Alive() {
		if target == "maintenance" {
			fmt.Fprintln(e.Out, "No running maintenance found.")
		}
		return
	}
	fmt.Fprintf(e.Out, "Stopping maintenance (PID: %d, stage: %s, %s)...\n", m.PID, m.Stage, m.Context)
	touch(e.maintenanceStopFlag())
	if immediate {
		runlock.Append(e.Layout.MaintenanceLock(), "stopped", e.Now())
		signalTree(m.PID, syscall.SIGTERM)
		syscall.Kill(m.PID, syscall.SIGTERM)
	}
}

// signalTree sends sig to everything pid started: a child that leads its own process group
// gets it group-wide (so a hook's own programs are included), any other child gets it
// itself and its descendants likewise. pid itself is left alone.
func signalTree(pid int, sig syscall.Signal) {
	for _, c := range children(pid) {
		if pgid, err := syscall.Getpgid(c); err == nil && pgid == c {
			syscall.Kill(-c, sig)
			continue
		}
		syscall.Kill(c, sig)
		signalTree(c, sig)
	}
}

// children lists pid's child processes from /proc.
func children(pid int) []int {
	entries, _ := os.ReadDir("/proc")
	var out []int
	for _, e := range entries {
		n, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if err != nil {
			continue
		}
		// The command name is parenthesized and may contain spaces: fields follow its ')'.
		s := string(b)
		i := strings.LastIndexByte(s, ')')
		if i < 0 {
			continue
		}
		f := strings.Fields(s[i+1:])
		if len(f) > 1 && f[1] == strconv.Itoa(pid) {
			out = append(out, n)
		}
	}
	return out
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func touch(path string) {
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		f.Close()
	}
}
