package copier

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SisyphusMD/archiver/internal/inuse"
)

// AllowLarge lets the next mirror pass delete more than half of an ID's revisions on the
// target: ADR 12's explicit override, for an intended change such as shorter retention.
func (w *Worker) AllowLarge() {
	w.mu.Lock()
	w.allowLarge = true
	w.mu.Unlock()
	w.nudge()
}

// ForceExhaustive has the next pass run an exhaustive prune whether or not one is due.
func (w *Worker) ForceExhaustive() {
	w.mu.Lock()
	w.forceExhaustive = true
	w.mu.Unlock()
	w.nudge()
}

// due reports whether something done last at last is due again every interval.
func (w *Worker) due(last int64, interval time.Duration) bool {
	return interval > 0 && w.Clock.Now().Sub(time.Unix(last, 0)) >= interval-slack(interval)
}

// slack keeps a fixed daily schedule from drifting one slot later each cycle, without
// swallowing a short interval: at most a tenth of it.
func slack(interval time.Duration) time.Duration {
	return min(grace, interval/10)
}

// exhaustiveInterval is how often this worker prunes unreferenced chunks: never when it
// does not maintain its target.
func (w *Worker) exhaustiveInterval() time.Duration {
	if !w.Upkeep.Mirror {
		return 0
	}
	return w.Upkeep.Exhaustive
}

// nextWake is how long an idle worker sleeps: until its next check or exhaustive prune,
// at most SafetyWake. Called with w.mu held.
func (w *Worker) nextWake() time.Duration {
	next := SafetyWake
	now := w.Clock.Now()
	for _, u := range []struct {
		last     int64
		interval time.Duration
	}{{max(w.state.LastCheck, w.state.CheckTried), w.Upkeep.Check}, {w.state.LastExhaustive, w.exhaustiveInterval()}} {
		if u.interval <= 0 {
			continue
		}
		d := time.Unix(u.last, 0).Add(u.interval - slack(u.interval)).Sub(now)
		if d < time.Second {
			d = time.Second
		}
		if d < next {
			next = d
		}
	}
	return next
}

// upkeep maintains a caught-up target: mirror local's retention, then an exhaustive prune
// when due, then a check when due. A failed mirror or prune backs off like a failed copy;
// a failed check is reported but does not, since it changes nothing.
func (w *Worker) upkeep(ctx context.Context, stops int) (ok bool) {
	defer func() {
		w.mu.Lock()
		if w.stops == stops && w.state.Status != Retrying && w.state.Status != Down {
			w.state.Status, w.state.Since = Idle, w.Clock.Now().Unix()
			w.save()
		}
		w.mu.Unlock()
	}()
	if w.Upkeep.Mirror && !w.mirror(ctx, stops) {
		return false
	}
	w.mu.Lock()
	w.state.CheckEvery = int64(w.Upkeep.Check / time.Second)
	force := w.forceExhaustive
	exhaustive := w.Upkeep.Mirror && (force || w.due(w.state.LastExhaustive, w.exhaustiveInterval()))
	// A failed check is tried again an interval later, like a successful one, not at once.
	check := w.due(max(w.state.LastCheck, w.state.CheckTried), w.Upkeep.Check)
	w.mu.Unlock()
	if exhaustive {
		w.log("INFO", fmt.Sprintf("Exhaustive prune of %s storage (unreferenced chunks).", w.Target))
		err, stopped := w.run(ctx, stops, Pruning, 0, false, func() (Copy, error) {
			return w.Runner.Start("prune", "-all", "-exhaustive", "-storage", w.Target, "-threads", w.Upkeep.Threads)
		})
		if stopped {
			return false
		}
		if err != nil {
			w.failed(stops, fmt.Errorf("exhaustive prune: %w", err))
			return false
		}
		w.record(stops, func(s *State) { s.LastExhaustive = w.Clock.Now().Unix() })
		if force {
			w.mu.Lock()
			w.forceExhaustive = false
			w.mu.Unlock()
		}
		w.log("INFO", fmt.Sprintf("Exhaustive prune of %s storage completed.", w.Target))
	}
	w.mu.Lock()
	changed := w.wakes != w.passWakes
	w.mu.Unlock()
	// A backup since this pass began needs copying first; the check waits for the next
	// idle moment.
	if check && !changed {
		w.log("INFO", fmt.Sprintf("Checking %s storage.", w.Target))
		err, stopped := w.run(ctx, stops, Checking, 0, true, func() (Copy, error) {
			return w.Runner.Start("check", "-all", "-storage", w.Target, "-fossils", "-resurrect", "-stats", "-threads", w.Upkeep.Threads)
		})
		if stopped {
			w.log("INFO", fmt.Sprintf("Check of %s storage interrupted; it runs again when the worker is next idle.", w.Target))
			return !w.stoppedSince(stops)
		}
		if err != nil {
			msg := fmt.Sprintf("Check of %s storage failed (%v). Review copies.log.", w.Target, err)
			w.record(stops, func(s *State) { s.CheckFailed, s.CheckTried = msg, w.Clock.Now().Unix() })
			w.log("ERROR", msg)
			if w.Events.Notify != nil {
				w.Events.Notify("Storage Check Failed", msg)
			}
			return true
		}
		w.record(stops, func(s *State) {
			s.LastCheck, s.CheckTried, s.CheckFailed = w.Clock.Now().Unix(), w.Clock.Now().Unix(), ""
		})
		w.log("INFO", fmt.Sprintf("Check of %s storage completed.", w.Target))
	}
	return true
}

// record changes the state unless a stop came after the pass started.
func (w *Worker) record(stops int, f func(*State)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stops == stops {
		f(&w.state)
		w.save()
	}
}

// mirror deletes on the target the revisions local has pruned. false means the pass is
// over (failed or stopped).
func (w *Worker) mirror(ctx context.Context, stops int) bool {
	// A restore reading from the target registers its revision under this gate; held
	// from listing to the last deletion, no revision a restore reads is deleted (ADR 19).
	if w.InUseDir != "" {
		release, err := inuse.Gate(ctx, w.InUseDir, w.Target, true)
		if err != nil {
			if !w.stoppedSince(stops) {
				w.failed(stops, fmt.Errorf("mirror: %w", err))
			}
			return false
		}
		defer release()
	}
	local, err := w.Runner.Revisions(ctx, w.Primary)
	if err == nil {
		var target map[string]bool
		target, err = w.Runner.Revisions(ctx, w.Target)
		if err == nil {
			return w.applyMirror(ctx, stops, local, target)
		}
	}
	if w.stoppedSince(stops) {
		return false
	}
	// Nothing is deleted without both lists.
	w.failed(stops, fmt.Errorf("mirror listing: %w", err))
	return false
}

func (w *Worker) applyMirror(ctx context.Context, stops int, local, target map[string]bool) bool {
	w.mu.Lock()
	allow := w.allowLarge
	w.mu.Unlock()
	plan := PlanMirror(local, target, w.owns(), allow)
	if w.InUseDir != "" {
		busy, err := inuse.Read(w.InUseDir, w.Target)
		if err != nil {
			w.failed(stops, fmt.Errorf("mirror: reading revisions in use: %w", err))
			return false
		}
		w.leaveInUse(plan, busy)
	}

	var refused []string
	for id, why := range plan.Refused {
		refused = append(refused, id+" ("+why+")")
	}
	sort.Strings(refused)
	note := strings.Join(refused, ", ")
	w.mu.Lock()
	report := note != "" && note != w.state.MirrorRefused
	w.state.MirrorRefused = note
	w.save()
	w.mu.Unlock()
	if note != "" {
		msg := fmt.Sprintf("Mirror deletions on %s storage refused: %s. If local's retention was shortened on purpose, run 'archiver mirror --allow-large'.", w.Target, note)
		w.log("WARNING", msg)
		if report && w.Events.Notify != nil {
			w.Events.Notify("Mirror Refused", msg)
		}
	}

	ids := make([]string, 0, len(plan.Delete))
	for id := range plan.Delete {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		args := []string{"prune", "-storage", w.Target, "-id", id}
		var revs []string
		for _, r := range plan.Delete[id] {
			args = append(args, "-r", strconv.Itoa(r))
			revs = append(revs, strconv.Itoa(r))
		}
		w.log("INFO", fmt.Sprintf("Mirror: deleting revisions %s of %s from %s storage, as local has pruned them.", strings.Join(revs, ","), id, w.Target))
		err, stopped := w.run(ctx, stops, Mirroring, 0, false, func() (Copy, error) { return w.Runner.Start(args...) })
		if stopped {
			return false
		}
		if err != nil {
			w.failed(stops, fmt.Errorf("mirror prune of %s: %w", id, err))
			return false
		}
	}
	// A prune also deletes the fossils of earlier ones once duplicacy's criteria are met. A
	// pass with nothing to delete runs one with no revisions selected, so they are reclaimed
	// even when retention deletes nothing for a while.
	if len(ids) == 0 {
		err, stopped := w.run(ctx, stops, Mirroring, 0, false, func() (Copy, error) {
			return w.Runner.Start("prune", "-storage", w.Target, "-threads", w.Upkeep.Threads)
		})
		if stopped {
			return false
		}
		if err != nil {
			w.failed(stops, fmt.Errorf("fossil collection prune: %w", err))
			return false
		}
	}
	w.record(stops, func(s *State) { s.LastMirror = w.Clock.Now().Unix() })
	if allow {
		w.mu.Lock()
		w.allowLarge = false
		w.mu.Unlock()
	}
	return true
}

// PlanNow lists both storages and returns the plan a mirror pass would carry out now,
// without deleting anything.
func (w *Worker) PlanNow(ctx context.Context) (MirrorPlan, error) {
	if err := w.Runner.Prepare(ctx); err != nil {
		return MirrorPlan{}, err
	}
	local, err := w.Runner.Revisions(ctx, w.Primary)
	if err != nil {
		return MirrorPlan{}, err
	}
	target, err := w.Runner.Revisions(ctx, w.Target)
	if err != nil {
		return MirrorPlan{}, err
	}
	w.mu.Lock()
	allow := w.allowLarge
	w.mu.Unlock()
	return PlanMirror(local, target, w.owns(), allow), nil
}

// Describe renders a plan on one line, for `archiver mirror --dry-run`.
func (p MirrorPlan) Describe() string {
	var parts []string
	ids := make([]string, 0, len(p.Delete))
	for id := range p.Delete {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		var revs []string
		for _, r := range p.Delete[id] {
			revs = append(revs, strconv.Itoa(r))
		}
		parts = append(parts, fmt.Sprintf("delete %s revisions %s", id, strings.Join(revs, ",")))
	}
	refused := make([]string, 0, len(p.Refused))
	for id, why := range p.Refused {
		refused = append(refused, fmt.Sprintf("refuse %s (%s)", id, why))
	}
	sort.Strings(refused)
	parts = append(parts, refused...)
	if len(parts) == 0 {
		return "nothing to delete"
	}
	return strings.Join(parts, "; ")
}

// owns reports whether a snapshot ID is this deployment's; with no Own set, none is.
func (w *Worker) owns() func(string) bool {
	if w.Upkeep.Own == nil {
		return func(string) bool { return false }
	}
	ids := w.Upkeep.Own()
	return func(id string) bool { return ids[id] }
}

// leaveInUse drops from a plan the revisions a restore is reading; the next pass deletes them.
func (w *Worker) leaveInUse(plan MirrorPlan, busy *inuse.Set) {
	for id, revs := range plan.Delete {
		var keep, held []int
		for _, r := range revs {
			if busy.Has(id, r) {
				held = append(held, r)
			} else {
				keep = append(keep, r)
			}
		}
		if len(held) > 0 {
			w.log("INFO", fmt.Sprintf("Mirror: leaving revisions %s of %s on %s storage for the next pass: a restore is reading them.", joinInts(held), id, w.Target))
		}
		if len(keep) == 0 {
			delete(plan.Delete, id)
		} else {
			plan.Delete[id] = keep
		}
	}
}

func joinInts(revs []int) string {
	s := make([]string, len(revs))
	for i, r := range revs {
		s[i] = strconv.Itoa(r)
	}
	return strings.Join(s, ",")
}
