package daemon

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// instantClock jumps straight to whatever time is waited for, so a schedule plays out
// without real waiting. A job advances it by its own duration.
type instantClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *instantClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *instantClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	ch := make(chan time.Time, 1)
	ch <- c.now
	return ch
}

func (c *instantClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

var start = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func mustJobs(t *testing.T, m map[string]string) []Job {
	t.Helper()
	jobs, err := Jobs(env(m), start)
	if err != nil {
		t.Fatal(err)
	}
	return jobs
}

func TestJobs(t *testing.T) {
	if jobs := mustJobs(t, nil); len(jobs) != 0 {
		t.Fatalf("no schedules gave %d jobs", len(jobs))
	}
	jobs := mustJobs(t, map[string]string{"BACKUP_SCHEDULE": "0 3 * * *", "MAINTENANCE_SCHEDULE": "*/5 * * * * * *"})
	if len(jobs) != 2 || jobs[0].Name != "backup" || jobs[1].Name != "maintenance" {
		t.Fatalf("got %+v", jobs)
	}
	for _, tc := range []struct{ env, spec, want string }{
		{"BACKUP_SCHEDULE", "not a cron line", "invalid BACKUP_SCHEDULE"},
		{"MAINTENANCE_SCHEDULE", "0 3 * * * * * *", "invalid MAINTENANCE_SCHEDULE"},
		{"BACKUP_SCHEDULE", "0 0 30 2 *", "never fires"},
	} {
		_, err := Jobs(env(map[string]string{tc.env: tc.spec}), start)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s=%q: got %v, want an error containing %q", tc.env, tc.spec, err, tc.want)
		}
	}
}

// runs plays a schedule until it has started n runs, each lasting took, and returns their
// start times.
func runs(t *testing.T, spec string, took time.Duration, n int) []time.Time {
	t.Helper()
	clock := &instantClock{now: start}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var got []time.Time
	Run(ctx, clock, io.Discard, mustJobs(t, map[string]string{"BACKUP_SCHEDULE": spec}), func(Job) int {
		got = append(got, clock.Now())
		clock.advance(took)
		if len(got) == n {
			cancel()
		}
		return 0
	})
	return got
}

func TestRunsOnSchedule(t *testing.T) {
	got := runs(t, "0 3 * * *", time.Hour, 2)
	want := []time.Time{start.Add(15 * time.Hour), start.Add(39 * time.Hour)}
	if len(got) != 2 || !got[0].Equal(want[0]) || !got[1].Equal(want[1]) {
		t.Fatalf("ran at %v, want %v", got, want)
	}
}

// A run that overruns later slots skips them rather than starting again at once, the
// behavior supercronic had.
func TestOverrunSkipsSlots(t *testing.T) {
	got := runs(t, "0 * * * *", 150*time.Minute, 2)
	want := []time.Time{start.Add(time.Hour), start.Add(4 * time.Hour)}
	if len(got) != 2 || !got[0].Equal(want[0]) || !got[1].Equal(want[1]) {
		t.Fatalf("ran at %v, want %v", got, want)
	}
}

// Shutdown waits for a running job instead of abandoning it.
func TestRunWaitsForRunningJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	finished := false
	done := make(chan struct{})
	go func() {
		Run(ctx, RealClock, io.Discard, mustJobs(t, map[string]string{"BACKUP_SCHEDULE": "* * * * * * *"}), func(Job) int {
			cancel()
			<-release
			finished = true
			return 0
		})
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("Run returned while its job was still running")
	case <-time.After(1500 * time.Millisecond):
	}
	close(release)
	<-done
	if !finished {
		t.Fatal("job did not finish")
	}
}

func TestSocket(t *testing.T) {
	path := t.TempDir() + "/d.sock"
	var got []string
	ln, err := Serve(path, func(cmd string) string { got = append(got, cmd); return ReplyOK })
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if r, err := Send(path, CmdLocalChanged); err != nil || r != ReplyOK {
		t.Fatalf("reply %q, %v", r, err)
	}
	if len(got) != 1 || got[0] != CmdLocalChanged {
		t.Fatalf("handled %q", got)
	}
	if _, err := Serve(path, func(string) string { return ReplyOK }); err == nil {
		t.Fatal("a second daemon replaced a live socket")
	}
	if _, err := Send(t.TempDir()+"/none.sock", CmdStop); err == nil {
		t.Fatal("no daemon must be an error")
	}
}
