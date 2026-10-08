package maintenance

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
)

// The record keeps each storage's other fields when one changes, and reads back what bash
// wrote ("<name> <check> <prune> <exhaustive>").
func TestState(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".maintenance-state")
	os.WriteFile(p, []byte("local 10 20 30\noffsite 1 2 3\n"), 0o600)
	s := readState(p)
	s.set("local", fieldPrune, 99)
	s.set("new", fieldCheck, 5)
	if err := s.write(p); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "local 10 99 30\nnew 5 0 0\noffsite 1 2 3\n" {
		t.Fatalf("got %q", b)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
}

func TestExhaustiveDue(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	day := int64(86400)
	r := &Run{cfg: &config.Config{PruneExhaustiveFrequency: "monthly"}, Now: func() time.Time { return now }}
	s := state{}
	if !r.exhaustiveDue(s, "local") {
		t.Error("never run: due")
	}
	s.set("local", fieldExhaustive, now.Unix()-29*day)
	if r.exhaustiveDue(s, "local") {
		t.Error("29 days after a monthly exhaustive: not due")
	}
	s.set("local", fieldExhaustive, now.Unix()-30*day+1800)
	if !r.exhaustiveDue(s, "local") {
		t.Error("within the hour of grace: due")
	}
	r.ForceExhaustive = true
	s.set("local", fieldExhaustive, now.Unix())
	if !r.exhaustiveDue(s, "local") {
		t.Error("forced: due")
	}
	r.ForceExhaustive = false
	r.cfg.PruneExhaustiveFrequency = "off"
	s.set("local", fieldExhaustive, 0)
	if r.exhaustiveDue(s, "local") {
		t.Error("off: never due")
	}
}

// Collections move from every service repository registering the same storage, renumbered
// after the repository's own; a repository that means another storage by the name keeps its.
func TestAdoptFossils(t *testing.T) {
	root := t.TempDir()
	dot := filepath.Join(root, "repo", ".duplicacy")
	write := func(path, content string) {
		t.Helper()
		os.MkdirAll(filepath.Dir(path), 0o700)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dot, "cache", "local", "fossils", "4"), "own")
	prefs := func(url string) string {
		return `[{"name":"local","id":"x","storage":"` + url + `"},{"name":"offsite","storage":"sftp://h/p"}]`
	}
	var dirs []string
	for _, svc := range []struct{ name, url string }{{"a", "/storage/1"}, {"b", "/storage/1"}, {"c", "/elsewhere"}} {
		d := filepath.Join(root, svc.name)
		dirs = append(dirs, d)
		write(filepath.Join(d, ".duplicacy", "preferences"), prefs(svc.url))
		write(filepath.Join(d, ".duplicacy", "cache", "local", "fossils", "1"), svc.name+"1")
		write(filepath.Join(d, ".duplicacy", "cache", "local", "fossils", "2"), svc.name+"2")
		write(filepath.Join(d, ".duplicacy", "cache", "local", "fossils", "notes"), "not a collection")
	}
	dirs = append(dirs, filepath.Join(root, "uninitialized"))

	moved, err := adoptFossils(dot, "local", "/storage/1", dirs)
	if err != nil || moved != 4 {
		t.Fatalf("moved %d, %v", moved, err)
	}
	for n, want := range map[string]string{"4": "own", "5": "a1", "6": "a2", "7": "b1", "8": "b2"} {
		if b, _ := os.ReadFile(filepath.Join(dot, "cache", "local", "fossils", n)); string(b) != want {
			t.Errorf("collection %s = %q, want %q", n, b, want)
		}
	}
	for _, svc := range []string{"a", "b"} {
		left, _ := os.ReadDir(filepath.Join(root, svc, ".duplicacy", "cache", "local", "fossils"))
		if len(left) != 1 || left[0].Name() != "notes" {
			t.Errorf("%s still holds %v", svc, left)
		}
	}
	if left, _ := os.ReadDir(filepath.Join(root, "c", ".duplicacy", "cache", "local", "fossils")); len(left) != 3 {
		t.Errorf("another storage's collections moved: c holds %d files", len(left))
	}
	if moved, err := adoptFossils(dot, "local", "/storage/1", dirs); moved != 0 || err != nil {
		t.Fatalf("second pass moved %d, %v", moved, err)
	}
}

func TestIdentitiesRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), identities)
	want := map[string]string{"local": "/storage/1", "offsite": "sftp://u@h:22/p q"}
	if err := writeIdentities(p, want); err != nil {
		t.Fatal(err)
	}
	got, err := readIdentities(p)
	if err != nil || len(got) != 2 || got["local"] != want["local"] || got["offsite"] != want["offsite"] {
		t.Fatalf("got %v, %v", got, err)
	}
	if m, err := readIdentities(filepath.Join(t.TempDir(), "absent")); err != nil || len(m) != 0 {
		t.Fatalf("absent file: %v, %v", m, err)
	}
	if _, err := readIdentities(t.TempDir()); err == nil {
		t.Fatal("an unreadable file read as empty")
	}
}
