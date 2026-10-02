package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
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
