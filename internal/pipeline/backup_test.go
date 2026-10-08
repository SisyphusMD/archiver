package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCarryOverStorage(t *testing.T) {
	dir := t.TempDir()
	prefs := filepath.Join(dir, "preferences")
	old := `[{"name":"local","storage":"/s"},{"name":"offsite","storage":"sftp://u@h:22//p","encrypted":true}]`
	os.WriteFile(prefs, []byte(`[{"name":"local","storage":"/s"}]`), 0o600)
	if err := carryOverStorage(prefs, []byte(old), "offsite", "sftp://u@h:22//p"); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	b, _ := os.ReadFile(prefs)
	if err := json.Unmarshal(b, &got); err != nil || len(got) != 2 || got[1]["name"] != "offsite" || got[1]["storage"] != "sftp://u@h:22//p" || got[1]["encrypted"] != true {
		t.Fatalf("preferences now %s (%v)", b, err)
	}
	if err := carryOverStorage(prefs, []byte(old), "offsite", "sftp://u@h:22//p"); err != nil {
		t.Fatalf("carrying over twice must be a no-op: %v", err)
	}
	if b2, _ := os.ReadFile(prefs); string(b2) != string(b) {
		t.Fatal("a second carry-over changed the file")
	}
	if err := carryOverStorage(prefs, []byte(old), "offsite2", "sftp://new//p"); err == nil {
		t.Fatal("a storage not registered before must be added, not carried over")
	}
	os.WriteFile(prefs, []byte(`[{"name":"local","storage":"/s"}]`), 0o600)
	if err := carryOverStorage(prefs, []byte(old), "offsite", "sftp://u@h:2222//p"); err == nil {
		t.Fatal("a changed URL must be added again, not carried over")
	}
	if err := carryOverStorage(prefs, nil, "other", "/x"); err == nil {
		t.Fatal("nothing to carry over must be an error")
	}
}

// Preferences written before credentials moved to environment variables held them in
// "keys"; a carried-over entry must not bring them back.
func TestCarryOverDropsStoredKeys(t *testing.T) {
	dir := t.TempDir()
	prefs := filepath.Join(dir, "preferences")
	os.WriteFile(prefs, []byte(`[{"name":"local","storage":"/s"}]`), 0o600)
	old := `[{"name":"offsite","storage":"/o","keys":{"offsite_password":"hunter2hunter2","b2_key":"K"}}]`
	if err := carryOverStorage(prefs, []byte(old), "offsite", "/o"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(prefs)
	if strings.Contains(string(b), "hunter2") || strings.Contains(string(b), `"K"`) {
		t.Fatalf("stored credentials carried over:\n%s", b)
	}
}

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
