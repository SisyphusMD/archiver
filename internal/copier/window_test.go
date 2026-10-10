package copier

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
)

func window(t *testing.T, s string) *config.Window {
	t.Helper()
	w, err := config.ParseWindow(s)
	if err != nil {
		t.Fatal(err)
	}
	return &w
}

func logged(rec *recorder, part string) bool {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, l := range rec.logs {
		if strings.Contains(l, part) {
			return true
		}
	}
	return false
}

// Outside its window the worker copies nothing and says when it will; inside it copies.
func TestWindowHoldsUntilOpen(t *testing.T) {
	r := &fakeRunner{local: revs("a:1"), target: revs()}
	rec := &recorder{}
	w, c := newWorker(r, rec, State{}) // 03:00
	w.Window = window(t, "04:00-06:00")
	w.pass(w.stops, nil)
	s := w.State()
	if r.copies != 0 || s.Status != Held || s.HeldUntil != t0.Add(time.Hour).Unix() {
		t.Fatalf("copies %d, state %+v", r.copies, s)
	}
	if !logged(rec, "wait for its copy window (04:00-06:00), which opens at 04:00") {
		t.Fatalf("logs %q", rec.logs)
	}
	w.pass(w.stops, nil)
	if len(rec.logs) != 1 {
		t.Fatalf("a second held pass logged again: %q", rec.logs)
	}
	c.Advance(time.Hour)
	w.pass(w.stops, nil)
	if s := w.State(); r.copies != 1 || s.Status != Idle || s.HeldUntil != 0 {
		t.Fatalf("copies %d, state %+v", r.copies, s)
	}
}

// The window closing ends a running copy without counting it a failure.
func TestWindowCloseEndsCopy(t *testing.T) {
	r := &fakeRunner{local: revs("a:1"), target: revs(), block: true, started: make(chan *fakeCopy, 1)}
	rec := &recorder{}
	w, _ := newWorker(r, rec, State{})
	w.Window = window(t, "02:00-04:00")
	done := make(chan struct{})
	go func() { w.pass(w.stops, nil); close(done) }()
	cp := <-r.started
	w.hold()
	<-done
	s := w.State()
	if !cp.stopped || s.Status != Held || s.FailingSince != 0 || s.NextRetry != 0 || s.HeldUntil == 0 {
		t.Fatalf("copy stopped %v, state %+v", cp.stopped, s)
	}
	if !logged(rec, "ended as its copy window (02:00-04:00) closed") {
		t.Fatalf("logs %q", rec.logs)
	}
}

// A failing target held by its window stays failing: health must still see it.
func TestWindowKeepsRetryState(t *testing.T) {
	r := &fakeRunner{local: revs("a:1"), target: revs(), fail: errors.New("refused")}
	w, c := newWorker(r, &recorder{}, State{})
	w.Window = window(t, "02:00-04:00")
	w.pass(w.stops, nil)
	if w.State().Status != Retrying {
		t.Fatalf("state %+v", w.State())
	}
	c.Advance(2 * time.Hour)
	w.pass(w.stops, nil)
	if s := w.State(); s.Status != Retrying || s.HeldUntil == 0 || s.FailingSince == 0 || r.copies != 1 {
		t.Fatalf("state %+v after %d copies", s, r.copies)
	}
}

// Run ends the copy as the window closes and starts again when it opens.
func TestRunFollowsWindow(t *testing.T) {
	r := &fakeRunner{local: revs("a:1"), target: revs(), block: true, started: make(chan *fakeCopy, 2)}
	w, c := newWorker(r, &recorder{}, State{})
	w.Window = window(t, "02:00-04:00")
	done := make(chan struct{})
	defer close(done)
	go w.Run(done)
	first := <-r.started
	waitFor(t, func() bool { return c.hasTimerAt(time.Hour) })
	c.Advance(time.Hour) // 04:00: closes
	waitFor(t, func() bool { return w.State().Status == Held })
	waitFor(t, func() bool { first.mu.Lock(); defer first.mu.Unlock(); return first.stopped })
	waitFor(t, func() bool { return c.hasTimerAt(22 * time.Hour) })
	c.Advance(22 * time.Hour) // 02:00 the next day: opens
	select {
	case second := <-r.started:
		second.Terminate()
	case <-time.After(5 * time.Second):
		t.Fatal("no copy when the window opened")
	}
}

// A stop outside the window is a stop: status no longer says the worker waits for it.
func TestStopClearsHeld(t *testing.T) {
	r := &fakeRunner{local: revs("a:1"), target: revs()}
	w, _ := newWorker(r, &recorder{}, State{})
	w.Window = window(t, "04:00-06:00")
	w.pass(w.stops, nil)
	w.Stop()
	if s := w.State(); s.Status != Stopped || s.HeldUntil != 0 {
		t.Fatalf("state %+v", s)
	}
}
