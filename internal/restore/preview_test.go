package restore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseListing(t *testing.T) {
	h := strings.Repeat("ab", 32)
	listing := "Snapshot h-app revision 3 created at 2026-10-09 12:00 \nFiles: 3\n" +
		"     5 2026-10-09 12:00:01 " + h + " a.txt\n" +
		"123456 2026-10-09 12:00:02 " + h + " dir/with space.bin\n" +
		"     0 2026-10-09 12:00:03 " + strings.Repeat(" ", 64) + " empty\n" +
		"     1 2026-10-09 12:00:04 " + h + "  lead\n" +
		"Total size: 123461, file chunks: 1, metadata chunks: 3\n"
	got := parseListing(listing)
	if len(got) != 4 || got[0].path != "a.txt" || got[1].path != "dir/with space.bin" || got[1].size != 123456 || got[2].path != "empty" || got[3].path != " lead" {
		t.Fatalf("%+v", got)
	}
}

// A plan sorts the revision's files into added, replaced (or left, without overwrite) and
// unchanged by size and time, and lists what DELETE_EXTRA would remove, within RESTORE_PATHS.
func TestPlan(t *testing.T) {
	dest := t.TempDir()
	os.MkdirAll(filepath.Join(dest, "cfg"), 0o755)
	when := time.Date(2026, 10, 9, 12, 0, 1, 0, time.Local)
	os.WriteFile(filepath.Join(dest, "cfg", "same"), []byte("12345"), 0o644)
	later := when.Add(time.Second) // within duplicacy's one-second tolerance
	os.Chtimes(filepath.Join(dest, "cfg", "same"), later, later)
	os.WriteFile(filepath.Join(dest, "cfg", "emptied"), []byte("data"), 0o644)
	os.WriteFile(filepath.Join(dest, "cfg", "changed"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dest, "cfg", "extra"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dest, "outside"), []byte("x"), 0o644)
	files := []entry{
		{"cfg/same", 5, "2026-10-09 12:00:01"},
		{"cfg/changed", 9, "2026-10-09 12:00:01"},
		{"cfg/new", 3, "2026-10-09 12:00:01"},
		{"cfg/emptied", 0, "2026-10-09 12:00:01"},
		{"data/db", 7, "2026-10-09 12:00:01"},
	}
	p := plan(files, dest, Options{Overwrite: true, Delete: true, Paths: []string{"cfg"}})
	names := func(l []entry) string {
		var s []string
		for _, e := range l {
			s = append(s, e.path)
		}
		return strings.Join(s, ",")
	}
	if names(p.Same) != "cfg/same" || names(p.Replace) != "cfg/changed,cfg/emptied" || names(p.Add) != "cfg/new" {
		t.Fatalf("same %s replace %s add %s", names(p.Same), names(p.Replace), names(p.Add))
	}
	// With restore paths Duplicacy deletes nothing, so neither does the plan.
	if len(p.Delete) != 0 {
		t.Fatalf("deletions predicted with RESTORE_PATHS: %s", names(p.Delete))
	}
	if q := plan(files, dest, Options{Paths: []string{"cfg"}}); names(q.Conflict) != "cfg/changed" || names(q.Replace) != "cfg/emptied" || len(q.Delete) != 0 {
		t.Fatalf("without overwrite: conflict %s replace %s delete %s", names(q.Conflict), names(q.Replace), names(q.Delete))
	}
	os.Symlink("cfg/same", filepath.Join(dest, "link"))
	if q := plan(files, dest, Options{Overwrite: true, Delete: true}); names(q.Delete) != "cfg/extra,outside" {
		t.Fatalf("a full restore with DELETE_EXTRA: delete %s (links are left out)", names(q.Delete))
	}
}

// DRY_RUN shows the plan and restores nothing: the destination is not even created.
func TestAutoDryRun(t *testing.T) {
	f := newFixture(t)
	f.revs(1, "2")
	dest := filepath.Join(f.root, "never")
	f.vars["SNAPSHOT_ID"], f.vars["LOCAL_DIR"], f.vars["DRY_RUN"] = "h-app", dest, "1"
	if code := f.env.Auto(); code != OK {
		t.Fatalf("exit %d: %s %s", code, f.out.String(), f.err.String())
	}
	if !strings.Contains(f.out.String(), "Dry run: revision 2") || !strings.Contains(f.out.String(), "3 file(s) added") {
		t.Fatalf("output %s", f.out.String())
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("a dry run created the destination")
	}
}

// RESTORE_PATHS reaches duplicacy as include patterns after the flags.
func TestAutoRestorePaths(t *testing.T) {
	f := newFixture(t)
	f.revs(1, "2")
	f.vars["SNAPSHOT_ID"], f.vars["LOCAL_DIR"], f.vars["RESTORE_PATHS"] = "h-app", filepath.Join(f.root, "r"), "/config/, data/db"
	if code := f.env.Auto(); code != OK {
		t.Fatalf("exit %d: %s", code, f.err.String())
	}
	if got := f.read(t, "r/flags.txt"); !strings.HasSuffix(got, `-- i:^config(/.*)?$ i:^data/$ i:^data/db(/.*)?$`) {
		t.Fatalf("flags %q", got)
	}
}

// A RESTORE_PATHS that names no path is refused rather than restoring everything.
func TestAutoRestorePathsEmpty(t *testing.T) {
	f := newFixture(t)
	f.revs(1, "2")
	f.vars["SNAPSHOT_ID"], f.vars["LOCAL_DIR"], f.vars["RESTORE_PATHS"] = "h-app", filepath.Join(f.root, "r"), " , "
	if code := f.env.Auto(); code != Unreachable {
		t.Fatalf("exit %d, want %d", code, Unreachable)
	}
}
