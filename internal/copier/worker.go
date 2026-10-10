// Package copier keeps each secondary storage caught up with the primary (ADR 11): one
// worker per target, copying every snapshot the target lacks, one copy at a time, and
// retrying a failed copy on a fixed backoff until the target has caught up (ADR 16).
package copier

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SisyphusMD/archiver/internal/inuse"
	"github.com/SisyphusMD/archiver/internal/notify"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

// Backoff is the wait before each retry of a failed copy; the last repeats.
var Backoff = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute}

const (
	// DownAfter is how long a target must keep failing to count as down.
	DownAfter = 30 * time.Minute
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
	// Upkeep after a catch-up (ADRs 12, 17, 18).
	Mirroring = "mirroring" // deleting on the target what local has pruned
	Pruning   = "pruning"   // exhaustive prune of unreferenced chunks
	Checking  = "checking"
)

// State is what a worker reports and keeps across restarts.
type State struct {
	Target         string `json:"target"`
	Status         string `json:"status"`
	Paused         bool   `json:"paused,omitempty"`
	Since          int64  `json:"since"`
	Behind         int    `json:"behind"`
	BehindSince    int64  `json:"behind_since,omitempty"` // when it last fell behind, kept until it catches up
	LastSuccess    int64  `json:"last_success,omitempty"`
	FailingSince   int64  `json:"failing_since,omitempty"`
	DownSince      int64  `json:"down_since,omitempty"` // kept through retries, so down is reported once
	NextRetry      int64  `json:"next_retry,omitempty"`
	LastError      string `json:"last_error,omitempty"`
	LastMirror     int64  `json:"last_mirror,omitempty"`
	LastExhaustive int64  `json:"last_exhaustive,omitempty"`
	LastCheck      int64  `json:"last_check,omitempty"`
	CheckEvery     int64  `json:"check_every,omitempty"` // the check interval in seconds, for status
	CheckTried     int64  `json:"check_tried,omitempty"` // the last check attempt; the schedule counts from it
	CheckFailed    string `json:"check_failed,omitempty"`
	MirrorRefused  string `json:"mirror_refused,omitempty"` // last refusal reported, so it is reported once
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
	// Start starts another duplicacy command (prune, check) in the target's repository.
	Start(args ...string) (Copy, error)
}

// Clock is the time source, replaced in tests.
type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

// Events are what a worker tells the rest of Archiver.
type Events struct {
	Log func(level, msg string)
	// Raise and Clear open and close an incident (ADRs 33, 36): notified once, repeated on
	// ALERT_REPEAT_INTERVAL while it lasts, and its recovery said.
	Raise func(key string, k notify.Kind, title, msg string)
	Clear func(key, title, msg string)
	// Checkin tells the target's check-in URL it is caught up, or that it is failing (ADR 37).
	Checkin func(ok bool, msg string)
	Save    func(State)
}

// Upkeep is a worker's maintenance of its target (ADRs 12, 17, 18). Shared storages are
// maintained by one deployment only: the others leave Mirror and Exhaustive off (with
// PRUNE_BACKUPS=false) and Check off (with CHECK_BACKUPS=false).
type Upkeep struct {
	Own        func() map[string]bool // this deployment's snapshot IDs, exactly
	Mirror     bool                   // delete on the target what local has pruned
	Exhaustive time.Duration          // how often to prune unreferenced chunks; 0 is never
	Check      time.Duration          // how often to check the target; 0 is never
	Threads    string
}

// grace keeps a fixed daily schedule from drifting one slot later each cycle.
const grace = time.Hour

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
	// InUseDir is the registry of revisions in use (layout.InUseDir); empty registers none.
	InUseDir string
	// Upkeep is what the worker maintains on its target once it has caught up.
	Upkeep Upkeep

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
	checking           bool           // a check is current; a wake interrupts it (a check never delays a copy)
	passWakes          int            // wakes when the current pass began: a later one means local changed meanwhile
	forceExhaustive    bool           // the next pass prunes exhaustively whether or not one is due
	held               *inuse.Holding // local revisions this pass's copy still reads; only the Run goroutine touches it
	listedNewest       map[string]int // the newest revision of each ID at the last local listing, likewise
	allowLarge         bool           // the next mirror pass may delete more than half of an ID (ADR 12's override)
	// Probe checks the target can be reached before each pass, ended by ctx; nil skips it.
	Probe func(ctx context.Context) error
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
// nudge starts a pass for a queued upkeep request. Unlike Wake it is not a change to
// local, so a stopped worker stays stopped and runs the request after its next wake.
func (w *Worker) nudge() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *Worker) Wake() {
	w.mu.Lock()
	w.wakes++
	var check Copy
	if w.checking {
		check = w.current
	}
	w.mu.Unlock()
	if check != nil {
		check.Terminate()
	}
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
			timer = w.Clock.After(w.nextWake())
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
		w.passWakes = w.wakes
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

// missing lists what the target lacks, and registers those revisions as in use on local so
// no local prune deletes them under the copy (ADR 19).
func (w *Worker) missing(ctx context.Context) ([]string, error) {
	local, err := w.listLocal(ctx)
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
	if w.held != nil {
		if err := w.held.Narrow(w.revisions(w.Primary, out)); err != nil {
			return nil, fmt.Errorf("registering revisions in use: %w", err)
		}
	}
	return out, nil
}

// listLocal lists local and, until the target's listing shows which it lacks, registers
// all of them, both under the gate a local prune holds exclusively.
func (w *Worker) listLocal(ctx context.Context) (map[string]bool, error) {
	if w.InUseDir == "" {
		return w.Runner.Revisions(ctx, w.Primary)
	}
	release, err := inuse.Gate(ctx, w.InUseDir, w.Primary, false)
	if err != nil {
		return nil, err
	}
	defer release()
	local, err := w.Runner.Revisions(ctx, w.Primary)
	if err != nil {
		return nil, err
	}
	all := make([]string, 0, len(local))
	w.listedNewest = map[string]int{}
	for r := range local {
		all = append(all, r)
		id, rev, _ := strings.Cut(r, ":")
		if n, err := strconv.Atoi(rev); err == nil && n > w.listedNewest[id] {
			w.listedNewest[id] = n
		}
	}
	revs := w.revisions(w.Primary, all)
	if w.held == nil {
		w.held, err = inuse.Hold(w.InUseDir, "copy-"+w.Target, revs)
	} else {
		err = w.held.Narrow(revs)
	}
	if err != nil {
		return nil, fmt.Errorf("registering revisions in use: %w", err)
	}
	return local, nil
}

// revisions turns "id:revision" keys into in-use entries on storage. The all-snapshot copy
// also takes whatever local gained after the listing, so every revision past the newest
// listed of each ID, and every revision of an ID the listing did not have, is in use too.
func (w *Worker) revisions(storage string, keys []string) []inuse.Revision {
	out := make([]inuse.Revision, 0, len(keys)+len(w.listedNewest)+1)
	for _, k := range keys {
		id, rev, ok := strings.Cut(k, ":")
		if n, err := strconv.Atoi(rev); ok && err == nil {
			out = append(out, inuse.Revision{Storage: storage, ID: id, Rev: n})
		}
	}
	for id, newest := range w.listedNewest {
		out = append(out, inuse.Revision{Storage: storage, ID: id, Rev: newest + 1, AndNewer: true})
	}
	return append(out, inuse.Revision{Storage: storage, ID: "*", Rev: 1, AndNewer: true})
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
		w.held.Release()
		w.held = nil
	}()
	// A listing or preparation already running when a pause comes finishes (both are
	// short reads); none starts while paused, and a copy, the long part, is frozen.
	if !w.ready(stops) {
		return
	}
	// Probed first (ADR 34): a target still down costs seconds, not duplicacy's retries.
	if !w.reachable(ctx, stops) {
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
			// Failure history clears only once upkeep has worked too, so a mirror or
			// exhaustive prune that keeps failing still reaches down.
			if w.upkeep(ctx, stops) {
				w.caughtUp(stops)
			}
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

// copyOnce runs one copy; false means the pass is over (failed or stopped).
func (w *Worker) copyOnce(ctx context.Context, stops, behind int) bool {
	began := w.Clock.Now()
	w.log("INFO", fmt.Sprintf("Copying %d revisions to %s storage.", behind, w.Target))
	err, stopped := w.run(ctx, stops, Copying, behind, false, func() (Copy, error) { return w.Runner.StartCopy(w.Target) })
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

// run runs one duplicacy command against the target under the target's copy lock. It is
// started and registered under the worker's lock, so a pause or stop sees it or prevents
// it. stopped means a stop ended it (or it never started); a check also reports stopped
// when a wake interrupted it.
func (w *Worker) run(ctx context.Context, stops int, status string, behind int, check bool, start func() (Copy, error)) (err error, stopped bool) {
	if w.CopyLock != nil {
		release, err := runlock.Exclusive(ctx, w.CopyLock(w.Target))
		if err != nil {
			if w.stoppedSince(stops) || ctx.Err() != nil {
				return nil, true
			}
			return err, false
		}
		defer release()
	}
	var c Copy
	wakes := 0
	for {
		if !w.ready(stops) || ctx.Err() != nil {
			return nil, true
		}
		w.mu.Lock()
		// A check never starts once a backup has woken the worker: copying comes first.
		if w.stops != stops || (check && w.wakes != w.passWakes) {
			w.mu.Unlock()
			return nil, true
		}
		if w.paused {
			w.mu.Unlock()
			continue
		}
		c, err = start()
		if err != nil {
			w.mu.Unlock()
			return err, false
		}
		w.current, w.checking, wakes = c, check, w.passWakes
		w.state.Status, w.state.Since, w.state.Behind = status, w.Clock.Now().Unix(), behind
		if behind > 0 && w.state.BehindSince == 0 {
			w.state.BehindSince = w.state.Since
		}
		w.save()
		w.mu.Unlock()
		break
	}
	err = c.Wait()
	w.mu.Lock()
	w.current, w.checking = nil, false
	stopped = w.stops != stops || (check && w.wakes != wakes)
	w.mu.Unlock()
	if stopped {
		return nil, true
	}
	return err, false
}

func (w *Worker) caughtUp(stops int) {
	w.mu.Lock()
	if w.stops != stops {
		w.mu.Unlock()
		return
	}
	now := w.Clock.Now()
	w.state.Status, w.state.Since, w.state.Behind, w.state.BehindSince = Idle, now.Unix(), 0, 0
	w.state.LastSuccess, w.state.FailingSince, w.state.NextRetry, w.state.LastError = now.Unix(), 0, 0, ""
	w.state.DownSince = 0
	w.attempt = 0
	w.save()
	w.mu.Unlock()
	// Every caught-up pass, not only the first: a recovery notice an outage kept from a
	// destination is retried here (Clear says nothing when no incident is open).
	w.clear("copy:"+w.Target, "Storage Recovered", fmt.Sprintf("%s storage is caught up again.", w.Target))
	// Caught up is not good while its last check failed: the monitor hears that until a
	// check passes.
	w.mu.Lock()
	checkFailed := w.state.CheckFailed
	w.mu.Unlock()
	if checkFailed != "" {
		w.checkin(false, checkFailed)
	} else {
		w.checkin(true, "caught up")
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
	var note string
	if failing >= DownAfter {
		if w.state.DownSince == 0 {
			w.state.DownSince = now.Unix()
		}
		// Raised at every failure once down; the incident repeats only on its interval.
		note = fmt.Sprintf("Copies to %s storage have failed for %s (%v). Archiver keeps retrying every %s.", w.Target, failing.Round(time.Minute), err, Backoff[len(Backoff)-1])
	}
	w.state.Status, w.state.Since = Retrying, now.Unix()
	if w.state.DownSince != 0 {
		w.state.Status = Down
	}
	w.save()
	w.mu.Unlock()
	w.log("WARNING", fmt.Sprintf("Copy to %s storage failed (%v); retrying in %s.", w.Target, err, delay))
	w.checkin(false, err.Error())
	if note != "" {
		w.raise("copy:"+w.Target, notify.Failure, "Storage Down", note)
	}
}

func (w *Worker) checkin(ok bool, msg string) {
	if w.Events.Checkin != nil {
		w.Events.Checkin(ok, msg)
	}
}

func (w *Worker) raise(key string, k notify.Kind, title, msg string) {
	if w.Events.Raise != nil {
		w.Events.Raise(key, k, title, msg)
	}
}

func (w *Worker) clear(key, title, msg string) {
	if w.Events.Clear != nil {
		w.Events.Clear(key, title, msg)
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
