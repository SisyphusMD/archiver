package inuse

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestHoldReadRelease(t *testing.T) {
	dir := t.TempDir()
	h, err := Hold(dir, "copy", []Revision{{"local", "nas-app", 3, false}, {"local", "nas-app", 1, false}, {"offsite", "nas-app", 9, false}, {"local", "nas-db", 2, false}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Read(dir, "local")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Has("nas-app", 1) || !got.Has("nas-app", 3) || got.Has("nas-app", 2) || !got.Has("nas-db", 2) || got.Has("nas-app", 9) {
		t.Fatalf("got %+v", got)
	}
	if err := h.Narrow([]Revision{{"local", "nas-db", 2, false}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := Read(dir, "local"); got.Has("nas-app", 1) || !got.Has("nas-db", 2) {
		t.Fatalf("after narrowing: %+v", got)
	}
	h.Release()
	if got, _ := Read(dir, "local"); !got.Empty() {
		t.Fatalf("after release: %+v", got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("left behind %v", entries)
	}
}

// A file left by a reader that died (nobody holds its lock) is ignored and removed.
func TestDeadReaderIgnored(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "restore-123"), []byte("local nas-app 4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := Read(dir, "local"); !got.Empty() {
		t.Fatalf("a dead reader's revisions counted: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "restore-123")); !os.IsNotExist(err) {
		t.Fatal("dead reader's file not removed")
	}
}

// A file still being written is invisible, so a prune can never see a partial set.
func TestUnfinishedFileIgnored(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".tmp-copy-1"), []byte("local nas-app 4\n"), 0o600)
	if got, _ := Read(dir, "local"); !got.Empty() {
		t.Fatalf("got %+v", got)
	}
}

// Another process's lock counts, as a bash restore holds it with flock(1).
func TestHeldByAnotherProcess(t *testing.T) {
	if _, err := exec.LookPath("flock"); err != nil {
		t.Skip("no flock(1)")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "restore-1")
	os.WriteFile(path, []byte("local nas-app 7\n"), 0o600)
	cmd := exec.Command("flock", path, "sleep", "5")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	time.Sleep(300 * time.Millisecond)
	if got, _ := Read(dir, "local"); !got.Has("nas-app", 7) || got.Has("nas-app", 6) {
		t.Fatalf("got %+v", got)
	}
}

func TestGateExcludesReaders(t *testing.T) {
	dir := t.TempDir()
	release, err := Gate(context.Background(), dir, "local", true)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := Gate(ctx, dir, "local", false); err == nil {
		t.Fatal("a reader passed a held prune gate")
	}
	other, err := Gate(context.Background(), dir, "offsite", false)
	if err != nil {
		t.Fatal("another storage's gate is independent")
	}
	other()
	release()
	r1, err := Gate(context.Background(), dir, "local", false)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Gate(context.Background(), dir, "local", false)
	if err != nil {
		t.Fatal("readers share the gate")
	}
	r1()
	r2()
}

// "And newer" covers revisions created after the registration; "*" covers IDs it does not
// name, and only those.
func TestNewerAndUnnamed(t *testing.T) {
	dir := t.TempDir()
	h, err := Hold(dir, "copy", []Revision{
		{"local", "nas-app", 4, false}, {"local", "nas-app", 7, true}, {"local", "*", 1, true},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Release()
	got, _ := Read(dir, "local")
	for _, c := range []struct {
		id   string
		rev  int
		want bool
	}{{"nas-app", 4, true}, {"nas-app", 5, false}, {"nas-app", 7, true}, {"nas-app", 30, true}, {"nas-new", 1, true}} {
		if got.Has(c.id, c.rev) != c.want {
			t.Errorf("Has(%s, %d) = %v", c.id, c.rev, !c.want)
		}
	}
}

// A registration replaced while being read is read in its new form, never skipped.
func TestReadDuringNarrow(t *testing.T) {
	dir := t.TempDir()
	h, err := Hold(dir, "copy", []Revision{{"local", "nas-app", 1, false}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Release()
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			h.Narrow([]Revision{{"local", "nas-app", 1, false}, {"local", "nas-app", 2 + i%2, false}})
		}
	}()
	defer func() { close(stop); <-stopped }()
	for range 500 {
		got, err := Read(dir, "local")
		if err != nil {
			t.Fatal(err)
		}
		if !got.Has("nas-app", 1) {
			t.Fatal("a registration being replaced was skipped")
		}
	}
}
