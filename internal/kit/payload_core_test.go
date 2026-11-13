package kit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The core fingerprint ignores extra files; the full one, which existing state records, does not.
func TestCoreFingerprintIgnoresExtras(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("archiver.env", "A=1\n")
	write("extra/bundle", "v1")
	full1, core1, err := fingerprintsContext(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if only, _ := fingerprintContext(context.Background(), dir); only != full1 {
		t.Fatal("the full fingerprint changed meaning")
	}
	write("extra/bundle", "v2")
	full2, core2, _ := fingerprintsContext(context.Background(), dir)
	if full2 == full1 || core2 != core1 {
		t.Fatalf("an extra file changed: full %v, core %v", full2 != full1, core2 != core1)
	}
	write("archiver.env", "A=2\n")
	_, core3, _ := fingerprintsContext(context.Background(), dir)
	if core3 == core2 {
		t.Fatal("a changed setting left the core fingerprint as it was")
	}
}

func coreOn(t *testing.T, f *fixture) []string {
	t.Helper()
	f.run.Environ = f.environ()
	_, same, _, err := f.run.Placement()
	if err != nil {
		t.Fatal(err)
	}
	return same
}

// A storage left alone (here, skipped) keeps its place on the core record when only extras
// changed, and a run with nothing to upload still writes the record.
func TestCoreRecordKeepsUntouchedStorages(t *testing.T) {
	f := newFixture(t, 2)
	extra := filepath.Join(t.TempDir(), "bundle")
	os.WriteFile(extra, []byte("v1"), 0o600)
	f.vars["RECOVERY_KIT_EXTRA_PATHS"] = extra
	if code := f.execute(); code != OK {
		t.Fatalf("first placement: %d", code)
	}
	os.Remove(f.run.Layout.KitCoreState())
	if code := f.execute(); code != OK {
		t.Fatalf("second run: %d", code)
	}
	if got := coreOn(t, f); len(got) != 2 {
		t.Fatalf("a run with nothing to upload left the core record at %v", got)
	}
	os.WriteFile(extra, []byte("v2"), 0o600)
	f.run.Skip = []string{"store2"}
	if code := f.execute(); code != OK {
		t.Fatalf("partial refresh: %d", code)
	}
	f.run.Skip = nil
	current, same, _, _ := f.run.Placement()
	if len(current) != 1 || len(same) != 2 {
		t.Fatalf("after a refresh that skipped store2: current %v, same core %v", current, same)
	}
}
