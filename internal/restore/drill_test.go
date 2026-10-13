package restore

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/inuse"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/notify"
	"github.com/SisyphusMD/archiver/internal/resume"
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
	if r := s.Results["local"]["app"]; r.OK || !strings.Contains(r.Message, "file4 is missing") {
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

// A killed drill leaves its repository behind as well as its copy: the next drill deletes it,
// but only a real directory of Archiver's own named as workRepo names them, directly in the
// temporary directory, since the record is on the logs volume.
func TestDrillDeletesInterruptedRepositories(t *testing.T) {
	f := drillFixture(t, "app")
	f.revs(1, "1")
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	mk := func(p string, mode os.FileMode) string {
		t.Helper()
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
		os.Chmod(p, mode)
		os.WriteFile(filepath.Join(p, "marker"), nil, 0o600)
		return p
	}
	old := mk(filepath.Join(tmp, "archiver-restore-old"), 0o700)
	open := mk(filepath.Join(tmp, "archiver-restore-open"), 0o777)
	target := mk(filepath.Join(f.root, "target"), 0o700)
	link := filepath.Join(tmp, "archiver-restore-link")
	os.Symlink(target, link)
	outside := mk(filepath.Join(f.root, "archiver-restore-outside"), 0o700)
	run, err := resume.Begin(f.env.Layout.RunRecord("drill"), resume.Record{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{old, open, link, outside} {
		run.Set(p, resume.Pending)
	}

	if code := f.env.Drill(DrillOptions{}); code != 0 {
		t.Fatalf("exit %d: %s %s", code, f.out.String(), f.err.String())
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("the interrupted drill's repository was not deleted: %v", err)
	}
	for _, p := range []string{open, filepath.Join(target, "marker"), link, outside} {
		if _, err := os.Lstat(p); err != nil {
			t.Fatalf("%s was deleted: %v", p, err)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(tmp, "archiver-restore-*")); len(left) != 2 {
		t.Fatalf("this drill's repository was left behind: %v", left)
	}
}

func TestDrillFailureRemainsDue(t *testing.T) {
	f := drillFixture(t, "app", "db")
	f.revs(1, "1")
	hook := filepath.Join(f.root, "services", "app", "drill")
	os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o700)
	for i := 0; i < 2; i++ {
		if f.env.Drill(DrillOptions{Storage: "local"}) != 1 {
			t.Fatal("failed drill passed")
		}
		s := f.drillState(t)
		if _, ok := s.Results["local"]["db"]; ok {
			t.Fatal("rotation skipped a failed service")
		}
		if len(s.Pending["local"]) != 1 || s.Pending["local"][0] != "app" {
			t.Fatalf("pending: %+v", s.Pending)
		}
	}
	os.Remove(hook)
	if f.env.Drill(DrillOptions{Storage: "local"}) != 0 {
		t.Fatal("retry failed")
	}
	r := f.drillState(t).Results["local"]["app"]
	if r.LastSuccess == 0 || r.Seconds <= 0 {
		t.Fatalf("missing timing: %+v", r)
	}
	if f.env.Drill(DrillOptions{Storage: "local"}) != 0 {
		t.Fatal("next rotation failed")
	}
	if !f.drillState(t).Results["local"]["db"].OK {
		t.Fatal("rotation did not resume")
	}
}

func TestDrillRunTimeoutRemainsDue(t *testing.T) {
	f := drillFixture(t, "app", "db")
	f.revs(1, "1")
	hook := filepath.Join(f.root, "services", "app", "drill")
	os.WriteFile(hook, []byte("#!/bin/sh\nsleep 30 & wait\n"), 0o700)
	f.vars["RESTORE_DRILL_TIMEOUT"] = "100ms"
	began := time.Now()
	if f.env.Drill(DrillOptions{Storage: "local"}) != 1 {
		t.Fatal("timed out drill passed")
	}
	if time.Since(began) > time.Second {
		t.Fatal("whole-run timeout did not end hook")
	}
	s := f.drillState(t)
	if !s.LastFailed || len(s.Pending["local"]) != 1 || s.LastPass != 0 {
		t.Fatalf("state: %+v", s)
	}
	delete(f.vars, "RESTORE_DRILL_TIMEOUT")
	os.Remove(hook)
	if f.env.Drill(DrillOptions{Storage: "local"}) != 0 || !f.drillState(t).Results["local"]["app"].OK {
		t.Fatal("interrupted selection was not retried")
	}
}

func TestDrillInterruptedBeforeRestoreRemainsDue(t *testing.T) {
	for _, stage := range []string{"after connect", "before restore", "gate stop", "gate timeout"} {
		t.Run(stage, func(t *testing.T) {
			f := drillFixture(t, "app", "db")
			f.revs(1, "1")
			releaseGate := func() {}
			if strings.HasPrefix(stage, "gate ") {
				release, err := inuse.Gate(context.Background(), f.env.Layout.InUseDir(), "local", true)
				if err != nil {
					t.Fatal(err)
				}
				defer release()
				releaseGate = release
			}
			if stage == "gate timeout" {
				f.vars["RESTORE_DRILL_TIMEOUT"] = "500ms"
			} else {
				command := "init)"
				if stage == "before restore" {
					command = "cat)"
				}
				script := strings.Replace(fakeDuplicacy, command, command+"\n  touch '"+f.env.Layout.DrillStopFlag()+"'", 1)
				if err := os.WriteFile(f.env.Duplicacy, []byte(script), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if code := f.env.Drill(DrillOptions{Storage: "local"}); code != 1 {
				t.Fatalf("interrupted drill exited %d", code)
			}
			s := f.drillState(t)
			if r := s.Results["local"]["app"]; !r.Interrupted || r.Skipped || r.OK {
				t.Fatalf("interruption was not classified explicitly: %+v", r)
			}
			if !s.LastFailed || s.LastPass != 0 || len(s.Pending["local"]) != 1 || s.Pending["local"][0] != "app" {
				t.Fatalf("interrupted selection was forgotten: %+v", s)
			}
			releaseGate()
			delete(f.vars, "RESTORE_DRILL_TIMEOUT")
			if err := os.WriteFile(f.env.Duplicacy, []byte(fakeDuplicacy), 0o755); err != nil {
				t.Fatal(err)
			}
			if code := f.env.Drill(DrillOptions{Storage: "local"}); code != 0 {
				t.Fatalf("retry exited %d: %s", code, f.out.String())
			}
			s = f.drillState(t)
			if !s.Results["local"]["app"].OK || len(s.Pending["local"]) != 0 {
				t.Fatalf("interrupted selection did not pass on retry: %+v", s)
			}
			if _, ok := s.Results["local"]["db"]; ok {
				t.Fatal("rotation advanced before retrying interrupted app")
			}
		})
	}
}

func TestNamedDrillPreservesOtherPendingServices(t *testing.T) {
	f := drillFixture(t, "app", "db")
	f.revs(1, "1")
	hook := filepath.Join(f.root, "services", "app", "drill")
	os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o700)
	if f.env.Drill(DrillOptions{Storage: "local"}) != 1 {
		t.Fatal("failed drill passed")
	}
	if f.env.Drill(DrillOptions{Service: "db", Storage: "local"}) != 0 {
		t.Fatal("named drill failed")
	}
	s := f.drillState(t)
	if len(s.Pending["local"]) != 1 || s.Pending["local"][0] != "app" {
		t.Fatalf("named drill forgot failure: %+v", s.Pending)
	}
}

func TestDrillRetriesPersistedInterruptedSelection(t *testing.T) {
	f := drillFixture(t, "app", "db")
	f.revs(1, "1")
	s := lockstate.DrillState{Rotation: map[string]int{"local": 1}, Pending: map[string][]string{"local": {"app"}}}
	if err := lockstate.WriteDrillState(f.env.Layout.DrillState(), s); err != nil {
		t.Fatal(err)
	}
	if f.env.Drill(DrillOptions{Storage: "local"}) != 0 {
		t.Fatal("interrupted selection retry failed")
	}
	s = f.drillState(t)
	if !s.Results["local"]["app"].OK || len(s.Pending["local"]) != 0 {
		t.Fatalf("state: %+v", s)
	}
	if _, ok := s.Results["local"]["db"]; ok {
		t.Fatal("interrupted selection skipped")
	}
}

func TestDrillRepeatedSkipsAdvanceRotation(t *testing.T) {
	f := drillFixture(t, "app", "db")
	for i := 0; i < 4; i++ {
		if code := f.env.Drill(DrillOptions{Storage: "local"}); code != 0 {
			t.Fatalf("run %d: exit %d", i, code)
		}
		s := f.drillState(t)
		if len(s.Pending["local"]) != 0 {
			t.Fatalf("run %d: skipped service remains pending: %+v", i, s.Pending)
		}
		if i > 0 && !s.Results["local"]["db"].Skipped {
			t.Fatal("rotation did not reach db")
		}
	}
}

func TestDrillFailureFollowedBySkipsRemainsDue(t *testing.T) {
	f := drillFixture(t, "app", "db")
	f.revs(1, "1")
	if err := os.WriteFile(filepath.Join(f.root, "services", "app", "drill"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if f.env.Drill(DrillOptions{Storage: "local"}) != 1 {
		t.Fatal("failed drill passed")
	}
	f.revs(1, "")
	for i := 0; i < 2; i++ {
		if f.env.Drill(DrillOptions{Storage: "local"}) != 0 {
			t.Fatal("skip failed")
		}
		s := f.drillState(t)
		if len(s.Pending["local"]) != 1 || s.Pending["local"][0] != "app" {
			t.Fatalf("failure forgotten after skip: %+v", s.Pending)
		}
		if _, ok := s.Results["local"]["db"]; ok {
			t.Fatal("rotation passed unresolved failure")
		}
	}
}

func TestDrillFailureNotifiesWhenResultWriteFails(t *testing.T) {
	f := drillFixture(t, "app")
	f.revs(1, "1")
	sent := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sent++ }))
	defer srv.Close()
	f.vars["NTFY_URL"] = srv.URL + "/archiver"
	f.vars["NTFY_ON"] = "failures"
	state := f.env.Layout.DrillState()
	script := fmt.Sprintf("#!/bin/sh\nrm '%s' && mkdir '%s'\nexit 1\n", state, state)
	if err := os.WriteFile(filepath.Join(f.root, "services", "app", "drill"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if f.env.Drill(DrillOptions{Storage: "local"}) != 1 {
		t.Fatal("failed drill passed")
	}
	if !strings.Contains(f.out.String(), "Could not record the drill") {
		t.Fatalf("state write did not fail: %s", f.out.String())
	}
	if _, ok := notify.OpenIncidents(f.env.Layout.Incidents())["drill:local:app"]; !ok || sent != 1 {
		t.Fatalf("failure lost: incident=%v notifications=%d", ok, sent)
	}
}
