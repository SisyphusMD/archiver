package entrypoint

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/layout"
)

func env(t *testing.T, vars map[string]string) *Env {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"secrets", "lock", "app"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	return &Env{Layout: layout.Layout{Root: filepath.Join(root, "app"), Lock: filepath.Join(root, "lock")},
		Getenv: func(k string) string { return vars[k] }, SecretsDir: filepath.Join(root, "secrets")}
}

func TestRefuseBundle(t *testing.T) {
	e := env(t, map[string]string{})
	if f := e.RefuseBundle(); f != "" {
		t.Fatalf("nothing bundle-era, but %q", f)
	}
	os.WriteFile(filepath.Join(e.SecretsDir, "bundle_password"), nil, 0o600)
	if f := e.RefuseBundle(); f != filepath.Join(e.SecretsDir, "bundle_password") {
		t.Fatalf("got %q", f)
	}
	e = env(t, map[string]string{"BUNDLE_PASSWORD": "x"})
	if f := e.RefuseBundle(); f != "BUNDLE_PASSWORD in the environment" {
		t.Fatalf("got %q", f)
	}
}

func TestPlaceKeys(t *testing.T) {
	e := env(t, map[string]string{})
	if err := e.PlaceKeys(); err == nil || !strings.Contains(err.Error(), "no RSA key files found") {
		t.Fatalf("no keys: %v", err)
	}
	os.WriteFile(filepath.Join(e.SecretsDir, "rsa_private_key"), []byte("PRIV"), 0o644)
	alt := filepath.Join(t.TempDir(), "pub")
	os.WriteFile(alt, []byte("PUB"), 0o600)
	e.Getenv = func(k string) string {
		if k == "RSA_PUBLIC_KEY_FILE" {
			return alt
		}
		return ""
	}
	if err := e.PlaceKeys(); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]os.FileMode{"private.pem": 0o600, "public.pem": 0o644} {
		fi, err := os.Stat(filepath.Join(e.Layout.Root, "keys", name))
		if err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s: %v %v, want mode %o", name, err, fi, want)
		}
	}
	if _, err := os.Stat(filepath.Join(e.Layout.Root, "keys", "id_ed25519")); err == nil {
		t.Error("an SSH key appeared from nowhere")
	}
}

func TestClearLocksAndLocksClear(t *testing.T) {
	e := env(t, map[string]string{})
	for _, n := range []string{"archiver-main.lock", "archiver-stop-requested", "archiver-maintenance.lock", "archiver-in-use"} {
		os.WriteFile(filepath.Join(e.Layout.Lock, n), nil, 0o600)
	}
	if e.LocksClear() {
		t.Fatal("locks held, but clear")
	}
	e.ClearLocks()
	if !e.LocksClear() {
		t.Fatal("locks left")
	}
	if _, err := os.Stat(filepath.Join(e.Layout.Lock, "archiver-in-use")); err != nil {
		t.Error("cleared more than the pipelines' lock state")
	}
}

func TestWarnings(t *testing.T) {
	if fatal, _ := env(t, map[string]string{"CRON_SCHEDULE": "x"}).Warnings(); !strings.Contains(fatal, "renamed to BACKUP_SCHEDULE") {
		t.Fatalf("CRON_SCHEDULE: %q", fatal)
	}
	_, w := env(t, map[string]string{"BACKUP_SCHEDULE": "x", "ROTATE_BACKUPS": "true"}).Warnings()
	if len(w) != 3 || !strings.Contains(w[0], "ROTATE_BACKUPS is deprecated") || !strings.Contains(w[1], "MAINTENANCE_SCHEDULE is not set") {
		t.Fatalf("%q", w)
	}
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for range 100 {
		if ok() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal(what)
}

func appendTo(p, s string) {
	f, _ := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	f.WriteString(s)
	f.Close()
}

// From the end once the file appears, then through a rotation and a truncation.
func TestFollow(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "archiver.log")
	var out syncBuf
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { Follow(p, "Archiver Logs", &out, stop); close(done) }()
	time.Sleep(300 * time.Millisecond)
	appendTo(p, "old line\n")
	eventually(t, "no banner", func() bool { return strings.Contains(out.String(), "--- Archiver Logs ---") })
	appendTo(p, "one\npart")
	eventually(t, "no first line", func() bool { return strings.Contains(out.String(), "one\n") })
	appendTo(p, "ial\n")
	eventually(t, "a split line was not joined", func() bool { return strings.Contains(out.String(), "partial\n") })
	os.Rename(p, p+".1")
	appendTo(p, "after rotation\n")
	eventually(t, "lost the rotation", func() bool { return strings.Contains(out.String(), "after rotation\n") })
	os.Truncate(p, 0)
	time.Sleep(400 * time.Millisecond)
	appendTo(p, "x\n")
	eventually(t, "lost the truncation", func() bool { return strings.HasSuffix(out.String(), "x\n") })
	close(stop)
	<-done
	if strings.Contains(out.String(), "old line") {
		t.Error("printed what was there before it followed")
	}
}

func TestLegacyServices(t *testing.T) {
	svcs := t.TempDir()
	for _, s := range []string{"app", "db"} {
		os.MkdirAll(filepath.Join(svcs, s), 0o755)
	}
	os.WriteFile(filepath.Join(svcs, "db", "service-backup-settings.sh"), nil, 0o644)
	src := config.Source{Getenv: func(k string) string {
		if k == "SERVICE_DIRECTORIES" {
			return svcs + "/*/"
		}
		return ""
	}, SecretsDir: t.TempDir()}
	got := LegacyServices(src)
	if len(got) != 1 || filepath.Base(got[0]) != "db" {
		t.Fatalf("got %v", got)
	}
	if !strings.Contains(LegacyHelp(got), "docker compose run --rm archiver migrate hooks") {
		t.Error("the help does not name the conversion")
	}
}
