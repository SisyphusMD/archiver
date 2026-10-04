package e2e

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/SisyphusMD/archiver/tests/e2e/harness"
)

// The last bash release, by digest, so the data under test is what deployed
// installations actually hold.
const defaultBaseline = "ghcr.io/sisyphusmd/archiver:0.11.0@sha256:44cdd90b37be891a60c1543e347ba4e47e0aeba131c17683a9857aa6604058b2"

// The 0.11 release that converts a bundle in one `docker run` (`run migrate`), by digest:
// what v1 tells a bundle deployment to run (ADR 22).
const defaultConverter = "ghcr.io/sisyphusmd/archiver:0.11.4@sha256:5eb55b3b2852f671d72cdc41d0378ceac0e37237d72aa458eeeeaa599ae78a62"

func converterImage() string {
	if c := os.Getenv("ARCHIVER_CONVERTER_IMAGE"); c != "" {
		return c
	}
	return defaultConverter
}

func images(t *testing.T) (baseline, current string) {
	t.Helper()
	baseline = os.Getenv("ARCHIVER_BASELINE_IMAGE")
	if baseline == "" {
		baseline = defaultBaseline
	}
	current = os.Getenv("ARCHIVER_IMAGE")
	if current == "" {
		t.Fatal("ARCHIVER_IMAGE is unset")
	}
	return baseline, current
}

// TestCrossVersionUpgrade is the never-break list (ADR 1) as one upgrade: an installation
// written by the baseline release is taken over by the image under test, which must keep
// the same snapshot ID, extend the same storages (primary and bit-identical copy), restore
// what the baseline wrote, and leave data the baseline can still restore, so a rollback
// works. Both kits must open with stock openssl and carry the keys.
func TestCrossVersionUpgrade(t *testing.T) {
	baseline, current := images(t)
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

	keys := harness.NewKeys(t, baseline, keysDir)
	install := func(image string) *harness.Deployment {
		return &harness.Deployment{
			Image:            image,
			Hostname:         "e2e-host",
			Services:         map[string]string{"app": svc},
			Storages:         []harness.Storage{{Name: "primary", Dir: primary}, {Name: "offsite", Dir: copyDir}},
			Keys:             keys,
			RecoveryPassword: "e2e-recovery-password",
			RestoreRoot:      restores,
		}
	}

	writeFixtures(t, svc, 1)
	v1 := harness.Snapshot(t, svc)

	old := install(baseline)
	old.Start(t)
	if r := old.Archiver(t, nil, "backup"); r.Code != 0 {
		t.Fatalf("baseline backup exited %d:\n%s", r.Code, r.Output())
	}
	old.Stop(t)
	checkStorages(t, "baseline", []int{1}, primary, copyDir, keys)

	writeFixtures(t, svc, 2)
	v2 := harness.Snapshot(t, svc)

	cur := install(current)
	cur.Start(t)
	if r := cur.Archiver(t, nil, "backup"); r.Code != 0 {
		t.Fatalf("backup by %s over baseline data exited %d:\n%s", current, r.Code, r.Output())
	}
	checkStorages(t, "current", []int{1, 2}, primary, copyDir, keys)

	for _, c := range []struct {
		storage string
		rev     int
		want    harness.Tree
	}{
		{"primary", 1, v1}, {"primary", 2, v2}, {"offsite", 1, v1}, {"offsite", 2, v2},
	} {
		got := harness.Snapshot(t, cur.Restore(t, "app", c.rev, c.storage))
		if d := c.want.Diff(got); len(d) > 0 {
			t.Errorf("current restore of rev %d from %s differs: %v", c.rev, c.storage, d)
		}
	}
	cur.Stop(t)

	back := install(baseline)
	back.Start(t)
	for _, storage := range []string{"primary", "offsite"} {
		got := harness.Snapshot(t, back.Restore(t, "app", 2, storage))
		if d := v2.Diff(got); len(d) > 0 {
			t.Errorf("baseline restore of the upgraded revision from %s differs: %v", storage, d)
		}
	}
}

// checkStorages asserts the state both storages must be in after a backup by who: the
// same snapshot ID holding exactly wantRevs, a copy with exactly the primary's chunks and
// snapshot files (what -bit-identical promises), and a recovery kit on each.
func checkStorages(t *testing.T, who string, wantRevs []int, primary, copyDir string, k harness.Keys) {
	t.Helper()
	const id = "e2e-host-app"
	for _, dir := range []string{primary, copyDir} {
		if got := harness.Revisions(t, dir, id); !slices.Equal(got, wantRevs) {
			t.Fatalf("after the %s backup, revisions of %s in %s = %v, want %v", who, id, dir, got, wantRevs)
		}
		checkKit(t, dir, k, who)
	}
	for _, sub := range []string{"chunks", "snapshots"} {
		p, c := harness.ObjectNames(t, primary, sub), harness.ObjectNames(t, copyDir, sub)
		if len(p) == 0 || !slices.Equal(p, c) {
			t.Errorf("after the %s backup, the copy's %s are not bit-identical to the primary's:\nprimary %v\ncopy    %v", who, sub, p, c)
		}
	}
}

func checkKit(t *testing.T, storage string, k harness.Keys, who string) {
	t.Helper()
	dir := harness.OpenKit(t, harness.KitPath(storage, "e2e-host"), "e2e-recovery-password")
	priv, err := os.ReadFile(k.PrivatePath)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{
		"RSA private key":  priv,
		"RSA passphrase":   []byte(k.Passphrase),
		"storage password": []byte(harness.StoragePassword),
	} {
		if !harness.ContainsFile(t, dir, content) {
			t.Errorf("%s kit lacks the %s", who, name)
		}
	}
}

// writeFixtures lays down generation gen of the service data: varied owners and modes, a
// symlink, and in generation 2 an edited, an added, and a deleted file, so a restore of
// the wrong revision cannot pass.
func writeFixtures(t *testing.T, dir string, gen int) {
	t.Helper()
	write := func(rel, content string, mode os.FileMode, uid, gid int) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Lchown(p, uid, gid); err != nil {
			t.Fatal(err)
		}
	}
	switch gen {
	case 1:
		write("root.txt", "root-owned v1\n", 0o644, 0, 0)
		write("sub/u1000.txt", "uid 1000 v1\n", 0o640, 1000, 1000)
		write("sub/u5000.bin", string(make([]byte, 8192)), 0o600, 5000, 5000)
		write("doomed.txt", "deleted in v2\n", 0o644, 0, 0)
		if err := os.Symlink("root.txt", filepath.Join(dir, "link")); err != nil {
			t.Fatal(err)
		}
	case 2:
		write("sub/u1000.txt", "uid 1000 v2, edited\n", 0o640, 1000, 1000)
		write("added.txt", "new in v2\n", 0o604, 1234, 1234)
		if err := os.Remove(filepath.Join(dir, "doomed.txt")); err != nil {
			t.Fatal(err)
		}
	}
}
