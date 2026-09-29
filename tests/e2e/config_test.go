package e2e

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/SisyphusMD/archiver/tests/e2e/harness"
)

// TestConfigServiceDirectoryErrors: a configured service directory that is not backed up
// fails the run, whether the entry matches nothing or the directory's name cannot form a
// snapshot ID. The valid service alongside it is only the control; whether it still backs
// up is not part of the contract.
func TestConfigServiceDirectoryErrors(t *testing.T) {
	baseline, image := images(t)
	work := harness.WorkDir(t)
	keysDir := filepath.Join(work, "keys")
	if err := os.MkdirAll(keysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	keys := harness.NewKeys(t, baseline, keysDir)

	for _, c := range []struct {
		name      string
		bad       string // a second service directory, empty for the control
		unmatched bool   // configured but never mounted
		wantOK    bool
	}{
		{name: "control", wantOK: true},
		{name: "unmatched entry", bad: "missing", unmatched: true},
		{name: "space in snapshot ID", bad: "with space"},
		{name: "dot in snapshot ID", bad: "dot.name"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := harness.WorkDir(t)
			app := filepath.Join(dir, "svc", "app")
			store := filepath.Join(dir, "storage")
			for _, d := range []string{app, store} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(app, "f.txt"), []byte("app data\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			d := &harness.Deployment{
				Image:    image,
				Hostname: "e2e-host",
				Services: map[string]string{"app": app},
				Storages: []harness.Storage{{Name: "primary", Dir: store}},
				Keys:     keys,
			}
			switch {
			case c.unmatched:
				d.AddUnmatchedService(c.bad)
			case c.bad != "":
				bad := filepath.Join(dir, "svc", c.bad)
				if err := os.MkdirAll(bad, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(bad, "f.txt"), []byte("bad data\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				d.Services[c.bad] = bad
			}
			d.Start(t)

			r := d.Archiver(t, nil, "backup")
			if c.wantOK {
				if r.Code != 0 {
					t.Fatalf("backup of valid services exited %d:\n%s", r.Code, r.Output())
				}
				if got := harness.Revisions(t, store, d.SnapshotID("app")); !slices.Equal(got, []int{1}) {
					t.Fatalf("revisions of %s = %v, want [1]", d.SnapshotID("app"), got)
				}
				return
			}
			if r.Code == 0 {
				t.Fatalf("backup exited 0 although service directory %q was not backed up:\n%s", c.bad, r.Output())
			}
		})
	}
}

// TestConfigSecretsStayHidden: no secret ever appears on a process's argv while archiver
// backs up (primary, copy, recovery kit) and restores, and none is left in plaintext on
// disk afterwards: not in the service directories, the storages, the restore targets, or
// anything the container wrote. The recovery kit is encrypted, so it must not match either.
func TestConfigSecretsStayHidden(t *testing.T) {
	baseline, image := images(t)
	work := harness.WorkDir(t)
	svc := filepath.Join(work, "svc", "app")
	primary := filepath.Join(work, "storage-primary")
	copyDir := filepath.Join(work, "storage-copy")
	restores := filepath.Join(work, "restores")
	keysDir := filepath.Join(work, "keys")
	for _, d := range []string{svc, primary, copyDir, restores, keysDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	d := &harness.Deployment{
		Image:            image,
		Hostname:         "e2e-host",
		Services:         map[string]string{"app": svc},
		Storages:         []harness.Storage{{Name: "primary", Dir: primary}, {Name: "offsite", Dir: copyDir}},
		Keys:             harness.NewKeys(t, baseline, keysDir),
		RecoveryPassword: "e2e-recovery-password",
		RestoreRoot:      restores,
	}
	secrets := d.Secrets()
	d.Start(t)

	watch := harness.WatchArgv(t, d, secrets)
	writeFixtures(t, svc, 1)
	if r := d.Archiver(t, nil, "backup"); r.Code != 0 {
		t.Fatalf("first backup exited %d:\n%s", r.Code, r.Output())
	}
	writeFixtures(t, svc, 2)
	if r := d.Archiver(t, nil, "backup"); r.Code != 0 {
		t.Fatalf("second backup exited %d:\n%s", r.Code, r.Output())
	}
	for _, storage := range []string{"primary", "offsite"} {
		d.Restore(t, "app", 2, storage)
	}
	started, leaks := watch.Stop(t)
	if started == 0 {
		t.Fatal("the argv watch saw no process start during the backups and restores, so it proves nothing")
	}
	for _, l := range leaks {
		t.Errorf("secret on argv: %s", l)
	}

	for _, h := range harness.PlaintextSecrets(t, secrets, svc, primary, copyDir, restores) {
		t.Errorf("plaintext secret on disk: %s", h)
	}
	for _, h := range d.PlaintextSecretsInContainer(t, secrets) {
		t.Errorf("plaintext secret in the container: %s", h)
	}
}

// TestConfigMigrateBundle: migration turns a bundle, current or legacy (`openssl -k`,
// array SERVICE_DIRECTORIES), into env-native configuration that continues the same
// installation. The baseline release backs up from the bundle, the image under test
// migrates it, and the migrated deployment alone must add the next revision under the same
// snapshot ID to the primary and the copy and restore both revisions from each.
func TestConfigMigrateBundle(t *testing.T) {
	baseline, image := images(t)
	for _, c := range []struct {
		name   string
		legacy bool
	}{
		{"current bundle", false},
		{"legacy -k bundle", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			work := harness.WorkDir(t)
			svc := filepath.Join(work, "svc", "app")
			primary := filepath.Join(work, "storage-primary")
			copyDir := filepath.Join(work, "storage-copy")
			restores := filepath.Join(work, "restores")
			keysDir := filepath.Join(work, "keys")
			for _, d := range []string{svc, primary, copyDir, restores, keysDir} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			install := &harness.Deployment{
				Image:       baseline,
				Hostname:    "e2e-host",
				Services:    map[string]string{"app": svc},
				Storages:    []harness.Storage{{Name: "primary", Dir: primary}, {Name: "offsite", Dir: copyDir}},
				Keys:        harness.NewKeys(t, baseline, keysDir),
				RestoreRoot: restores,
			}
			bundle := harness.NewBundle(t, install, work, c.legacy)
			id := install.SnapshotID("app")

			writeFixtures(t, svc, 1)
			v1 := harness.Snapshot(t, svc)
			old := *install
			old.StartWithBundle(t, bundle)
			if r := old.Archiver(t, nil, "backup"); r.Code != 0 {
				t.Fatalf("baseline backup from the bundle exited %d:\n%s", r.Code, r.Output())
			}
			old.Stop(t)

			migrated := harness.MigrateBundle(t, image, bundle, install)
			writeFixtures(t, svc, 2)
			v2 := harness.Snapshot(t, svc)
			migrated.Start(t)
			if r := migrated.Archiver(t, nil, "backup"); r.Code != 0 {
				t.Fatalf("backup from the migrated configuration exited %d:\n%s", r.Code, r.Output())
			}
			for _, dir := range []string{primary, copyDir} {
				if got := harness.Revisions(t, dir, id); !slices.Equal(got, []int{1, 2}) {
					t.Fatalf("after the migrated backup, revisions of %s in %s = %v, want [1 2]", id, dir, got)
				}
			}
			for _, r := range []struct {
				storage string
				rev     int
				want    harness.Tree
			}{
				{"primary", 1, v1}, {"primary", 2, v2}, {"offsite", 1, v1}, {"offsite", 2, v2},
			} {
				got := harness.Snapshot(t, migrated.Restore(t, "app", r.rev, r.storage))
				if d := r.want.Diff(got); len(d) > 0 {
					t.Errorf("migrated restore of rev %d from %s differs: %v", r.rev, r.storage, d)
				}
			}
		})
	}
}
