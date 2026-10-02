package e2e

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/tests/e2e/harness"
)

// exitStopInstall is a deployment with local primary and copy storages, a signal directory
// its hooks and the test share, and one data directory per service.
type exitStopInstall struct {
	d                *harness.Deployment
	work             string
	svc              map[string]string
	primary, copyDir string
	signal           string // host side of harness.SignalDir
}

// newExitStopInstall prepares, but does not start, an installation of the image under test.
func newExitStopInstall(t *testing.T, services ...string) *exitStopInstall {
	t.Helper()
	_, image := images(t)
	work := harness.WorkDir(t)
	in := &exitStopInstall{
		work:    work,
		svc:     map[string]string{},
		primary: filepath.Join(work, "storage-primary"),
		copyDir: filepath.Join(work, "storage-copy"),
		signal:  filepath.Join(work, "signal"),
	}
	keysDir := filepath.Join(work, "keys")
	restores := filepath.Join(work, "restores")
	for _, s := range services {
		in.svc[s] = filepath.Join(work, "svc", s)
	}
	for _, dir := range append([]string{in.primary, in.copyDir, in.signal, keysDir, restores}, mapValues(in.svc)...) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range in.svc {
		addData(t, dir)
	}
	in.d = &harness.Deployment{
		Image:       image,
		Hostname:    "e2e-host",
		Services:    in.svc,
		Storages:    []harness.Storage{{Name: "primary", Dir: in.primary}, {Name: "offsite", Dir: in.copyDir}},
		Keys:        harness.NewKeys(t, image, keysDir),
		RestoreRoot: restores,
	}
	in.d.Mount(in.signal, harness.SignalDir)
	return in
}

func (in *exitStopInstall) mustBackUp(t *testing.T) {
	t.Helper()
	if r := in.d.Backup(t); r.Code != 0 {
		t.Fatalf("setup backup exited %d:\n%s", r.Code, r.Output())
	}
}

// TestExitStopFailedBackup: a run whose backup or copy cannot complete must exit non-zero,
// never report success. The storage breaks after a good first run, as a disk does.
func TestExitStopFailedBackup(t *testing.T) {
	for _, c := range []struct {
		name   string
		broken func(*exitStopInstall) string
		// primaryRevs is what the primary must hold after the failed run, which pins the
		// failure to the leg under test: a broken copy still leaves a good primary backup.
		primaryRevs []int
	}{
		{"primary storage broken", func(in *exitStopInstall) string { return in.primary }, nil},
		{"copy storage broken", func(in *exitStopInstall) string { return in.copyDir }, []int{1, 2}},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := newExitStopInstall(t, "app")
			in.d.Start(t)
			in.mustBackUp(t)

			harness.BreakStorage(t, c.broken(in))
			addData(t, in.svc["app"])
			r := in.d.Backup(t)
			if r.Code == 0 {
				t.Fatalf("backup with the %s exited 0 (success):\n%s", c.name, r.Output())
			}
			if c.primaryRevs != nil {
				if got := harness.Revisions(t, in.primary, in.d.SnapshotID("app")); !slices.Equal(got, c.primaryRevs) {
					t.Errorf("primary revisions = %v, want %v: the failure was not the copy", got, c.primaryRevs)
				}
			}
		})
	}
}

// TestExitStopFailedRestore: a restore that cannot deliver the requested revision must exit
// non-zero.
func TestExitStopFailedRestore(t *testing.T) {
	for _, c := range []struct {
		name     string
		revision int
		breakIt  bool
	}{
		{"revision that does not exist", 99, false},
		{"storage that lost its chunks", 1, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := newExitStopInstall(t, "app")
			in.d.Start(t)
			in.mustBackUp(t)
			if c.breakIt {
				harness.BreakStorage(t, in.primary)
			}
			if r, _ := in.d.TryRestore(t, "app", c.revision, "primary"); r.Code == 0 {
				t.Fatalf("restore of rev %d from a %s exited 0 (success):\n%s", c.revision, c.name, r.Output())
			}
		})
	}
}

// TestExitStopStoppedBackup: a backup stopped mid-run exits non-zero, and nothing that
// follows a backup (copies, check, prune) happens after the stop. Service a is backed up
// before the stop, so a copy or prune that wrongly ran would have work to do.
func TestExitStopStoppedBackup(t *testing.T) {
	in := newExitStopInstall(t, "a", "b")
	// A deployment that maintains its storages, so a prune after the stop could happen.
	in.d.SetMaintenance(harness.MaintenanceConfig{Check: true, Prune: true, Exhaustive: harness.ExhaustiveDaily})
	harness.InstallServiceHooks(t, in.d.Image, in.svc["b"], harness.ServiceHooks{
		Pre: `  if [ -e ` + harness.SignalDir + `/block ]; then
    touch ` + harness.SignalDir + `/pre-started
    while [ ! -e ` + harness.SignalDir + `/release ]; do sleep 0.2; done
  fi`,
		Post: `  touch ` + harness.SignalDir + `/post-ran`,
	})
	in.d.Start(t)
	in.mustBackUp(t)
	if err := os.Remove(filepath.Join(in.signal, "post-ran")); err != nil {
		t.Fatal(err)
	}

	orphans := []string{harness.PlantOrphanChunk(t, in.primary), harness.PlantOrphanChunk(t, in.copyDir)}
	addData(t, in.svc["a"])
	touch(t, filepath.Join(in.signal, "block"))
	done := in.d.StartBackup(t)
	waitForFile(t, filepath.Join(in.signal, "pre-started"), 3*time.Minute, done)
	in.d.RequestStop(t)
	touch(t, filepath.Join(in.signal, "release"))

	r := done.Wait(t, 3*time.Minute)
	if r.Code == 0 {
		t.Errorf("stopped backup exited 0 (success):\n%s", r.Output())
	}
	if !exists(filepath.Join(in.signal, "post-ran")) {
		t.Error("the stopped service's post hook did not run")
	}
	// Without a revision the copy lacks, "no copy followed" would hold even if one had run.
	if got := harness.Revisions(t, in.primary, in.d.SnapshotID("a")); !slices.Equal(got, []int{1, 2}) {
		t.Errorf("primary revisions of a = %v, want [1 2]: a was not backed up before the stop, so the copy check proves nothing", got)
	}
	for svc, want := range map[string][]int{"a": {1}, "b": {1}} {
		if got := harness.Revisions(t, in.copyDir, in.d.SnapshotID(svc)); !slices.Equal(got, want) {
			t.Errorf("copy storage revisions of %s after the stop = %v, want %v: a copy followed the stop", svc, got, want)
		}
	}
	for _, o := range orphans {
		if !exists(o) {
			t.Errorf("unreferenced chunk %s was pruned after the stop", o)
		}
	}
}

// TestExitStopSIGTERM: `docker stop` during a backup stops it gracefully: the container
// waits for the running service's hooks, runs its post hook, and only then exits, on its
// own, without the SIGKILL a grace period ends in.
func TestExitStopSIGTERM(t *testing.T) {
	in := newExitStopInstall(t, "app")
	// The pre hook outlives any immediate exit by far, so an entrypoint that does not wait
	// for the run is caught deterministically.
	harness.InstallServiceHooks(t, in.d.Image, in.svc["app"], harness.ServiceHooks{
		Pre:  `  touch ` + harness.SignalDir + `/pre-started; sleep 8`,
		Post: `  touch ` + harness.SignalDir + `/post-ran`,
	})
	in.d.Start(t)
	done := in.d.StartBackup(t)
	waitForFile(t, filepath.Join(in.signal, "pre-started"), 2*time.Minute, done)

	in.d.SendSIGTERM(t)
	if !in.d.WaitStopped(t, 100*time.Second) {
		t.Fatal("container still running 100s after SIGTERM")
	}
	if !exists(filepath.Join(in.signal, "post-ran")) {
		t.Error("container exited without running the post hook of the service it was backing up")
	}
	if got := harness.Revisions(t, in.primary, in.d.SnapshotID("app")); len(got) != 0 {
		t.Errorf("primary holds revisions %v: the backup ran on after SIGTERM instead of stopping", got)
	}
}

// TestExitStopFailureNotifies: with a notifier configured, a failed run sends a
// notification.
func TestExitStopFailureNotifies(t *testing.T) {
	in := newExitStopInstall(t, "app")
	n := harness.NewNotifier(t)
	notifierDir := filepath.Join(in.work, "notifier")
	if err := os.MkdirAll(notifierDir, 0o755); err != nil {
		t.Fatal(err)
	}
	in.d.NotifyTo(t, n, notifierDir)
	in.d.Start(t)
	in.mustBackUp(t)

	before := n.Arrived()
	harness.BreakStorage(t, in.primary)
	addData(t, in.svc["app"])
	if r := in.d.Backup(t); r.Code == 0 {
		t.Fatalf("backup to a broken storage exited 0:\n%s", r.Output())
	}
	// Delivery can trail the exit of an implementation that notifies asynchronously.
	for deadline := time.Now().Add(30 * time.Second); n.Arrived() == before; time.Sleep(200 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("no notification arrived for the failed backup")
		}
	}
}

// addData adds a file of fresh random bytes, so the next backup has new chunks to write.
func addData(t *testing.T, dir string) {
	t.Helper()
	b := make([]byte, 64<<10)
	rand.Read(b)
	f, err := os.CreateTemp(dir, "data-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(b); err != nil {
		t.Fatal(err)
	}
}

// waitForFile polls for path, failing early if the run it waits on ends first.
func waitForFile(t *testing.T, path string, timeout time.Duration, run *harness.Run) {
	t.Helper()
	for deadline := time.Now().Add(timeout); !exists(path); time.Sleep(200 * time.Millisecond) {
		if run.Done() {
			r := run.Wait(t, 0)
			t.Fatalf("run ended (exit %d) before %s appeared:\n%s", r.Code, filepath.Base(path), r.Output())
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s did not appear within %s", filepath.Base(path), timeout)
		}
	}
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func mapValues(m map[string]string) []string {
	v := make([]string, 0, len(m))
	for _, s := range m {
		v = append(v, s)
	}
	return v
}

// TestExitStopStoppedMaintenance: a stop applies to maintenance too. A maintenance pass
// stopped partway exits non-zero, and the storages it had not reached are left alone.
// Each storage carries an unreferenced chunk that an exhaustive prune removes, so the
// test can see where the pass got to and stops it after the first storage.
func TestExitStopStoppedMaintenance(t *testing.T) {
	const storages = 20
	in := newExitStopInstall(t, "app")
	in.d.Storages = nil
	for i := range storages {
		dir := filepath.Join(in.work, "storage-"+string(rune('a'+i)))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		in.d.Storages = append(in.d.Storages, harness.Storage{Name: "s" + string(rune('a'+i)), Dir: dir})
	}
	in.d.SetMaintenance(harness.MaintenanceConfig{Check: true, Prune: true, Exhaustive: harness.ExhaustiveDaily})
	in.d.Start(t)
	in.mustBackUp(t)

	var orphans []string
	for _, s := range in.d.Storages {
		orphans = append(orphans, harness.PlantOrphanChunk(t, s.Dir))
	}
	pruned := func() int {
		n := 0
		for _, o := range orphans {
			if !exists(o) {
				n++
			}
		}
		return n
	}

	run := in.d.StartMaintenance(t)
	for deadline := time.Now().Add(3 * time.Minute); pruned() == 0; time.Sleep(50 * time.Millisecond) {
		if run.Done() || time.Now().After(deadline) {
			t.Fatalf("maintenance pruned nothing before it ended or timed out:\n%s", run.Wait(t, 0).Output())
		}
	}
	in.d.RequestMaintenanceStop(t)
	r := run.Wait(t, 3*time.Minute)

	if n := pruned(); n == len(orphans) {
		t.Fatalf("maintenance reached all %d storages before the stop landed; the test proves nothing", n)
	}
	if r.Code == 0 {
		t.Errorf("stopped maintenance exited 0 (success):\n%s", r.Output())
	}
}
