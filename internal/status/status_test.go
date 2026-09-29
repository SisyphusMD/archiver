package status

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/layout"
)

func testLayout(t *testing.T) layout.Layout {
	t.Helper()
	l := layout.Layout{Root: t.TempDir(), Lock: t.TempDir()}
	if err := os.MkdirAll(l.LogDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	return l
}

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func report(t *testing.T, l layout.Layout, now time.Time) string {
	t.Helper()
	var b strings.Builder
	if err := Write(&b, l, now); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestIdle(t *testing.T) {
	got := report(t, testLayout(t), time.Unix(0, 0))
	want := "Backup: not running.\nMaintenance: not running.\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRunningPausedAndStale(t *testing.T) {
	l := testLayout(t)
	me := os.Getpid()

	put(t, l.BackupLock(), fmt.Sprintf("%d service:/srv/app backup\n100 running\n", me))
	put(t, l.MaintenanceLock(), fmt.Sprintf("%d storage:local prune\n100 running\n", me))
	got := report(t, l, time.Unix(200, 0))
	for _, want := range []string{
		fmt.Sprintf("Backup: running (PID: %d, stage: backup).", me),
		fmt.Sprintf("Maintenance: running (PID: %d, stage: prune, storage:local).", me),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}

	put(t, l.BackupLock(), fmt.Sprintf("%d service:/srv/app backup\n100 running\n150 paused\n", me))
	if got := report(t, l, time.Unix(200, 0)); !strings.Contains(got, fmt.Sprintf("Backup: paused (PID: %d).", me)) {
		t.Errorf("paused run not reported:\n%s", got)
	}

	// A lock whose holder died is stale, not a running backup.
	put(t, l.BackupLock(), "1073741824 service:/srv/app backup\n100 running\n")
	if got := report(t, l, time.Unix(200, 0)); !strings.Contains(got, "Backup: not running.") {
		t.Errorf("stale lock reported as running:\n%s", got)
	}
}

func TestMaintenanceRecency(t *testing.T) {
	l := testLayout(t)
	now := time.Unix(1_000_000, 0)
	put(t, l.MaintenanceState(), "local 999880 996400 0\n")
	got := report(t, l, now)
	want := "Storage maintenance (last success):\n  local: check 2m ago, prune 1h ago, exhaustive never\n"
	if !strings.HasSuffix(got, want) {
		t.Errorf("got:\n%s\nwant suffix:\n%s", got, want)
	}
}

func TestAge(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	for _, c := range []struct {
		ago  int64
		want string
	}{{0, "0m ago"}, {3599, "59m ago"}, {3600, "1h ago"}, {172799, "47h ago"}, {172800, "2d ago"}} {
		if got := Age(now.Unix()-c.ago, now); got != c.want {
			t.Errorf("Age(%ds ago) = %q, want %q", c.ago, got, c.want)
		}
	}
	if got := Age(0, now); got != "never" {
		t.Errorf("Age(0) = %q", got)
	}
}
