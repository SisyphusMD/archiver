// Package logview implements `archiver logs`: follow the running backup's log until the
// backup ends. It reads in-process rather than wrapping tail(1), so an interrupt ends it
// at once instead of leaving a background tail behind.
package logview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
)

// Exit statuses.
const (
	Done        = 0   // the backup ended
	NoBackup    = 1   // nothing to follow
	Interrupted = 130 // the viewer was interrupted, as a shell reports SIGINT
)

// Options tunes timing; the zero value is the production behavior.
type Options struct {
	Poll      time.Duration // how often to look for new output (100ms)
	FindFor   time.Duration // how long to wait for a backup to appear (2s)
	Linger    time.Duration // how long to keep reading after the backup ends (1s)
	TailLines int           // lines of existing log shown first (10)
}

func (o Options) withDefaults() Options {
	if o.Poll == 0 {
		o.Poll = 100 * time.Millisecond
	}
	if o.FindFor == 0 {
		o.FindFor = 2 * time.Second
	}
	if o.Linger == 0 {
		o.Linger = time.Second
	}
	if o.TailLines == 0 {
		o.TailLines = 10
	}
	return o
}

// Follow shows the backup log and returns an exit status. ctx ends the view early.
func Follow(ctx context.Context, w io.Writer, l layout.Layout, o Options) int {
	o = o.withDefaults()

	lock, found, err := waitForLock(ctx, l, o)
	if ctx.Err() != nil {
		return Interrupted
	}
	if err != nil || !found {
		fmt.Fprintln(w, "No running backup found (for maintenance output use 'docker logs' or logs/maintenance.log).")
		return NoBackup
	}
	// A lock whose holder is not up yet belongs to a backup still starting: wait for the log
	// it rotates in, so the previous run's log is not shown as this one.
	if !lock.Alive() {
		for !startedSince(l.BackupLog(), lock.StartedAt()) {
			if !sleep(ctx, o.Poll) {
				return Interrupted
			}
		}
	}

	if logo, err := os.ReadFile(l.Logo()); err == nil {
		_, _ = w.Write(logo)
		if len(logo) > 0 && logo[len(logo)-1] != '\n' {
			fmt.Fprintln(w)
		}
	}

	f, id, err := openFromTail(l.BackupLog(), o.TailLines)
	if err != nil {
		fmt.Fprintf(w, "archiver: cannot read %s: %v\n", l.BackupLog(), err)
		return NoBackup
	}
	defer func() { f.Close() }()

	for {
		_, _ = io.Copy(w, f)
		if !exists(l.BackupLock()) {
			if !sleep(ctx, o.Linger) {
				return Interrupted
			}
			_, _ = io.Copy(w, f)
			fmt.Fprintln(w, "\nBackup completed. Exiting log viewer.")
			return Done
		}
		if cur, err := linkID(l.BackupLog()); err == nil && cur != id {
			fmt.Fprintln(w, "Log file has changed. Following the new log file...")
			f.Close()
			if f, id, err = openFromTail(l.BackupLog(), o.TailLines); err != nil {
				fmt.Fprintf(w, "archiver: cannot read %s: %v\n", l.BackupLog(), err)
				return NoBackup
			}
		}
		if !sleep(ctx, o.Poll) {
			return Interrupted
		}
	}
}

func waitForLock(ctx context.Context, l layout.Layout, o Options) (lockstate.Lock, bool, error) {
	deadline := time.Now().Add(o.FindFor)
	for {
		lock, found, err := lockstate.ReadLock(l.BackupLock())
		if err != nil || found {
			return lock, found, err
		}
		if time.Now().After(deadline) || !sleep(ctx, o.Poll) {
			return lockstate.Lock{}, false, nil
		}
	}
}

// openFromTail opens the log (following the symlink) positioned at its last n lines, and
// returns the identity of the link itself, which the backup replaces at each rotation.
func openFromTail(path string, n int) (*os.File, uint64, error) {
	id, err := linkID(path)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	off, err := tailOffset(f, n)
	if err == nil {
		_, err = f.Seek(off, io.SeekStart)
	}
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, id, nil
}

// tailOffset is where the last n lines of f begin.
func tailOffset(f *os.File, n int) (int64, error) {
	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := fi.Size()
	const block = 16 << 10
	newlines := 0
	for end := size; end > 0; {
		start := end - block
		if start < 0 {
			start = 0
		}
		buf := make([]byte, end-start)
		if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
		for i := len(buf) - 1; i >= 0; i-- {
			// A trailing newline ends the last line rather than starting a new one.
			if buf[i] != '\n' || start+int64(i) == size-1 {
				continue
			}
			if newlines++; newlines == n {
				return start + int64(i) + 1, nil
			}
		}
		end = start
	}
	return 0, nil
}

func linkID(path string) (uint64, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	return fi.Sys().(*syscall.Stat_t).Ino, nil
}

func startedSince(path string, epoch int64) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.ModTime().Unix() >= epoch
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// sleep waits d unless ctx ends first, and reports whether it slept the full time.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
