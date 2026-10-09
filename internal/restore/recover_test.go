package restore

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A fresh host's service directories do not exist yet: a glob places each of the host's
// snapshots beside its siblings, a literal path places the one named like it, and other
// hosts' snapshots are left alone.
func TestPlaceServices(t *testing.T) {
	listing := map[string]SnapshotInfo{"nas-app": {}, "nas-db": {}, "nas-media": {}, "other-app": {}}
	plan, unplaced := placeServices(listing, "nas", []string{"/srv/*/", "/home/u/db"})
	var got []string
	for _, p := range plan {
		got = append(got, p.id+"="+p.dir)
	}
	// The literal /home/u/db is more specific than the glob, so db goes there.
	if strings.Join(got, " ") != "nas-app=/srv/app nas-db=/home/u/db nas-media=/srv/media" || len(unplaced) != 0 {
		t.Fatalf("plan %v unplaced %v", got, unplaced)
	}
	plan, unplaced = placeServices(listing, "nas", []string{"/home/u/db"})
	if len(plan) != 1 || plan[0].dir != "/home/u/db" || strings.Join(unplaced, " ") != "nas-app nas-media" {
		t.Fatalf("literal only: plan %v unplaced %v", plan, unplaced)
	}
}

func TestReadEnvAndHostname(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "archiver.env"), []byte("SERVICE_DIRECTORIES=/srv/*/:/x\nSTORAGE_TARGET_1_NAME=local\n# note\n"), 0o644)
	env, err := readEnvFile(filepath.Join(dir, "archiver.env"))
	if err != nil || env["SERVICE_DIRECTORIES"] != "/srv/*/:/x" || env["STORAGE_TARGET_1_NAME"] != "local" || len(env) != 2 {
		t.Fatalf("%v %v", env, err)
	}
	os.WriteFile(filepath.Join(dir, "RECREATE.txt"), []byte("Facts this deployment depended on:\n  - hostname: nas   (keep it)\n"), 0o644)
	if h := recordedHostname(filepath.Join(dir, "RECREATE.txt")); h != "nas" {
		t.Fatalf("hostname %q", h)
	}
}

// A kit opens with its password, the way its README opens it by hand, and not without.
func TestDecryptKit(t *testing.T) {
	src, out := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(src, "archiver.env"), []byte("X=1\n"), 0o644)
	kit := filepath.Join(t.TempDir(), "kit.tar.enc")
	cmd := exec.Command("sh", "-c", `tar -C "$1" -cf - archiver.env | openssl enc -aes-256-cbc -pbkdf2 -pass pass:the-password -out "$2"`, "sh", src, kit)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("openssl: %v %s", err, b)
	}
	if err := decryptKit(kit, "wrong-password", out); err == nil {
		t.Fatal("a wrong password opened the kit")
	}
	os.RemoveAll(out)
	os.MkdirAll(out, 0o700)
	if err := decryptKit(kit, "the-password", out); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "archiver.env")); string(b) != "X=1\n" {
		t.Fatalf("got %q", b)
	}
}

// Without the kit's list, a hyphenated name only a glob would place may be another host's
// and is left unplaced; a literal path that names it places it.
func TestPlaceServicesHyphenated(t *testing.T) {
	listing := map[string]SnapshotInfo{"nas-app": {}, "nas-home-app": {}, "nas-my-db": {}}
	plan, unplaced := placeServices(listing, "nas", []string{"/srv/*/", "/data/my-db"})
	var got []string
	for _, p := range plan {
		got = append(got, p.id+"="+p.dir)
	}
	if strings.Join(got, " ") != "nas-app=/srv/app nas-my-db=/data/my-db" || strings.Join(unplaced, " ") != "nas-home-app" {
		t.Fatalf("plan %v unplaced %v", got, unplaced)
	}
}

// With the kit's list, exactly the listed services are planned, other IDs are ignored, and a
// listed service with no snapshot is missing.
func TestPlaceRecorded(t *testing.T) {
	listing := map[string]SnapshotInfo{"nas-app": {}, "nas-home-app": {}}
	plan, unplaced, missing := placeRecorded([]string{"app", "db"}, listing, "nas", []string{"/srv/*/"})
	if len(plan) != 1 || plan[0].id != "nas-app" || len(unplaced) != 0 || strings.Join(missing, " ") != "db" {
		t.Fatalf("plan %v unplaced %v missing %v", plan, unplaced, missing)
	}
}

// A literal path is taken literally even when it holds glob characters, as backups take it.
func TestDirForLiteralWithGlobCharacters(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "tenant[1]"), 0o755)
	lit := filepath.Join(root, "tenant[1]", "db")
	if got := dirFor("db", []string{"/srv/*/", lit}); got != lit {
		t.Fatalf("got %q", got)
	}
	// A glob in the parent with no such directory: left for a person, not created.
	if got := dirFor("app", []string{filepath.Join(root, "tenant[12]", "app")}); got != "" {
		t.Fatalf("an ambiguous parent glob was placed at %q", got)
	}
}

// Patterns match as backups expand them: [!...] negation included.
func TestDirForBashGlob(t *testing.T) {
	if got := dirFor("app1", []string{"/srv/app[!2]/"}); got != "/srv/app1" {
		t.Fatalf("got %q", got)
	}
	if got := dirFor("app2", []string{"/srv/app[!2]/"}); got != "" {
		t.Fatalf("a negated name was placed at %q", got)
	}
}

// Escapes are removed from the directory used, as backups remove them.
func TestDirForEscapedParent(t *testing.T) {
	if got := dirFor("app", []string{`/srv/tenant\-one/*/`}); got != "/srv/tenant-one/app" {
		t.Fatalf("got %q", got)
	}
}
