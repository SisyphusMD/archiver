// Package copier keeps each secondary storage caught up with the primary (ADR 11): one
// worker per target, copying every snapshot the target lacks, one copy at a time, and
// retrying a failed copy on a fixed backoff until the target has caught up (ADR 16).
package copier

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/SisyphusMD/archiver/internal/runlock"
)

// Backoff is the wait before each retry of a failed copy; the last repeats.
var Backoff = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute}

const (
	// DownAfter is how long a target must keep failing to count as down.
	DownAfter = 30 * time.Minute
	// Remind is how often a down target is reported again.
	Remind = 24 * time.Hour
	// SafetyWake re-checks a caught-up target even when nothing says local changed.
	SafetyWake = 6 * time.Hour
)

// Statuses a worker reports.
const (
	Idle     = "idle" // caught up
	Copying  = "copying"
	Retrying = "retrying" // the last copy failed; another is scheduled
	Down     = "down"     // failing for DownAfter or longer; still retrying
	Stopped  = "stopped"  // `archiver stop`: no retry until local next changes
)

// State is what a worker reports and keeps across restarts.
type State struct {
	Target       string `json:"target"`
	Status       string `json:"status"`
	Paused       bool   `json:"paused,omitempty"`
	Since        int64  `json:"since"`
	Behind       int    `json:"behind"`
	LastSuccess  int64  `json:"last_success,omitempty"`
	FailingSince int64  `json:"failing_since,omitempty"`
	DownSince    int64  `json:"down_since,omitempty"` // kept through retries, so down is reported once
	NextRetry    int64  `json:"next_retry,omitempty"`
	LastError    string `json:"last_error,omitempty"`
	LastAlert    int64  `json:"last_alert,omitempty"`
}

// Copy is a running copy.
type Copy interface {
	Wait() error
	Terminate()
	Pause()
	Resume()
}

// Runner reaches the storages.
type Runner interface {
	// Prepare readies the runner before a pass; it is retried like a copy until it works.
	Prepare(ctx context.Context) error
	// Revisions lists every snapshot revision on a storage, as "id:revision", until ctx
	// ends.
	Revisions(ctx context.Context, storage string) (map[string]bool, error)
	// StartCopy starts copying every snapshot the target lacks from the primary.
	StartCopy(target string) (Copy, error)
}

// Clock is the time source, replaced in tests.
type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

// Events are what a worker tells the rest of Archiver.
type Events struct {
	Log    func(level, msg string)
	Notify func(title, msg string)
	Save   func(State)
}

// Worker keeps one target caught up.
type Worker struct {
	Target  string // the target's storage name
	Primary string
	Runner  Runner
	Clock   Clock
	Events  Events
	// CopyLock names the lock held around each copy into the target (layout.CopyLock),
	// shared with a backup that copies inline.
	CopyLock func(target string) string

	wake chan struct{}

	mu      sync.Mutex
	state   State
	attempt int
	current Copy
	cancel  context.CancelFunc // ends the pass's listing
	paused  bool
	resumed chan struct{} // closed by Resume or Stop
	// stops counts Stop calls. A pass is handed the count when it is dispatched and does
	// nothing more once it changes, so a stop wins wherever it lands: during a listing, a
	// copy, a pause, the start of a copy, or between deciding on a pass and starting it.
	stops int
	// wakes counts Wake calls; wakesAtStop is the count when the last stop came. Only a wake
	// after it (a backup after the stop) ends the stop.
	wakes, wakesAtStop int
}

// New makes a worker that starts from a saved state, so a restart neither re-alerts nor
// forgets that the target is down.
func New(target, primary string, r Runner, c Clock, ev Events, saved State) *Worker {
	w := &Worker{Target: target, Primary: primary, Runner: r, Clock: c, Events: ev, wake: make(chan struct{}, 1)}
	w.state = saved
	w.state.Target = target
	w.state.Paused = false
	// A copy or a stop does not outlive the process; after a restart the worker compares
	// revisions and carries on. A target that was down stays down, so it is not re-alerted.
	if w.state.Status == "" || w.state.Status == Copying || w.state.Status == Stopped {
		w.state.Status = Idle
	}
	return w
}

// Wake tells the worker local's revisions changed. Wakes arriving while it is busy
// coalesce into one more pass.
func (w *Worker) Wake() {
	w.mu.Lock()
	w.wakes++
	w.mu.Unlock()
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// State is a copy of the worker's state.
func (w *Worker) State() State {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.state
}

// Run keeps the target caught up until done is closed. It makes a first pass at once,
// unless it starts stopped.
func (w *Worker) Run(done <-chan struct{}) {
	first := true
	for {
		var timer <-chan time.Time
		w.mu.Lock()
		switch w.state.Status {
		case Retrying, Down:
			timer = w.Clock.After(time.Unix(w.state.NextRetry, 0).Sub(w.Clock.Now()))
		case Stopped:
		default:
			timer = w.Clock.After(SafetyWake)
		}
		w.mu.Unlock()
		if !first {
			select {
			case <-done:
				return
			case <-w.wake:
			case <-timer:
			}
		}
		first = false
		select {
		case <-done:
			return
		default:
		}
		w.mu.Lock()
		if w.state.Status == Stopped {
			if w.wakes == w.wakesAtStop {
				w.mu.Unlock()
				continue
			}
			w.state.Status = Idle
		}
		stops := w.stops
		w.mu.Unlock()
		w.pass(stops, done)
	}
}

// ready waits out a pause and reports whether the pass that started at stops may go on.
func (w *Worker) ready(stops int) bool {
	for {
		w.mu.Lock()
		if w.stops != stops {
			w.mu.Unlock()
			return false
		}
		if !w.paused {
			w.mu.Unlock()
			return true
		}
		ch := w.resumed
		w.mu.Unlock()
		<-ch
	}
}

func (w *Worker) stoppedSince(stops int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stops != stops
}

func (w *Worker) log(level, msg string) {
	if w.Events.Log != nil {
		w.Events.Log(level, msg)
	}
}

func (w *Worker) save() {
	if w.Events.Save != nil {
		w.Events.Save(w.state)
	}
}

// missing lists what the target lacks.
func (w *Worker) missing(ctx context.Context) ([]string, error) {
	local, err := w.Runner.Revisions(ctx, w.Primary)
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", w.Primary, err)
	}
	target, err := w.Runner.Revisions(ctx, w.Target)
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", w.Target, err)
	}
	var out []string
	for r := range local {
		if !target[r] {
			out = append(out, r)
		}
	}
	sort.Strings(out)
	return out, nil
}

// pass copies until the target has caught up, the copy fails, or a stop ends it. A copy
// that reports success but leaves a revision it was meant to send still missing fails the
// pass, rather than looping.
func (w *Worker) pass(stops int, done <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-done:
			cancel()
		case <-ctx.Done():
		}
	}()
	w.mu.Lock()
	if w.stops != stops {
		w.mu.Unlock()
		cancel()
		return
	}
	w.cancel = cancel
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.cancel = nil
		w.mu.Unlock()
		cancel()
	}()
	// A listing or preparation already running when a pause comes finishes (both are
	// short reads); none starts while paused, and a copy, the long part, is frozen.
	if !w.ready(stops) {
		return
	}
	if err := w.Runner.Prepare(ctx); err != nil {
		if !w.stoppedSince(stops) {
			w.failed(stops, err)
		}
		return
	}
	var before map[string]bool
	for {
		if !w.ready(stops) {
			return
		}
		missing, err := w.missing(ctx)
		if w.stoppedSince(stops) {
			return
		}
		if err != nil {
			w.failed(stops, err)
			return
		}
		if len(missing) == 0 {
			w.caughtUp(stops)
			return
		}
		for _, r := range missing {
			if before[r] {
				w.failed(stops, fmt.Errorf("the copy completed but %s is still missing on %s", r, w.Target))
				return
			}
		}
		before = map[string]bool{}
		for _, r := range missing {
			before[r] = true
		}
		if !w.copyOnce(ctx, stops, len(missing)) {
			return
		}
	}
}

// copyOnce runs one copy; false means the pass is over (failed or stopped). The copy is
// started and registered under the lock, so a pause or stop sees it or prevents it.
func (w *Worker) copyOnce(ctx context.Context, stops, behind int) bool {
	var c Copy
	began := w.Clock.Now()
	if w.CopyLock != nil {
		release, err := runlock.Exclusive(ctx, w.CopyLock(w.Target))
		if err != nil {
			if !w.stoppedSince(stops) && ctx.Err() == nil {
				w.failed(stops, err)
			}
			return false
		}
		defer release()
	}
	for {
		if !w.ready(stops) {
			return false
		}
		if ctx.Err() != nil {
			return false
		}
		w.mu.Lock()
		if w.stops != stops {
			w.mu.Unlock()
			return false
		}
		if w.paused {
			w.mu.Unlock()
			continue
		}
		var err error
		c, err = w.Runner.StartCopy(w.Target)
		if err != nil {
			w.mu.Unlock()
			w.failed(stops, err)
			return false
		}
		w.current = c
		w.state.Status, w.state.Since, w.state.Behind = Copying, began.Unix(), behind
		w.save()
		w.mu.Unlock()
		break
	}
	w.log("INFO", fmt.Sprintf("Copying %d revisions to %s storage.", behind, w.Target))
	err := c.Wait()
	w.mu.Lock()
	w.current = nil
	stopped := w.stops != stops
	w.mu.Unlock()
	if stopped {
		w.log("INFO", fmt.Sprintf("Copy to %s storage stopped; it copies again after the next backup.", w.Target))
		return false
	}
	if err != nil {
		w.failed(stops, err)
		return false
	}
	w.log("INFO", fmt.Sprintf("Copy to %s storage completed in %s.", w.Target, w.Clock.Now().Sub(began).Round(time.Second)))
	return true
}

func (w *Worker) caughtUp(stops int) {
	w.mu.Lock()
	if w.stops != stops {
		w.mu.Unlock()
		return
	}
	wasDown := w.state.DownSince != 0
	failingSince := w.state.FailingSince
	now := w.Clock.Now()
	w.state.Status, w.state.Since, w.state.Behind = Idle, now.Unix(), 0
	w.state.LastSuccess, w.state.FailingSince, w.state.NextRetry, w.state.LastError = now.Unix(), 0, 0, ""
	w.state.DownSince = 0
	w.attempt = 0
	w.save()
	w.mu.Unlock()
	if wasDown && w.Events.Notify != nil {
		w.Events.Notify("Storage Recovered", fmt.Sprintf("%s storage is caught up again after %s of failed copies.", w.Target, now.Sub(time.Unix(failingSince, 0)).Round(time.Minute)))
	}
}

// failed schedules the next try and reports the target down once it has kept failing. A
// stop that came after the pass started wins: nothing is recorded.
func (w *Worker) failed(stops int, err error) {
	w.mu.Lock()
	if w.stops != stops {
		w.mu.Unlock()
		return
	}
	now := w.Clock.Now()
	if w.state.FailingSince == 0 {
		w.state.FailingSince = now.Unix()
	}
	delay := Backoff[min(w.attempt, len(Backoff)-1)]
	w.attempt++
	// One retry lands on the DownAfter mark, so a target that keeps failing is reported
	// down then, not at whichever retry happens to follow it.
	if w.state.DownSince == 0 {
		if toDown := time.Unix(w.state.FailingSince, 0).Add(DownAfter).Sub(now); toDown > 0 && toDown < delay {
			delay = toDown
		}
	}
	w.state.NextRetry, w.state.LastError = now.Add(delay).Unix(), err.Error()
	failing := now.Sub(time.Unix(w.state.FailingSince, 0))
	var title, note string
	switch {
	case failing >= DownAfter && w.state.DownSince == 0:
		w.state.DownSince, w.state.LastAlert = now.Unix(), now.Unix()
		title, note = "Storage Down", fmt.Sprintf("Copies to %s storage have failed for %s (%v). Archiver keeps retrying every %s.", w.Target, failing.Round(time.Minute), err, Backoff[len(Backoff)-1])
	case w.state.DownSince != 0 && now.Sub(time.Unix(w.state.LastAlert, 0)) >= Remind:
		w.state.LastAlert = now.Unix()
		title, note = "Storage Still Down", fmt.Sprintf("Copies to %s storage have failed for %s (%v).", w.Target, failing.Round(time.Minute), err)
	}
	w.state.Status, w.state.Since = Retrying, now.Unix()
	if w.state.DownSince != 0 {
		w.state.Status = Down
	}
	w.save()
	w.mu.Unlock()
	w.log("WARNING", fmt.Sprintf("Copy to %s storage failed (%v); retrying in %s.", w.Target, err, delay))
	if title != "" && w.Events.Notify != nil {
		w.Events.Notify(title, note)
	}
}

// Stop ends whatever the worker is doing and cancels any scheduled retry; it copies again
// only when local next changes (ADR 15).
func (w *Worker) Stop() {
	w.mu.Lock()
	w.stops++
	w.wakesAtStop = w.wakes
	// A wake queued before the stop is a change the stop already covers.
	select {
	case <-w.wake:
	default:
	}
	c, paused, cancel := w.current, w.paused, w.cancel
	w.paused, w.state.Paused = false, false
	if w.resumed != nil {
		close(w.resumed)
		w.resumed = nil
	}
	w.state.Status, w.state.Since, w.state.NextRetry = Stopped, w.Clock.Now().Unix(), 0
	w.attempt = 0
	w.save()
	// Signals go out under the lock, so a pause or resume racing this cannot reorder them.
	if c != nil && paused {
		c.Resume()
	}
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if c != nil {
		c.Terminate()
	}
}

// Pause freezes a running copy and starts nothing new until Resume.
func (w *Worker) Pause() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.paused {
		return
	}
	w.paused, w.resumed = true, make(chan struct{})
	w.state.Paused = true
	w.save()
	if w.current != nil {
		w.current.Pause()
	}
}

// Resume continues what Pause froze.
func (w *Worker) Resume() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.paused {
		return
	}
	w.paused = false
	w.state.Paused = false
	close(w.resumed)
	w.resumed = nil
	w.save()
	if w.current != nil {
		w.current.Resume()
	}
}
