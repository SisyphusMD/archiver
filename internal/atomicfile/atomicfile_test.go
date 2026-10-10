package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

// A link planted at the old fixed temporary name, or at any other, is never written
// through; the file is replaced whole with the mode asked for.
func TestWriteNeverFollowsPlantedLinks(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state.json")
	os.Symlink(victim, path+".tmp")
	os.Symlink(victim, filepath.Join(dir, ".state.json.planted.tmp"))
	for _, content := range []string{"one", "two"} {
		if err := Write(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(path); string(b) != content {
			t.Fatalf("got %q", b)
		}
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Fatalf("a planted link was written through: %q", b)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 4 { // victim, state.json and the two planted links: no temporary left
		t.Fatalf("left %d entries", len(entries))
	}
}
