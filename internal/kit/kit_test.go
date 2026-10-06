package kit

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/logging"
)

func TestModeGrants(t *testing.T) {
	for _, c := range []struct {
		have, want os.FileMode
		ok         bool
	}{{0o644, 0o644, true}, {0o600, 0o644, false}, {0o640, 0o644, false}, {0o644, 0o600, true}, {0o604, 0o640, false}, {0o755, 0o644, true}} {
		if got := modeGrants(c.have, c.want); got != c.ok {
			t.Errorf("modeGrants(%o, %o) = %v, want %v", c.have, c.want, got, c.ok)
		}
	}
}

func TestSymbolicMode(t *testing.T) {
	for in, want := range map[string]os.FileMode{"rwxr-xr-x": 0o755, "rw-r--r--": 0o644, "rwsr-S--T": 0o740, "---------": 0} {
		if got := symbolicMode(in); got != want {
			t.Errorf("%s: %o, want %o", in, got, want)
		}
	}
}

func inode(t *testing.T, p string) uint64 {
	t.Helper()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Sys().(*syscall.Stat_t).Ino
}

// A share that ignores the chmod leaves the kit less readable than the storage's files:
// placed, but Unverified, and the run keeps it out of the state record so the next one
// re-places it.
func TestUnverifiedIsNotRecorded(t *testing.T) {
	f := newFixture(t, 1)
	os.Chmod(filepath.Join(f.store[0], "config"), 0o644)
	chmod = func(string, os.FileMode) error { return nil }
	defer func() { chmod = os.Chmod }()
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	if code := f.execute(); code != Unverified {
		t.Fatalf("exit %d, want %d", code, Unverified)
	}
	if _, err := os.Stat(f.kit(0)); err != nil {
		t.Fatal("an unverified kit was not placed")
	}
	state, _ := os.ReadFile(filepath.Join(f.root, "logs", ".recovery-kit-state"))
	if strings.Contains(string(state), "store1") {
		t.Fatalf("an unverified placement was recorded: %q", state)
	}
	chmod = os.Chmod
	if code := f.execute(); code != OK {
		t.Fatalf("the retry: exit %d", code)
	}
}

func TestValidatePassword(t *testing.T) {
	if ValidatePassword("short", "storage-pw") == nil {
		t.Error("a short password passed")
	}
	if ValidatePassword("same-password", "same-password") == nil {
		t.Error("a password equal to the storage password passed")
	}
	if err := ValidatePassword("a-long-recovery-pw", "storage-pw"); err != nil {
		t.Error(err)
	}
}

type fixture struct {
	run   *Run
	vars  map[string]string
	root  string
	store []string
	out   bytes.Buffer
}

func newFixture(t *testing.T, stores int) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{root: root, vars: map[string]string{"SERVICE_DIRECTORIES": "/data/a/:/data/b/"}}
	secrets := filepath.Join(root, "secrets")
	for _, d := range []string{secrets, filepath.Join(root, "keys"), filepath.Join(root, "logs")} {
		os.MkdirAll(d, 0o700)
	}
	os.WriteFile(filepath.Join(secrets, "storage_password"), []byte("storage-pw\n"), 0o600)
	os.WriteFile(filepath.Join(secrets, "rsa_passphrase"), []byte("rp"), 0o600)
	os.WriteFile(filepath.Join(secrets, "recovery_password"), []byte("recovery-pw-123"), 0o600)
	os.WriteFile(filepath.Join(root, "keys", "private.pem"), []byte("PRIVATE"), 0o600)
	os.WriteFile(filepath.Join(root, "keys", "public.pem"), []byte("PUBLIC"), 0o644)
	for i := 1; i <= stores; i++ {
		s := filepath.Join(root, "store"+string(rune('0'+i)))
		os.MkdirAll(s, 0o755)
		os.WriteFile(filepath.Join(s, "config"), nil, 0o644)
		f.store = append(f.store, s)
		p := "STORAGE_TARGET_" + string(rune('0'+i)) + "_"
		f.vars[p+"NAME"], f.vars[p+"TYPE"], f.vars[p+"LOCAL_PATH"] = "store"+string(rune('0'+i)), "local", s
	}
	f.run = &Run{
		Layout:       layout.Layout{Root: root, Lock: filepath.Join(root, "lock")},
		Source:       config.Source{Getenv: func(k string) string { return f.vars[k] }, SecretsDir: secrets},
		Hostname:     "h",
		Log:          &logging.Log{Dir: filepath.Join(root, "logs"), Basename: "archiver", Stdout: &f.out},
		DockerSocket: filepath.Join(root, "no-socket"),
		Rclone:       fakeRclone(t, root),
	}
	return f
}

// fakeRclone stands in for rclone with a local remote: lsjson fails for a missing local path,
// and copyto writes a temporary file and renames it over the destination, as rclone does. It
// records its arguments and RCLONE_ environment.
func fakeRclone(t *testing.T, root string) string {
	t.Helper()
	p := filepath.Join(root, "rclone")
	os.WriteFile(p, []byte(`#!/bin/sh
echo "$@" >> "$(dirname "$0")/rclone.args"
env | grep '^RCLONE_' | sort >> "$(dirname "$0")/rclone.env"
for a in "$@"; do src="$dst"; dst="$a"; done
if [ "$3" = lsjson ]; then
  case "$dst" in KIT:/*) [ -e "${dst#KIT:}" ] || exit 3 ;; esac
  exit 0
fi
path="${dst#KIT:}"
[ -d "$(dirname "$path")" ] || { echo "directory not found" >&2; exit 3; }
cp "$src" "$path.partial" && mv "$path.partial" "$path"
`), 0o755)
	return p
}

func (f *fixture) environ() []string {
	var env []string
	for k, v := range f.vars {
		env = append(env, k+"="+v)
	}
	return env
}

func (f *fixture) execute() int {
	f.run.Environ = f.environ()
	return f.run.Execute()
}

func (f *fixture) kit(i int) string {
	return filepath.Join(f.store[i], "archiver-recovery-kit-h.tar.enc")
}

// decrypt opens the kit as a recovery would: stock openssl with only the password.
func decrypt(t *testing.T, kit, into string) {
	t.Helper()
	os.MkdirAll(into, 0o700)
	dec := exec.Command("sh", "-c", `openssl enc -d -aes-256-cbc -pbkdf2 -pass pass:recovery-pw-123 -in "$1" | tar -xf - -C "$2"`, "sh", kit, into)
	if out, err := dec.CombinedOutput(); err != nil {
		t.Fatalf("decrypt: %v: %s", err, out)
	}
}

func TestExecute(t *testing.T) {
	f := newFixture(t, 2)
	if code := f.execute(); code != OK {
		t.Fatalf("exit %d: %s", code, f.out.String())
	}
	out := filepath.Join(f.root, "out")
	decrypt(t, f.kit(1), out)
	env, _ := os.ReadFile(filepath.Join(out, "archiver.env"))
	if !strings.HasPrefix(string(env), "SERVICE_DIRECTORIES=/data/a/:/data/b/\nDUPLICACY_THREADS=4\nSTORAGE_TARGET_1_LOCAL_PATH=") {
		t.Fatalf("archiver.env:\n%s", env)
	}
	for name, want := range map[string]string{"storage_password": "storage-pw", "recovery_password": "recovery-pw-123", "rsa_private_key": "PRIVATE"} {
		if b, _ := os.ReadFile(filepath.Join(out, "secrets", name)); string(b) != want {
			t.Errorf("secrets/%s = %q, want %q", name, b, want)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(f.store[0], "archiver-recovery-kit-h.README.txt")); !bytes.Contains(b, []byte("openssl enc -d -aes-256-cbc -pbkdf2 -in archiver-recovery-kit-h.tar.enc")) {
		t.Errorf("README: %s", b)
	}
	state, _ := os.ReadFile(filepath.Join(f.root, "logs", ".recovery-kit-state"))
	if lines := strings.Split(string(state), "\n"); !strings.HasPrefix(lines[0], "v5:") || lines[1] != "store1" || lines[2] != "store2" {
		t.Fatalf("state %q", state)
	}

	// Unchanged: nothing is rewritten.
	before := inode(t, f.kit(0))
	if code := f.execute(); code != OK || inode(t, f.kit(0)) != before {
		t.Fatalf("an unchanged kit was rewritten (exit %d)", code)
	}
	if !strings.Contains(readLog(f), "Recovery kit is up to date on all storage targets.") {
		t.Error("no up-to-date message")
	}
	// A changed setting rewrites it; force rewrites it unchanged.
	f.vars["DUPLICACY_THREADS"] = "5"
	if f.execute(); inode(t, f.kit(0)) == before {
		t.Fatal("a changed setting did not rewrite the kit")
	}
	before = inode(t, f.kit(0))
	f.run.Force = true
	if f.execute(); inode(t, f.kit(0)) == before {
		t.Fatal("force did not rewrite the kit")
	}
}

func readLog(f *fixture) string {
	b, _ := os.ReadFile(filepath.Join(f.root, "logs", "archiver.log"))
	return string(b)
}

func TestSecondaryFailureAndRetry(t *testing.T) {
	f := newFixture(t, 2)
	marker := filepath.Join(f.root, "marker")
	f.run.PrimaryMarker = marker
	// Same path (the path is in the kit), missing for now.
	os.RemoveAll(f.store[1])
	if code := f.execute(); code != SecondaryFailed {
		t.Fatalf("exit %d, want %d", code, SecondaryFailed)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Error("the primary marker was not created")
	}
	before := inode(t, f.kit(0))
	os.MkdirAll(f.store[1], 0o755)
	os.WriteFile(filepath.Join(f.store[1], "config"), nil, 0o644)
	// The secondary is retried; the primary, recorded, is not rewritten.
	if code := f.execute(); code != OK || inode(t, f.kit(0)) != before {
		t.Fatalf("exit %d, or the recorded primary was rewritten", code)
	}
	if _, err := os.Stat(f.kit(1)); err != nil {
		t.Fatal("the failed secondary was not retried")
	}

	f.run.Force, f.run.Skip = true, []string{"store2"}
	os.RemoveAll(f.store[0])
	os.Remove(marker)
	if code := f.execute(); code != Failed {
		t.Fatalf("a failed primary: exit %d, want %d", code, Failed)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the primary marker was created although the primary failed")
	}
}

func TestNotConfigured(t *testing.T) {
	f := newFixture(t, 1)
	os.Remove(filepath.Join(f.root, "secrets", "recovery_password"))
	if code := f.execute(); code != OK {
		t.Fatalf("exit %d", code)
	}
	if _, err := os.Stat(f.kit(0)); err == nil {
		t.Fatal("a kit without a recovery password")
	}
	if !strings.Contains(readLog(f), "Recovery kit not configured") {
		t.Error("no skip message")
	}
}

func TestSnapshot(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "storage_password"), []byte("pw\r\n\n"), 0o600)
	os.WriteFile(filepath.Join(d, "pushover_user_key"), nil, 0o600)
	os.WriteFile(filepath.Join(d, "storage_target_1_b2_id"), []byte("b2id"), 0o600)
	os.WriteFile(filepath.Join(d, "storage_target_2_s3_id"), []byte("not-s3"), 0o600) // target 2 is b2
	env := map[string]string{
		"SERVICE_DIRECTORIES":   "/a/\n/b/::",
		"ROTATE_BACKUPS":        "false",
		"STORAGE_TARGET_1_NAME": "one", "STORAGE_TARGET_1_TYPE": "b2",
		"STORAGE_TARGET_2_NAME": "two", "STORAGE_TARGET_2_TYPE": "b2",
		"STORAGE_TARGET_10_NAME":     "ten",
		"STORAGE_TARGET_1_S3_REGION": "",
		"UNRELATED":                  "x",
	}
	var environ []string
	for k, v := range env {
		environ = append(environ, k+"="+v)
	}
	s, err := config.Snapshot(config.Source{Getenv: func(k string) string { return env[k] }, SecretsDir: d}, environ)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, kv := range s.NonSecret {
		names = append(names, kv.Name+"="+kv.Value)
	}
	want := "DUPLICACY_THREADS=4 PRUNE_BACKUPS=false SERVICE_DIRECTORIES=/a/:/b/ STORAGE_TARGET_1_NAME=one STORAGE_TARGET_1_S3_REGION= STORAGE_TARGET_1_TYPE=b2 STORAGE_TARGET_2_NAME=two STORAGE_TARGET_2_TYPE=b2 STORAGE_TARGET_10_NAME=ten"
	if got := strings.Join(names, " "); got != want {
		t.Fatalf("settings\n got %s\nwant %s", got, want)
	}
	var secrets []string
	for _, kv := range s.Secrets {
		secrets = append(secrets, kv.Name+"="+kv.Value)
	}
	if got := strings.Join(secrets, " "); got != "STORAGE_PASSWORD=pw PUSHOVER_USER_KEY= STORAGE_TARGET_1_B2_ID=b2id" {
		t.Fatalf("secrets %s", got)
	}
}

// Every storage type a configuration accepts can hold the kit: a type without an rclone
// remote fails here, not at a deployment's first kit refresh.
func TestEveryStorageTypeHasARemote(t *testing.T) {
	for _, name := range config.StorageTypes {
		if config.Types[name].Remote == nil {
			t.Errorf("storage type %q has no recovery-kit remote", name)
		}
	}
}

// Credentials reach rclone in its environment, never its arguments.
func TestRemoteCredentialsStayOffArgv(t *testing.T) {
	f := newFixture(t, 0)
	for _, tgt := range []config.Target{
		{Name: "b", Type: "b2", Values: config.Values{"B2_BUCKETNAME": "bkt", "B2_ID": "keyid", "B2_KEY": "b2-secret"}},
		{Name: "s", Type: "s3", Values: config.Values{"S3_BUCKETNAME": "bkt", "S3_ENDPOINT": "s3.example.com", "S3_ID": "akid", "S3_SECRET": "s3-secret"}},
	} {
		os.Remove(filepath.Join(f.root, "rclone.args"))
		os.Remove(filepath.Join(f.root, "rclone.env"))
		f.run.upload(tgt, filepath.Join(f.root, "keys", "public.pem"), filepath.Join(f.root, "keys", "public.pem"))
		args, _ := os.ReadFile(filepath.Join(f.root, "rclone.args"))
		env, _ := os.ReadFile(filepath.Join(f.root, "rclone.env"))
		if !strings.Contains(string(args), "--ignore-times") {
			t.Errorf("%s: a kit chosen for upload must always be sent: %s", tgt.Type, args)
		}
		if strings.Contains(string(args), "secret") {
			t.Errorf("%s: a credential on rclone's argv: %s", tgt.Type, args)
		}
		if !strings.Contains(string(args), "KIT:bkt/archiver-recovery-kit-h.tar.enc") {
			t.Errorf("%s: wrong destination: %s", tgt.Type, args)
		}
		want := map[string][]string{"b2": {"RCLONE_CONFIG_KIT_ACCOUNT=keyid", "RCLONE_CONFIG_KIT_KEY=b2-secret"},
			"s3": {"RCLONE_CONFIG_KIT_SECRET_ACCESS_KEY=s3-secret", "RCLONE_CONFIG_KIT_ENDPOINT=https://s3.example.com", "RCLONE_CONFIG_KIT_FORCE_PATH_STYLE=true", "RCLONE_CONFIG_KIT_REGION=us-east-1"}}[tgt.Type]
		for _, w := range want {
			if !strings.Contains(string(env), w+"\n") {
				t.Errorf("%s: rclone's environment lacks %s:\n%s", tgt.Type, w, env)
			}
		}
	}
}

// The local kit takes the storage's mode, through a fresh inode, and a failed upload leaves
// the previous kit in place.
func TestLocalPlacement(t *testing.T) {
	f := newFixture(t, 1)
	os.Chmod(filepath.Join(f.store[0], "config"), 0o644)
	os.WriteFile(f.kit(0), []byte("old"), 0o600)
	before := inode(t, f.kit(0))
	if code := f.execute(); code != OK {
		t.Fatalf("exit %d: %s", code, f.out.String())
	}
	if inode(t, f.kit(0)) == before {
		t.Error("replaced in place, not through a fresh inode")
	}
	if fi, _ := os.Stat(f.kit(0)); fi.Mode().Perm() != 0o644 {
		t.Errorf("mode %o, want the storage's 644", fi.Mode().Perm())
	}
	os.WriteFile(f.kit(0), []byte("current"), 0o644)
	f.vars["STORAGE_TARGET_1_LOCAL_PATH"] = filepath.Join(f.root, "gone")
	if code := f.execute(); code != Failed {
		t.Fatalf("a missing storage: exit %d", code)
	}
	if b, _ := os.ReadFile(f.kit(0)); string(b) != "current" {
		t.Fatal("a failed upload touched the kit")
	}
}

// Only the host-key types already trusted are offered, and an RSA key allows SHA-2 too.
func TestTrustedAlgorithms(t *testing.T) {
	d := t.TempDir()
	known := filepath.Join(d, "known_hosts")
	os.WriteFile(known, []byte("[nas.lan]:2222 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGnJ+7uNcHwJy3bXTBWdzEw1v4s4m0U5n6aRZkLGvH0U\n"+
		"other ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7\n"), 0o600)
	if got := trusted(known, "[nas.lan]:2222"); got != "ssh-ed25519" {
		t.Errorf("nas.lan: %q", got)
	}
	if got := trusted(known, "other"); got != "rsa-sha2-512 rsa-sha2-256 ssh-rsa" {
		t.Errorf("other: %q", got)
	}
	if got := trusted(known, "unknown"); got != "" {
		t.Errorf("unknown: %q", got)
	}
}
