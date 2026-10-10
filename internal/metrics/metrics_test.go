package metrics

import (
	"strings"
	"testing"

	"github.com/SisyphusMD/archiver/internal/backuphealth"
	"github.com/SisyphusMD/archiver/internal/copier"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/status"
)

func TestRender(t *testing.T) {
	s := status.Snapshot{
		BackupHealth: backuphealth.Health{State: backuphealth.Degraded},
		Services: map[string]lockstate.ServiceResult{
			"/srv/app": {LastAttempt: 200, LastSuccess: 200, Result: "success", Seconds: 12, Revision: 7, Uploaded: 4096},
			"/srv/db":  {LastAttempt: 300, Result: "failed"},
		},
		Copies:    map[string]copier.State{"offsite": {Target: "offsite", Behind: 2, FailingSince: 100, LastSuccess: 50}},
		Drills:    map[string]map[string]lockstate.DrillResult{"local": {"app": {At: 400, OK: true}, "db": {Skipped: true}}},
		Incidents: map[string]string{"copy:offsite": `Storage "Down"`},
	}
	got := Render(s)
	for _, want := range []string{
		"# TYPE archiver_backup_health gauge\narchiver_backup_health 1\n",
		`archiver_service_last_success_timestamp_seconds{service="app",directory="/srv/app"} 200`,
		`archiver_service_last_revision{service="app",directory="/srv/app"} 7`,
		`archiver_service_last_uploaded_bytes{service="app",directory="/srv/app"} 4096`,
		`archiver_service_last_ok{service="db",directory="/srv/db"} 0`,
		`archiver_copy_behind_revisions{target="offsite"} 2`,
		`archiver_copy_failing{target="offsite"} 1`,
		`archiver_drill_last_ok{storage="local",service="app"} 1`,
		`archiver_incident_open{key="copy:offsite",title="Storage \"Down\""} 1`,
		"archiver_incidents_open 1\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// A time never reached is left out, a skipped drill too; HELP and TYPE once per name.
	for _, absent := range []string{`archiver_service_last_success_timestamp_seconds{service="db"`, `service="db"} 0` + "\narchiver_drill", `archiver_drill_last_ok{storage="local",service="db"}`} {
		if strings.Contains(got, absent) {
			t.Errorf("unexpected %q", absent)
		}
	}
	if strings.Count(got, "# TYPE archiver_service_last_ok gauge") != 1 {
		t.Error("TYPE repeated")
	}
}
