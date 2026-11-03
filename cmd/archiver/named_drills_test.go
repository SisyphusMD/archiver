package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/resume"
)

func TestNamedDrillSchedulingAndCatchUp(t *testing.T) {
	l := layout.Layout{Root: t.TempDir(), Lock: t.TempDir()}
	if err := os.MkdirAll(l.LogDir(), 0700); err != nil {
		t.Fatal(err)
	}
	noteDrillSchedule(l, []daemon.Job{{Name: "backup"}})
	if _, err := os.Stat(l.DrillState()); !os.IsNotExist(err) {
		t.Fatalf("backup-only scheduling created drill state: %v", err)
	}
	s := lockstate.DrillState{LastPass: 42, Named: map[string]*lockstate.DrillState{"GARAGE": {NextStorage: 1}}}
	if err := lockstate.WriteDrillState(l.DrillState(), s); err != nil {
		t.Fatal(err)
	}
	noteDrillSchedule(l, []daemon.Job{{Name: "drill"}, {Name: "drill:GARAGE"}})
	got, err := lockstate.ReadDrillState(l.DrillState())
	if err != nil {
		t.Fatal(err)
	}
	if got.Scheduled == 0 || got.LastPass != 42 || got.Named["GARAGE"].Scheduled == 0 || got.Named["GARAGE"].NextStorage != 1 {
		t.Fatalf("%+v", got)
	}
	run, err := resume.Begin(l.RunRecord("drill"), resume.Record{Drill: "GARAGE"})
	if err != nil {
		t.Fatal(err)
	}
	defer run.End()
	if kinds := interruptedRuns(l); len(kinds) != 1 || kinds[0] != "drill:GARAGE" {
		t.Fatalf("%v", kinds)
	}
}

func TestInterruptedDeepChecksCatchUp(t *testing.T) {
	l := layout.Layout{Root: t.TempDir(), Lock: t.TempDir()}
	if err := os.MkdirAll(l.LogDir(), 0700); err != nil {
		t.Fatal(err)
	}
	// Records are filed under the sanitized storage name and carry the configured one.
	for configured, file := range map[string]string{"local-backups": "local_backups", "backblaze": "backblaze"} {
		if _, err := resume.Begin(l.RunRecord("deep-check-"+file), resume.Record{Services: map[string]string{configured: resume.Pending}}); err != nil {
			t.Fatal(err)
		}
	}
	got := interruptedRuns(l)
	if len(got) != 2 || got[0] != "deep-check:backblaze" || got[1] != "deep-check:local-backups" {
		t.Fatalf("%v", got)
	}
	env := map[string]string{"STORAGE_TARGET_1_NAME": "local-backups", "STORAGE_TARGET_1_DEEP_CHECK": "0 6 1 * *"}
	jobs, err := daemon.Jobs(func(k string) string { return env[k] }, time.Now(), nil)
	if err != nil || len(jobs) != 1 || jobs[0].Name != got[1] {
		t.Fatalf("scheduled deep check %v does not match catch-up name %q: %v", jobs, got[1], err)
	}
	// Without a matching schedule (or without the daemon) the catch-up name must still run.
	for kind, want := range map[string]string{
		"deep-check:local-backups": "deep-check local-backups",
		"drill:GARAGE":             "drill --named GARAGE",
		"maintenance":              "maintenance",
	} {
		if args := strings.Join(catchUpArgs(kind), " "); args != want {
			t.Errorf("catchUpArgs(%q) = %q, want %q", kind, args, want)
		}
	}
}
