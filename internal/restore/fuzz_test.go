package restore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// archiver.env as a recovery reads it from a kit (ADR 39).
func FuzzReadEnvFile(f *testing.F) {
	f.Add("A=1\n# c\nB=\"two\"\n")
	f.Add("=\n\n==\nNOVALUE\n")
	f.Fuzz(func(t *testing.T, content string) {
		p := filepath.Join(t.TempDir(), "archiver.env")
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		env, err := readEnvFile(p)
		if err != nil {
			return
		}
		for k := range env {
			if k == "" || strings.ContainsAny(k, "\n=") {
				t.Fatalf("key %q from %q", k, content)
			}
		}
	})
}
