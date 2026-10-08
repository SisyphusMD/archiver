package e2e

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/tests/e2e/harness"
)

// runsInstall is one deployment with a single service "app" and a single local storage,
// the smallest installation that has a backup and a maintenance pipeline.
type runsInstall struct {
	*harness.Deployment
	svc, storage string
}

func newRunsInstall(t *testing.T) runsInstall {
	t.Helper()
	_, image := images(t)
	work := harness.WorkDir(t)
	in := runsInstall{svc: filepath.Join(work, "svc", "app"), storage: filepath.Join(work, "storage")}
	restores := filepath.Join(work, "restores")
	keysDir := filepath.Join(work, "keys")
	for _, d := range []string{in.svc, in.storage, restores, keysDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	in.Deployment = &harness.Deployment{
		Image:       image,
		Hostname:    "e2e-runs",
		Services:    map[string]string{"app": in.svc},
		Storages:    []harness.Storage{{Name: "primary", Dir: in.storage}},
		Keys:        harness.NewKeys(t, image, keysDir),
		RestoreRoot: restores,
	}
	return in
}

func (in runsInstall) mustBackup(t *testing.T, wantRevs ...int) {
	t.Helper()
	if r := in.Backup(t); r.Code != 0 {
		t.Fatalf("backup exited %d:\n%s", r.Code, r.Output())
	}
	in.wantRevisions(t, "after the backup", wantRevs...)
}

func (in runsInstall) mustMaintain(t *testing.T) {
	t.Helper()
	if r := in.Maintenance(t); r.Code != 0 {
		t.Fatalf("maintenance exited %d:\n%s", r.Code, r.Output())
	}
}

func (in runsInstall) wantRevisions(t *testing.T, when string, want ...int) {
	t.Helper()
	if got := harness.Revisions(t, in.storage, in.SnapshotID("app")); !slices.Equal(got, want) {
		t.Fatalf("%s, revisions = %v, want %v", when, got, want)
	}
}

// TestRunsPauseResume: pausing a backup mid-upload suspends every duplicacy process and
// stops the storage changing; resuming lets the same run finish successfully with a
// revision that restores the data exactly.
func TestRunsPauseResume(t *testing.T) {
	in := newRunsInstall(t)
	in.SingleThreaded()
	// Random so nothing dedups or compresses: at one thread the upload takes long
	// enough to pause well before it ends.
	writeRandom(t, filepath.Join(in.svc, "big.bin"), 512<<20)
	want := harness.Snapshot(t, in.svc)
	in.Start(t)

	run := in.StartBackup(t)
	harness.Poll(t, 2*time.Minute, "the backup to start uploading chunks", func() bool {
		return run.Done() || harness.ChunkCount(t, in.storage) >= 2
	})
	if run.Done() {
		t.Fatalf("backup ended before it could be paused: %+v", run.Wait(t, 0))
	}

	in.Pause(t)
	allStopped := func() bool {
		s := in.DuplicacyStates(t)
		return len(s) > 0 && !slices.ContainsFunc(s, func(st string) bool { return !strings.HasPrefix(st, "T") })
	}
	harness.Poll(t, 10*time.Second, "every duplicacy process to be stopped (state T) after pause", allStopped)
	frozen := harness.Fingerprint(t, in.storage)
	harness.Hold(t, 3*time.Second, "a paused backup (duplicacy stopped, storage unchanged, run alive)", func() bool {
		return allStopped() && !run.Done() && harness.Fingerprint(t, in.storage) == frozen
	})

	in.Resume(t)
	harness.Poll(t, 10*time.Second, "duplicacy to leave the stopped state after resume", func() bool {
		return run.Done() || !slices.ContainsFunc(in.DuplicacyStates(t), func(st string) bool { return strings.HasPrefix(st, "T") })
	})
	if r := run.Wait(t, 5*time.Minute); r.Code != 0 {
		t.Fatalf("resumed backup exited %d:\n%s", r.Code, r.Output())
	}
	in.wantRevisions(t, "after the resumed backup", 1)
	if d := want.Diff(harness.Snapshot(t, in.Restore(t, "app", 1, "primary"))); len(d) > 0 {
		t.Errorf("restore of the paused and resumed revision differs: %v", d)
	}
}

// TestRunsSecondBackupRefused: while a backup is running, later ones exit non-zero
// without ever starting their own run of the service (its hook is entered once and
// they add no revision), and the first still completes.
func TestRunsSecondBackupRefused(t *testing.T) {
	in := newRunsInstall(t)
	writeSmall(t, in.svc, "one")
	gate := in.GatePreHook(t, "app")
	in.Start(t)

	first := in.StartBackup(t)
	harness.Poll(t, time.Minute, "the first backup to reach its pre-backup hook", func() bool {
		return first.Done() || gate.Entered(t) == 1
	})
	if first.Done() {
		t.Fatalf("first backup ended at the gate: %+v", first.Wait(t, 0))
	}

	// Twice: a refusal that releases the running backup's claim (say, a cleanup that
	// removes a lock it never took) admits the next attempt.
	for _, which := range []string{"second", "third"} {
		late := in.StartBackup(t)
		harness.Poll(t, time.Minute, "the "+which+" backup to be refused", func() bool {
			return late.Done() || gate.Entered(t) > 1
		})
		if n := gate.Entered(t); n != 1 {
			t.Fatalf("the %s backup ran the service's backup concurrently (hook entered %d times)", which, n)
		}
		if r := late.Wait(t, 0); r.Code == 0 {
			t.Fatalf("%s backup exited 0 while one was running:\n%s", which, r.Output())
		}
		if first.Done() {
			t.Fatalf("refusing the %s backup ended the first: %+v", which, first.Wait(t, 0))
		}
	}

	gate.Open(t)
	if r := first.Wait(t, 2*time.Minute); r.Code != 0 {
		t.Fatalf("first backup exited %d:\n%s", r.Code, r.Output())
	}
	in.wantRevisions(t, "after the first backup finished", 1)
}

// TestRunsRefusedBackupNotifies: a backup refused because another still holds the lock is
// reported, not silent. A scheduled run that never happens must reach whoever relies on it
// (ADR 13).
func TestRunsRefusedBackupNotifies(t *testing.T) {
	in := newRunsInstall(t)
	writeSmall(t, in.svc, "one")
	gate := in.GatePreHook(t, "app")
	n := harness.NewNotifier(t)
	notifierDir, err := os.MkdirTemp(filepath.Dir(in.Keys.PrivatePath), "notifier-")
	if err != nil {
		t.Fatal(err)
	}
	in.NotifyTo(t, n, notifierDir)
	in.Start(t)

	first := in.StartBackup(t)
	harness.Poll(t, time.Minute, "the first backup to reach its pre-backup hook", func() bool {
		return first.Done() || gate.Entered(t) == 1
	})
	if first.Done() {
		t.Fatalf("first backup ended at the gate: %+v", first.Wait(t, 0))
	}

	// The first run is parked in its hook, so anything that arrives now is about the refusal.
	before := n.Arrived()
	if r := in.Backup(t); r.Code == 0 {
		t.Fatalf("second backup exited 0 while one was running:\n%s", r.Output())
	}
	harness.Poll(t, 30*time.Second, "a notification about the refused backup", func() bool {
		return n.Arrived() > before
	})

	gate.Open(t)
	if r := first.Wait(t, 2*time.Minute); r.Code != 0 {
		t.Fatalf("first backup exited %d:\n%s", r.Code, r.Output())
	}
}

// TestRunsBackupRefusedDuringRestore: while a restore into a service directory runs (here
// parked in its post-restore hook), a backup is refused and writes nothing: it would save
// a half-restored directory. Once the restore ends, backups run again.
func TestRunsBackupRefusedDuringRestore(t *testing.T) {
	in := newRunsInstall(t)
	gate := in.GatePostRestore(t, "app")
	writeSmall(t, in.svc, "one")
	in.Start(t)
	in.mustBackup(t, 1)

	restore := in.StartArchiver(t, map[string]string{
		"SNAPSHOT_ID": in.SnapshotID("app"), "LOCAL_DIR": in.ServiceDir("app"),
		"OVERWRITE": "1", "RUN_RESTORE_SERVICE": "1",
	}, "auto-restore")
	harness.Poll(t, time.Minute, "the restore to reach its post-restore hook", func() bool {
		return restore.Done() || gate.Entered(t) == 1
	})
	if restore.Done() {
		t.Fatalf("restore ended before its hook: %+v", restore.Wait(t, 0))
	}

	writeSmall(t, in.svc, "two")
	if r := in.Backup(t); r.Code == 0 {
		t.Fatalf("a backup ran during a restore into its service directory:\n%s", r.Output())
	}
	in.wantRevisions(t, "after the backup refused during the restore", 1)

	gate.Open(t)
	if r := restore.Wait(t, 2*time.Minute); r.Code != 0 {
		t.Fatalf("restore exited %d:\n%s", r.Code, r.Output())
	}
	in.mustBackup(t, 1, 2)
}

// TestRunsMaintenanceAlongsideBackup: maintenance succeeds while a backup is in progress,
// and the backup still completes.
func TestRunsMaintenanceAlongsideBackup(t *testing.T) {
	in := newRunsInstall(t)
	in.SetMaintenance(harness.MaintenanceConfig{Check: true, Prune: true, Exhaustive: harness.ExhaustiveDaily})
	writeSmall(t, in.svc, "one")
	gate := in.GatePreHook(t, "app")
	in.Start(t)

	// Maintenance needs something on storage to maintain.
	gate.Open(t)
	in.mustBackup(t, 1)
	gate.Close(t)

	writeSmall(t, in.svc, "two")
	backup := in.StartBackup(t)
	harness.Poll(t, time.Minute, "the backup to reach its pre-backup hook", func() bool {
		return backup.Done() || gate.Entered(t) == 2
	})
	if backup.Done() {
		t.Fatalf("backup ended at the gate: %+v", backup.Wait(t, 0))
	}

	if r := in.Maintenance(t); r.Code != 0 {
		t.Fatalf("maintenance alongside a running backup exited %d:\n%s", r.Code, r.Output())
	}
	if backup.Done() {
		t.Fatalf("maintenance ended the running backup: %+v", backup.Wait(t, 0))
	}

	gate.Open(t)
	if r := backup.Wait(t, 2*time.Minute); r.Code != 0 {
		t.Fatalf("backup that overlapped maintenance exited %d:\n%s", r.Code, r.Output())
	}
	in.wantRevisions(t, "after the overlapped backup", 1, 2)
}

// TestRunsPruneOnlyInMaintenance follows orphan chunks through duplicacy's two-step
// collection. Only an exhaustive prune fossilizes an orphan, and only a later prune
// deletes that fossil once a newer revision exists, so where each step happens shows
// which runs pruned: backups never do (not even exhaustively when one is due), the first
// maintenance prunes exhaustively (never run, so due), and the next one within the daily
// interval prunes but not exhaustively.
func TestRunsPruneOnlyInMaintenance(t *testing.T) {
	in := newRunsInstall(t)
	in.SetMaintenance(harness.MaintenanceConfig{Check: true, Prune: true, Exhaustive: harness.ExhaustiveDaily})
	in.Start(t)

	// Planted before the first backup, so even a backup that prunes exhaustively only
	// once (the first time it is due) is caught.
	a := harness.PlantOrphanChunk(t, in.storage)
	writeSmall(t, in.svc, "one")
	in.mustBackup(t, 1)
	if s := harness.StateOf(t, a); s != harness.ChunkLive {
		t.Fatalf("a backup pruned: the orphan is %s, want %s", s, harness.ChunkLive)
	}

	in.mustMaintain(t)
	if s := harness.StateOf(t, a); s != harness.ChunkFossil {
		t.Fatalf("the first maintenance did not prune exhaustively: the orphan is %s, want %s", s, harness.ChunkFossil)
	}

	// Duplicacy deletes a fossil only after a backup that started later than the collection
	// was made, and compares at one-second resolution: a backup in the same second as the
	// first maintenance's prune does not count.
	time.Sleep(1100 * time.Millisecond)
	b := harness.PlantOrphanChunk(t, in.storage)
	writeSmall(t, in.svc, "two")
	in.mustBackup(t, 1, 2)
	if s := harness.StateOf(t, a); s != harness.ChunkFossil {
		t.Fatalf("a backup pruned: the fossil is %s, want %s", s, harness.ChunkFossil)
	}

	in.mustMaintain(t)
	if s := harness.StateOf(t, a); s != harness.ChunkGone {
		t.Fatalf("the second maintenance did not prune: the fossil is %s, want %s", s, harness.ChunkGone)
	}
	if s := harness.StateOf(t, b); s != harness.ChunkLive {
		t.Fatalf("the second maintenance pruned exhaustively within the daily interval: the new orphan is %s, want %s", s, harness.ChunkLive)
	}
	in.wantRevisions(t, "after maintenance with the default retention", 1, 2)
}

// TestRunsFossilsKeptForNewService: a fossil collection one maintenance leaves pending is
// finished by the next, even after a service that sorts first is added: duplicacy keeps
// the collection in the cache of the repository that pruned, and a new service must not
// take that repository's place. It holds both for a collection this image made (logs
// mounted, as compose does) and for one the baseline release left in the first service's
// repository, which the upgrade takes over.
func TestRunsFossilsKeptForNewService(t *testing.T) {
	baseline, current := images(t)
	for _, c := range []struct{ name, first string }{
		{"made by this image", current},
		{"made by the baseline release", baseline},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := newRunsInstall(t)
			in.SetMaintenance(harness.MaintenanceConfig{Prune: true, Exhaustive: harness.ExhaustiveDaily})
			logs := filepath.Join(filepath.Dir(filepath.Dir(in.svc)), "logs")
			if err := os.MkdirAll(logs, 0o755); err != nil {
				t.Fatal(err)
			}
			in.Mount(logs, "/opt/archiver/logs")
			in.Image = c.first
			in.Start(t)

			orphan := harness.PlantOrphanChunk(t, in.storage)
			writeSmall(t, in.svc, "one")
			in.mustBackup(t, 1)
			in.mustMaintain(t)
			if s := harness.StateOf(t, orphan); s != harness.ChunkFossil {
				t.Fatalf("the first maintenance did not prune exhaustively: the orphan is %s, want %s", s, harness.ChunkFossil)
			}
			in.Stop(t)

			// "aaa" sorts before "app", and has a repository once it is backed up.
			early := filepath.Join(filepath.Dir(in.svc), "aaa")
			if err := os.MkdirAll(early, 0o755); err != nil {
				t.Fatal(err)
			}
			writeSmall(t, early, "early")
			in.Services["aaa"] = early
			in.Image = current
			in.Start(t)
			time.Sleep(1100 * time.Millisecond) // see TestRunsPruneOnlyInMaintenance
			writeSmall(t, in.svc, "two")
			in.mustBackup(t, 1, 2)

			in.mustMaintain(t)
			if s := harness.StateOf(t, orphan); s != harness.ChunkGone {
				t.Fatalf("maintenance after adding a service did not finish the pending collection: the fossil is %s, want %s", s, harness.ChunkGone)
			}
		})
	}
}

// TestRunsMaintenanceGoesOnPastBadSecondary: a secondary maintenance cannot open fails on
// its own, and the primary is still pruned; the run reports the failure.
func TestRunsMaintenanceGoesOnPastBadSecondary(t *testing.T) {
	in := newRunsInstall(t)
	in.SetMaintenance(harness.MaintenanceConfig{Prune: true, Exhaustive: harness.ExhaustiveDaily})
	writeSmall(t, in.svc, "one")
	in.Start(t)
	in.mustBackup(t, 1)
	in.Stop(t)

	// A secondary whose path is a file: no storage can be made or opened there.
	bad := filepath.Join(filepath.Dir(in.storage), "not-a-dir")
	if err := os.WriteFile(bad, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	in.Storages = append(in.Storages, harness.Storage{Name: "offsite", Dir: bad})
	in.Start(t)
	orphan := harness.PlantOrphanChunk(t, in.storage)
	if r := in.Maintenance(t); r.Code == 0 {
		t.Fatalf("maintenance exited 0 with a secondary it cannot open:\n%s", r.Output())
	}
	if s := harness.StateOf(t, orphan); s != harness.ChunkFossil {
		t.Fatalf("the primary was not pruned past the bad secondary: the orphan is %s, want %s", s, harness.ChunkFossil)
	}
}

// TestRunsExhaustivePruneOff: with the exhaustive interval off, maintenance prunes but
// never exhaustively, so an orphan survives.
func TestRunsExhaustivePruneOff(t *testing.T) {
	in := newRunsInstall(t)
	in.SetMaintenance(harness.MaintenanceConfig{Check: true, Prune: true, Exhaustive: harness.ExhaustiveOff})
	in.Start(t)

	writeSmall(t, in.svc, "one")
	in.mustBackup(t, 1)
	orphan := harness.PlantOrphanChunk(t, in.storage)
	in.mustMaintain(t)
	if s := harness.StateOf(t, orphan); s != harness.ChunkLive {
		t.Fatalf("maintenance pruned exhaustively with the interval off: the orphan is %s, want %s", s, harness.ChunkLive)
	}
}

// TestRunsCheckOnlyInMaintenance removes a data chunk a revision references. A check
// always notices a missing chunk, so maintenance must fail; a backup of unchanged data
// never needs that chunk, so it succeeds unless it checks the storage too.
func TestRunsCheckOnlyInMaintenance(t *testing.T) {
	in := newRunsInstall(t)
	in.SetMaintenance(harness.MaintenanceConfig{Check: true, Prune: false, Exhaustive: harness.ExhaustiveOff})
	writeSmall(t, in.svc, "one")
	writeRandom(t, filepath.Join(in.svc, "big.bin"), 20<<20)
	in.Start(t)

	in.mustBackup(t, 1)
	in.mustMaintain(t)

	if err := os.Remove(harness.LargestChunk(t, in.storage)); err != nil {
		t.Fatal(err)
	}
	if r := in.Backup(t); r.Code != 0 {
		t.Fatalf("a backup checked the storage (it failed on a chunk it does not need), exit %d:\n%s", r.Code, r.Output())
	}
	in.wantRevisions(t, "after the backup over a damaged storage", 1, 2)
	if r := in.Maintenance(t); r.Code == 0 {
		t.Fatalf("maintenance exited 0 on a storage missing a referenced chunk; it did not check:\n%s", r.Output())
	}
}

// writeSmall makes the service's small file hold content, so each backup has a change.
func writeSmall(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "small.txt"), []byte(content+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeRandom writes size bytes that neither compress nor dedup.
func writeRandom(t *testing.T, path string, size int) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var seed [32]byte
	for i := range seed {
		seed[i] = byte(rand.Uint32())
	}
	src := rand.NewChaCha8(seed)
	buf := make([]byte, 1<<20)
	for written := 0; written < size; written += len(buf) {
		src.Read(buf)
		if _, err := f.Write(buf); err != nil {
			t.Fatal(err)
		}
	}
}
