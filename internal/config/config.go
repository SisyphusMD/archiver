// Package config resolves Archiver's env-native configuration: plain environment variables
// for settings, and files for secrets (ADR 4). The storage names and the DUPLICACY_<NAME>_*
// variables they map to address existing storages and must never change (never-break list).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
	// Values are the type's settings and secrets by field name (Types); a secret passed as
	// a file path also has <NAME>_FILE.
	Values Values
	// CheckInterval is STORAGE_TARGET_<N>_CHECK_INTERVAL, empty for the default.
	CheckInterval string
}

// Get returns one of the target's values.
func (t Target) Get(name string) string { return t.Values[name] }

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
	Parallelism              string // BACKUP_PARALLELISM: services backed up at once
	HooksDir                 string // HOOKS_DIR: hooks kept outside the backed-up data (ADR 45)
}

var globalSecret = regexp.MustCompile(`^(STORAGE_PASSWORD|RSA_PASSPHRASE|RECOVERY_PASSWORD|PUSHOVER_USER_KEY|PUSHOVER_API_TOKEN)$`)
var targetVar = regexp.MustCompile(`^STORAGE_TARGET_[0-9]+_(.+)$`)

// IsSecret reports whether a variable name is a secret, which is read only from a file: a
// global one, a storage type's secret field, its break-glass variant, or the break-glass SSH key.
func IsSecret(name string) bool {
	if globalSecret.MatchString(name) {
		return true
	}
	m := targetVar.FindStringSubmatch(name)
	if m == nil {
		return false
	}
	field := strings.TrimPrefix(m[1], "BREAKGLASS_")
	if m[1] == "BREAKGLASS_SSH_KEY" {
		return true
	}
	for _, t := range Types {
		for _, f := range t.Fields {
			if f.Secret && f.Name == field && (field == m[1] || f.BreakGlass) {
				return true
			}
		}
	}
	return false
}

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
		Parallelism:              src.Getenv("BACKUP_PARALLELISM"),
		HooksDir:                 src.Getenv("HOOKS_DIR"),
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

	// Targets are numbered from 1; the first missing name ends the list.
	for n := 1; ; n++ {
		p := "STORAGE_TARGET_" + strconv.Itoa(n) + "_"
		t := Target{N: n, Name: src.Getenv(p + "NAME")}
		if t.Name == "" {
			break
		}
		t.Type = src.Getenv(p + "TYPE")
		t.CheckInterval = src.Getenv(p + "CHECK_INTERVAL")
		t.Values = Values{}
		for _, f := range Types[t.Type].Fields {
			if f.Secret {
				t.Values[f.Name] = secret(p + f.Name)
				if f.Path {
					t.Values[f.Name+"_FILE"] = secretPath(src, p+f.Name)
				}
				continue
			}
			v := src.Getenv(p + f.Name)
			if v == "" {
				v = f.Default
			}
			t.Values[f.Name] = v
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

// secretPath is where a secret is read from: <NAME>_FILE if set, else <secrets dir>/<name>.
func secretPath(src Source, name string) string {
	if p := src.Getenv(name + "_FILE"); p != "" {
		return p
	}
	return filepath.Join(src.SecretsDir, strings.ToLower(name))
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

// Validate reports the first configuration error a run must refuse to start with.
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
	if _, err := c.BackupParallelism(); err != nil {
		return err
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
	// Here rather than in Validate alone: a restore validates only this, and a missing
	// hooks mount would otherwise skip its restore hook as though there were none.
	if c.HooksDir != "" {
		if !filepath.IsAbs(c.HooksDir) {
			return fmt.Errorf("HOOKS_DIR must be an absolute path (got '%s').", c.HooksDir)
		}
		if fi, err := os.Stat(c.HooksDir); err != nil || !fi.IsDir() {
			return fmt.Errorf("HOOKS_DIR '%s' is not a directory; mount it or unset HOOKS_DIR.", c.HooksDir)
		}
	}
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
		if !slices.Contains(StorageTypes, t.Type) {
			return fmt.Errorf("The storage type %s is not supported. Please check your %sTYPE configuration.", t.Type, p)
		}
		for _, f := range Types[t.Type].Fields {
			if f.Optional || f.Default != "" || t.Values[f.Name] != "" {
				continue
			}
			if f.Secret {
				return missingSecret(strings.ToUpper(t.Type), f.Name)
			}
			return missing(strings.ToUpper(t.Type), f.Name)
		}
		if req := Types[t.Type].Require; req != nil {
			if name := req(t.withDefaults()); name != "" {
				return missing(strings.ToUpper(t.Type), name)
			}
		}
	}
	for _, s := range []struct{ name, v string }{{"STORAGE_PASSWORD", c.StoragePassword}, {"RSA_PASSPHRASE", c.RSAPassphrase}} {
		if s.v == "" {
			return fmt.Errorf("The required secret %s is not set. Provide it via %s_FILE or %s.", s.name, s.name, secretFile(s.name))
		}
	}
	// Duplicacy rejects a shorter one only at init, with an opaque message. Bytes count,
	// as they always have.
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
	if d := Types[t.Type].CheckInterval; d > 0 {
		return d
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

// BackupParallelism is how many services a backup processes at once: BACKUP_PARALLELISM,
// 2 by default. 1 backs them up one after another.
func (c *Config) BackupParallelism() (int, error) {
	if c.Parallelism == "" {
		return 2, nil
	}
	n, err := strconv.Atoi(c.Parallelism)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("BACKUP_PARALLELISM must be a whole number of at least 1 (got '%s').", c.Parallelism)
	}
	return n, nil
}
