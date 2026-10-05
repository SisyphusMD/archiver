// Package config resolves Archiver's env-native configuration: plain environment variables
// for settings, and files for secrets (ADR 4). It must resolve exactly what
// lib/core/config-loader.sh does, above all the storage names and the DUPLICACY_<NAME>_*
// variables they map to, which address existing storages (never-break list).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Source is where configuration comes from. Tests supply their own.
type Source struct {
	Getenv     func(string) string
	SecretsDir string // /run/secrets unless SECRETS_DIR says otherwise
}

// FromEnvironment reads the process environment.
func FromEnvironment() Source {
	dir := os.Getenv("SECRETS_DIR")
	if dir == "" {
		dir = "/run/secrets"
	}
	return Source{Getenv: os.Getenv, SecretsDir: dir}
}

// Target is one STORAGE_TARGET_<N>_* storage. N is 1 for the primary.
type Target struct {
	N    int
	Name string
	Type string

	LocalPath  string
	SFTPURL    string
	SFTPPort   string
	SFTPUser   string
	SFTPPath   string
	B2Bucket   string
	S3Bucket   string
	S3Endpoint string
	S3Region   string
	// CheckInterval is STORAGE_TARGET_<N>_CHECK_INTERVAL, empty for the default.
	CheckInterval string

	B2ID     string
	B2Key    string
	S3ID     string
	S3Secret string
}

// Config is the resolved configuration.
type Config struct {
	ServiceDirectories []string // patterns, in order, before glob expansion
	Targets            []Target

	StoragePassword  string
	RSAPassphrase    string
	RecoveryPassword string

	NotificationService string // "Pushover" or "None", as configured
	PushoverUserKey     string
	PushoverAPIToken    string

	PruneBackups             bool
	CheckBackups             bool
	PruneKeep                string
	PruneExhaustiveFrequency string
	Threads                  string
	CheckInterval            string // CHECK_INTERVAL: the default for every target
}

var secretVar = regexp.MustCompile(`^(STORAGE_PASSWORD|RSA_PASSPHRASE|RECOVERY_PASSWORD|PUSHOVER_USER_KEY|PUSHOVER_API_TOKEN|STORAGE_TARGET_[0-9]+_(B2_ID|B2_KEY|S3_ID|S3_SECRET|BREAKGLASS_(B2_ID|B2_KEY|S3_ID|S3_SECRET|SSH_KEY)))$`)

// IsSecret reports whether a variable name is a secret, which is read only from a file.
func IsSecret(name string) bool { return secretVar.MatchString(name) }

// Load resolves the configuration. Secrets passed as plain environment variables are
// ignored, with a warning each: they would leak through /proc and docker inspect. Load does
// not validate; call Validate before using the result for a run.
func Load(src Source, environ []string) (*Config, []string, error) {
	var warnings []string
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if IsSecret(name) {
			warnings = append(warnings, fmt.Sprintf("Ignoring %s from the environment: secrets are file-only. Put it in %s (or point %s_FILE at it) and remove the env var.",
				name, filepath.Join(src.SecretsDir, strings.ToLower(name)), name))
		}
	}

	var err error
	secret := func(name string) string {
		if err != nil {
			return ""
		}
		var v string
		v, err = readSecret(src, name)
		return v
	}

	c := &Config{
		ServiceDirectories:       splitServiceDirectories(src.Getenv("SERVICE_DIRECTORIES")),
		StoragePassword:          secret("STORAGE_PASSWORD"),
		RSAPassphrase:            secret("RSA_PASSPHRASE"),
		RecoveryPassword:         secret("RECOVERY_PASSWORD"),
		NotificationService:      src.Getenv("NOTIFICATION_SERVICE"),
		PushoverUserKey:          secret("PUSHOVER_USER_KEY"),
		PushoverAPIToken:         secret("PUSHOVER_API_TOKEN"),
		PruneKeep:                src.Getenv("PRUNE_KEEP"),
		PruneExhaustiveFrequency: strings.ToLower(src.Getenv("PRUNE_EXHAUSTIVE_FREQUENCY")),
		Threads:                  src.Getenv("DUPLICACY_THREADS"),
		CheckInterval:            src.Getenv("CHECK_INTERVAL"),
	}

	prune := src.Getenv("PRUNE_BACKUPS")
	if prune == "" {
		prune = src.Getenv("ROTATE_BACKUPS")
	}
	c.PruneBackups = boolSetting(prune)
	c.CheckBackups = boolSetting(src.Getenv("CHECK_BACKUPS"))
	if c.PruneKeep == "" {
		c.PruneKeep = "-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1"
	}
	if c.PruneExhaustiveFrequency == "" {
		c.PruneExhaustiveFrequency = "monthly"
	}
	if c.Threads == "" {
		c.Threads = "4"
	}

	// Targets are numbered from 1; the first missing name ends the list, as in bash.
	for n := 1; ; n++ {
		p := "STORAGE_TARGET_" + strconv.Itoa(n) + "_"
		t := Target{N: n, Name: src.Getenv(p + "NAME")}
		if t.Name == "" {
			break
		}
		t.Type = src.Getenv(p + "TYPE")
		t.LocalPath = src.Getenv(p + "LOCAL_PATH")
		t.SFTPURL = src.Getenv(p + "SFTP_URL")
		t.SFTPPort = src.Getenv(p + "SFTP_PORT")
		t.SFTPUser = src.Getenv(p + "SFTP_USER")
		t.SFTPPath = src.Getenv(p + "SFTP_PATH")
		t.B2Bucket = src.Getenv(p + "B2_BUCKETNAME")
		t.S3Bucket = src.Getenv(p + "S3_BUCKETNAME")
		t.S3Endpoint = src.Getenv(p + "S3_ENDPOINT")
		t.S3Region = src.Getenv(p + "S3_REGION")
		t.CheckInterval = src.Getenv(p + "CHECK_INTERVAL")
		switch t.Type {
		case "b2":
			t.B2ID, t.B2Key = secret(p+"B2_ID"), secret(p+"B2_KEY")
		case "s3":
			t.S3ID, t.S3Secret = secret(p+"S3_ID"), secret(p+"S3_SECRET")
		}
		c.Targets = append(c.Targets, t)
	}
	if err != nil {
		return nil, warnings, err
	}
	return c, warnings, nil
}

// boolSetting is "true" in any case, with true as the default; anything else is false.
func boolSetting(v string) bool {
	return v == "" || strings.ToLower(v) == "true"
}

// readSecret reads <NAME>_FILE if set, else <secrets dir>/<name>. A missing default file
// leaves the secret empty; an explicitly named file that is missing is an error. Trailing
// newlines go (as $(<file) drops them), then one carriage return (a CRLF-edited file).
func readSecret(src Source, name string) (string, error) {
	path := src.Getenv(name + "_FILE")
	explicit := path != ""
	if !explicit {
		path = filepath.Join(src.SecretsDir, strings.ToLower(name))
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if !explicit && os.IsNotExist(err) {
			return "", nil
		}
		if explicit && os.IsNotExist(err) {
			return "", fmt.Errorf("%s_FILE is set to '%s', but no such file exists", name, path)
		}
		return "", fmt.Errorf("read %s: %w", name, err)
	}
	v := strings.TrimRight(string(b), "\n")
	return strings.TrimSuffix(v, "\r"), nil
}

// splitServiceDirectories splits on colons and newlines, dropping empty entries.
func splitServiceDirectories(raw string) []string {
	var out []string
	for p := range strings.SplitSeq(strings.ReplaceAll(raw, "\n", ":"), ":") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Validate reports the first configuration error a run must refuse to start with, in the
// order and words the bash loader uses.
func (c *Config) Validate(secretsDir string) error {
	if len(c.ServiceDirectories) == 0 {
		return fmt.Errorf("SERVICE_DIRECTORIES is not set. Set the SERVICE_DIRECTORIES environment variable (colon-delimited).")
	}
	if err := c.ValidateStorage(secretsDir); err != nil {
		return err
	}
	if strings.ToLower(c.NotificationService) == "pushover" {
		for _, s := range []struct{ name, v string }{{"PUSHOVER_USER_KEY", c.PushoverUserKey}, {"PUSHOVER_API_TOKEN", c.PushoverAPIToken}} {
			if s.v == "" {
				return fmt.Errorf("Notification service is set to %s, but %s is not set. Provide it as a secret file.", c.NotificationService, s.name)
			}
		}
	}
	switch c.PruneExhaustiveFrequency {
	case "off", "daily", "weekly", "monthly":
	default:
		return fmt.Errorf("PRUNE_EXHAUSTIVE_FREQUENCY must be one of: off, daily, weekly, monthly (got '%s').", c.PruneExhaustiveFrequency)
	}
	if _, err := ParseInterval(c.CheckInterval); err != nil {
		return fmt.Errorf("CHECK_INTERVAL: %v", err)
	}
	for _, t := range c.Targets {
		if _, err := ParseInterval(t.CheckInterval); err != nil {
			return fmt.Errorf("STORAGE_TARGET_%d_CHECK_INTERVAL: %v", t.N, err)
		}
	}
	return nil
}

// ValidateStorage checks only what reaching the storages needs: the targets' settings and
// credentials, the storage password and the RSA passphrase. A restore needs no more.
func (c *Config) ValidateStorage(secretsDir string) error {
	if len(c.Targets) == 0 {
		return fmt.Errorf("No storage targets specified. Provide at least one via the STORAGE_TARGET_N_* environment variables.")
	}
	secretFile := func(v string) string { return filepath.Join(secretsDir, strings.ToLower(v)) }
	for _, t := range c.Targets {
		p := "STORAGE_TARGET_" + strconv.Itoa(t.N) + "_"
		if t.Name == "" || t.Type == "" {
			return fmt.Errorf("Missing storage name or type for storage target %d. Please check your configuration.", t.N)
		}
		missing := func(kind, setting string) error {
			return fmt.Errorf("Missing %s configuration setting %s for the %s storage. Please check your 'STORAGE_TARGET_%d' configuration.", kind, setting, t.Name, t.N)
		}
		missingSecret := func(kind, setting string) error {
			return fmt.Errorf("Missing %s secret %s for the %s storage. Secrets are file-only (never env vars): provide %s or set %s%s_FILE.", kind, setting, t.Name, secretFile(p+setting), p, setting)
		}
		switch t.Type {
		case "local":
			if t.LocalPath == "" {
				return fmt.Errorf("Missing LOCAL_PATH configuration for the %s storage. Please check your 'STORAGE_TARGET_%d' configuration.", t.Name, t.N)
			}
		case "sftp":
			for _, f := range []struct{ k, v string }{{"SFTP_URL", t.SFTPURL}, {"SFTP_PORT", t.SFTPPort}, {"SFTP_USER", t.SFTPUser}, {"SFTP_PATH", t.SFTPPath}} {
				if f.v == "" {
					return missing("SFTP", f.k)
				}
			}
		case "b2":
			if t.B2Bucket == "" {
				return missing("B2", "B2_BUCKETNAME")
			}
			if t.B2ID == "" {
				return missingSecret("B2", "B2_ID")
			}
			if t.B2Key == "" {
				return missingSecret("B2", "B2_KEY")
			}
		case "s3":
			if t.S3Bucket == "" {
				return missing("S3", "S3_BUCKETNAME")
			}
			if t.S3Endpoint == "" {
				return missing("S3", "S3_ENDPOINT")
			}
			if t.S3ID == "" {
				return missingSecret("S3", "S3_ID")
			}
			if t.S3Secret == "" {
				return missingSecret("S3", "S3_SECRET")
			}
		default:
			return fmt.Errorf("The storage type %s is not supported. Please check your %sTYPE configuration.", t.Type, p)
		}
	}
	for _, s := range []struct{ name, v string }{{"STORAGE_PASSWORD", c.StoragePassword}, {"RSA_PASSPHRASE", c.RSAPassphrase}} {
		if s.v == "" {
			return fmt.Errorf("The required secret %s is not set. Provide it via %s_FILE or %s.", s.name, s.name, secretFile(s.name))
		}
	}
	// Duplicacy rejects a shorter one only at init, with an opaque message. bash counts
	// bytes here, since the image runs in the C locale.
	if len(c.StoragePassword) < 8 {
		return fmt.Errorf("STORAGE_PASSWORD must be at least 8 characters (a Duplicacy requirement); got %d.", len(c.StoragePassword))
	}
	return nil
}

// ParseInterval reads an interval such as "7d", "12h" or "90m"; empty is zero (unset).
func ParseInterval(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("'%s' is not an interval (use e.g. 1d, 7d, 12h)", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("'%s' is not an interval (use e.g. 1d, 7d, 12h)", s)
	}
	return d, nil
}

// TargetCheckInterval is how often a secondary is checked (ADR 17): its own setting, else
// CHECK_INTERVAL, else a week for SFTP (where a check lists every chunk, for hours) and a
// day for every other type. Zero when CHECK_BACKUPS is off.
func (c *Config) TargetCheckInterval(t Target) time.Duration {
	if !c.CheckBackups {
		return 0
	}
	for _, s := range []string{t.CheckInterval, c.CheckInterval} {
		if d, err := ParseInterval(s); err == nil && d > 0 {
			return d
		}
	}
	if t.Type == "sftp" {
		return 7 * 24 * time.Hour
	}
	return 24 * time.Hour
}

// ExhaustiveInterval is PRUNE_EXHAUSTIVE_FREQUENCY as a duration; zero is never.
func (c *Config) ExhaustiveInterval() time.Duration {
	switch c.PruneExhaustiveFrequency {
	case "daily":
		return 24 * time.Hour
	case "weekly":
		return 7 * 24 * time.Hour
	case "monthly":
		return 30 * 24 * time.Hour
	}
	return 0
}

// Pushover reports whether notifications go to Pushover.
func (c *Config) Pushover() bool { return strings.ToLower(c.NotificationService) == "pushover" }
