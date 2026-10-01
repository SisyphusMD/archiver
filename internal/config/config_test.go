package config

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSharedStorageNameTable(t *testing.T) {
	f, err := os.Open("../../tests/fixtures/storage-names.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) != 3 {
			t.Fatalf("bad row %q", line)
		}
		tg := Target{Name: cols[0], Type: "local"}
		if got := tg.StorageName(); got != cols[1] {
			t.Errorf("Sanitize(%q) = %q, want %q", cols[0], got, cols[1])
		}
		if got := tg.EnvPrefix(); got != cols[2] {
			t.Errorf("EnvPrefix(%q) = %q, want %q", cols[0], got, cols[2])
		}
		rows++
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if rows < 10 {
		t.Fatalf("only %d rows read", rows)
	}
}

func TestURL(t *testing.T) {
	for _, tc := range []struct {
		t    Target
		want string
	}{
		{Target{Type: "local", LocalPath: "/mnt/backups/local"}, "/mnt/backups/local"},
		{Target{Type: "sftp", SFTPUser: "backup", SFTPURL: "sftp.example.com", SFTPPort: "2222", SFTPPath: "srv/duplicacy"}, "sftp://backup@sftp.example.com:2222//srv/duplicacy"},
		{Target{Type: "b2", B2Bucket: "my-b2-bucket"}, "b2://my-b2-bucket"},
		{Target{Type: "s3", S3Region: "us-east-1", S3Endpoint: "s3.amazonaws.com", S3Bucket: "archive"}, "s3://us-east-1@s3.amazonaws.com/archive"},
		{Target{Type: "s3", S3Endpoint: "nyc3.digitaloceanspaces.com", S3Bucket: "archive"}, "s3://none@nyc3.digitaloceanspaces.com/archive"},
	} {
		got, err := tc.t.URL()
		if err != nil || got != tc.want {
			t.Errorf("%s URL = %q, %v; want %q", tc.t.Type, got, err, tc.want)
		}
	}
	if _, err := (Target{Type: "ftp"}).URL(); err == nil {
		t.Error("ftp URL: want an error")
	}
}

func TestDuplicacyEnv(t *testing.T) {
	c := &Config{StoragePassword: "pw", RSAPassphrase: "rp"}
	for _, tc := range []struct {
		t    Target
		want []string
	}{
		{Target{Name: "localdisk", Type: "local"}, []string{"DUPLICACY_LOCALDISK_PASSWORD=pw", "DUPLICACY_LOCALDISK_RSA_PASSPHRASE=rp"}},
		{Target{Name: "default", Type: "b2", B2ID: "keyid", B2Key: "appkey"}, []string{"DUPLICACY_PASSWORD=pw", "DUPLICACY_RSA_PASSPHRASE=rp", "DUPLICACY_B2_ID=keyid", "DUPLICACY_B2_KEY=appkey"}},
		{Target{Name: "do-spaces", Type: "s3", S3ID: "AKIAxxx", S3Secret: "shhh"}, []string{"DUPLICACY_DO_SPACES_PASSWORD=pw", "DUPLICACY_DO_SPACES_RSA_PASSPHRASE=rp", "DUPLICACY_DO_SPACES_S3_ID=AKIAxxx", "DUPLICACY_DO_SPACES_S3_SECRET=shhh"}},
		{Target{Name: "cowanserver", Type: "sftp"}, []string{"DUPLICACY_COWANSERVER_PASSWORD=pw", "DUPLICACY_COWANSERVER_RSA_PASSPHRASE=rp", "DUPLICACY_COWANSERVER_SSH_KEY_FILE=/k/id_ed25519"}},
	} {
		if got := c.DuplicacyEnv(tc.t, "/k/id_ed25519"); !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.t.Name, got, tc.want)
		}
	}
}

// source builds a Source over a map and a secrets directory holding files.
func source(t *testing.T, env map[string]string, secrets map[string]string) (Source, []string) {
	t.Helper()
	dir := t.TempDir()
	for name, v := range secrets {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(v), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var environ []string
	for k, v := range env {
		environ = append(environ, k+"="+v)
	}
	return Source{Getenv: func(k string) string { return env[k] }, SecretsDir: dir}, environ
}

func mustLoad(t *testing.T, env, secrets map[string]string) (*Config, []string) {
	t.Helper()
	src, environ := source(t, env, secrets)
	c, warnings, err := Load(src, environ)
	if err != nil {
		t.Fatal(err)
	}
	return c, warnings
}

func TestSecrets(t *testing.T) {
	c, warnings := mustLoad(t, map[string]string{"STORAGE_PASSWORD": "from-env", "RSA_PASSPHRASE": "also-env"},
		map[string]string{"storage_password": "from-file\n\n", "rsa_passphrase": "crlf\r\n"})
	if c.StoragePassword != "from-file" {
		t.Errorf("StoragePassword = %q: a file must win and env must never be read", c.StoragePassword)
	}
	if c.RSAPassphrase != "crlf" {
		t.Errorf("RSAPassphrase = %q, want CRLF stripped", c.RSAPassphrase)
	}
	if len(warnings) != 2 || !strings.Contains(warnings[0]+warnings[1], "secrets are file-only") {
		t.Errorf("warnings = %v, want one per raw env secret", warnings)
	}

	c, _ = mustLoad(t, map[string]string{"STORAGE_PASSWORD": "env-only"}, nil)
	if c.StoragePassword != "" {
		t.Errorf("a raw env secret was used: %q", c.StoragePassword)
	}

	src, environ := source(t, map[string]string{"RSA_PASSPHRASE_FILE": "/nonexistent/x"}, nil)
	if _, _, err := Load(src, environ); err == nil || !strings.Contains(err.Error(), "RSA_PASSPHRASE_FILE is set to '/nonexistent/x'") {
		t.Errorf("explicit missing _FILE: got %v", err)
	}

	other := filepath.Join(t.TempDir(), "pw")
	os.WriteFile(other, []byte("via-file-var"), 0o600)
	c, _ = mustLoad(t, map[string]string{"STORAGE_PASSWORD_FILE": other}, map[string]string{"storage_password": "default-path"})
	if c.StoragePassword != "via-file-var" {
		t.Errorf("_FILE must win over the default path, got %q", c.StoragePassword)
	}
}

func TestTargetsAndDefaults(t *testing.T) {
	c, _ := mustLoad(t, map[string]string{
		"SERVICE_DIRECTORIES":   "/a/*/:/b/\n/c/::",
		"STORAGE_TARGET_1_NAME": "local", "STORAGE_TARGET_1_TYPE": "local", "STORAGE_TARGET_1_LOCAL_PATH": "/s",
		"STORAGE_TARGET_2_NAME": "bb", "STORAGE_TARGET_2_TYPE": "b2", "STORAGE_TARGET_2_B2_BUCKETNAME": "bk",
		"STORAGE_TARGET_4_NAME": "after-gap",
		"ROTATE_BACKUPS":        "FALSE",
	}, map[string]string{"storage_target_2_b2_id": "id", "storage_target_2_b2_key": "key"})
	if !slices.Equal(c.ServiceDirectories, []string{"/a/*/", "/b/", "/c/"}) {
		t.Errorf("ServiceDirectories = %q", c.ServiceDirectories)
	}
	if len(c.Targets) != 2 || c.Targets[1].B2ID != "id" || c.Targets[1].B2Key != "key" {
		t.Errorf("Targets = %+v (the list stops at the first gap)", c.Targets)
	}
	if c.PruneBackups || !c.CheckBackups {
		t.Errorf("PruneBackups=%v (ROTATE_BACKUPS=FALSE) CheckBackups=%v (default true)", c.PruneBackups, c.CheckBackups)
	}
	if c.PruneKeep != "-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1" || c.PruneExhaustiveFrequency != "monthly" || c.Threads != "4" {
		t.Errorf("defaults: %q %q %q", c.PruneKeep, c.PruneExhaustiveFrequency, c.Threads)
	}
	c, _ = mustLoad(t, map[string]string{"PRUNE_BACKUPS": "True", "ROTATE_BACKUPS": "false", "CHECK_BACKUPS": "no"}, nil)
	if !c.PruneBackups || c.CheckBackups {
		t.Errorf("PRUNE_BACKUPS must win over ROTATE_BACKUPS, and only 'true' is true: %v %v", c.PruneBackups, c.CheckBackups)
	}
}

func TestValidate(t *testing.T) {
	valid := func() *Config {
		return &Config{
			ServiceDirectories: []string{"/srv/*/"},
			Targets:            []Target{{N: 1, Name: "local", Type: "local", LocalPath: "/s"}},
			StoragePassword:    "longenough", RSAPassphrase: "rp",
			PruneExhaustiveFrequency: "monthly",
		}
	}
	if err := valid().Validate("/run/secrets"); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"no service dirs", func(c *Config) { c.ServiceDirectories = nil }, "SERVICE_DIRECTORIES is not set"},
		{"no targets", func(c *Config) { c.Targets = nil }, "No storage targets specified"},
		{"no type", func(c *Config) { c.Targets[0].Type = "" }, "Missing storage name or type for storage target 1"},
		{"local path", func(c *Config) { c.Targets[0].LocalPath = "" }, "Missing LOCAL_PATH configuration for the local storage"},
		{"sftp port", func(c *Config) {
			c.Targets[0] = Target{N: 1, Name: "s", Type: "sftp", SFTPURL: "h", SFTPUser: "u", SFTPPath: "p"}
		}, "Missing SFTP configuration setting SFTP_PORT"},
		{"b2 key", func(c *Config) { c.Targets[0] = Target{N: 1, Name: "b", Type: "b2", B2Bucket: "x", B2ID: "i"} },
			"Missing B2 secret B2_KEY for the b storage. Secrets are file-only (never env vars): provide /run/secrets/storage_target_1_b2_key or set STORAGE_TARGET_1_B2_KEY_FILE."},
		{"s3 endpoint", func(c *Config) { c.Targets[0] = Target{N: 1, Name: "s", Type: "s3", S3Bucket: "x"} }, "Missing S3 configuration setting S3_ENDPOINT"},
		{"type", func(c *Config) { c.Targets[0].Type = "ftp" }, "The storage type ftp is not supported. Please check your STORAGE_TARGET_1_TYPE configuration."},
		{"rsa", func(c *Config) { c.RSAPassphrase = "" }, "The required secret RSA_PASSPHRASE is not set"},
		{"short password", func(c *Config) { c.StoragePassword = "seven77" }, "STORAGE_PASSWORD must be at least 8 characters (a Duplicacy requirement); got 7."},
		{"pushover", func(c *Config) { c.NotificationService = "PushOver"; c.PushoverUserKey = "u" }, "Notification service is set to PushOver, but PUSHOVER_API_TOKEN is not set"},
		{"frequency", func(c *Config) { c.PruneExhaustiveFrequency = "hourly" }, "PRUNE_EXHAUSTIVE_FREQUENCY must be one of"},
	} {
		c := valid()
		tc.mutate(c)
		if err := c.Validate("/run/secrets"); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", tc.name, err, tc.want)
		}
	}
}

func mkdirs(t *testing.T, root string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// The cases of tests/service_directories.bats, against the Go expansion.
func TestExpandServiceDirectories(t *testing.T) {
	svc := t.TempDir()
	mkdirs(t, svc, "app1", "app2", "other", ".hidden", "customer[1]", "with space", "]x", "a-b", "[open", "\xc3\xa9", "z", ".dot")
	os.WriteFile(filepath.Join(svc, "app1", "notadir.txt"), nil, 0o644)
	s := func(p string) string { return svc + "/" + p }

	for _, tc := range []struct {
		name          string
		patterns      []string
		dirs, unmatch []string
	}{
		{"trailing-slash glob, no dot dirs, sorted", []string{s("*/")},
			[]string{s("[open"), s("]x"), s("a-b"), s("app1"), s("app2"), s("customer[1]"), s("other"), s("with space"), s("z"), s("\xc3\xa9")}, nil},
		{"files are skipped", []string{s("app1/*")}, nil, []string{s("app1/*")}},
		{"glob and literal mix", []string{s("app*/"), s("other/")}, []string{s("app1"), s("app2"), s("other")}, nil},
		{"unmatched recorded", []string{s("nomatch*/"), s("app1/"), s("missing/")}, []string{s("app1")}, []string{s("nomatch*/"), s("missing/")}},
		{"literal glob characters", []string{s("customer[1]/")}, []string{s("customer[1]")}, nil},
		{"space stays one entry", []string{s("with space/")}, []string{s("with space")}, nil},
		{"leading-dot pattern matches dot dirs", []string{s(".h*/")}, []string{s(".hidden")}, nil},
		{"bash negation and classes", []string{s("app[![:alpha:]2]/"), s("[[:lower:]]ther/")}, []string{s("app1"), s("other")}, nil},
		{"literal ] first in a class", []string{s("[]o]*/")}, []string{s("]x"), s("other")}, nil},
		{"literal - first and last", []string{s("a[-]b/"), s("a[x-]b/")}, []string{s("a-b"), s("a-b")}, nil},
		{"unclosed [ is literal", []string{s("[open/")}, []string{s("[open")}, nil},
		{"? is one byte (C locale)", []string{s("?/")}, []string{s("z")}, nil},
		{"every POSIX class", []string{s("[[:graph:]]/"), s("[[:print:]]/"), s("[[:word:]]/")}, []string{s("z"), s("z"), s("z")}, nil},
		{"equivalence class and collating symbol", []string{s("app[[=1=]]/"), s("app[[.2.]]/")}, []string{s("app1"), s("app2")}, nil},
		{"escaped leading dot matches dot dirs", []string{s(`\.d*/`)}, []string{s(".dot")}, nil},
	} {
		dirs, unmatched := ExpandServiceDirectories(tc.patterns)
		if !slices.Equal(dirs, tc.dirs) || !slices.Equal(unmatched, tc.unmatch) {
			t.Errorf("%s:\n dirs %q, want %q\n unmatched %q, want %q", tc.name, dirs, tc.dirs, unmatched, tc.unmatch)
		}
	}
}
