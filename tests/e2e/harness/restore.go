package harness

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

const containerRestoreRoot = "/restore"

// Restore restores one revision of a service from the named storage into a fresh host
// directory and returns it. The deployment must have been given a RestoreRoot before Start.
// It drives the 0.11 `auto-restore` interface, which is incidental (ADR 1) and gets a
// second surface when v1 changes it.
func (d *Deployment) Restore(t testing.TB, service string, revision int, storage string) string {
	t.Helper()
	r, dir := d.TryRestore(t, service, revision, storage)
	if r.Code != 0 {
		t.Fatalf("restore %s rev %d from %s with %s exited %d:\n%s",
			d.SnapshotID(service), revision, storage, d.Image, r.Code, r.Output())
	}
	return dir
}

// TryRestore is Restore for a restore that may fail: it returns the command's result and
// the host directory it restored into, whatever happened.
func (d *Deployment) TryRestore(t testing.TB, service string, revision int, storage string) (Result, string) {
	t.Helper()
	if d.RestoreRoot == "" {
		t.Fatal("Restore needs Deployment.RestoreRoot set before Start")
	}
	sub := "r-" + randomSuffix()
	env := map[string]string{
		"SNAPSHOT_ID":    d.SnapshotID(service),
		"LOCAL_DIR":      containerRestoreRoot + "/" + sub,
		"REVISION":       strconv.Itoa(revision),
		"STORAGE_TARGET": storage,
	}
	return d.Archiver(t, env, "auto-restore"), filepath.Join(d.RestoreRoot, sub)
}

// KitPath is where a local storage holds the recovery kit of a host.
func KitPath(storageDir, hostname string) string {
	return filepath.Join(storageDir, "archiver-recovery-kit-"+hostname+".tar.enc")
}

// OpenKit decrypts a recovery kit exactly as the never-break rule promises a user can:
// stock `openssl enc -d -aes-256-cbc -pbkdf2` piped into tar, with only the password.
// It returns the directory the kit was unpacked into.
func OpenKit(t testing.TB, kit, password string) string {
	t.Helper()
	out := t.TempDir()
	pw := filepath.Join(t.TempDir(), "pw")
	if err := os.WriteFile(pw, []byte(password), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", `openssl enc -d -aes-256-cbc -pbkdf2 -in "$1" -pass file:"$2" | tar -xf - -C "$3"`,
		"sh", kit, pw, out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("stock openssl could not open %s: %v\n%s", kit, err, b)
	}
	return out
}

// ContainsFile reports whether any regular file under dir has exactly the given content.
// Kits are judged by what they carry, not by where they put it.
func ContainsFile(t testing.TB, dir string, content []byte) bool {
	t.Helper()
	found := false
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || found || !info.Mode().IsRegular() {
			return err
		}
		b, err := os.ReadFile(p)
		if err == nil && string(b) == string(content) {
			found = true
		}
		return err
	})
	return found
}
