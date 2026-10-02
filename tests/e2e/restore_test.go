package e2e

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/tests/e2e/harness"
)

// installation is one host's service, storages, and keys, which deployments with any
// image or capability set can then run.
type installation struct {
	svc      string
	storages []harness.Storage
	keys     harness.Keys
	restores string
	image    string
}

func newInstallation(t *testing.T, storageNames ...string) *installation {
	t.Helper()
	image := os.Getenv("ARCHIVER_IMAGE")
	if image == "" {
		t.Fatal("ARCHIVER_IMAGE is unset")
	}
	work := harness.WorkDir(t)
	in := &installation{svc: filepath.Join(work, "svc", "app"), restores: filepath.Join(work, "restores"), image: image}
	keysDir := filepath.Join(work, "keys")
	dirs := []string{in.svc, in.restores, keysDir}
	for _, name := range storageNames {
		s := harness.Storage{Name: name, Dir: filepath.Join(work, "storage-"+name)}
		in.storages = append(in.storages, s)
		dirs = append(dirs, s.Dir)
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	in.keys = harness.NewKeys(t, image, keysDir)
	return in
}

// deploy starts this installation with exactly caps on top of --cap-drop ALL.
func (in *installation) deploy(t *testing.T, caps []string) *harness.Deployment {
	t.Helper()
	d := &harness.Deployment{
		Image:       in.image,
		Hostname:    "e2e-host",
		Services:    map[string]string{"app": in.svc},
		Storages:    in.storages,
		Keys:        in.keys,
		RestoreRoot: in.restores,
		Caps:        caps,
		// The kit is part of a full backup, so it too must work within the caps under test.
		RecoveryPassword: "e2e-recovery-password",
	}
	d.Start(t)
	return d
}

// writeTimedFixtures lays down the owner and mode fixtures with a distinct, whole-second
// mtime per file, far enough in the past that a restore stamping "now" cannot match.
func writeTimedFixtures(t *testing.T, dir string) {
	t.Helper()
	writeFixtures(t, dir, 1)
	base := time.Date(2020, 3, 4, 5, 6, 7, 0, time.UTC)
	for i, rel := range []string{"root.txt", "sub/u1000.txt", "sub/u5000.bin", "doomed.txt"} {
		mt := base.Add(time.Duration(i) * 36 * time.Hour)
		if err := os.Chtimes(filepath.Join(dir, rel), mt, mt); err != nil {
			t.Fatal(err)
		}
	}
}

func mustBackup(t *testing.T, d *harness.Deployment) {
	t.Helper()
	if r := d.Backup(t); r.Code != 0 {
		t.Fatalf("backup with caps %v exited %d:\n%s", d.Caps, r.Code, r.Output())
	}
}

// TestRestoreWithoutOwnershipCaps: a disaster-recovery restore is never refused for want of
// CHOWN or FOWNER. It must succeed and deliver every path's content; only owners may be lost.
func TestRestoreWithoutOwnershipCaps(t *testing.T) {
	in := newInstallation(t, "primary")
	writeTimedFixtures(t, in.svc)
	want := harness.Snapshot(t, in.svc).Content()
	b := in.deploy(t, harness.MinimalCaps)
	mustBackup(t, b)
	b.Stop(t)

	for _, c := range []struct {
		name string
		caps []string
	}{
		{"no capabilities", []string{}},
		{"DAC_OVERRIDE only", []string{"DAC_OVERRIDE"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := in.deploy(t, c.caps)
			got := harness.Snapshot(t, d.Restore(t, "app", 1, "primary")).Content()
			if diff := want.Diff(got); len(diff) > 0 {
				t.Errorf("restore with caps %v lost content: %v", c.caps, diff)
			}
		})
	}
}

// TestRestorePreservesOwnerModeAndTimes: with CHOWN and FOWNER granted, a restore recreates
// every path's owner, mode, and content, and every file's mtime.
func TestRestorePreservesOwnerModeAndTimes(t *testing.T) {
	in := newInstallation(t, "primary")
	writeTimedFixtures(t, in.svc)
	want, wantTimes := harness.Snapshot(t, in.svc), harness.ModTimes(t, in.svc)
	d := in.deploy(t, harness.MinimalCaps)
	mustBackup(t, d)

	dir := d.Restore(t, "app", 1, "primary")
	if diff := want.Diff(harness.Snapshot(t, dir)); len(diff) > 0 {
		t.Errorf("restore differs in owner, mode, or content: %v", diff)
	}
	if got := harness.ModTimes(t, dir); !maps.Equal(wantTimes, got) {
		t.Errorf("restored mtimes differ:\nwant %v\ngot  %v", wantTimes, got)
	}
}

// TestRestoreFiltersSelectExactFiles: a service's include/exclude patterns decide exactly
// which files a backup holds, nothing extra and nothing missing. Each case is its own
// service so one backup judges both an exclude-list and an include-list.
func TestRestoreFiltersSelectExactFiles(t *testing.T) {
	cases := []struct {
		service  string
		patterns []string
		files    []string // every file the service holds
		want     []string // exactly the ones the patterns select
	}{
		{
			service:  "excludes",
			patterns: []string{"-*.log", "-cache/", "+*"},
			files:    []string{"a.txt", "b.log", "sub/c.txt", "sub/d.log", "cache/e.txt", "sub/cache/f.txt"},
			want:     []string{"a.txt", "sub/c.txt", "sub/cache/f.txt"},
		},
		{
			service:  "includes",
			patterns: []string{"+keep/", "+keep/*", "+top.txt", "-*"},
			files:    []string{"top.txt", "other.txt", "keep/x.txt", "keep/y.bin", "drop/z.txt", "sub/top.txt"},
			want:     []string{"top.txt", "keep/x.txt", "keep/y.bin"},
		},
	}

	image := os.Getenv("ARCHIVER_IMAGE")
	if image == "" {
		t.Fatal("ARCHIVER_IMAGE is unset")
	}
	work := harness.WorkDir(t)
	primary, restores, keysDir := filepath.Join(work, "storage"), filepath.Join(work, "restores"), filepath.Join(work, "keys")
	services := map[string]string{}
	wants := map[string]harness.Tree{}
	for _, d := range []string{primary, restores, keysDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range cases {
		dir := filepath.Join(work, "svc", c.service)
		services[c.service] = dir
		for _, rel := range c.files {
			p := filepath.Join(dir, rel)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(c.service+":"+rel+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		harness.SetFilters(t, image, dir, c.patterns)
		all := harness.Snapshot(t, dir)
		want := harness.Tree{}
		for _, rel := range c.want {
			want[rel] = all[rel]
		}
		wants[c.service] = want
	}

	d := &harness.Deployment{
		Image:       image,
		Hostname:    "e2e-host",
		Services:    services,
		Storages:    []harness.Storage{{Name: "primary", Dir: primary}},
		Keys:        harness.NewKeys(t, image, keysDir),
		RestoreRoot: restores,
	}
	d.Start(t)
	mustBackup(t, d)

	for _, c := range cases {
		t.Run(c.service, func(t *testing.T) {
			got := harness.Snapshot(t, d.Restore(t, c.service, 1, "primary")).Files()
			if diff := wants[c.service].Diff(got); len(diff) > 0 {
				t.Errorf("patterns %q backed up the wrong files: %v", c.patterns, diff)
			}
		})
	}
}

// TestRestoreFullCycleWithMinimalCaps: DAC_OVERRIDE, CHOWN, and FOWNER are all archiver
// ever needs. Backup, copy, maintenance, and restore from either storage all succeed with
// exactly those, and the restores are faithful.
func TestRestoreFullCycleWithMinimalCaps(t *testing.T) {
	in := newInstallation(t, "primary", "offsite")
	writeTimedFixtures(t, in.svc)
	want := harness.Snapshot(t, in.svc)
	d := &harness.Deployment{
		Image:            in.image,
		Hostname:         "e2e-host",
		Services:         map[string]string{"app": in.svc},
		Storages:         in.storages,
		Keys:             in.keys,
		RestoreRoot:      in.restores,
		Caps:             harness.MinimalCaps,
		RecoveryPassword: "e2e-recovery-password",
	}
	d.SetMaintenance(harness.MaintenanceConfig{Check: true, Prune: true, Exhaustive: harness.ExhaustiveDaily})
	d.Start(t)

	mustBackup(t, d)
	for _, s := range in.storages {
		if got := harness.Revisions(t, s.Dir, d.SnapshotID("app")); !slices.Equal(got, []int{1}) {
			t.Fatalf("after backup, revisions in %s = %v, want [1]", s.Name, got)
		}
		checkKit(t, s.Dir, in.keys, "minimal-caps")
	}
	if r := d.Maintenance(t); r.Code != 0 {
		t.Fatalf("maintenance with caps %v exited %d:\n%s", d.Caps, r.Code, r.Output())
	}
	for _, s := range in.storages {
		if diff := want.Diff(harness.Snapshot(t, d.Restore(t, "app", 1, s.Name))); len(diff) > 0 {
			t.Errorf("restore from %s differs: %v", s.Name, diff)
		}
	}
}

// TestRestoreBackupNeedsOnlyDACOverride: backing up, copies included, needs nothing but
// DAC_OVERRIDE; what it wrote restores faithfully.
func TestRestoreBackupNeedsOnlyDACOverride(t *testing.T) {
	in := newInstallation(t, "primary", "offsite")
	writeTimedFixtures(t, in.svc)
	want := harness.Snapshot(t, in.svc)

	b := in.deploy(t, []string{"DAC_OVERRIDE"})
	mustBackup(t, b)
	b.Stop(t)
	for _, s := range in.storages {
		if got := harness.Revisions(t, s.Dir, b.SnapshotID("app")); !slices.Equal(got, []int{1}) {
			t.Fatalf("after backup, revisions in %s = %v, want [1]", s.Name, got)
		}
		checkKit(t, s.Dir, in.keys, "DAC_OVERRIDE-only")
	}

	r := in.deploy(t, harness.MinimalCaps)
	for _, s := range in.storages {
		if diff := want.Diff(harness.Snapshot(t, r.Restore(t, "app", 1, s.Name))); len(diff) > 0 {
			t.Errorf("restore from %s of the DAC_OVERRIDE-only backup differs: %v", s.Name, diff)
		}
	}
}
