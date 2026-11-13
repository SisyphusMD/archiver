package main

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

func idle() bool { return false }

// syncLog is a log several goroutines write and the test reads.
type syncLog struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *syncLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *syncLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for range 2000 {
		if ok() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("never: %s", what)
}

// Two drills due at one moment both run: each as the real commands do, refusing when the
// lock they share is held.
func TestDrillsDueTogetherBothRun(t *testing.T) {
	l := layout.Layout{Root: t.TempDir(), Lock: t.TempDir()}
	if err := os.MkdirAll(l.LogDir(), 0700); err != nil {
		t.Fatal(err)
	}
	var log syncLog
	turn := newDrillTurn(&log, func() bool { return drillRunning(l) })
	run := turn.inTurn(context.Background(), func(daemon.Job) int {
		lock, _, err := runlock.Acquire(l.DrillLock(), l.DrillStopFlag(), "drill", "running")
		if err != nil {
			return 1
		}
		defer lock.Release()
		// Longer than the moment a lock being released is waited for.
		time.Sleep(150 * time.Millisecond)
		return 0
	})
	var wg sync.WaitGroup
	for _, name := range []string{"drill", "drill:GARAGE", "kit-drill"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code := run(daemon.Job{Name: name}); code != 0 {
				t.Errorf("%s exited %d", name, code)
			}
		}()
	}
	wg.Wait()
	if got := strings.Count(log.String(), "waits for the drill that is running"); got != 2 {
		t.Fatalf("%d drills said they waited, want 2:\n%s", got, log.String())
	}
}

func TestDrillTurnLeavesOtherJobsAlone(t *testing.T) {
	turn := newDrillTurn(io.Discard, idle)
	if !turn.tryTake() {
		t.Fatal("the turn was not free")
	}
	run := turn.inTurn(context.Background(), func(daemon.Job) int { return 7 })
	for _, name := range []string{"backup", "maintenance", "deep-check:cloud"} {
		if code := run(daemon.Job{Name: name}); code != 7 {
			t.Fatalf("%s: %d while a drill has the turn", name, code)
		}
	}
}

// A drill started by hand holds the lock without the turn.
func TestDrillTurnWaitsForADrillStartedByHand(t *testing.T) {
	var busy atomic.Bool
	busy.Store(true)
	var log syncLog
	turn := newDrillTurn(&log, busy.Load)
	turn.poll = time.Millisecond
	if turn.tryTake() {
		t.Fatal("took the turn while a drill holds the lock")
	}
	took := make(chan bool, 1)
	go func() { took <- turn.take(context.Background(), "drill") }()
	eventually(t, "the drill says it waits", func() bool { return strings.Contains(log.String(), "drill waits for") })
	select {
	case <-took:
		t.Fatal("took the turn while a drill holds the lock")
	default:
	}
	busy.Store(false)
	if !<-took {
		t.Fatal("no turn once the lock was free")
	}
	if !strings.Contains(log.String(), "drill starts now") {
		t.Fatalf("log: %s", log.String())
	}
	if turn.tryTake() {
		t.Fatal("two drills hold the turn")
	}
}

func TestStopDropsWaitingDrills(t *testing.T) {
	var log syncLog
	turn := newDrillTurn(&log, idle)
	if !turn.take(context.Background(), "drill") {
		t.Fatal("no turn")
	}
	if strings.Contains(log.String(), "waits") {
		t.Fatalf("a drill with nothing to wait for said it waited: %s", log.String())
	}
	var started atomic.Int32
	run := turn.inTurn(context.Background(), func(daemon.Job) int {
		started.Add(1)
		return 0
	})
	codes := make(chan int, 2)
	for _, name := range []string{"drill:GARAGE", "kit-drill"} {
		go func() { codes <- run(daemon.Job{Name: name}) }()
	}
	eventually(t, "both drills wait", func() bool { return strings.Count(log.String(), "waits for") == 2 })
	turn.drop()
	for range 2 {
		if code := <-codes; code != daemon.NotStarted {
			t.Fatalf("a dropped drill returned %d", code)
		}
	}
	// The stopped drill ends after the drop, as `archiver stop` orders it.
	turn.done()
	if started.Load() != 0 {
		t.Fatal("a drill started after the stop")
	}
	if got := strings.Count(log.String(), "not started: archiver stop ended its wait"); got != 2 {
		t.Fatalf("log: %s", log.String())
	}
	// The next slot runs as normal.
	if code := run(daemon.Job{Name: "kit-drill"}); code != 0 || started.Load() != 1 {
		t.Fatalf("after the stop: exit %d, %d started", code, started.Load())
	}
}

// The stop reaches a drill that has its turn and waits for one started by hand.
func TestStopDropsADrillWaitingForTheLock(t *testing.T) {
	var log syncLog
	turn := newDrillTurn(&log, func() bool { return true })
	turn.poll = time.Hour
	took := make(chan bool, 1)
	go func() { took <- turn.take(context.Background(), "drill") }()
	eventually(t, "the drill says it waits", func() bool { return strings.Contains(log.String(), "waits for") })
	turn.drop()
	if <-took {
		t.Fatal("a dropped drill took the turn")
	}
	turn.busy = idle
	if !turn.tryTake() {
		t.Fatal("the dropped drill kept the turn")
	}
}

func TestShutdownEndsADrillsWait(t *testing.T) {
	var log syncLog
	turn := newDrillTurn(&log, idle)
	turn.tryTake()
	ctx, cancel := context.WithCancel(context.Background())
	var started atomic.Int32
	run := turn.inTurn(ctx, func(daemon.Job) int {
		started.Add(1)
		return 0
	})
	code := make(chan int, 1)
	go func() { code <- run(daemon.Job{Name: "drill"}) }()
	eventually(t, "the drill says it waits", func() bool { return strings.Contains(log.String(), "waits for") })
	cancel()
	if got := <-code; got != daemon.NotStarted {
		t.Fatalf("returned %d", got)
	}
	turn.done()
	if started.Load() != 0 {
		t.Fatal("a drill started after the shutdown")
	}
	// Nor does one whose turn comes as the daemon shuts down.
	if got := run(daemon.Job{Name: "drill"}); got != daemon.NotStarted || started.Load() != 0 {
		t.Fatalf("after the shutdown: %d, %d started", got, started.Load())
	}
	if !turn.tryTake() {
		t.Fatal("the turn was kept")
	}
}

func TestFirstKitDrillNeverWaitsItsTurn(t *testing.T) {
	turn := newDrillTurn(io.Discard, idle)
	turn.tryTake()
	var runs atomic.Int32
	f := &firstKitDrill{log: io.Discard, turn: turn, due: func() bool { return true }, run: func(daemon.Job) int {
		runs.Add(1)
		return 0
	}}
	f.try(context.Background())
	f.finished.Wait()
	if runs.Load() != 0 {
		t.Fatal("the first kit drill ran beside another drill")
	}
	turn.done()
	f.try(context.Background())
	f.finished.Wait()
	if runs.Load() != 1 {
		t.Fatalf("%d runs once the turn was free", runs.Load())
	}
	if !turn.tryTake() {
		t.Fatal("the first kit drill kept the turn")
	}
}

func TestWaitingDrillsAreOnRecord(t *testing.T) {
	l := layout.Layout{Root: t.TempDir(), Lock: t.TempDir()}
	var log syncLog
	turn := newDrillTurn(&log, idle)
	turn.note = noteDrillsWaiting(l)
	turn.tryTake()
	took := make(chan bool, 1)
	go func() { took <- turn.take(context.Background(), "drill:GARAGE") }()
	eventually(t, "the waiting drill is on record", func() bool {
		b, _ := os.ReadFile(l.DrillsWaiting())
		return string(b) == "drill:GARAGE\n"
	})
	turn.done()
	if !<-took {
		t.Fatal("no turn")
	}
	if _, err := os.Stat(l.DrillsWaiting()); !os.IsNotExist(err) {
		t.Fatalf("a drill that started is still on record as waiting: %v", err)
	}
}
