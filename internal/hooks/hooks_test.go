package hooks

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func TestEnvironHidesSecrets(t *testing.T) {
	base := []string{"PATH=/bin", "STORAGE_PASSWORD=x", "RSA_PASSPHRASE=y", "DUPLICACY_LOCAL_PASSWORD=z",
		"DUPLICACY_B2_KEY=k", "STORAGE_TARGET_2_S3_SECRET=s", "ARCHIVER_SERVICE=spoofed", "HOSTNAME=h"}
	env := Environ(base, Service{Name: "app", Dir: "/srv/app", SnapshotID: "h-app", StateDir: "/tmp/s"}, Failed)
	want := []string{"PATH=/bin", "HOSTNAME=h", "ARCHIVER_SERVICE=app", "ARCHIVER_SERVICE_DIR=/srv/app",
		"ARCHIVER_SNAPSHOT_ID=h-app", "ARCHIVER_STATE_DIR=/tmp/s", "ARCHIVER_BACKUP_RESULT=failed"}
	if !slices.Equal(env, want) {
		t.Fatalf("got %q\nwant %q", env, want)
	}
}

func TestExistsRequiresExecutable(t *testing.T) {
	dir := t.TempDir()
	if ok, err := Exists(dir, PreBackup); ok || err != nil {
		t.Fatalf("missing hook: %v %v", ok, err)
	}
	write(t, filepath.Join(dir, PreBackup), "#!/bin/sh\n", 0o644)
	if _, err := Exists(dir, PreBackup); err == nil || !strings.Contains(err.Error(), "chmod +x") {
		t.Fatalf("non-executable hook: %v", err)
	}
	os.Chmod(filepath.Join(dir, PreBackup), 0o755)
	if ok, err := Exists(dir, PreBackup); !ok || err != nil {
		t.Fatalf("executable hook: %v %v", ok, err)
	}
}

func TestReadFilters(t *testing.T) {
	dir := t.TempDir()
	if got, _ := ReadFilters(dir); !slices.Equal(got, []string{"+*"}) {
		t.Fatalf("default = %q", got)
	}
	write(t, filepath.Join(dir, Filters), "+a/\r\n\n-*\n", 0o644)
	if got, _ := ReadFilters(dir); !slices.Equal(got, []string{"+a/", "-*"}) {
		t.Fatalf("got %q", got)
	}
}

func TestBrokenSymlinksAreErrors(t *testing.T) {
	dir := t.TempDir()
	os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, Filters))
	os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, PreBackup))
	if f, err := ReadFilters(dir); err == nil {
		t.Fatalf("a broken filters symlink read as %q", f)
	}
	if ok, err := Exists(dir, PreBackup); err == nil {
		t.Fatalf("a broken pre-backup symlink read as present=%v", ok)
	}
}
