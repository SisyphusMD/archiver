package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
)

// drillTurn has the drills the daemon starts take turns. Kit and restore drills share one
// lock, and a drill that finds it held is refused until its next slot: a month away for a
// monthly drill whose slot falls inside another drill's run.
type drillTurn struct {
	log io.Writer
	// busy says whether a drill holds the lock. Asked with the turn in hand, so it can only
	// be one the daemon did not start.
	busy func() bool
	poll time.Duration
	free chan struct{}

	mu      sync.Mutex
	dropped chan struct{} // closed, and replaced, when the waiting drills are dropped
}

func newDrillTurn(log io.Writer, busy func() bool) *drillTurn {
	t := &drillTurn{log: log, busy: busy, poll: 5 * time.Second, free: make(chan struct{}, 1), dropped: make(chan struct{})}
	t.free <- struct{}{}
	return t
}

// take waits for name's turn. It is false when the wait ended without one: the daemon is
// shutting down, or `archiver stop` dropped the drills that were waiting.
func (t *drillTurn) take(ctx context.Context, name string) bool {
	t.mu.Lock()
	dropped := t.dropped
	t.mu.Unlock()
	waited := false
	waiting := func() {
		if !waited {
			waited = true
			fmt.Fprintf(t.log, "archiver daemon: %s waits for the drill that is running\n", name)
		}
	}
	select {
	case <-t.free:
	default:
		waiting()
		select {
		case <-t.free:
		case <-dropped:
			return t.stopped(name)
		case <-ctx.Done():
			return false
		}
	}
	for {
		// Checked before the lock: the stop that drops a wait is sent before the running
		// drill is stopped, and must win when both are seen at once.
		select {
		case <-dropped:
			t.done()
			return t.stopped(name)
		default:
		}
		if ctx.Err() != nil {
			t.done()
			return false
		}
		if !t.busy() {
			break
		}
		waiting()
		select {
		case <-time.After(t.poll):
		case <-dropped:
		case <-ctx.Done():
		}
	}
	if waited {
		fmt.Fprintf(t.log, "archiver daemon: %s starts now: the other drill ended\n", name)
	}
	return true
}

func (t *drillTurn) stopped(name string) bool {
	fmt.Fprintf(t.log, "archiver daemon: %s not started: archiver stop ended its wait\n", name)
	return false
}

// tryTake takes the turn only if no drill runs.
func (t *drillTurn) tryTake() bool {
	select {
	case <-t.free:
	default:
		return false
	}
	if t.busy() {
		t.done()
		return false
	}
	return true
}

func (t *drillTurn) done() { t.free <- struct{}{} }

// drop ends every wait under way without a turn.
func (t *drillTurn) drop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	close(t.dropped)
	t.dropped = make(chan struct{})
}

// inTurn is run with each drill waiting for its turn; any other job starts at once.
func (t *drillTurn) inTurn(ctx context.Context, run func(daemon.Job) int) func(daemon.Job) int {
	return func(j daemon.Job) int {
		if !isDrill(j.Name) {
			return run(j)
		}
		if !t.take(ctx, j.Name) {
			return daemon.NotStarted
		}
		defer t.done()
		return run(j)
	}
}

// isDrill says whether a job wants the lock kit and restore drills share.
func isDrill(job string) bool {
	return job == "kit-drill" || job == "drill" || strings.HasPrefix(job, "drill:")
}

func drillRunning(l layout.Layout) bool {
	lock, held, err := lockstate.ReadLock(l.DrillLock())
	return err == nil && held && lock.Alive()
}
