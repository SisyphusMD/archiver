package config

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Every type's URL is the one Duplicacy 3.2.5 parses into the intended parts, and its
// credentials reach Duplicacy under the keys its storage code reads.
func TestEveryTypeURLAndCredentials(t *testing.T) {
	for _, c := range []struct {
		typ  string
		v    Values
		url  string
		keys []string // DUPLICACY_OFF_<KEY> variables beyond PASSWORD and RSA_PASSPHRASE
	}{
		{"local", Values{"LOCAL_PATH": "/mnt/b"}, "/mnt/b", nil},
		{"sftp", Values{"SFTP_URL": "h", "SFTP_PORT": "2222", "SFTP_USER": "u", "SFTP_PATH": "srv/d"}, "sftp://u@h:2222//srv/d", []string{"SSH_KEY_FILE"}},
		{"sftpc", Values{"SFTP_URL": "h", "SFTP_USER": "u", "SFTP_PATH": "d"}, "sftpc://u@h:22//d", []string{"SSH_KEY_FILE"}},
		{"b2", Values{"B2_BUCKETNAME": "bk", "B2_ID": "i", "B2_KEY": "k"}, "b2://bk", []string{"B2_ID", "B2_KEY"}},
		{"b2", Values{"B2_BUCKETNAME": "bk", "B2_PATH": "/x/y/", "B2_ID": "i", "B2_KEY": "k"}, "b2://bk/x/y", []string{"B2_ID", "B2_KEY"}},
		{"b2-custom", Values{"B2_DOWNLOAD_HOST": "f002.example.com", "B2_BUCKETNAME": "bk", "B2_ID": "i", "B2_KEY": "k"}, "b2-custom://f002.example.com/bk", []string{"B2_ID", "B2_KEY"}},
		{"s3", Values{"S3_BUCKETNAME": "bk", "S3_ENDPOINT": "s3.amazonaws.com", "S3_REGION": "us-east-1", "S3_ID": "i", "S3_SECRET": "s"}, "s3://us-east-1@s3.amazonaws.com/bk", []string{"S3_ID", "S3_SECRET"}},
		{"s3c", Values{"S3_BUCKETNAME": "bk", "S3_ENDPOINT": "e", "S3_PATH": "p", "S3_ID": "i", "S3_SECRET": "s"}, "s3c://none@e/bk/p", []string{"S3_ID", "S3_SECRET"}},
		{"minio", Values{"S3_BUCKETNAME": "bk", "S3_ENDPOINT": "garage:3900", "S3_ID": "i", "S3_SECRET": "s"}, "minio://none@garage:3900/bk", []string{"S3_ID", "S3_SECRET"}},
		{"minios", Values{"S3_BUCKETNAME": "bk", "S3_ENDPOINT": "e", "S3_ID": "i", "S3_SECRET": "s"}, "minios://none@e/bk", []string{"S3_ID", "S3_SECRET"}},
		{"wasabi", Values{"WASABI_BUCKETNAME": "bk", "WASABI_KEY": "i", "WASABI_SECRET": "s"}, "wasabi://us-east-1@s3.wasabisys.com/bk", []string{"WASABI_KEY", "WASABI_SECRET"}},
		{"azure", Values{"AZURE_ACCOUNT": "acct", "AZURE_CONTAINER": "c", "AZURE_KEY": "k"}, "azure://acct/c", []string{"AZURE_KEY"}},
		{"gcs", Values{"GCS_BUCKETNAME": "bk", "GCS_PATH": "p", "GCS_TOKEN": "{}", "GCS_TOKEN_FILE": "/run/secrets/t"}, "gcs://bk/p", []string{"GCS_TOKEN"}},
		{"gcd", Values{"GCD_PATH": "backups", "GCD_TOKEN": "{}", "GCD_TOKEN_FILE": "/run/secrets/t"}, "gcd://backups", []string{"GCD_TOKEN"}},
		{"gcd", Values{"GCD_PATH": "b", "GCD_DRIVE": "Team", "GCD_TOKEN": "{}", "GCD_TOKEN_FILE": "/t"}, "gcd://Team@b", []string{"GCD_TOKEN"}},
		{"dropbox", Values{"DROPBOX_PATH": "/backups/", "DROPBOX_APP_KEY": "ak", "DROPBOX_APP_SECRET": "as", "DROPBOX_TOKEN": "rt"}, "dropbox://backups", []string{"DROPBOX_CLIENT_ID", "DROPBOX_CLIENT_SECRET", "DROPBOX_TOKEN"}},
		{"dropbox", Values{"DROPBOX_PATH": "a/b/c", "DROPBOX_APP_KEY": "ak", "DROPBOX_APP_SECRET": "as", "DROPBOX_TOKEN": "rt"}, "dropbox://a//b/c", []string{"DROPBOX_CLIENT_ID", "DROPBOX_CLIENT_SECRET", "DROPBOX_TOKEN"}},
		{"gcd", Values{"GCD_DRIVE": "Team", "GCD_TOKEN": "{}", "GCD_TOKEN_FILE": "/t"}, "gcd://Team@", []string{"GCD_TOKEN"}},
		{"swift", Values{"SWIFT_URL": "u@auth.example.com/v3/c/p?domain=d", "SWIFT_KEY": "k"}, "swift://u@auth.example.com/v3/c/p?domain=d", []string{"SWIFT_KEY"}},
		{"webdav", Values{"WEBDAV_HOST": "dav.example.com:8443", "WEBDAV_USER": "u", "WEBDAV_PATH": "/dav/b", "WEBDAV_PASSWORD": "p"}, "webdav://u@dav.example.com:8443/dav/b", []string{"WEBDAV_PASSWORD"}},
		{"webdav-http", Values{"WEBDAV_HOST": "h", "WEBDAV_USER": "u", "WEBDAV_PATH": "b", "WEBDAV_PASSWORD": "p"}, "webdav-http://u@h/b", []string{"WEBDAV_PASSWORD"}},
		{"smb", Values{"SMB_HOST": "nas:4445", "SMB_USER": "u", "SMB_SHARE": "backup", "SMB_PATH": "d", "SMB_PASSWORD": "p"}, "smb://u@nas:4445/backup/d", []string{"SMB_PASSWORD"}},
		{"smb", Values{"SMB_HOST": "nas", "SMB_USER": "u", "SMB_SHARE": "backup", "SMB_PASSWORD": "p"}, "smb://u@nas/backup/", []string{"SMB_PASSWORD"}},
		{"storj", Values{"STORJ_SATELLITE": "us1.storj.io:7777", "STORJ_BUCKET": "bk", "STORJ_KEY": "k", "STORJ_PASSPHRASE": "pp"}, "storj://us1.storj.io:7777/bk", []string{"STORJ_KEY", "STORJ_PASSPHRASE"}},
		{"fabric", Values{"FABRIC_ENDPOINT": "fabric.example.com", "FABRIC_PATH": "b", "FABRIC_TOKEN": "t"}, "fabric://fabric.example.com/b", []string{"FABRIC_TOKEN"}},
	} {
		tg := Target{Name: "off", Type: c.typ, Values: c.v}
		if got, err := tg.URL(); err != nil || got != c.url {
			t.Errorf("%s URL = %q, %v; want %q", c.typ, got, err, c.url)
		}
		var keys []string
		for _, kv := range (&Config{StoragePassword: "pw", RSAPassphrase: "rp"}).DuplicacyEnv(tg, "/k/id_ed25519") {
			name, _, _ := strings.Cut(kv, "=")
			if name != "DUPLICACY_OFF_PASSWORD" && name != "DUPLICACY_OFF_RSA_PASSPHRASE" {
				keys = append(keys, strings.TrimPrefix(name, "DUPLICACY_OFF_"))
			}
		}
		slices.Sort(keys)
		want := slices.Clone(c.keys)
		slices.Sort(want)
		if !slices.Equal(keys, want) {
			t.Errorf("%s credentials %v, want %v", c.typ, keys, want)
		}
	}
}

// A token Duplicacy reads from a file gets the file's path, never its contents.
func TestTokenFilesPassedByPath(t *testing.T) {
	tg := Target{Name: "g", Type: "gcs", Values: Values{"GCS_BUCKETNAME": "b", "GCS_TOKEN": `{"secret":1}`, "GCS_TOKEN_FILE": "/run/secrets/storage_target_1_gcs_token"}}
	env := (&Config{}).DuplicacyEnv(tg, "")
	if !slices.Contains(env, "DUPLICACY_G_GCS_TOKEN=/run/secrets/storage_target_1_gcs_token") {
		t.Fatalf("env %v", env)
	}
}

// Every type's secrets are file-only secrets, its settings are settings, and break-glass
// variants exist only where declared.
func TestSecretAndSettingNames(t *testing.T) {
	for _, n := range []string{"STORAGE_TARGET_3_AZURE_KEY", "STORAGE_TARGET_1_BREAKGLASS_WEBDAV_PASSWORD", "STORAGE_TARGET_2_DROPBOX_TOKEN", "STORAGE_TARGET_2_BREAKGLASS_SSH_KEY", "STORAGE_TARGET_1_B2_KEY"} {
		if !IsSecret(n) || IsSetting(n) {
			t.Errorf("%s should be a secret", n)
		}
	}
	for _, n := range []string{"STORAGE_TARGET_3_AZURE_ACCOUNT", "STORAGE_TARGET_1_DROPBOX_APP_KEY", "STORAGE_TARGET_2_S3_PATH", "STORAGE_TARGET_1_SFTP_PORT", "STORAGE_TARGET_1_BREAKGLASS_SFTP_USER", "BACKUP_PARALLELISM"} {
		if !IsSetting(n) || IsSecret(n) {
			t.Errorf("%s should be a setting", n)
		}
	}
	if IsSecret("STORAGE_TARGET_1_BREAKGLASS_DROPBOX_TOKEN") {
		t.Error("a break-glass variant where none is declared")
	}
}

// Duplicacy's URL parser (3.2.5 duplicacy_storage.go) reads every URL into the intended parts:
// its regular expression and each type's own splitting, mirrored here.
func TestURLsParseAsIntended(t *testing.T) {
	re := regexp.MustCompile(`^([\w-]+)://([\w\-@\.\!]+@)?([^/]+)(/(.+))?`)
	for _, c := range []struct {
		url, dir string
		split    func(m []string) string
	}{
		{"dropbox://a//b/c", "a/b/c", func(m []string) string { return m[3] + m[5] }},
		{"dropbox://backups", "backups", func(m []string) string { return m[3] + m[5] }},
		{"smb://u@nas/backup/", "", func(m []string) string { _, d, _ := strings.Cut(m[5], "/"); return d }},
		{"gcd://Team@", "", func(m []string) string { return strings.TrimSuffix(m[3], "Team@") + m[4] }},
	} {
		m := re.FindStringSubmatch(c.url)
		if m == nil {
			t.Errorf("%s does not parse", c.url)
			continue
		}
		if got := c.split(m); got != c.dir {
			t.Errorf("%s: directory %q, want %q", c.url, got, c.dir)
		}
	}
	for _, u := range []string{"one://", "gcd://", "dropbox://"} {
		if re.MatchString(u) {
			t.Errorf("%s parses, so requirePath is unneeded", u)
		}
	}
}

// A drive path is required unless a drive is named; a Dropbox path always.
func TestDrivePathRequired(t *testing.T) {
	base := func(typ string, v Values) *Config {
		return &Config{ServiceDirectories: []string{"/s"}, StoragePassword: "longenough", RSAPassphrase: "r", PruneExhaustiveFrequency: "monthly",
			Targets: []Target{{N: 1, Name: "d", Type: typ, Values: v}}}
	}
	tok := Values{"ONE_CLIENT_ID": "c", "ONE_CLIENT_SECRET": "s", "ONE_TOKEN": "{}"}
	if err := base("one", tok).Validate("/run/secrets"); err == nil || !strings.Contains(err.Error(), "ONE_PATH") {
		t.Errorf("one without a path or drive: %v", err)
	}
	// Only a business drive can be named (Duplicacy ignores a personal drive ID).
	if err := base("odb", Values{"ODB_DRIVE_ID": "b!x", "ODB_CLIENT_ID": "c", "ODB_CLIENT_SECRET": "s", "ODB_TOKEN": "{}"}).Validate("/run/secrets"); err != nil {
		t.Errorf("odb with a drive: %v", err)
	}
	if err := base("dropbox", Values{"DROPBOX_APP_KEY": "k", "DROPBOX_APP_SECRET": "s", "DROPBOX_TOKEN": "t"}).Validate("/run/secrets"); err == nil || !strings.Contains(err.Error(), "DROPBOX_PATH") {
		t.Errorf("dropbox without a path: %v", err)
	}
}

// The kit carries the token as Duplicacy last refreshed it, not the one first configured;
// a new token from the user replaces both.
func TestSnapshotCarriesRefreshedToken(t *testing.T) {
	dir := t.TempDir()
	TokenDir = filepath.Join(dir, "tokens")
	defer func() { TokenDir = "/opt/archiver/logs/.tokens" }()
	secrets := filepath.Join(dir, "secrets")
	os.MkdirAll(secrets, 0o700)
	// Newline-terminated, as an editor or echo leaves it: read as a secret, the newline is dropped.
	os.WriteFile(filepath.Join(secrets, "storage_target_1_one_token"), []byte(`{"refresh_token":"first"}`+"\n"), 0o600)
	env := map[string]string{"STORAGE_TARGET_1_NAME": "od", "STORAGE_TARGET_1_TYPE": "one", "STORAGE_TARGET_1_ONE_PATH": "b"}
	src := Source{Getenv: func(k string) string { return env[k] }, SecretsDir: secrets}
	c, _, err := Load(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	copyPath := WritableToken(c.Targets[0], Types["one"].Fields[len(Types["one"].Fields)-1])
	os.WriteFile(copyPath, []byte(`{"refresh_token":"refreshed"}`), 0o600)
	token := func() string {
		s, err := Snapshot(src, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, kv := range s.Secrets {
			if kv.Name == "STORAGE_TARGET_1_ONE_TOKEN" {
				return kv.Value
			}
		}
		return ""
	}
	if got := token(); !strings.Contains(got, "refreshed") {
		t.Errorf("kit token %s, want the refreshed one", got)
	}
	os.WriteFile(filepath.Join(secrets, "storage_target_1_one_token"), []byte(`{"refresh_token":"new"}`), 0o600)
	if got := token(); !strings.Contains(got, `"new"`) {
		t.Errorf("kit token %s, want the user's new one", got)
	}
}

// The kit's Dropbox path is relative: an absolute one makes rclone send a path-root header
// that Dropbox refuses for an app-folder app ("path root is not supported for sandbox app").
func TestDropboxRemotePathRelative(t *testing.T) {
	if _, dir := Types["dropbox"].Remote(Values{"DROPBOX_PATH": "/archiver/primary/"}); dir != "archiver/primary" {
		t.Errorf("dir %q", dir)
	}
}

// A token copy never writes through what is already at its path.
func TestWritePrivateReplacesALink(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tokens")
	os.Mkdir(dir, 0o700)
	planted := filepath.Join(t.TempDir(), "readable")
	os.WriteFile(planted, nil, 0o644)
	p := filepath.Join(dir, "od-one_token")
	os.Symlink(planted, p)
	if err := WritePrivate(p, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(planted); len(b) != 0 {
		t.Fatal("the token was written through the link")
	}
	fi, err := os.Lstat(p)
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm() != 0o600 {
		t.Fatalf("%v %v", fi.Mode(), err)
	}
	os.Chmod(dir, 0o777)
	if WritePrivate(p, []byte("tok")) == nil {
		t.Fatal("a world-writable directory was accepted")
	}
}

// The recovery kit carries the notification secrets, so a recovered host keeps notifying.
func TestSnapshotCarriesNotificationSecrets(t *testing.T) {
	secrets := t.TempDir()
	os.WriteFile(filepath.Join(secrets, "apprise_url"), []byte("http://u:p@apprise:8000/notify/a\n"), 0o600)
	os.WriteFile(filepath.Join(secrets, "ntfy_token"), []byte("tk_1"), 0o600)
	src := Source{Getenv: func(string) string { return "" }, SecretsDir: secrets}
	s, err := Snapshot(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, kv := range s.Secrets {
		got[kv.Name] = kv.Value
	}
	if got["APPRISE_URL"] != "http://u:p@apprise:8000/notify/a" || got["NTFY_TOKEN"] != "tk_1" {
		t.Fatalf("snapshot secrets %v", got)
	}
}
