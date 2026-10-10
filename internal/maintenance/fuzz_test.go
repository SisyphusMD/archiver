package maintenance

import (
	"os"
	"path/filepath"
	"testing"
)

// The preferences file a repository keeps (ADR 39): read for which storage it registers,
// whatever it holds.
func FuzzSameStorage(f *testing.F) {
	f.Add(`[{"name":"local","id":"x","storage":"/b","encrypted":true}]`, "local", "/b")
	f.Add(`{}`, "", "")
	f.Add(`[{"name":null,"storage":7}]`, "a", "b")
	f.Fuzz(func(t *testing.T, prefs, storage, url string) {
		p := filepath.Join(t.TempDir(), "preferences")
		if err := os.WriteFile(p, []byte(prefs), 0o600); err != nil {
			t.Fatal(err)
		}
		sameStorage(p, storage, url)
	})
}
