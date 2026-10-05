package kit

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

func TestPlaceLocal(t *testing.T) {
	d := t.TempDir()
	src, dst, ref := filepath.Join(d, "src"), filepath.Join(d, "dst"), filepath.Join(d, "config")
	os.WriteFile(src, []byte("new kit"), 0o600)
	os.WriteFile(ref, nil, 0o644)
	os.Chmod(ref, 0o644)
	os.WriteFile(dst, []byte("old kit"), 0o600)
	before := inode(t, dst)
	if got := placeLocal(src, dst, ref); got != OK {
		t.Fatalf("placeLocal = %d", got)
	}
	if b, _ := os.ReadFile(dst); string(b) != "new kit" {
		t.Fatalf("dst holds %q", b)
	}
	if inode(t, dst) == before {
		t.Error("replaced in place, not through a fresh inode")
	}
	if fi, _ := os.Stat(dst); fi.Mode().Perm() != 0o644 {
		t.Errorf("mode %o, want the reference's 644", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(d)
	if len(entries) != 3 {
		t.Errorf("a staging file was left: %v", entries)
	}

	// The reference less readable than the target can be made: unverified, still placed.
	os.Chmod(ref, 0o640)
	os.Chmod(d, 0o755)
	if got := placeLocal(src, dst, ref); got != OK {
		t.Fatalf("a 640 reference: %d", got)
	}

	// A kit that cannot be staged leaves the previous one.
	os.WriteFile(dst, []byte("old kit"), 0o644)
	if got := placeLocal(filepath.Join(d, "missing"), dst, ref); got != Failed {
		t.Fatalf("missing source: %d", got)
	}
	if b, _ := os.ReadFile(dst); string(b) != "old kit" {
		t.Fatal("a failed placement replaced the kit")
	}
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
	}
	return f
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

// fakeB2 is enough of the B2 API for the kit: authorize (with an unrestricted key, so the
// bucket is looked up by name), list_buckets, get_upload_url and upload_file.
func fakeB2(t *testing.T) (*httptest.Server, map[string][]byte) {
	t.Helper()
	files := map[string][]byte{}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v2/b2_authorize_account":
			if u, p, ok := r.BasicAuth(); !ok || u != "keyid" || p != "secret\"key" {
				http.Error(w, "unauthorized", 401)
				return
			}
			fmt.Fprintf(w, `{"accountId":"acct","apiUrl":%q,"authorizationToken":"tok","allowed":{"bucketId":null,"bucketName":null}}`, srv.URL)
		case "/b2api/v2/b2_list_buckets":
			var req map[string]string
			json.NewDecoder(r.Body).Decode(&req)
			if r.Header.Get("Authorization") != "tok" || req["bucketName"] != "bkt" || req["accountId"] != "acct" {
				http.Error(w, "bad list", 400)
				return
			}
			fmt.Fprint(w, `{"buckets":[{"bucketId":"bid","bucketName":"bkt"}]}`)
		case "/b2api/v2/b2_get_upload_url":
			fmt.Fprintf(w, `{"bucketId":"bid","uploadUrl":%q,"authorizationToken":"up"}`, srv.URL+"/upload")
		case "/upload":
			body, _ := io.ReadAll(r.Body)
			sum := sha1.Sum(body)
			if r.Header.Get("Authorization") != "up" || r.Header.Get("X-Bz-Content-Sha1") != hex.EncodeToString(sum[:]) {
				http.Error(w, "bad upload", 400)
				return
			}
			files[r.Header.Get("X-Bz-File-Name")] = body
			fmt.Fprint(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, files
}

func TestUploadB2(t *testing.T) {
	srv, files := fakeB2(t)
	b2API = srv.URL
	defer func() { b2API = "https://api.backblazeb2.com" }()
	d := t.TempDir()
	kit, readme := filepath.Join(d, "kit"), filepath.Join(d, "readme")
	os.WriteFile(kit, []byte("encrypted"), 0o600)
	os.WriteFile(readme, []byte("readme"), 0o600)
	var out bytes.Buffer
	r := &Run{Hostname: "h", Log: &logging.Log{Dir: d, Basename: "archiver", Stdout: &out}}
	tgt := config.Target{Type: "b2", B2Bucket: "bkt", B2ID: "keyid", B2Key: `secret"key`}
	if got := r.upload(tgt, kit, readme); got != OK {
		t.Fatalf("upload = %d: %s", got, out.String())
	}
	if string(files["archiver-recovery-kit-h.tar.enc"]) != "encrypted" || string(files["archiver-recovery-kit-h.README.txt"]) != "readme" {
		t.Fatalf("uploaded %v", files)
	}
	tgt.B2Key = "wrong"
	if got := r.upload(tgt, kit, readme); got != Failed {
		t.Fatalf("bad credentials: %d", got)
	}
}

// Every storage type a configuration accepts can hold the kit: a type added to config without
// an uploader fails here, not at a deployment's first kit refresh.
func TestEveryStorageTypeHasAnUploader(t *testing.T) {
	for _, typ := range config.StorageTypes {
		if _, ok := uploaders[typ]; !ok {
			t.Errorf("storage type %q has no recovery-kit uploader", typ)
		}
	}
	for typ := range uploaders {
		if !slices.Contains(config.StorageTypes, typ) {
			t.Errorf("uploader for %q, a type config does not accept", typ)
		}
	}
}
