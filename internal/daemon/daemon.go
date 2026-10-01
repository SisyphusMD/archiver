// Package daemon runs the container's schedules (ADR 14). It parses schedules with
// supercronic's own cron parser, so every schedule that ran under supercronic keeps its
// meaning, and it keeps supercronic's timing: a job never overlaps itself, and a run that
// overruns its next slot skips that slot rather than starting late.
package daemon

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/aptible/supercronic/cronexpr"
)

// Job is one scheduled command.
type Job struct {
	Name string // the command run, also its name in the log
	Env  string // the variable its schedule came from
	Spec string
	expr *cronexpr.Expression
}

// Jobs reads the schedules from the environment. A malformed schedule, or one that can
// never fire, is an error: the container must refuse to start rather than never back up.
func Jobs(getenv func(string) string, now time.Time) ([]Job, error) {
	var jobs []Job
	for _, j := range []Job{
		{Name: "backup", Env: "BACKUP_SCHEDULE"},
		{Name: "maintenance", Env: "MAINTENANCE_SCHEDULE"},
	} {
		j.Spec = getenv(j.Env)
		if j.Spec == "" {
			continue
		}
		expr, err := cronexpr.ParseStrict(j.Spec)
		if err != nil {
			return nil, fmt.Errorf("invalid %s %q: %v", j.Env, j.Spec, err)
		}
		if expr.Next(now).IsZero() {
			return nil, fmt.Errorf("invalid %s %q: it never fires", j.Env, j.Spec)
		}
		j.expr = expr
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// Clock is the time source, replaced in tests.
type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// RealClock is the wall clock, in the container's TZ.
var RealClock Clock = realClock{}

// Run starts each job on its schedule until ctx ends, then waits for running jobs to
// finish; it never cuts one short, since stopping a run is `archiver stop`'s job.
func Run(ctx context.Context, clock Clock, log io.Writer, jobs []Job, run func(Job) int) {
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			schedule(ctx, clock, log, j, run)
		}()
	}
	wg.Wait()
}

func schedule(ctx context.Context, clock Clock, log io.Writer, j Job, run func(Job) int) {
	next := clock.Now()
	for {
		next = j.expr.Next(next)
		if next.IsZero() {
			fmt.Fprintf(log, "archiver daemon: %s schedule %q has no further runs\n", j.Name, j.Spec)
			return
		}
		now := clock.Now()
		delay := next.Sub(now)
		if delay < 0 {
			fmt.Fprintf(log, "archiver daemon: %s ran past its %s slot; skipping it\n", j.Name, next.Format(time.RFC3339))
			next = now
			continue
		}
		fmt.Fprintf(log, "archiver daemon: next %s at %s\n", j.Name, next.Format(time.RFC3339))
		select {
		case <-ctx.Done():
			return
		case <-clock.After(delay):
		}
		// select picks at random when both are ready; a shutdown must never start a run.
		if ctx.Err() != nil {
			return
		}
		fmt.Fprintf(log, "archiver daemon: starting %s\n", j.Name)
		code := run(j)
		fmt.Fprintf(log, "archiver daemon: %s exited %d\n", j.Name, code)
	}
}
