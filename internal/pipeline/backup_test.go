package pipeline

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGroupServices(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"srv/app", "srv/web", "other/app", "srv/db"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	os.Symlink(filepath.Join(root, "srv/db"), filepath.Join(root, "srv/database"))
	p := func(rel string) string { return filepath.Join(root, rel) }
	dirs := []string{p("srv/app") + "/", p("srv/web"), p("other/app"), p("srv/db"), p("srv/database"), p("srv/app")}
	// One snapshot ID (app, three times) and one directory under two names (db) each go
	// one after another; web runs alongside.
	if got, want := groupServices(dirs, 2), [][]int{{0, 2, 5}, {1}, {3, 4}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("parallel: got %v, want %v", got, want)
	}
	// One at a time keeps the configured order exactly.
	if got, want := groupServices(dirs, 1), [][]int{{0, 1, 2, 3, 4, 5}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("serial: got %v, want %v", got, want)
	}
}

// An upload limit is shared by the services backing up at once.
func TestRateShared(t *testing.T) {
	for _, c := range []struct {
		share int
		limit string
		want  string
	}{{0, "1000", "1000"}, {1, "1000", "1000"}, {2, "1000", "500"}, {3, "1000", "333"}, {4, "2", "1"}} {
		if got := (&Backup{share: c.share}).rate(c.limit); got != c.want {
			t.Errorf("share %d of %s: %s, want %s", c.share, c.limit, got, c.want)
		}
	}
}
