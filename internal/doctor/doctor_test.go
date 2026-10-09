package doctor

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/restore"
)

type fixture struct {
	env  Env
	vars map[string]string
	root string
	out  strings.Builder
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{root: root, vars: map[string]string{
		"SERVICE_DIRECTORIES":         filepath.Join(root, "services") + "/*/",
		"STORAGE_TARGET_1_NAME":       "local",
		"STORAGE_TARGET_1_TYPE":       "local",
		"STORAGE_TARGET_1_LOCAL_PATH": filepath.Join(root, "store"),
	}}
	secrets := filepath.Join(root, "secrets")
	keys := filepath.Join(root, "keys")
	for _, d := range []string{secrets, keys, filepath.Join(root, "logs"), filepath.Join(root, "services", "app")} {
		os.MkdirAll(d, 0o755)
	}
	os.Chmod(filepath.Join(root, "services"), 0o755)
	os.Chmod(filepath.Join(root, "services", "app"), 0o755)
	os.WriteFile(filepath.Join(secrets, "storage_password"), []byte("password1"), 0o600)
	os.WriteFile(filepath.Join(secrets, "rsa_passphrase"), []byte("rsa-pass"), 0o600)
	priv := filepath.Join(keys, "private.pem")
	if out, err := exec.Command("openssl", "genrsa", "-aes256", "-passout", "pass:rsa-pass", "-out", priv, "-traditional", "2048").CombinedOutput(); err != nil {
		t.Skipf("openssl genrsa: %v %s", err, out)
	}
	exec.Command("openssl", "rsa", "-in", priv, "-passin", "pass:rsa-pass", "-pubout", "-out", filepath.Join(keys, "public.pem")).Run()
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.Local)
	f.env = Env{
		Layout:    layout.Layout{Root: root, Lock: filepath.Join(root, "lock")},
		Source:    config.Source{Getenv: func(k string) string { return f.vars[k] }, SecretsDir: secrets},
		Hostname:  "h",
		Out:       &f.out,
		CapEff:    0b1011,
		Now:       func() time.Time { return now },
		HasConfig: func(config.Target) error { return nil },
		Probe: func(config.Target) (map[string]restore.SnapshotInfo, error) {
			return map[string]restore.SnapshotInfo{"h-app": {Revision: 7, Created: now.Add(-3 * time.Hour)}}, nil
		},
	}
	return f
}

// A healthy deployment passes, warning only about what it lacks (no notifier, no kit).
func TestDoctorHealthy(t *testing.T) {
	f := newFixture(t)
	if code := Run(f.env); code != 0 {
		t.Fatalf("exit %d:\n%s", code, f.out.String())
	}
	out := f.out.String()
	for _, want := range []string{"RSA key pair decrypts", "Storage 'local' (local, primary) is reachable", "h-app: newest revision 7", "No notification destination", "No recovery_password"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// A wrong passphrase, an unreadable storage, missing capabilities, an unsafe hook and a
// stale backup are each reported, and any failure fails the run.
func TestDoctorFindsProblems(t *testing.T) {
	f := newFixture(t)
	os.WriteFile(filepath.Join(f.root, "secrets", "rsa_passphrase"), []byte("wrong"), 0o600)
	f.env.HasConfig = func(config.Target) error { return errors.New("no Duplicacy storage at /x (its config is missing)") }
	f.env.CapEff = 0
	hook := filepath.Join(f.root, "services", "app", "pre-backup")
	os.WriteFile(hook, []byte("#!/bin/sh\n"), 0o755)
	os.Chmod(hook, 0o777)
	if code := Run(f.env); code != 1 {
		t.Fatalf("exit %d, want 1:\n%s", code, f.out.String())
	}
	out := f.out.String()
	for _, want := range []string{"does not decrypt with rsa_passphrase", "cannot be read: no Duplicacy storage", "DAC_OVERRIDE is not granted", "chmod go-w"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	g := newFixture(t)
	g.vars["BACKUP_SCHEDULE"] = "0 3 * * *"
	g.env.Probe = func(config.Target) (map[string]restore.SnapshotInfo, error) {
		return map[string]restore.SnapshotInfo{"h-app": {Revision: 2, Created: g.env.Now().Add(-5 * 24 * time.Hour)}}, nil
	}
	Run(g.env)
	if !strings.Contains(g.out.String(), "more than twice BACKUP_SCHEDULE's interval") {
		t.Errorf("a stale backup was not flagged:\n%s", g.out.String())
	}
}

func TestUnderMount(t *testing.T) {
	info := "22 1 0:21 / / rw - overlay overlay rw\n30 22 8:1 /srv/archiver /opt/archiver rw - ext4 /dev/sda1 rw\n31 22 8:1 /x /data/my\\040dir rw - ext4 /dev/sda1 rw\n"
	for dir, want := range map[string]bool{"/opt/archiver/logs": true, "/opt/archiverx": false, "/var/log": false, "/data/my dir/logs": true} {
		if got := underMount(dir, info); got != want {
			t.Errorf("%s: %v, want %v", dir, got, want)
		}
	}
}
