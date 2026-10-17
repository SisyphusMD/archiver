package pipeline

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/hooks"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

// A hook held back by a pause and then stopped: a pre-backup hook never starts (it would
// stop something for a backup that will not happen), a post-backup hook still runs.
func TestStopDuringPauseSkipsOnlyThePreHook(t *testing.T) {
	root := t.TempDir()
	b, svc, _ := newBackup(t, root, false)
	marker := filepath.Join(root, "ran")
	for _, name := range []string{hooks.PreBackup, hooks.PostBackup} {
		script := "#!/bin/sh\necho " + name + " >> " + marker + "\n"
		if err := os.WriteFile(filepath.Join(svc, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	l, _, err := runlock.Acquire(b.Layout.BackupLock(), filepath.Join(b.Layout.Lock, "archiver-stop-requested"), "duplicacy", "pre-backup")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	b.lock = l
	s := hooks.Service{Name: "app", Dir: svc, SnapshotID: "h-app"}
	for _, c := range []struct {
		name    string
		skip    bool
		wantRan bool
	}{{hooks.PreBackup, true, false}, {hooks.PostBackup, false, true}} {
		os.Remove(l.StopFlag)
		if err := runlock.Append(l.Path, "paused", time.Now()); err != nil {
			t.Fatal(err)
		}
		go func() {
			time.Sleep(300 * time.Millisecond)
			os.WriteFile(l.StopFlag, nil, 0o600)
		}()
		_, ran, err := b.runHook(s, c.name, "", c.skip)
		if err != nil || ran != c.wantRan {
			t.Fatalf("%s: ran %v, err %v; want ran %v", c.name, ran, err, c.wantRan)
		}
		if err := runlock.Append(l.Path, "running", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := os.ReadFile(marker); string(got) != hooks.PostBackup+"\n" {
		t.Fatalf("hooks that ran: %q, want only the post-backup hook", got)
	}
}
