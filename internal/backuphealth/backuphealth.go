// Package backuphealth is backup health (ADR 32), apart from the liveness healthcheck:
// whether the backups are good, read from what each part records. Nothing restarts on it.
package backuphealth

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/copier"
	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/notify"
)

const (
	OK       = "OK"
	Degraded = "DEGRADED"
	Failing  = "FAILING"
)

// Health is backup health and why.
type Health struct {
	State   string   `json:"state"`
	Reasons []string `json:"reasons,omitempty"`
}

// ExitCode is the exit status monitors read: 0 OK, 1 DEGRADED, 2 FAILING.
func (h Health) ExitCode() int {
	switch h.State {
	case Failing:
		return 2
	case Degraded:
		return 1
	}
	return 0
}

// failingIncidents are the open incidents that mean backups are not good: a backup
// refused, the primary down, the kit not current, a drill failing, a log nobody can read.
// A backup that failed before any service (backup-aborted), or could not record its
// results (backup-state), fails too. The rest degrade: a
// secondary down, a check failed, a mirror refused, maintenance, and the run-level backup
// incident, whose primary failures each service's own result already fails (what is left
// of it, such as an inline copy failing, leaves the backups good).
var failingIncidents = []string{"backup-aborted", "backup-state", "backup-skipped", "primary-down", "kit", "drill", "log:"}

// Compute is backup health: FAILING when two scheduled backups have come since a service's
// last good primary backup, its last backup failed, or a failing incident is open;
// DEGRADED when a secondary is behind or another incident is open; OK otherwise.
func Compute(l layout.Layout, getenv func(string) string, now time.Time) Health {
	var failing, degraded []string
	state, err := lockstate.ReadBackupState(l.BackupState())
	if err != nil {
		failing = append(failing, "the backup state cannot be read: "+err.Error())
	}
	every, scheduled := daemon.Interval(getenv("BACKUP_SCHEDULE"), now)
	patterns := config.SplitServiceDirectories(getenv("SERVICE_DIRECTORIES"))
	if len(patterns) == 0 {
		failing = append(failing, "SERVICE_DIRECTORIES is not set, so nothing is backed up")
	}
	dirs, unmatched := config.ExpandServiceDirectories(patterns)
	for _, u := range unmatched {
		// Nothing there is backed up: a backup fails on it too.
		failing = append(failing, fmt.Sprintf("SERVICE_DIRECTORIES entry %s matches no directory", u))
	}
	for _, d := range dirs {
		// As the pipeline records it: the absolute directory.
		name := d
		if abs, err := filepath.Abs(d); err == nil {
			name = abs
		}
		r, ok := state.Services[name]
		switch {
		case !ok && err != nil:
		case !ok:
			degraded = append(degraded, fmt.Sprintf("%s: not backed up yet", name))
		case r.LastSuccess == 0:
			failing = append(failing, fmt.Sprintf("%s: no successful backup yet (last %s %s)", name, r.Result, ago(r.LastAttempt, now)))
		case missedTwice(getenv("BACKUP_SCHEDULE"), r.LastSuccess, now):
			failing = append(failing, fmt.Sprintf("%s: last good backup %s, and two scheduled backups have come since", name, ago(r.LastSuccess, now)))
		case r.Result == "failed" || r.Result == "skipped":
			failing = append(failing, fmt.Sprintf("%s: last backup %s %s (last good %s)", name, r.Result, ago(r.LastAttempt, now), ago(r.LastSuccess, now)))
		}
	}
	open, ierr := notify.ReadIncidents(l.Incidents())
	if ierr != nil {
		failing = append(failing, "the open incidents cannot be read: "+ierr.Error())
	}
	keys := make([]string, 0, len(open))
	for k := range open {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		line := fmt.Sprintf("%s (%s)", open[k], k)
		if isFailing(k) {
			failing = append(failing, line)
		} else {
			degraded = append(degraded, line)
		}
	}
	// A secondary behind: retrying before its incident opens (a storage down opens one only
	// after 30 minutes), or still copying (or paused) for longer than a backup interval. A
	// secondary is behind after every backup until its copy lands, which is not trouble.
	behindFor := 24 * time.Hour
	if scheduled {
		behindFor = every
	}
	workers, werr := (&copier.Store{Path: l.CopyWorkersState()}).Read()
	if werr != nil {
		degraded = append(degraded, "the copy workers' state cannot be read: "+werr.Error())
	}
	names := make([]string, 0, len(workers))
	for n := range workers {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		s := workers[name]
		if _, inc := open["copy:"+name]; inc {
			continue
		}
		switch {
		// FailingSince outlives a retry in progress: until a copy succeeds it is not over.
		case s.Status == copier.Retrying || s.Status == copier.Down || s.FailingSince != 0:
			degraded = append(degraded, fmt.Sprintf("copies to %s retrying since %s", name, ago(s.FailingSince, now)))
		case s.Behind > 0 && now.Sub(time.Unix(caughtUp(s), 0)) > behindFor:
			degraded = append(degraded, fmt.Sprintf("%s behind by %d revision(s), last caught up %s", name, s.Behind, ago(caughtUp(s), now)))
		}
	}
	switch {
	case len(failing) > 0:
		return Health{Failing, append(failing, degraded...)}
	case len(degraded) > 0:
		return Health{Degraded, degraded}
	}
	return Health{State: OK}
}

// missedTwice reports whether two scheduled backups have come since the last good one:
// counted on the schedule itself, so a weekday-only schedule is not overdue at the weekend.
func missedTwice(spec string, last int64, now time.Time) bool {
	n, ok := daemon.Missed(spec, time.Unix(last, 0), now, 2)
	return ok && n >= 2
}

func isFailing(key string) bool {
	for _, f := range failingIncidents {
		if key == f || strings.HasPrefix(key, f+":") || (strings.HasSuffix(f, ":") && strings.HasPrefix(key, f)) {
			return true
		}
	}
	return false
}

// caughtUp is when the worker was last caught up: copies that each finish quickly while it
// never catches up are still a lag.
func caughtUp(s copier.State) int64 {
	switch {
	case s.BehindSince != 0:
		return s.BehindSince
	case s.LastSuccess != 0:
		return s.LastSuccess
	}
	return s.Since
}

func ago(epoch int64, now time.Time) string {
	if epoch == 0 {
		return "never"
	}
	return logging.Duration(int64(now.Sub(time.Unix(epoch, 0)).Seconds())) + " ago"
}
