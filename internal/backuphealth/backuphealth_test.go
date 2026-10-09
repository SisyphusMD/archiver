package backuphealth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/copier"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
)

func setup(t *testing.T) (layout.Layout, func(string) string, time.Time) {
	t.Helper()
	root := t.TempDir()
	l := layout.Layout{Root: root, Lock: filepath.Join(root, "lock")}
	os.MkdirAll(l.LogDir(), 0o755)
	srv := filepath.Join(root, "srv")
	for _, s := range []string{"app", "db"} {
		os.MkdirAll(filepath.Join(srv, s), 0o755)
	}
	env := map[string]string{"SERVICE_DIRECTORIES": srv + "/*/", "BACKUP_SCHEDULE": "0 3 * * *"}
	return l, func(k string) string { return env[k] }, time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
}

// record writes results by service name, keyed as the pipeline keys them: the directory.
func record(t *testing.T, l layout.Layout, s map[string]lockstate.ServiceResult) {
	t.Helper()
	byDir := map[string]lockstate.ServiceResult{}
	for name, r := range s {
		byDir[filepath.Join(l.Root, "srv", name)] = r
	}
	if err := lockstate.WriteBackupState(l.BackupState(), lockstate.BackupState{Services: byDir}); err != nil {
		t.Fatal(err)
	}
}

func TestCompute(t *testing.T) {
	l, env, now := setup(t)
	hour := func(h int) int64 { return now.Add(-time.Duration(h) * time.Hour).Unix() }

	// Nothing backed up yet: degraded, not failing, on a fresh deployment.
	if h := Compute(l, env, now); h.State != Degraded || !strings.Contains(strings.Join(h.Reasons, "|"), "app: not backed up yet") {
		t.Fatalf("fresh: %+v", h)
	}
	record(t, l, map[string]lockstate.ServiceResult{
		"app": {LastAttempt: hour(9), LastSuccess: hour(9), Result: "success"},
		"db":  {LastAttempt: hour(9), LastSuccess: hour(9), Result: "success"},
	})
	if h := Compute(l, env, now); h.State != OK || h.ExitCode() != 0 {
		t.Fatalf("good: %+v", h)
	}
	// A service whose last good backup is over twice the daily schedule ago fails.
	record(t, l, map[string]lockstate.ServiceResult{
		"app": {LastAttempt: hour(9), LastSuccess: hour(9), Result: "success"},
		"db":  {LastAttempt: hour(9), LastSuccess: hour(49), Result: "failed"},
	})
	if h := Compute(l, env, now); h.State != Failing || h.ExitCode() != 2 || !strings.Contains(h.Reasons[0], "/srv/db: last good backup") {
		t.Fatalf("stale: %+v", h)
	}
	// A failed last backup fails even within the window.
	record(t, l, map[string]lockstate.ServiceResult{
		"app": {LastAttempt: hour(9), LastSuccess: hour(9), Result: "success"},
		"db":  {LastAttempt: hour(9), LastSuccess: hour(33), Result: "failed"},
	})
	if h := Compute(l, env, now); h.State != Failing {
		t.Fatalf("failed: %+v", h)
	}
}

// Open incidents decide too: a secondary down degrades, the kit failing fails; a retrying
// worker with no incident yet degrades.
func TestIncidentsAndWorkers(t *testing.T) {
	l, env, now := setup(t)
	good := lockstate.ServiceResult{LastAttempt: now.Add(-time.Hour).Unix(), LastSuccess: now.Add(-time.Hour).Unix(), Result: "success"}
	record(t, l, map[string]lockstate.ServiceResult{"app": good, "db": good})
	os.WriteFile(l.Incidents(), []byte(`{"copy:offsite":{"title":"Storage Down"},"check:offsite":{"title":"Storage Check Failed"}}`), 0o644)
	if h := Compute(l, env, now); h.State != Degraded || h.ExitCode() != 1 || len(h.Reasons) != 2 {
		t.Fatalf("degraded: %+v", h)
	}
	os.WriteFile(l.Incidents(), []byte(`{"kit":{"title":"Recovery Kit Failed"},"drill:local:app":{"title":"Restore Drill Failed"},"log:archiver":{"title":"Log Unwritable"},"backup":{"title":"Backup Failed"}}`), 0o644)
	if h := Compute(l, env, now); h.State != Failing || len(h.Reasons) != 4 {
		t.Fatalf("failing: %+v", h)
	}
	os.Remove(l.Incidents())
	(&copier.Store{Path: l.CopyWorkersState()}).Save(copier.State{Target: "offsite", Status: copier.Retrying, FailingSince: now.Add(-10 * time.Minute).Unix()})
	if h := Compute(l, env, now); h.State != Degraded || !strings.Contains(h.Reasons[0], "offsite retrying") {
		t.Fatalf("retrying: %+v", h)
	}
}

// A configured path matching nothing fails; a secondary behind longer than the backup
// interval degrades, one just behind after a backup does not.
func TestUnmatchedAndBehind(t *testing.T) {
	l, env, now := setup(t)
	good := lockstate.ServiceResult{LastAttempt: now.Add(-time.Hour).Unix(), LastSuccess: now.Add(-time.Hour).Unix(), Result: "success"}
	record(t, l, map[string]lockstate.ServiceResult{"app": good, "db": good})
	store := &copier.Store{Path: l.CopyWorkersState()}
	store.Save(copier.State{Target: "offsite", Status: copier.Copying, Behind: 2, Since: now.Add(-time.Hour).Unix(), LastSuccess: now.Add(-3 * time.Hour).Unix()})
	if h := Compute(l, env, now); h.State != OK {
		t.Fatalf("just behind: %+v", h)
	}
	// Each copy recent, but not caught up for 30 hours: a lag all the same.
	store.Save(copier.State{Target: "offsite", Status: copier.Copying, Behind: 2, Since: now.Add(-time.Hour).Unix(), LastSuccess: now.Add(-30 * time.Hour).Unix()})
	if h := Compute(l, env, now); h.State != Degraded || !strings.Contains(h.Reasons[0], "offsite behind by 2") {
		t.Fatalf("long behind: %+v", h)
	}
	missing := func(k string) string {
		if k == "SERVICE_DIRECTORIES" {
			return env(k) + "\n/nowhere/mounted"
		}
		return env(k)
	}
	if h := Compute(l, missing, now); h.State != Failing || !strings.Contains(h.Reasons[0], "/nowhere/mounted matches no directory") {
		t.Fatalf("unmatched: %+v", h)
	}
}

// An unreadable record fails rather than reading as a fresh deployment.
func TestUnreadableState(t *testing.T) {
	l, env, now := setup(t)
	os.WriteFile(l.BackupState(), []byte("{not json"), 0o644)
	if h := Compute(l, env, now); h.State != Failing || !strings.Contains(h.Reasons[0], "cannot be read") || len(h.Reasons) != 1 {
		t.Fatalf("%+v", h)
	}
}

// Freshness counts the schedule's own runs: a weekday-only schedule is not overdue at the
// weekend.
func TestWeekdaySchedule(t *testing.T) {
	l, env, _ := setup(t)
	weekdays := func(k string) string {
		if k == "BACKUP_SCHEDULE" {
			return "0 3 * * 1-5"
		}
		return env(k)
	}
	friday := time.Date(2026, 10, 9, 5, 0, 0, 0, time.UTC) // a Friday
	good := lockstate.ServiceResult{LastAttempt: friday.Unix(), LastSuccess: friday.Unix(), Result: "success"}
	record(t, l, map[string]lockstate.ServiceResult{"app": good, "db": good})
	if h := Compute(l, weekdays, friday.Add(55*time.Hour)); h.State != OK {
		t.Fatalf("Sunday noon: %+v", h)
	}
	if h := Compute(l, weekdays, friday.Add(95*time.Hour)); h.State != Failing {
		t.Fatalf("Tuesday 04:00, Monday's and Tuesday's backups missed: %+v", h)
	}
}

// Nothing configured, an unreadable incident record, and a run that failed before any
// service all fail.
func TestNothingToKnow(t *testing.T) {
	l, env, now := setup(t)
	good := lockstate.ServiceResult{LastAttempt: now.Add(-time.Hour).Unix(), LastSuccess: now.Add(-time.Hour).Unix(), Result: "success"}
	record(t, l, map[string]lockstate.ServiceResult{"app": good, "db": good})
	if h := Compute(l, func(string) string { return "" }, now); h.State != Failing || !strings.Contains(h.Reasons[0], "not set") {
		t.Fatalf("unset: %+v", h)
	}
	os.WriteFile(l.Incidents(), []byte("{broken"), 0o644)
	if h := Compute(l, env, now); h.State != Failing || !strings.Contains(h.Reasons[0], "incidents cannot be read") {
		t.Fatalf("unreadable incidents: %+v", h)
	}
	os.WriteFile(l.Incidents(), []byte(`{"backup-aborted":{"title":"Backup Failed"}}`), 0o644)
	if h := Compute(l, env, now); h.State != Failing {
		t.Fatalf("aborted: %+v", h)
	}
}
