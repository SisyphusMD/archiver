package resume

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestRecordLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", ".run-backup.json")
	if _, ok := Read(path); ok {
		t.Fatal("a record before any run")
	}
	r, err := Begin(path, Record{Services: map[string]string{"/a": Pending, "/b": Pending}})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, d := range []string{"/a", "/b"} {
		wg.Add(1)
		go func() { defer wg.Done(); _ = r.Set(d, Hooked); _ = r.Set(d, Done) }()
	}
	wg.Wait()
	_ = r.Set("/c", Hooked)
	got, ok := Read(path)
	if !ok || got.Started == 0 || got.Unfinished("/a") || !got.Unfinished("/c") || got.Unfinished("/elsewhere") {
		t.Fatalf("record %+v", got)
	}
	r.End()
	if _, ok := Read(path); ok {
		t.Fatal("the record outlived End")
	}
	var none *Run
	_ = none.Set("/a", Done) // a run without a record (it could not be written) is no error
	none.End()
}
