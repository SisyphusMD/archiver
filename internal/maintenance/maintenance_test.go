package maintenance

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/inuse"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/runlock"
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

// A service directory's owner controls its legacy repository, and the collections taken
// from it feed a root prune: none is taken through a link, from a directory someone else
// owns or can write, or from a file that is a link, has other names or is someone else's.
func TestAdoptFossilsRefusesForeignTrees(t *testing.T) {
	root := t.TempDir()
	dot := filepath.Join(root, "repo", ".duplicacy")
	prefs := []byte(`[{"name":"local","id":"x","storage":"/storage/1"}]`)
	secret := filepath.Join(root, "secret")
	if err := os.WriteFile(secret, []byte("private key"), 0o600); err != nil {
		t.Fatal(err)
	}
	// legacy makes a service directory with a valid legacy repository holding collection 1.
	legacy := func(name string) (dir, fossils string) {
		t.Helper()
		dir = filepath.Join(root, name)
		fossils = filepath.Join(dir, ".duplicacy", "cache", "local", "fossils")
		if err := os.MkdirAll(fossils, 0o755); err != nil {
			t.Fatal(err)
		}
		for p := fossils; p != dir; p = filepath.Dir(p) {
			os.Chmod(p, 0o755) // whatever the umask
		}
		os.WriteFile(filepath.Join(dir, ".duplicacy", "preferences"), prefs, 0o600)
		os.WriteFile(filepath.Join(fossils, "1"), []byte(name), 0o600)
		return dir, fossils
	}
	var dirs []string
	add := func(dir string) { dirs = append(dirs, dir) }

	// The whole .duplicacy a link to a tree that would otherwise qualify.
	_, realFossils := legacy("real")
	linked := filepath.Join(root, "linked-dot")
	os.MkdirAll(linked, 0o755)
	os.Symlink(filepath.Join(root, "real", ".duplicacy"), filepath.Join(linked, ".duplicacy"))
	add(linked)
	// The fossils directory a link.
	d, fossils := legacy("linked-fossils")
	os.RemoveAll(fossils)
	os.Symlink(realFossils, fossils)
	add(d)
	// A group-writable .duplicacy, and a group-writable cache directory.
	d, _ = legacy("writable-dot")
	os.Chmod(filepath.Join(d, ".duplicacy"), 0o775)
	add(d)
	d, _ = legacy("writable")
	os.Chmod(filepath.Join(d, ".duplicacy", "cache"), 0o775)
	add(d)
	// Collections that are a symlink and a hard link to a file elsewhere.
	d, fossils = legacy("links")
	os.Remove(filepath.Join(fossils, "1"))
	os.Symlink(secret, filepath.Join(fossils, "1"))
	if err := os.Link(secret, filepath.Join(fossils, "2")); err != nil {
		t.Fatal(err)
	}
	add(d)
	// A preferences file that is a link to one that would match.
	d, _ = legacy("linked-prefs")
	os.Remove(filepath.Join(d, ".duplicacy", "preferences"))
	os.Symlink(filepath.Join(root, "real", ".duplicacy", "preferences"), filepath.Join(d, ".duplicacy", "preferences"))
	add(d)
	if os.Geteuid() == 0 {
		// Another user's tree, and another user's collection in Archiver's own tree.
		d, _ = legacy("foreign")
		os.Chown(filepath.Join(d, ".duplicacy"), 1000, 1000)
		add(d)
		d, fossils = legacy("foreign-file")
		os.Chown(filepath.Join(fossils, "1"), 1000, 1000)
		add(d)
	}

	fds := func() int { e, _ := os.ReadDir("/proc/self/fd"); return len(e) }
	before := fds()
	moved, err := adoptFossils(dot, "local", "/storage/1", dirs)
	if err != nil || moved != 0 {
		t.Fatalf("moved %d, %v; want nothing taken", moved, err)
	}
	if after := fds(); after != before {
		t.Fatalf("%d file descriptors left open by refused repositories", after-before)
	}
	if b, err := os.ReadFile(secret); err != nil || string(b) != "private key" {
		t.Fatalf("the file a link pointed at was changed: %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(realFossils, "1")); err != nil {
		t.Fatalf("a collection was taken through a link: %v", err)
	}
	// The real tree itself still qualifies.
	if moved, err := adoptFossils(dot, "local", "/storage/1", []string{filepath.Join(root, "real")}); err != nil || moved != 1 {
		t.Fatalf("the real repository: moved %d, %v", moved, err)
	}
}

func TestSecondaryPruneLeavesRevisionsInUse(t *testing.T) {
	for _, all := range []bool{false, true} {
		t.Run(map[bool]string{false: "revision", true: "storage-wide"}[all], func(t *testing.T) {
			root := t.TempDir()
			l := layout.Layout{Root: root, Lock: filepath.Join(root, "locks")}
			for _, dir := range []string{l.Lock, l.LogDir()} {
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			lock, _, err := runlock.Acquire(l.MaintenanceLock(), filepath.Join(l.Lock, "stop"), "maintenance", "prune")
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Release()
			rev := inuse.Revision{Storage: "second", ID: "host-app", Rev: 1}
			if all {
				rev.ID, rev.Rev, rev.AndNewer = "*", 0, true
			}
			hold, err := inuse.Hold(l.InUseDir(), "deep-check", []inuse.Revision{rev})
			if err != nil {
				t.Fatal(err)
			}
			defer hold.Release()
			calls := filepath.Join(root, "calls")
			bin := filepath.Join(root, "duplicacy")
			script := `#!/bin/sh
printf '%s\n' "$*" >> "$CALLS"
case " $* " in
  *" -dry-run "*)
    echo 'Deleting snapshot host-app at revision 1'
    echo 'Deleting snapshot host-app at revision 2'
    echo 'Deleting snapshot host-other at revision 1'
    ;;
esac
`
			if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			r := &Run{Layout: l, Duplicacy: bin, ForceExhaustive: true,
				cfg:  &config.Config{Targets: []config.Target{{Name: "local"}, {Name: "second"}}, PruneBackups: true, PruneKeep: "-keep 0:1", Threads: "1"},
				lock: lock, log: &logging.Log{Dir: l.LogDir(), Basename: "maintenance", Stdout: io.Discard},
				repo: root, env: append(os.Environ(), "CALLS="+calls), Now: time.Now,
				Probe: func(config.Target) error { return nil },
			}
			if !r.storage(1) || r.log.Errors() != 0 {
				t.Fatal("secondary prune failed")
			}
			b, err := os.ReadFile(calls)
			if err != nil {
				t.Fatal(err)
			}
			want := "-no-script prune -all -dry-run -storage second -keep 0:1 -threads 1\n"
			if !all {
				want += "-no-script prune -id host-app -storage second -threads 1 -r 2\n"
				want += "-no-script prune -id host-other -storage second -threads 1 -r 1\n"
			}
			want += "-no-script prune -all -storage second -threads 1 -exhaustive\n"
			if string(b) != want {
				t.Fatalf("prune calls:\n%s\nwant:\n%s", b, want)
			}
			if strings.Contains(string(b), "-exclusive") || r.pruned {
				t.Fatal("secondary prune was exclusive or woke primary copy workers")
			}
			if s := readState(l.MaintenanceState()); s.get("second", fieldPrune) == 0 || s.get("second", fieldExhaustive) == 0 {
				t.Fatal("successful secondary prune not recorded")
			}
		})
	}
}
