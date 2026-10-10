package restore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/lockstate"
)

func drillFixture(t *testing.T, services ...string) *fixture {
	t.Helper()
	f := newFixture(t)
	for _, s := range services {
		os.MkdirAll(filepath.Join(f.root, "services", s), 0o755)
	}
	os.MkdirAll(filepath.Join(f.root, "logs"), 0o755)
	f.vars["RESTORE_DRILL_DIR"] = filepath.Join(f.root, "drill")
	return f
}

func (f *fixture) drillState(t *testing.T) lockstate.DrillState {
	t.Helper()
	s, err := lockstate.ReadDrillState(f.env.Layout.DrillState())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// One service per storage per run by default, in rotation, each restored, counted and
// deleted; the next run takes the next service.
func TestDrillRotates(t *testing.T) {
	f := drillFixture(t, "app", "db", "web")
	f.revs(1, "1 2")
	f.revs(2, "1")
	if code := f.env.Drill(DrillOptions{}); code != 0 {
		t.Fatalf("exit %d: %s %s", code, f.out.String(), f.err.String())
	}
	s := f.drillState(t)
	if r := s.Results["local"]["app"]; !r.OK || r.Revision != 2 || r.Files != 3 {
		t.Fatalf("first run on local: %+v", s.Results["local"])
	}
	if r := s.Results["off_site"]["app"]; !r.OK || r.Revision != 1 {
		t.Fatalf("first run on off-site: %+v", s.Results)
	}
	if len(s.Results["local"]) != 1 || s.LastPass == 0 || s.LastFailed {
		t.Fatalf("state %+v", s)
	}
	entries, _ := os.ReadDir(filepath.Join(f.root, "drill"))
	if len(entries) != 0 {
		t.Fatalf("drill copies left behind: %v", entries)
	}
	if code := f.env.Drill(DrillOptions{}); code != 0 {
		t.Fatalf("second run: exit %d", code)
	}
	if _, ok := f.drillState(t).Results["local"]["db"]; !ok {
		t.Fatalf("second run did not move on to db: %+v", f.drillState(t).Results["local"])
	}
}

// A restore that does not match its listing fails the drill (and notifies through the
// log's error title); a storage without the revision yet is a skip, not a failure.
func TestDrillFailsOnMismatchAndSkipsMissing(t *testing.T) {
	f := drillFixture(t, "app")
	f.revs(1, "4")
	os.WriteFile(filepath.Join(f.root, "store1", "files"), []byte("7"), 0o644)
	if code := f.env.Drill(DrillOptions{}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	s := f.drillState(t)
	if r := s.Results["local"]["app"]; r.OK || !strings.Contains(r.Message, "restored 3 files, its listing has 7") {
		t.Fatalf("local: %+v", r)
	}
	if r := s.Results["off_site"]["app"]; !r.Skipped {
		t.Fatalf("off-site without revisions: %+v", r)
	}
	if !s.LastFailed || s.LastPass != 0 {
		t.Fatalf("state %+v", s)
	}
}

// RESTORE_DRILL_STORAGES=primary, RESTORE_DRILL_SERVICES=all and RESTORE_DRILL_EXCLUDE pick
// what is drilled; a named service and storage drill just that.
func TestDrillSelection(t *testing.T) {
	f := drillFixture(t, "app", "db", "web")
	f.revs(1, "1")
	f.revs(2, "1")
	f.vars["RESTORE_DRILL_STORAGES"], f.vars["RESTORE_DRILL_SERVICES"], f.vars["RESTORE_DRILL_EXCLUDE"] = "primary", "all", "web"
	if code := f.env.Drill(DrillOptions{}); code != 0 {
		t.Fatalf("exit %d: %s", code, f.out.String())
	}
	s := f.drillState(t)
	if len(s.Results) != 1 || len(s.Results["local"]) != 2 {
		t.Fatalf("results %+v", s.Results)
	}
	if _, ok := s.Results["local"]["web"]; ok {
		t.Fatal("an excluded service was drilled")
	}
	if code := f.env.Drill(DrillOptions{Service: "web", Storage: "off-site", Now: func() time.Time { return time.Unix(2e9, 0) }}); code != 0 {
		t.Fatalf("named drill: exit %d", code)
	}
	if r := f.drillState(t).Results["off_site"]["web"]; !r.OK || r.At != 2e9 {
		t.Fatalf("named drill result %+v", r)
	}
	if code := f.env.Drill(DrillOptions{Service: "nope"}); code != 1 {
		t.Fatal("an unknown service was accepted")
	}
}

// The drill directory is never a service directory, its parent or inside one; and a drill
// never deletes what it did not create there.
func TestDrillDirectorySafety(t *testing.T) {
	f := drillFixture(t, "app", "local-app")
	f.revs(1, "1")
	f.vars["RESTORE_DRILL_DIR"] = filepath.Join(f.root, "services")
	if code := f.env.Drill(DrillOptions{}); code != 1 {
		t.Fatal("a drill directory holding the services was accepted")
	}
	if s := f.drillState(t); !s.LastFailed || s.LastRun == 0 {
		t.Fatalf("an early failure was not recorded: %+v", s)
	}
	f.vars["RESTORE_DRILL_DIR"] = filepath.Join(f.root, "drill")
	keep := filepath.Join(f.root, "drill", "local-app")
	os.MkdirAll(keep, 0o755)
	if code := f.env.Drill(DrillOptions{Storage: "local"}); code != 0 {
		t.Fatalf("exit %d: %s", code, f.out.String())
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("the drill deleted a directory it did not create")
	}
}

// A revision without regular files (duplicacy prints no "Files:" line) passes when nothing
// comes back.
func TestDrillEmptyRevision(t *testing.T) {
	f := drillFixture(t, "app")
	f.revs(1, "1")
	os.WriteFile(filepath.Join(f.root, "store1", "files"), []byte("none"), 0o644)
	os.WriteFile(filepath.Join(f.root, "store1", "empty"), nil, 0o644)
	if code := f.env.Drill(DrillOptions{Storage: "local"}); code != 0 {
		t.Fatalf("exit %d: %s", code, f.out.String())
	}
	if r := f.drillState(t).Results["local"]["app"]; !r.OK || r.Files != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestMountedUnder(t *testing.T) {
	info := filepath.Join(t.TempDir(), "mountinfo")
	if err := os.WriteFile(info, []byte("22 1 0:21 / / rw - overlay overlay rw\n"+
		"30 22 8:1 /srv/db /tmp/archiver-drill/drill-x/inner rw - ext4 /dev/sda1 rw\n"+
		"31 22 8:1 /srv/a /tmp/with\\040space rw - ext4 /dev/sda1 rw\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for dir, want := range map[string]bool{
		"/tmp/archiver-drill/drill-x": true, // something is mounted inside it
		"/tmp/archiver-drill/drill-y": false,
		"/tmp/with space":             true,
		"/tmp/archiver-drill/drill":   false, // a prefix of a name is not a parent
	} {
		if got := mountedUnder(dir, info); got != want {
			t.Errorf("%s: %v", dir, got)
		}
	}
	if !mountedUnder("/x", filepath.Join(t.TempDir(), "missing")) {
		t.Error("an unreadable mount table must refuse")
	}
}
