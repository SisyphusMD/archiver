package restore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/notify"
)

func namedFixture(t *testing.T) *fixture {
	t.Helper()
	f := drillFixture(t, "app", "db")
	f.vars["RESTORE_DRILL_GARAGE_SCHEDULE"] = "0 8 * * 6#1"
	f.vars["RESTORE_DRILL_GARAGE_SERVICES"] = "app,db"
	f.vars["RESTORE_DRILL_GARAGE_STORAGES"] = "alternate"
	f.vars["RESTORE_DRILL_EXCLUDE"] = "app,db"
	for k, v := range f.vars {
		f.env.Environ = append(f.env.Environ, k+"="+v)
	}
	f.revs(1, "1")
	f.revs(2, "1")
	return f
}

func TestNamedAlternatePersistenceAndFailedRetry(t *testing.T) {
	f := namedFixture(t)
	o := DrillOptions{Named: "GARAGE"}
	for _, storage := range []string{"local", "off_site"} {
		if code := f.env.Drill(o); code != 0 {
			t.Fatalf("%s: %d %s", storage, code, f.out.String())
		}
		s := f.drillState(t)
		n := s.Named["GARAGE"]
		if !n.Results[storage]["app"].OK || !n.Results[storage]["db"].OK || len(s.Results) != 0 {
			t.Fatalf("%+v", s)
		}
	}
	if err := os.WriteFile(filepath.Join(f.root, "store1", "files"), []byte("7"), 0600); err != nil {
		t.Fatal(err)
	}
	if f.env.Drill(o) != 1 {
		t.Fatal("failed restore passed")
	}
	s := f.drillState(t)
	n := s.Named["GARAGE"]
	if n.NextStorage != 0 || n.Storage != "local" || !n.LastFailed {
		t.Fatalf("%+v", n)
	}
	if _, ok := notify.OpenIncidents(f.env.Layout.Incidents())["drill:GARAGE:local:app"]; !ok {
		t.Fatal("missing named incident")
	}
	// A newly constructed Env reads the selection from disk rather than a process cursor.
	next := *f.env
	f.env = &next
	if f.env.Drill(o) != 1 || f.drillState(t).Named["GARAGE"].Storage != "local" {
		t.Fatal("failed run advanced")
	}
	if err := os.Remove(filepath.Join(f.root, "store1", "files")); err != nil {
		t.Fatal(err)
	}
	if f.env.Drill(o) != 0 || f.drillState(t).Named["GARAGE"].NextStorage != 1 {
		t.Fatalf("retry: %s", f.out.String())
	}
}

func TestNamedAlternateSkipsAndInterruptedState(t *testing.T) {
	f := namedFixture(t)
	f.revs(1, "")
	if f.env.Drill(DrillOptions{Named: "GARAGE"}) != 0 || f.drillState(t).Named["GARAGE"].NextStorage != 1 {
		t.Fatal("genuine skip did not advance")
	}
	s := f.drillState(t)
	n := s.Named["GARAGE"]
	n.Storage = "local"
	n.NextStorage = 1
	n.Pending["local"] = []string{"app", "db"}
	n.Results["local"]["app"] = lockstate.DrillResult{Interrupted: true}
	if err := lockstate.WriteDrillState(f.env.Layout.DrillState(), s); err != nil {
		t.Fatal(err)
	}
	f.revs(1, "1")
	if f.env.Drill(DrillOptions{Named: "GARAGE"}) != 0 {
		t.Fatal(f.out.String())
	}
	if !f.drillState(t).Named["GARAGE"].Results["local"]["app"].OK {
		t.Fatal("interrupted storage did not retry")
	}
}

func TestNamedAlternateTimeoutAndSkippedFailureRemainDue(t *testing.T) {
	f := namedFixture(t)
	hook := filepath.Join(f.root, "services", "app", "drill")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nsleep 30 & wait\n"), 0700); err != nil {
		t.Fatal(err)
	}
	f.vars["RESTORE_DRILL_GARAGE_TIMEOUT"] = "100ms"
	if f.env.Drill(DrillOptions{Named: "GARAGE"}) != 1 {
		t.Fatal("timeout passed")
	}
	n := f.drillState(t).Named["GARAGE"]
	if n.Storage != "local" || n.NextStorage != 0 || len(n.Pending["local"]) != 2 || !n.LastFailed || !strings.Contains(n.Results["local"]["app"].Message, "RESTORE_DRILL_GARAGE_TIMEOUT") {
		t.Fatalf("timeout state: %+v", n)
	}
	delete(f.vars, "RESTORE_DRILL_GARAGE_TIMEOUT")
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.root, "store1", "files"), []byte("7"), 0600); err != nil {
		t.Fatal(err)
	}
	if f.env.Drill(DrillOptions{Named: "GARAGE"}) != 1 {
		t.Fatal("failure passed")
	}
	f.revs(1, "")
	if f.env.Drill(DrillOptions{Named: "GARAGE"}) != 1 {
		t.Fatal("a skip after an unresolved failure passed the named drill")
	}
	n = f.drillState(t).Named["GARAGE"]
	if n.Storage != "local" || n.NextStorage != 0 || len(n.Pending["local"]) != 2 || !n.LastFailed || n.LastPass != 0 {
		t.Fatalf("skip after failure advanced: %+v", n)
	}
}

func TestNamedDrillConfigurationDoesNotBlockBareRecovery(t *testing.T) {
	f := namedFixture(t)
	if err := os.RemoveAll(filepath.Join(f.root, "services")); err != nil {
		t.Fatal(err)
	}
	if !f.env.load() {
		t.Fatalf("a bare restore cannot load named drill configuration: %s", f.err.String())
	}
	if f.env.Drill(DrillOptions{Named: "GARAGE"}) != 1 {
		t.Fatal("a live drill accepted nonexistent services")
	}
}

func TestNamedAlternateSubsetAndListChanges(t *testing.T) {
	f := namedFixture(t)
	f.vars["STORAGE_TARGET_3_NAME"] = "third"
	f.vars["STORAGE_TARGET_3_TYPE"] = "local"
	f.vars["STORAGE_TARGET_3_LOCAL_PATH"] = filepath.Join(f.root, "store3")
	if err := os.Mkdir(filepath.Join(f.root, "store3"), 0700); err != nil {
		t.Fatal(err)
	}
	f.revs(3, "1")
	f.vars["RESTORE_DRILL_GARAGE_STORAGES"] = "alternate:third,off-site"
	o := DrillOptions{Named: "GARAGE"}
	for i, storage := range []string{"third", "off_site", "third"} {
		if code := f.env.Drill(o); code != 0 {
			t.Fatalf("run %d: %s %s", i, f.out.String(), f.err.String())
		}
		n := f.drillState(t).Named["GARAGE"]
		if !n.Results[storage]["app"].OK || n.NextStorage != (i+1)%2 || len(n.Results["local"]) != 0 {
			t.Fatalf("run %d: %+v", i, n)
		}
	}
	// Reordering resets a still-in-range cursor rather than interpreting it against a new list.
	f.vars["RESTORE_DRILL_GARAGE_STORAGES"] = "alternate:off-site,third"
	if err := os.WriteFile(filepath.Join(f.root, "store2", "files"), []byte("7"), 0600); err != nil {
		t.Fatal(err)
	}
	if f.env.Drill(o) != 1 || f.drillState(t).Named["GARAGE"].Storage != "off_site" {
		t.Fatal("changed list did not reset to its first storage")
	}
	f.vars["RESTORE_DRILL_GARAGE_STORAGES"] = "alternate:third,off-site"
	if f.env.Drill(o) != 1 || f.drillState(t).Named["GARAGE"].NextStorage != 1 {
		t.Fatal("pending storage still in the changed list did not retry")
	}
	// Removing the pending storage also shrinks the list below its persisted cursor.
	f.vars["RESTORE_DRILL_GARAGE_STORAGES"] = "alternate:third"
	if f.env.Drill(o) != 0 {
		t.Fatal(f.out.String())
	}
	n := f.drillState(t).Named["GARAGE"]
	if n.NextStorage != 0 || n.Storage != "" || !n.Results["third"]["app"].OK {
		t.Fatalf("shrunk list: %+v", n)
	}
}

func TestNamedAlternateRepeatedStorageKeepsFailedCursor(t *testing.T) {
	f := namedFixture(t)
	f.vars["RESTORE_DRILL_GARAGE_STORAGES"] = "alternate:local,off-site,local"
	if err := os.WriteFile(filepath.Join(f.root, "store1", "files"), []byte("7"), 0600); err != nil {
		t.Fatal(err)
	}
	o := DrillOptions{Named: "GARAGE"}
	if f.env.Drill(o) != 1 || f.drillState(t).Named["GARAGE"].NextStorage != 0 {
		t.Fatal("failed run moved to another occurrence of the same storage")
	}
	if err := os.Remove(filepath.Join(f.root, "store1", "files")); err != nil {
		t.Fatal(err)
	}
	for _, next := range []int{1, 2, 0} {
		if f.env.Drill(o) != 0 || f.drillState(t).Named["GARAGE"].NextStorage != next {
			t.Fatal("repeated-storage rotation did not preserve the listed order")
		}
	}
}
