package setup

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInit(t *testing.T) {
	root := t.TempDir()
	keys := filepath.Join(root, "keys")
	os.MkdirAll(keys, 0o700)
	os.WriteFile(filepath.Join(keys, "private.pem"), []byte("old"), 0o600)
	answers := strings.Join([]string{
		" /srv/*/ , /home/u ,", // directories, trimmed, empty entry dropped
		"", "my store-1",       // name: empty is asked again; spaces dropped, '-' becomes '_'
		"tape", "LOCAL", "/backup", // an unknown type is asked again
		"y", "off", "sftp", "nas.lan/", "2222", "backup", "/volume1/archiver/",
		"y", "cloud", "b2", "bucket", "", "keyid", "app key", // no path inside the bucket
		"y", "s3store", "s3", "bkt", "s3.example.com", "", "", "AKID", "sec", // default region, no path
		"n",
		"y", "ukey", "atoken",
	}, "\n") + "\n"
	var out bytes.Buffer
	s := &Init{KeysDir: keys, SetupDir: filepath.Join(root, "setup"), In: strings.NewReader(answers), Out: &out,
		Now: func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) }}
	if code := s.Run(); code != 0 {
		t.Fatalf("exit %d:\n%s", code, out.String())
	}
	env, _ := os.ReadFile(filepath.Join(root, "setup", "env-native", "archiver.env"))
	want := `SERVICE_DIRECTORIES=/srv/*/:/home/u
CHECK_BACKUPS=true
DUPLICACY_THREADS=4
NOTIFICATION_SERVICE=Pushover
PRUNE_BACKUPS=true
PRUNE_EXHAUSTIVE_FREQUENCY=monthly
PRUNE_KEEP=-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1
STORAGE_TARGET_1_LOCAL_PATH=/backup
STORAGE_TARGET_1_NAME=mystore_1
STORAGE_TARGET_1_TYPE=local
STORAGE_TARGET_2_NAME=off
STORAGE_TARGET_2_SFTP_PATH=volume1/archiver
STORAGE_TARGET_2_SFTP_PORT=2222
STORAGE_TARGET_2_SFTP_URL=nas.lan
STORAGE_TARGET_2_SFTP_USER=backup
STORAGE_TARGET_2_TYPE=sftp
STORAGE_TARGET_3_B2_BUCKETNAME=bucket
STORAGE_TARGET_3_NAME=cloud
STORAGE_TARGET_3_TYPE=b2
STORAGE_TARGET_4_NAME=s3store
STORAGE_TARGET_4_S3_BUCKETNAME=bkt
STORAGE_TARGET_4_S3_ENDPOINT=s3.example.com
STORAGE_TARGET_4_S3_REGION=none
STORAGE_TARGET_4_TYPE=s3
`
	if string(env) != want {
		t.Fatalf("archiver.env:\n%s\nwant:\n%s", env, want)
	}
	secrets := filepath.Join(root, "setup", "env-native", "secrets")
	read := func(n string) string { b, _ := os.ReadFile(filepath.Join(secrets, n)); return string(b) }
	for n, v := range map[string]string{"storage_target_3_b2_id": "keyid", "storage_target_3_b2_key": "app key", "storage_target_4_s3_id": "AKID",
		"storage_target_4_s3_secret": "sec", "pushover_user_key": "ukey", "pushover_api_token": "atoken"} {
		if got := read(n); got != v {
			t.Errorf("secrets/%s = %q, want %q", n, got, v)
		}
	}
	for _, n := range []string{"storage_password", "rsa_passphrase", "recovery_password"} {
		if len(read(n)) != 32 {
			t.Errorf("secrets/%s is not a 32-character password", n)
		}
	}
	if read("storage_password") == read("recovery_password") {
		t.Error("the recovery password equals the storage password")
	}
	if fi, _ := os.Stat(secrets); fi.Mode().Perm() != 0o700 {
		t.Errorf("secrets/ mode %o", fi.Mode().Perm())
	}
	// The RSA key is what every deployment has: PKCS#1 PEM, encrypted with the passphrase.
	key := read("rsa_private_key")
	if !strings.HasPrefix(key, "-----BEGIN RSA PRIVATE KEY-----\nProc-Type: 4,ENCRYPTED\nDEK-Info: AES-256-CBC,") {
		t.Fatalf("not an encrypted PKCS#1 key:\n%.120s", key)
	}
	check := exec.Command("openssl", "rsa", "-in", filepath.Join(secrets, "rsa_private_key"), "-passin", "pass:"+read("rsa_passphrase"), "-check", "-noout")
	if b, err := check.CombinedOutput(); err != nil {
		t.Fatalf("the key does not open with the passphrase: %v %s", err, b)
	}
	if !strings.HasPrefix(read("ssh_public_key"), "ssh-ed25519 ") {
		t.Error("no ed25519 public key")
	}
	if _, err := os.Stat(filepath.Join(keys, "private.pem.backup.20261005-120000")); err != nil {
		t.Error("the existing key was not set aside")
	}
	for _, msg := range []string{"Error: Storage name is required", "Error: Must be one of local, sftp, sftpc, b2,", "RECOVERY PASSWORD: the single key", read("recovery_password")} {
		if !strings.Contains(out.String(), msg) {
			t.Errorf("output lacks %q", msg)
		}
	}
	if strings.Contains(out.String(), read("storage_password")) {
		t.Error("the storage password was printed")
	}
}

func TestInitEndsWhenInputDoes(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	s := &Init{KeysDir: filepath.Join(root, "keys"), SetupDir: filepath.Join(root, "setup"), In: strings.NewReader("/srv/\n"), Out: &out}
	if code := s.Run(); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(root, "setup", "env-native")); err == nil {
		t.Error("materials written from incomplete answers")
	}
}

func TestInitEndingAfterPushoverYesFails(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	s := &Init{KeysDir: filepath.Join(root, "keys"), SetupDir: filepath.Join(root, "setup"),
		In: strings.NewReader("/srv/\nl\nlocal\n/b\nn\ny\nukey\n"), Out: &out}
	if code := s.Run(); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(root, "setup", "env-native")); err == nil {
		t.Error("materials written without the Pushover token")
	}
}

// A key that cannot be set aside is never generated over.
func TestInitKeepsAKeyItCannotSetAside(t *testing.T) {
	root := t.TempDir()
	keys := filepath.Join(root, "keys")
	os.MkdirAll(keys, 0o700)
	os.WriteFile(filepath.Join(keys, "private.pem"), []byte("only copy"), 0o600)
	// Where it would be set aside is a non-empty directory: the rename fails, root or not.
	blocker := filepath.Join(keys, "private.pem.backup.20261005-120000")
	os.MkdirAll(filepath.Join(blocker, "x"), 0o700)
	var out bytes.Buffer
	s := &Init{KeysDir: keys, SetupDir: filepath.Join(root, "setup"), In: strings.NewReader(""), Out: &out,
		Now: func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) }}
	if code := s.Run(); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if b, _ := os.ReadFile(filepath.Join(keys, "private.pem")); string(b) != "only copy" {
		t.Fatalf("the key was overwritten: %q", b)
	}
}

// A type beyond the first four takes its fields from the storage-type table: a token file's
// contents become its secret, and a plain secret is written as given.
func TestInitTableTypes(t *testing.T) {
	root := t.TempDir()
	token := filepath.Join(root, "sa.json")
	os.WriteFile(token, []byte(`{"type":"service_account"}`+"\n"), 0o600)
	answers := strings.Join([]string{
		"/srv/*/",
		"gstore", "gcs", "bkt", "", token, // no path inside the bucket
		"y", "az", "azure", "acct", "cont", "azkey",
		"n", "n",
	}, "\n") + "\n"
	var out bytes.Buffer
	s := &Init{KeysDir: filepath.Join(root, "keys"), SetupDir: filepath.Join(root, "setup"), In: strings.NewReader(answers), Out: &out}
	if code := s.Run(); code != 0 {
		t.Fatalf("exit %d:\n%s", code, out.String())
	}
	env, _ := os.ReadFile(filepath.Join(root, "setup", "env-native", "archiver.env"))
	for _, line := range []string{"STORAGE_TARGET_1_TYPE=gcs", "STORAGE_TARGET_1_GCS_BUCKETNAME=bkt", "STORAGE_TARGET_2_TYPE=azure",
		"STORAGE_TARGET_2_AZURE_ACCOUNT=acct", "STORAGE_TARGET_2_AZURE_CONTAINER=cont"} {
		if !strings.Contains(string(env), line+"\n") {
			t.Errorf("archiver.env lacks %s:\n%s", line, env)
		}
	}
	if strings.Contains(string(env), "GCS_PATH") {
		t.Error("an unanswered optional field was written")
	}
	secrets := filepath.Join(root, "setup", "env-native", "secrets")
	for n, v := range map[string]string{"storage_target_1_gcs_token": `{"type":"service_account"}`, "storage_target_2_azure_key": "azkey"} {
		if b, _ := os.ReadFile(filepath.Join(secrets, n)); string(b) != v {
			t.Errorf("secrets/%s = %q, want %q", n, b, v)
		}
	}
}
