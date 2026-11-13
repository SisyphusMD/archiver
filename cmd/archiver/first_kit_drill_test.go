package main

import (
	"context"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

func TestDrillInterrupted(t *testing.T) {
	for kinds, want := range map[string]bool{
		"":                    false,
		"backup maintenance":  false,
		"deep-check:cloud":    false,
		"drill":               true,
		"backup drill:GARAGE": true,
		"kit-drill":           true,
	} {
		if got := drillInterrupted(strings.Fields(kinds)); got != want {
			t.Errorf("drillInterrupted(%q) = %v, want %v", kinds, got, want)
		}
	}
}

func TestFirstKitDrillDue(t *testing.T) {
	ready := func() bool { return true }
	scheduled := []daemon.Job{{Name: "backup"}, {Name: "kit-drill"}}
	l := layout.Layout{Root: t.TempDir(), Lock: t.TempDir()}
	if err := os.MkdirAll(l.LogDir(), 0700); err != nil {
		t.Fatal(err)
	}
	if firstKitDrillDue(l, []daemon.Job{{Name: "backup"}}, ready) {
		t.Fatal("due with no kit drill schedule")
	}
	if !firstKitDrillDue(l, scheduled, ready) {
		t.Fatal("not due with a schedule and a free lock")
	}
	if firstKitDrillDue(l, scheduled, func() bool { return false }) {
		t.Fatal("due though the kit is not ready")
	}
	lock, _, err := runlock.Acquire(l.DrillLock(), "", "drill", "running")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	if firstKitDrillDue(l, scheduled, ready) {
		t.Fatal("due while a drill holds the lock")
	}
}

func TestFirstKitDrillRuns(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		due  bool
		want int32
	}{
		{name: "due", ctx: context.Background(), due: true, want: 1},
		{name: "not due", ctx: context.Background()},
		{name: "shutting down", ctx: cancelled, due: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var log strings.Builder
			var runs atomic.Int32
			f := &firstKitDrill{log: &log, turn: newDrillTurn(io.Discard, idle), due: func() bool { return tc.due }, run: func(j daemon.Job) int {
				if j.Name != "kit-drill" {
					t.Errorf("ran %s", j.Name)
				}
				runs.Add(1)
				return 0
			}}
			f.try(tc.ctx)
			f.finished.Wait()
			if runs.Load() != tc.want {
				t.Fatalf("%d runs, want %d", runs.Load(), tc.want)
			}
			if tc.want > 0 && !strings.Contains(log.String(), "kit-drill exited 0") {
				t.Fatalf("log: %s", log.String())
			}
		})
	}
}

func TestFirstKitDrillShutdownDuringCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var runs atomic.Int32
	f := &firstKitDrill{log: io.Discard, turn: newDrillTurn(io.Discard, idle), due: func() bool {
		cancel()
		return true
	}, run: func(daemon.Job) int {
		runs.Add(1)
		return 0
	}}
	f.try(ctx)
	f.finished.Wait()
	if runs.Load() != 0 {
		t.Fatal("a drill started after the shutdown")
	}
}

// A stalled drill holds up neither the backup that asked nor the next one, which starts no
// second drill.
func TestFirstKitDrillHoldsNothingUp(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var runs atomic.Int32
	f := &firstKitDrill{log: io.Discard, turn: newDrillTurn(io.Discard, idle), due: func() bool { return true }, run: func(daemon.Job) int {
		if runs.Add(1) == 1 {
			close(started)
		}
		<-release
		return 0
	}}
	f.try(context.Background())
	<-started
	f.try(context.Background())
	close(release)
	f.finished.Wait()
	if runs.Load() != 1 {
		t.Fatalf("%d runs", runs.Load())
	}
}
