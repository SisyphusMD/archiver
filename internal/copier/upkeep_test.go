package copier

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/inuse"
)

func upkeepWorker(r *fakeRunner, u Upkeep) (*Worker, *fakeClock, *recorder) {
	rec := &recorder{}
	w, c := newWorker(r, rec, State{})
	w.Upkeep = u
	return w, c, rec
}

func commandsNamed(r *fakeRunner, name string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, c := range r.commands {
		if c[0] == name {
			out = append(out, strings.Join(c, " "))
		}
	}
	return out
}

func TestMirrorAfterCatchUp(t *testing.T) {
	r := &fakeRunner{local: revisions("nas-app", 3, 4, 5), target: revisions("nas-app", 1, 2, 3, 4)}
	w, _, _ := upkeepWorker(r, Upkeep{Own: ownApp, Mirror: true})
	w.pass(w.stops, nil)
	if fmt.Sprint(r.target) != fmt.Sprint(revisions("nas-app", 3, 4, 5)) {
		t.Fatalf("target now %v, want local's revisions", r.target)
	}
	if got := commandsNamed(r, "prune"); len(got) != 1 || got[0] != "prune -storage offsite -id nas-app -r 1 -r 2" {
		t.Fatalf("prunes %q", got)
	}
	if s := w.State(); s.Status != Idle || s.LastMirror == 0 {
		t.Fatalf("state %+v", s)
	}
	if strings.Contains(fmt.Sprint(r.commands), "-exclusive") || strings.Contains(fmt.Sprint(r.commands), "-keep") {
		t.Fatal("a mirror prune must never be exclusive or apply retention of its own")
	}
}

func TestMirrorRefusalReportedOnceThenOverridden(t *testing.T) {
	r := &fakeRunner{local: revisions("nas-app", 9, 10), target: revisions("nas-app", 1, 2, 3, 9, 10)}
	w, _, rec := upkeepWorker(r, Upkeep{Own: ownApp, Mirror: true})
	w.pass(w.stops, nil)
	w.pass(w.stops, nil)
	if got := rec.titles(); len(got) != 1 || got[0] != "Mirror Refused" {
		t.Fatalf("notes %q: one alert for an unchanged refusal", got)
	}
	for _, p := range commandsNamed(r, "prune") {
		if strings.Contains(p, " -r ") {
			t.Fatalf("a refused plan deleted something: %s", p)
		}
	}
	w.AllowLarge()
	w.pass(w.stops, nil)
	if fmt.Sprint(r.target) != fmt.Sprint(revisions("nas-app", 9, 10)) {
		t.Fatalf("override did not apply: target %v", r.target)
	}
	w.mu.Lock()
	allow := w.allowLarge
	w.mu.Unlock()
	if allow {
		t.Fatal("the override must apply to one pass only")
	}
}

func TestExhaustiveOncePerInterval(t *testing.T) {
	r := &fakeRunner{local: revisions("nas-app", 1), target: revisions("nas-app", 1)}
	w, c, _ := upkeepWorker(r, Upkeep{Own: ownApp, Mirror: true, Exhaustive: 30 * 24 * time.Hour})
	w.pass(w.stops, nil)
	w.pass(w.stops, nil)
	if got := exhaustives(r); got != 1 {
		t.Fatalf("%d exhaustive prunes, want one", got)
	}
	c.Advance(30*24*time.Hour - grace)
	w.pass(w.stops, nil)
	if got := exhaustives(r); got != 2 {
		t.Fatalf("%d exhaustive prunes, want a second a month later", got)
	}
	// A deployment that does not maintain the storage never prunes it.
	r2 := &fakeRunner{local: revisions("nas-app", 1), target: revisions("nas-app", 1)}
	w2, _, _ := upkeepWorker(r2, Upkeep{Own: ownApp, Mirror: false, Exhaustive: time.Hour, Check: time.Hour})
	w2.pass(w2.stops, nil)
	if len(commandsNamed(r2, "prune")) != 0 || len(commandsNamed(r2, "check")) != 1 {
		t.Fatalf("commands %v", r2.commands)
	}
}

func TestCheckFailureReportsWithoutBackoff(t *testing.T) {
	r := &fakeRunner{local: revisions("nas-app", 1), target: revisions("nas-app", 1), failCmd: map[string]error{"check": errors.New("missing chunks")}}
	w, c, rec := upkeepWorker(r, Upkeep{Own: ownApp, Check: 24 * time.Hour})
	w.pass(w.stops, nil)
	s := w.State()
	if s.Status != Idle || s.NextRetry != 0 || s.CheckFailed == "" || s.LastCheck != 0 {
		t.Fatalf("state %+v: a failed check is reported, not retried", s)
	}
	if got := rec.titles(); len(got) != 1 || got[0] != "Storage Check Failed" {
		t.Fatalf("notes %q", got)
	}
	w.pass(w.stops, nil)
	if got := commandsNamed(r, "check"); len(got) != 1 {
		t.Fatalf("a failed check ran again at once (%d checks)", len(got))
	}
	r.mu.Lock()
	r.failCmd = nil
	r.mu.Unlock()
	c.Advance(24 * time.Hour)
	w.pass(w.stops, nil)
	if s := w.State(); s.LastCheck == 0 || s.CheckFailed != "" {
		t.Fatalf("state %+v: the failed check runs again and clears", s)
	}
	w.pass(w.stops, nil)
	if got := commandsNamed(r, "check"); len(got) != 2 {
		t.Fatalf("checks %d, want no third within the interval", len(got))
	}
}

// A mirror prune that keeps failing escalates like a failed copy: catching up again must
// not wipe its failure history.
func TestFailingMirrorReachesDown(t *testing.T) {
	r := &fakeRunner{local: revisions("nas-app", 3, 4, 5), target: revisions("nas-app", 1, 2, 3, 4, 5), failCmd: map[string]error{"prune": errors.New("permission denied")}}
	w, c, rec := upkeepWorker(r, Upkeep{Own: ownApp, Mirror: true})
	for i := 0; i < 8 && w.State().DownSince == 0; i++ {
		w.pass(w.stops, nil)
		c.Advance(time.Unix(w.State().NextRetry, 0).Sub(c.Now()))
	}
	if w.State().DownSince == 0 || len(rec.titles()) == 0 || rec.titles()[0] != "Storage Down" {
		t.Fatalf("state %+v, notes %q", w.State(), rec.titles())
	}
}

// A backup that arrives while upkeep runs puts off the check, so copying it comes first.
func TestWakeDuringUpkeepPutsOffCheck(t *testing.T) {
	r := &fakeRunner{local: revisions("nas-app", 1), target: revisions("nas-app", 1)}
	w, _, _ := upkeepWorker(r, Upkeep{Own: ownApp, Check: time.Hour})
	w.mu.Lock()
	w.passWakes = w.wakes
	w.mu.Unlock()
	w.Wake()
	w.pass(w.stops, nil)
	if got := commandsNamed(r, "check"); len(got) != 0 {
		t.Fatalf("checks %q started despite a pending backup", got)
	}
}

func TestShortCheckIntervalAndNoSpin(t *testing.T) {
	r := &fakeRunner{local: revisions("nas-app", 1), target: revisions("nas-app", 1)}
	w, c, _ := upkeepWorker(r, Upkeep{Own: ownApp, Check: 30 * time.Minute, Exhaustive: 30 * 24 * time.Hour})
	w.pass(w.stops, nil)
	c.Advance(10 * time.Minute)
	w.pass(w.stops, nil)
	if got := commandsNamed(r, "check"); len(got) != 1 {
		t.Fatalf("checks %d: a 30m interval must not check again after 10 minutes", len(got))
	}
	w.mu.Lock()
	next := w.nextWake()
	w.mu.Unlock()
	// Not mirroring, so the exhaustive deadline is not this worker's; the next wake is the
	// check's, about 17 minutes out (30m less a tenth, less the 10m that passed).
	if next < 15*time.Minute || next > 20*time.Minute {
		t.Fatalf("next wake in %v", next)
	}
}

func ownApp() map[string]bool { return map[string]bool{"nas-app": true} }

func exhaustives(r *fakeRunner) int {
	n := 0
	for _, p := range commandsNamed(r, "prune") {
		if strings.Contains(p, "-exhaustive") {
			n++
		}
	}
	return n
}

// A pass with nothing to delete still runs a prune that selects no revisions, which
// reclaims earlier passes' fossils.
func TestEmptyMirrorPassProcessesFossils(t *testing.T) {
	r := &fakeRunner{local: revisions("nas-app", 1), target: revisions("nas-app", 1)}
	w, _, _ := upkeepWorker(r, Upkeep{Own: ownApp, Mirror: true})
	w.pass(w.stops, nil)
	if got := commandsNamed(r, "prune"); len(got) != 1 || got[0] != "prune -storage offsite -threads " {
		t.Fatalf("prunes %q", got)
	}
}

func TestForcedExhaustive(t *testing.T) {
	r := &fakeRunner{local: revisions("nas-app", 1), target: revisions("nas-app", 1)}
	w, _, _ := upkeepWorker(r, Upkeep{Own: ownApp, Mirror: true})
	w.pass(w.stops, nil)
	if exhaustives(r) != 0 {
		t.Fatal("exhaustive with PRUNE_EXHAUSTIVE_FREQUENCY off")
	}
	w.ForceExhaustive()
	w.pass(w.stops, nil)
	w.pass(w.stops, nil)
	if got := exhaustives(r); got != 1 {
		t.Fatalf("%d exhaustive prunes after one forced", got)
	}
}

// An upkeep request is not a change to local: a stopped worker stays stopped.
func TestUpkeepRequestKeepsStop(t *testing.T) {
	r := &fakeRunner{local: revisions("nas-app", 1, 2), target: revisions("nas-app", 1)}
	w, _, _ := upkeepWorker(r, Upkeep{Own: ownApp, Mirror: true})
	w.Stop()
	done := make(chan struct{})
	go w.Run(done)
	w.ForceExhaustive()
	w.AllowLarge()
	time.Sleep(200 * time.Millisecond)
	close(done)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.copies != 0 || len(r.commands) != 0 {
		t.Fatalf("a stopped worker acted on an upkeep request: %d copies, %v", r.copies, r.commands)
	}
}

// A restore reading a revision on the target keeps mirroring off it until it ends.
func TestMirrorLeavesRevisionsInUse(t *testing.T) {
	r := &fakeRunner{local: revisions("nas-app", 3, 4, 5), target: revisions("nas-app", 1, 2, 3, 4, 5)}
	w, _, _ := upkeepWorker(r, Upkeep{Own: ownApp, Mirror: true})
	w.InUseDir = t.TempDir()
	h, err := inuse.Hold(w.InUseDir, "restore", []inuse.Revision{{Storage: "offsite", ID: "nas-app", Rev: 1}, {Storage: "local", ID: "nas-app", Rev: 2}})
	if err != nil {
		t.Fatal(err)
	}
	w.pass(w.stops, nil)
	if fmt.Sprint(r.target) != fmt.Sprint(revisions("nas-app", 1, 3, 4, 5)) {
		t.Fatalf("target %v: only the revision read on offsite is kept", r.target)
	}
	h.Release()
	w.pass(w.stops, nil)
	if fmt.Sprint(r.target) != fmt.Sprint(revisions("nas-app", 3, 4, 5)) {
		t.Fatalf("target %v after the restore ended", r.target)
	}
}

// A copy registers the local revisions it still needs for as long as its pass runs.
func TestCopyRegistersRevisionsInUse(t *testing.T) {
	r := &fakeRunner{local: revisions("nas-app", 1, 2, 3), target: revisions("nas-app", 1), block: true, started: make(chan *fakeCopy, 1)}
	w, _, _ := upkeepWorker(r, Upkeep{})
	w.InUseDir = t.TempDir()
	go w.pass(w.stops, nil)
	c := <-r.started
	busy, err := inuse.Read(w.InUseDir, "local")
	if err != nil || busy.Has("nas-app", 1) || !busy.Has("nas-app", 2) || !busy.Has("nas-app", 3) {
		t.Fatalf("in use during the copy: %+v %v", busy, err)
	}
	// The copy also takes revisions made after its listing, of any snapshot ID.
	if !busy.Has("nas-app", 4) || !busy.Has("nas-new", 1) {
		t.Fatalf("revisions newer than the listing not covered: %+v", busy)
	}
	close(c.release)
	for i := 0; i < 100; i++ {
		if busy, _ := inuse.Read(w.InUseDir, "local"); busy.Empty() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("still registered after the pass")
}
