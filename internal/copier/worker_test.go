package copier

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/notify"
)

// fakeClock moves only when told to, firing any timer it passes.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []fakeTimer
}

type fakeTimer struct {
	at time.Time
	ch chan time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	c.timers = append(c.timers, fakeTimer{c.now.Add(d), ch})
	return ch
}

// hasTimerAt reports whether a timer is set for now+d.
func (c *fakeClock) hasTimerAt(d time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range c.timers {
		if t.at.Equal(c.now.Add(d)) {
			return true
		}
	}
	return false
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	var keep []fakeTimer
	for _, t := range c.timers {
		if !t.at.After(c.now) {
			t.ch <- c.now
		} else {
			keep = append(keep, t)
		}
	}
	c.timers = keep
	c.mu.Unlock()
}

// fakeRunner holds the revisions of a primary and one target; a copy brings the target
// up to date unless told to fail.
type fakeRunner struct {
	mu       sync.Mutex
	local    map[string]bool
	target   map[string]bool
	fail     error // returned by the copy
	noEffect bool  // the copy "succeeds" without copying
	copies   int
	started  chan *fakeCopy
	block    bool // copies wait for release or Terminate
	commands [][]string
	failCmd  map[string]error // by command name (prune, check)
}

func (r *fakeRunner) Start(args ...string) (Copy, error) {
	r.mu.Lock()
	r.commands = append(r.commands, args)
	err := r.failCmd[args[0]]
	if err == nil && args[0] == "prune" {
		id := ""
		for i := 0; i+1 < len(args); i++ {
			switch args[i] {
			case "-id":
				id = args[i+1]
			case "-r":
				delete(r.target, id+":"+args[i+1])
			}
		}
	}
	r.mu.Unlock()
	c := &fakeCopy{r: r, release: make(chan struct{}), result: err, noApply: true}
	close(c.release)
	return c, nil
}

func (r *fakeRunner) Prepare(context.Context) error { return nil }

func (r *fakeRunner) Revisions(ctx context.Context, storage string) (map[string]bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	src := r.target
	if storage == "local" {
		src = r.local
	}
	out := map[string]bool{}
	for k := range src {
		out[k] = true
	}
	return out, nil
}

type fakeCopy struct {
	r       *fakeRunner
	release chan struct{}
	paused  int
	resumed int
	stopped bool
	mu      sync.Mutex
	result  error // for a non-copy command
	noApply bool  // not a copy: nothing to apply to the target
}

func (r *fakeRunner) StartCopy(string) (Copy, error) {
	r.mu.Lock()
	r.copies++
	c := &fakeCopy{r: r, release: make(chan struct{})}
	block := r.block
	r.mu.Unlock()
	if r.started != nil {
		r.started <- c
	}
	if !block {
		close(c.release)
	}
	return c, nil
}

func (c *fakeCopy) Wait() error {
	<-c.release
	c.mu.Lock()
	stopped := c.stopped
	c.mu.Unlock()
	if stopped {
		return errors.New("terminated")
	}
	if c.noApply {
		return c.result
	}
	c.r.mu.Lock()
	defer c.r.mu.Unlock()
	if c.r.fail != nil {
		return c.r.fail
	}
	if !c.r.noEffect {
		for k := range c.r.local {
			c.r.target[k] = true
		}
	}
	return nil
}

func (c *fakeCopy) Terminate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.stopped {
		c.stopped = true
		close(c.release)
	}
}
func (c *fakeCopy) Pause()  { c.mu.Lock(); c.paused++; c.mu.Unlock() }
func (c *fakeCopy) Resume() { c.mu.Lock(); c.resumed++; c.mu.Unlock() }

// recorder notes what reaches a person: an incident notifies when raised, again a day
// later while open, and on clearing (as notify's incidents do).
type recorder struct {
	mu    sync.Mutex
	notes []string
	logs  []string
	open  map[string]time.Time
	now   func() time.Time
}

func (r *recorder) events() Events {
	return Events{
		Log: func(level, msg string) { r.mu.Lock(); r.logs = append(r.logs, level+" "+msg); r.mu.Unlock() },
		Raise: func(key string, _ notify.Kind, title, msg string) {
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.open == nil {
				r.open = map[string]time.Time{}
			}
			if last, ok := r.open[key]; !ok || r.now().Sub(last) >= 24*time.Hour {
				r.open[key] = r.now()
				r.notes = append(r.notes, title)
			}
		},
		Clear: func(key, title, msg string) {
			r.mu.Lock()
			defer r.mu.Unlock()
			if _, ok := r.open[key]; ok {
				delete(r.open, key)
				r.notes = append(r.notes, title)
			}
		},
		Save: func(State) {},
	}
}

func (r *recorder) titles() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.notes...)
}

func revs(ids ...string) map[string]bool {
	m := map[string]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m
}

var t0 = time.Date(2026, 10, 2, 3, 0, 0, 0, time.UTC)

func newWorker(r *fakeRunner, rec *recorder, saved State) (*Worker, *fakeClock) {
	c := &fakeClock{now: t0}
	rec.now = c.Now
	return New("offsite", "local", r, c, rec.events(), saved), c
}

func TestPassCopiesUntilCaughtUp(t *testing.T) {
	r := &fakeRunner{local: revs("a:1", "a:2"), target: revs("a:1")}
	w, _ := newWorker(r, &recorder{}, State{})
	w.pass(w.stops, nil)
	if s := w.State(); s.Status != Idle || s.LastSuccess == 0 || r.copies != 1 {
		t.Fatalf("state %+v after %d copies", s, r.copies)
	}
	w.pass(w.stops, nil)
	if r.copies != 1 {
		t.Fatal("a caught-up target was copied to again")
	}
}

func TestBackoffDownReminderRecovered(t *testing.T) {
	r := &fakeRunner{local: revs("a:1"), target: revs(), fail: errors.New("connection refused")}
	rec := &recorder{}
	w, c := newWorker(r, rec, State{})
	var delays []time.Duration
	for i := 0; i < 6; i++ {
		w.pass(w.stops, nil)
		s := w.State()
		d := time.Unix(s.NextRetry, 0).Sub(c.Now())
		delays = append(delays, d)
		c.Advance(d)
	}
	// The fourth retry is pulled in to land on the 30-minute mark.
	want := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, 9 * time.Minute, 30 * time.Minute, 30 * time.Minute}
	if fmt.Sprint(delays) != fmt.Sprint(want) {
		t.Fatalf("delays %v, want %v", delays, want)
	}
	// Failing since t0; the 5th failure (at 30 minutes) declares it down.
	if s := w.State(); s.Status != Down || len(rec.titles()) != 1 || rec.titles()[0] != "Storage Down" {
		t.Fatalf("state %s, notes %q", s.Status, rec.titles())
	}
	c.Advance(24 * time.Hour)
	w.pass(w.stops, nil)
	w.pass(w.stops, nil)
	if got := rec.titles(); len(got) != 2 || got[1] != "Storage Down" {
		t.Fatalf("notes %q: one reminder per day", got)
	}
	r.mu.Lock()
	r.fail = nil
	r.mu.Unlock()
	w.pass(w.stops, nil)
	if s := w.State(); s.Status != Idle || s.FailingSince != 0 || rec.titles()[2] != "Storage Recovered" {
		t.Fatalf("state %+v, notes %q", s, rec.titles())
	}
}

func TestShortOutageIsNotDown(t *testing.T) {
	r := &fakeRunner{local: revs("a:1"), target: revs(), fail: errors.New("timeout")}
	rec := &recorder{}
	w, c := newWorker(r, rec, State{})
	w.pass(w.stops, nil)
	c.Advance(time.Minute)
	w.pass(w.stops, nil)
	c.Advance(5 * time.Minute)
	r.mu.Lock()
	r.fail = nil
	r.mu.Unlock()
	w.pass(w.stops, nil)
	if len(rec.titles()) != 0 || w.State().Status != Idle {
		t.Fatalf("a 6-minute outage notified %q", rec.titles())
	}
}

func TestCopyWithoutEffectFails(t *testing.T) {
	r := &fakeRunner{local: revs("a:1"), target: revs(), noEffect: true}
	w, _ := newWorker(r, &recorder{}, State{})
	w.pass(w.stops, nil)
	if s := w.State(); s.Status != Retrying || r.copies != 1 {
		t.Fatalf("state %+v after %d copies: a copy that moves nothing must not loop", s, r.copies)
	}
}

func TestStopEndsCopyWithoutRetry(t *testing.T) {
	r := &fakeRunner{local: revs("a:1"), target: revs(), block: true, started: make(chan *fakeCopy, 1)}
	w, _ := newWorker(r, &recorder{}, State{})
	done := make(chan struct{})
	go func() { w.pass(w.stops, nil); close(done) }()
	c := <-r.started
	w.Pause()
	w.Stop()
	<-done
	if s := w.State(); s.Status != Stopped || s.NextRetry != 0 || s.FailingSince != 0 {
		t.Fatalf("state %+v", s)
	}
	if c.paused != 1 || c.resumed != 1 || !c.stopped {
		t.Fatalf("copy paused %d resumed %d stopped %v: a paused copy is resumed so it can end", c.paused, c.resumed, c.stopped)
	}
}

func TestPauseResumeRunningCopy(t *testing.T) {
	r := &fakeRunner{local: revs("a:1"), target: revs(), block: true, started: make(chan *fakeCopy, 1)}
	w, _ := newWorker(r, &recorder{}, State{})
	done := make(chan struct{})
	go func() { w.pass(w.stops, nil); close(done) }()
	c := <-r.started
	w.Pause()
	if !w.State().Paused || c.paused != 1 {
		t.Fatal("pause did not freeze the copy")
	}
	w.Resume()
	close(c.release)
	<-done
	if s := w.State(); s.Paused || s.Status != Idle || c.resumed != 1 {
		t.Fatalf("state %+v, resumed %d", s, c.resumed)
	}
}

func TestRestartKeepsDownForgetsStop(t *testing.T) {
	r := &fakeRunner{}
	down := State{Status: Copying, FailingSince: t0.Add(-time.Hour).Unix(), DownSince: t0.Add(-time.Hour).Unix()}
	if w, _ := newWorker(r, &recorder{}, down); w.State().DownSince == 0 {
		t.Fatal("a down target must stay down across a restart, or it is alerted again")
	}
	for _, st := range []string{Stopped, Copying} {
		if w, _ := newWorker(r, &recorder{}, State{Status: st, Paused: true}); w.State().Status != Idle || w.State().Paused {
			t.Fatalf("%s must not survive a restart", st)
		}
	}
}

// The run loop makes a first pass at once and retries when the backoff timer fires.
func TestRunRetriesOnSchedule(t *testing.T) {
	r := &fakeRunner{local: revs("a:1"), target: revs(), fail: errors.New("down"), started: make(chan *fakeCopy, 8)}
	w, c := newWorker(r, &recorder{}, State{})
	done := make(chan struct{})
	defer close(done)
	go w.Run(done)
	<-r.started
	waitFor(t, func() bool { return w.State().Status == Retrying })
	waitFor(t, func() bool { return c.hasTimerAt(time.Minute) })
	c.Advance(time.Minute)
	select {
	case <-r.started:
	case <-time.After(5 * time.Second):
		t.Fatal("no retry after the first backoff")
	}
	waitFor(t, func() bool { return w.State().NextRetry == c.Now().Add(5*time.Minute).Unix() })
	// A wake while retrying passes at once too.
	r.mu.Lock()
	r.fail = nil
	r.mu.Unlock()
	w.Wake()
	waitFor(t, func() bool { return w.State().Status == Idle })
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A stop during the wait for a retry cancels the retry.
func TestStopCancelsPendingRetry(t *testing.T) {
	r := &fakeRunner{local: revs("a:1"), target: revs(), fail: errors.New("down"), started: make(chan *fakeCopy, 8)}
	w, c := newWorker(r, &recorder{}, State{})
	done := make(chan struct{})
	defer close(done)
	go w.Run(done)
	<-r.started
	waitFor(t, func() bool { return w.State().Status == Retrying })
	waitFor(t, func() bool { return c.hasTimerAt(time.Minute) })
	w.Stop()
	c.Advance(time.Hour)
	select {
	case <-r.started:
		t.Fatal("a stopped worker copied when its old retry timer fired")
	case <-time.After(300 * time.Millisecond):
	}
	if w.State().Status != Stopped {
		t.Fatalf("status %s", w.State().Status)
	}
	// The next backup's wake copies again.
	r.mu.Lock()
	r.fail = nil
	r.mu.Unlock()
	w.Wake()
	waitFor(t, func() bool { return w.State().Status == Idle })
}

// A pause that lands before a copy starts holds the copy until resume; a stop then starts none.
func TestPauseBeforeCopyHoldsIt(t *testing.T) {
	r := &fakeRunner{local: revs("a:1"), target: revs(), started: make(chan *fakeCopy, 1)}
	w, _ := newWorker(r, &recorder{}, State{})
	w.Pause()
	done := make(chan struct{})
	go func() { w.pass(w.stops, nil); close(done) }()
	select {
	case <-r.started:
		t.Fatal("a copy started while paused")
	case <-time.After(200 * time.Millisecond):
	}
	w.Resume()
	<-r.started
	<-done
	w.Pause()
	done = make(chan struct{})
	r.mu.Lock()
	r.local["a:2"] = true
	r.mu.Unlock()
	go func() { w.pass(w.stops, nil); close(done) }()
	time.Sleep(100 * time.Millisecond)
	w.Stop()
	<-done
	select {
	case <-r.started:
		t.Fatal("a stop during a pause still let a copy start")
	default:
	}
}

// A stop during a slow listing ends the pass with no copy and no failure.
func TestStopDuringListing(t *testing.T) {
	r := &slowLister{fakeRunner: fakeRunner{local: revs("a:1"), target: revs(), started: make(chan *fakeCopy, 1)}, listing: make(chan struct{})}
	w := New("offsite", "local", r, &fakeClock{now: t0}, (&recorder{}).events(), State{})
	done := make(chan struct{})
	go func() { w.pass(w.stops, nil); close(done) }()
	<-r.listing
	w.Stop()
	<-done
	if s := w.State(); s.Status != Stopped || s.FailingSince != 0 || r.copies != 0 {
		t.Fatalf("state %+v, copies %d", s, r.copies)
	}
}

type slowLister struct {
	fakeRunner
	listing chan struct{}
	once    sync.Once
}

func (s *slowLister) Revisions(ctx context.Context, storage string) (map[string]bool, error) {
	s.once.Do(func() { close(s.listing) })
	<-ctx.Done()
	return nil, ctx.Err()
}

// A wake queued before a stop does not restart the stopped worker.
func TestStopDiscardsQueuedWake(t *testing.T) {
	r := &fakeRunner{local: revs("a:1"), target: revs(), started: make(chan *fakeCopy, 1)}
	w, _ := newWorker(r, &recorder{}, State{})
	w.Wake()
	w.Stop()
	done := make(chan struct{})
	defer close(done)
	go w.Run(done)
	select {
	case <-r.started:
		t.Fatal("a wake from before the stop started a copy")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestDownDeclaredAtThirtyMinutes(t *testing.T) {
	r := &fakeRunner{local: revs("a:1"), target: revs(), fail: errors.New("refused")}
	rec := &recorder{}
	w, c := newWorker(r, rec, State{})
	for w.State().DownSince == 0 {
		w.pass(w.stops, nil)
		c.Advance(time.Unix(w.State().NextRetry, 0).Sub(c.Now()))
		if c.Now().Sub(t0) > time.Hour {
			t.Fatal("never declared down")
		}
	}
	if got := time.Unix(w.State().DownSince, 0).Sub(t0); got != 30*time.Minute {
		t.Fatalf("declared down after %v, want 30m", got)
	}
}

// A target the probe cannot reach fails the pass at once, without preparing or copying.
func TestProbeFailsFast(t *testing.T) {
	r := &fakeRunner{local: revs("a:1"), target: revs()}
	rec := &recorder{}
	w, _ := newWorker(r, rec, State{})
	w.Probe = func(context.Context) error { return errors.New("dial tcp: connection refused") }
	w.pass(w.stops, nil)
	if s := w.State(); s.Status != Retrying || !strings.Contains(s.LastError, "cannot be reached") || r.copies != 0 {
		t.Fatalf("state %+v, copies %d", s, r.copies)
	}
}
