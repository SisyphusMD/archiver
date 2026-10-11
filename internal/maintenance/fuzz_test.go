package maintenance

import (
	"testing"
)

// The preferences file a repository keeps (ADR 39): read for which storage it registers,
// whatever it holds.
func FuzzSameStorage(f *testing.F) {
	f.Add(`[{"name":"local","id":"x","storage":"/b","encrypted":true}]`, "local", "/b")
	f.Add(`{}`, "", "")
	f.Add(`[{"name":null,"storage":7}]`, "a", "b")
	f.Fuzz(func(_ *testing.T, prefs, storage, url string) {
		sameStorage([]byte(prefs), storage, url)
	})
}
