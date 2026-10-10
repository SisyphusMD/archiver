package web

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The page shows the log through its link into prior_logs, and nothing a link planted on
// the logs volume points to outside it; a FIFO or a directory in its place is not read.
func TestTailStaysInLogDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "logs")
	os.MkdirAll(filepath.Join(dir, "prior_logs"), 0o755)
	os.WriteFile(filepath.Join(dir, "prior_logs", "archiver_1.log"), []byte("line one\nline two\n"), 0o644)
	os.Symlink(filepath.Join("prior_logs", "archiver_1.log"), filepath.Join(dir, "archiver.log"))
	if got := tail(dir, "archiver.log", 10); strings.Join(got, "|") != "line one|line two" {
		t.Fatalf("got %q", got)
	}

	secret := filepath.Join(base, "rsa_passphrase")
	os.WriteFile(secret, []byte("hunter2\n"), 0o600)
	os.Symlink(secret, filepath.Join(dir, "maintenance.log"))
	os.MkdirAll(filepath.Join(base, "secrets"), 0o755)
	os.WriteFile(filepath.Join(base, "secrets", "x.log"), []byte("hunter2\n"), 0o600)
	os.Symlink(filepath.Join(base, "secrets"), filepath.Join(dir, "prior_logs", "swapped"))
	os.Symlink(filepath.Join("prior_logs", "swapped", "x.log"), filepath.Join(dir, "copies.log"))
	syscall.Mkfifo(filepath.Join(dir, "drill.log"), 0o644)
	os.MkdirAll(filepath.Join(dir, "dir.log"), 0o755)
	for _, name := range []string{"maintenance.log", "copies.log", "drill.log", "dir.log"} {
		began := time.Now()
		if got := tail(dir, name, 10); len(got) != 0 {
			t.Errorf("%s: read %q", name, got)
		}
		if time.Since(began) > 2*time.Second {
			t.Errorf("%s: blocked", name)
		}
	}
}
