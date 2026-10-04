package health

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/layout"
)

var now = time.Unix(2_000_000_000, 0)

func healthy(t *testing.T) layout.Layout {
	t.Helper()
	l := layout.Layout{Root: t.TempDir(), Lock: t.TempDir()}
	for _, d := range []string{l.LogDir(), filepath.Dir(l.RSAPrivateKey())} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	put(t, l.RSAPrivateKey(), "key")
	put(t, l.SSHPrivateKey(), "key")
	put(t, l.MaintenanceState(), fmt.Sprintf("local %d %d 0\n", now.Unix()-60, now.Unix()-60))
	setLog(t, l, "[INFO] started\n"+backupFinished+"\n")
	return l
}

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// setLog lays the backup log out the way the pipeline does: a run file, and archiver.log
// as a symlink to it whose own timestamp is when the run started.
func setLog(t *testing.T, l layout.Layout, content string) {
	t.Helper()
	run := filepath.Join(l.LogDir(), "archiver-run.log")
	put(t, run, content)
	os.Remove(l.BackupLog())
	if err := os.Symlink(run, l.BackupLog()); err != nil {
		t.Fatal(err)
	}
	stamp(t, l.BackupLog(), now.Add(-time.Hour))
}

func stamp(t *testing.T, link string, at time.Time) {
	t.Helper()
	tv := []syscallTimeval{{at.Unix(), 0}, {at.Unix(), 0}}
	if err := lutimes(link, tv); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, l layout.Layout, env map[string]string) (int, string) {
	t.Helper()
	var b strings.Builder
	code := Run(&b, l, func(k string) string { return env[k] }, now)
	return code, b.String()
}

func TestHealthy(t *testing.T) {
	code, out := run(t, healthy(t), nil)
	if code != 0 || !strings.HasSuffix(out, "Status:   HEALTHY\n") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
}

func TestUnhealthy(t *testing.T) {
	for _, c := range []struct {
		name    string
		breakIt func(*testing.T, layout.Layout)
		want    string
	}{
		{"no RSA key", func(t *testing.T, l layout.Layout) { os.Remove(l.RSAPrivateKey()) }, "ERROR: RSA private key not found"},
		{"errors without a finished run", func(t *testing.T, l layout.Layout) {
			setLog(t, l, "[ERROR] boom\n[ERROR] again\n")
		}, "ERROR: Found 2 errors in recent logs without a finished run"},
		{"maintenance errors with no live run", func(t *testing.T, l layout.Layout) {
			put(t, l.MaintenanceLog(), "[ERROR] prune failed\n")
		}, "ERROR: Found 1 errors in recent maintenance logs without a finished run"},
	} {
		t.Run(c.name, func(t *testing.T) {
			l := healthy(t)
			c.breakIt(t, l)
			code, out := run(t, l, nil)
			if code != 1 || !strings.Contains(out, c.want) || !strings.HasSuffix(out, "Status:   UNHEALTHY\n") {
				t.Errorf("exit %d, want 1 with %q:\n%s", code, c.want, out)
			}
		})
	}
}

// Warnings never fail the check: these are the states a scheduled container passes
// through normally, and the finished-but-failed run is deliberately among them.
func TestWarningsStayHealthy(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup func(*testing.T, layout.Layout)
		want  string
	}{
		{"finished run with errors", func(t *testing.T, l layout.Layout) {
			setLog(t, l, "[ERROR] copy failed\n"+backupFinished+"\n")
		}, "WARNING: Found 1 errors in recent logs, but the run finished"},
		{"stale log", func(t *testing.T, l layout.Layout) { stamp(t, l.BackupLog(), now.Add(-49*time.Hour)) },
			"WARNING: Log file is stale (not modified in 48 hours)"},
		{"stale lock", func(t *testing.T, l layout.Layout) {
			put(t, l.BackupLock(), "1073741824 duplicacy backup\n1 running\n")
		}, "WARNING: Stale Backup lock (will be reaped on next run)"},
		{"maintenance errors while a run is live", func(t *testing.T, l layout.Layout) {
			put(t, l.MaintenanceLog(), "[ERROR] one storage slow\n")
			put(t, l.MaintenanceLock(), fmt.Sprintf("%d storage:local check\n1 running\n", os.Getpid()))
		}, "WARNING: Found 1 errors in maintenance logs; run still in progress"},
		{"overdue prune", func(t *testing.T, l layout.Layout) {
			put(t, l.MaintenanceState(), fmt.Sprintf("local %d %d 0\n", now.Unix(), now.Unix()-9*86400))
		}, "WARNING: No successful prune on 'local' in over 8 days"},
	} {
		t.Run(c.name, func(t *testing.T) {
			l := healthy(t)
			c.setup(t, l)
			code, out := run(t, l, nil)
			if code != 0 || !strings.Contains(out, c.want) || !strings.HasSuffix(out, "Status:   HEALTHY (with warnings)\n") {
				t.Errorf("exit %d, want 0 with %q:\n%s", code, c.want, out)
			}
		})
	}
}

// Deployments that share a storage maintain it from one place and turn both off elsewhere;
// ROTATE_BACKUPS is still read as PRUNE_BACKUPS.
func TestMaintenanceToggles(t *testing.T) {
	l := healthy(t)
	put(t, l.MaintenanceState(), "local 0 0 0\n")

	_, out := run(t, l, map[string]string{"CHECK_BACKUPS": "false", "PRUNE_BACKUPS": "FALSE"})
	if strings.Contains(out, "No successful") {
		t.Errorf("toggled-off maintenance still warned:\n%s", out)
	}

	_, out = run(t, l, map[string]string{"CHECK_BACKUPS": "true", "ROTATE_BACKUPS": "false"})
	if !strings.Contains(out, "No successful check on 'local'") || strings.Contains(out, "No successful prune") {
		t.Errorf("ROTATE_BACKUPS=false should turn the prune warning off:\n%s", out)
	}
}

func TestLastLines(t *testing.T) {
	p := filepath.Join(t.TempDir(), "log")
	var b strings.Builder
	for i := range 200_000 {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	put(t, p, b.String())
	got := lastLines(p, 100)
	if len(got) != 100 || got[0] != "line 199900" || got[99] != "line 199999" {
		t.Errorf("got %d lines, first %q last %q", len(got), got[0], got[len(got)-1])
	}
}
