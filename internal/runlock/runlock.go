// Package runlock holds a pipeline's lock. A kernel lock (flock) on a side file is the
// mutual exclusion: the kernel drops it when its holder dies, so a crashed run never blocks
// the next. The lock file records the run (PID, context and stage, then state events) for
// stop, pause, resume and status.
package runlock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/atomicfile"
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

// releaseWait is how long Acquire waits for a run that is releasing the lock.
var releaseWait = 5 * time.Second

// Busy is returned when another run holds the lock.
type Busy struct{ Holder lockstate.Lock }

func (b *Busy) Error() string { return fmt.Sprintf("held by PID %d", b.Holder.PID) }

// Acquire takes the lock for a run starting in context and stage. stale reports a lock
// file left by a run that died, which Acquire replaced.
func Acquire(path, stopFlag, context, stage string) (l *Lock, stale bool, err error) {
	f, err := os.OpenFile(path+".flock", os.O_RDWR|os.O_CREATE, 0o644) //nolint:gosec // G302: deliberate mode: owner-only, or a log or lock that is not secret
	if err != nil {
		return nil, false, err
	}
	// A run releases by removing its lock file and then dropping the kernel lock, so a lock
	// held with no live holder in the file is being released: wait for it briefly rather
	// than refusing a backup over a run that has already finished.
	deadline := time.Now().Add(releaseWait)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			f.Close()
			return nil, false, err
		}
		holder, ok, _ := lockstate.ReadLock(path)
		if (ok && holder.Alive()) || time.Now().After(deadline) {
			f.Close()
			return nil, false, &Busy{Holder: holder}
		}
		time.Sleep(50 * time.Millisecond)
	}
	// A live PID in the lock file excludes too, though every holder also takes the flock.
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
	return atomicfile.Write(path, []byte(content), 0o644)
}

// edit runs fn holding the kernel lock that serializes edits of the lock file at path: the
// owner rewrites the file to record its stage while `archiver pause` and `resume` append
// to it, and an append between the owner's read and rename would otherwise be lost.
func edit(path string, fn func() error) error {
	f, err := os.OpenFile(path+".edit", os.O_RDWR|os.O_CREATE, 0o644) //nolint:gosec // G302: deliberate mode: owner-only, or a log or lock that is not secret
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	return fn()
}

// SetStage records where the run is, for stop, pause and status.
func (l *Lock) SetStage(context, stage string) error {
	return edit(l.Path, func() error {
		b, err := os.ReadFile(l.Path)
		if err != nil {
			return err
		}
		_, rest, _ := strings.Cut(string(b), "\n")
		return writeAtomic(l.Path, fmt.Sprintf("%d %s %s\n%s", l.pid, context, stage, rest))
	})
}

// Record appends a state event (completed, stopped).
func (l *Lock) Record(state string) error { return Append(l.Path, state, l.now()) }

// Append appends a state event to the lock file at path, as the run's owner or as another
// process (`archiver pause` records "paused" for the run).
func Append(path, state string, at time.Time) error {
	return edit(path, func() error {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644) //nolint:gosec // G302: deliberate mode: owner-only, or a log or lock that is not secret
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(f, "%d %s\n", at.Unix(), state)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		return err
	})
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

// Exclusive waits until it holds the kernel lock at path, or ctx ends, and returns its
// release. It serializes work that must never overlap within the container, such as
// creating a storage, without a lock file anyone reads.
func Exclusive(ctx context.Context, path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644) //nolint:gosec // G302: deliberate mode: owner-only, or a log or lock that is not secret
	if err != nil {
		return nil, err
	}
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { f.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// Hold takes the kernel lock at path without waiting and keeps it until the returned
// file is closed or the process ends; ok is false when another process holds it.
func Hold(path string) (f *os.File, ok bool, err error) {
	f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644) //nolint:gosec // G302: deliberate mode: owner-only, or a log or lock that is not secret
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return f, true, nil
}
