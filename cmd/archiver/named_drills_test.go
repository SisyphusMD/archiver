package main

import (
	"os"
	"testing"

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
