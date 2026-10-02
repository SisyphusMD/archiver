// Package runlock holds a pipeline's lock. A kernel lock (flock) on a side file is the
// mutual exclusion: the kernel drops it when its holder dies, so a crashed run never blocks
// the next. The lock file itself keeps the bash format (PID, context and stage, then state
// events), because the bash stop, pause, resume and status commands read it, and a bash run
// (a deployment not yet on the Go pipeline) is still refused by its PID being alive.
package runlock

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/lockstate"
)

// Lock is a held pipeline lock.
type Lock struct {
	Path     string // e.g. /var/lock/archiver-main.lock
	StopFlag string // e.g. /var/lock/archiver-stop-requested
	flock    *os.File
	pid      int
	now      func() time.Time
}

// Busy is returned when another run holds the lock.
type Busy struct{ Holder lockstate.Lock }

func (b *Busy) Error() string { return fmt.Sprintf("held by PID %d", b.Holder.PID) }

// Acquire takes the lock for a run starting in context and stage. stale reports a lock
// file left by a run that died, which Acquire replaced.
func Acquire(path, stopFlag, context, stage string) (l *Lock, stale bool, err error) {
	f, err := os.OpenFile(path+".flock", os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		holder, _, _ := lockstate.ReadLock(path)
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, &Busy{Holder: holder}
		}
		return nil, false, err
	}
	// A bash run holds no flock; its live PID in the lock file is what excludes it.
	if prev, ok, _ := lockstate.ReadLock(path); ok {
		if prev.PID != os.Getpid() && prev.Alive() {
			f.Close()
			return nil, false, &Busy{Holder: prev}
		}
		stale = true
	}
	l = &Lock{Path: path, StopFlag: stopFlag, flock: f, pid: os.Getpid(), now: time.Now}
	os.Remove(stopFlag)
	content := fmt.Sprintf("%d %s %s\n%d running\n", l.pid, context, stage, l.now().Unix())
	if err := writeAtomic(path, content); err != nil {
		l.Release()
		return nil, false, err
	}
	return l, stale, nil
}

func writeAtomic(path, content string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SetStage records where the run is, for stop, pause and status.
func (l *Lock) SetStage(context, stage string) error {
	b, err := os.ReadFile(l.Path)
	if err != nil {
		return err
	}
	_, rest, _ := strings.Cut(string(b), "\n")
	return writeAtomic(l.Path, fmt.Sprintf("%d %s %s\n%s", l.pid, context, stage, rest))
}

// Record appends a state event (completed, stopped).
func (l *Lock) Record(state string) error {
	f, err := os.OpenFile(l.Path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%d %s\n", l.now().Unix(), state)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// StopRequested reports whether `archiver stop` asked this run to end.
func (l *Lock) StopRequested() bool {
	_, err := os.Stat(l.StopFlag)
	return err == nil
}

// State reads the lock file back.
func (l *Lock) State() lockstate.Lock {
	s, _, _ := lockstate.ReadLock(l.Path)
	return s
}

// Release removes the lock file and the stop flag, then drops the kernel lock.
func (l *Lock) Release() {
	os.Remove(l.Path)
	os.Remove(l.StopFlag)
	if l.flock != nil {
		l.flock.Close()
		l.flock = nil
	}
}

// Summary is the run's timing from its state events: total, paused and active seconds,
// and the final state.
type Summary struct {
	Start, End     int64
	Paused, Active int64
	EndState       string
}

// Summarize totals a lock's events as log_lockfile_summary does.
func Summarize(s lockstate.Lock) Summary {
	var sum Summary
	if len(s.Events) == 0 {
		return sum
	}
	sum.Start = s.Events[0].At
	last := s.Events[len(s.Events)-1]
	sum.End, sum.EndState = last.At, last.State
	var pauseStart int64 = -1
	for _, e := range s.Events {
		switch {
		case e.State == "paused":
			pauseStart = e.At
		case e.State == "running" && pauseStart >= 0:
			sum.Paused += e.At - pauseStart
			pauseStart = -1
		}
	}
	if pauseStart >= 0 {
		sum.Paused += time.Now().Unix() - pauseStart
	}
	sum.Active = sum.End - sum.Start - sum.Paused
	return sum
}

// PID is the holder's process ID, for messages.
func (l *Lock) PID() string { return strconv.Itoa(l.pid) }
