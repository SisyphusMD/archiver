package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var globalSetting = regexp.MustCompile(`^(SERVICE_DIRECTORIES|ROTATE_BACKUPS|PRUNE_BACKUPS|CHECK_BACKUPS|PRUNE_KEEP|PRUNE_EXHAUSTIVE_FREQUENCY|CHECK_INTERVAL|ALERT_REPEAT_INTERVAL|METRICS_PORT|DUPLICACY_THREADS|BACKUP_PARALLELISM|HOOKS_DIR|RESTORE_DRILL_SERVICES|RESTORE_DRILL_STORAGES|RESTORE_DRILL_DIR|RESTORE_DRILL_EXCLUDE|LOG_FORMAT|NOTIFICATION_SERVICE|NOTIFY_ON|APPRISE_NOTIFY_ON|NTFY_NOTIFY_ON|PUSHOVER_NOTIFY_ON|APPRISE_TAGS|NTFY_URL|RECOVERY_KIT_EXTRA_PATHS)$`)

// IsSetting reports whether a variable name is a non-secret setting: a global one, or a
// storage target's name, type, check interval, break-glass SFTP user, or any storage type's
// non-secret field.
func IsSetting(name string) bool {
	if globalSetting.MatchString(name) {
		return true
	}
	m := targetVar.FindStringSubmatch(name)
	if m == nil {
		return false
	}
	switch m[1] {
	case "NAME", "TYPE", "CHECK_INTERVAL", "BREAKGLASS_SFTP_USER":
		return true
	}
	for _, t := range Types {
		for _, f := range t.Fields {
			if !f.Secret && f.Name == m[1] {
				return true
			}
		}
	}
	return false
}

// Setting is one configuration variable and its value.
type Setting struct{ Name, Value string }

// Settings is the configuration as provided: what
// the recovery kit (and init) serialize. Settings are in version order (STORAGE_TARGET_2_*
// before STORAGE_TARGET_10_*); secrets are the ones whose file exists, read only for a
// target of the type that uses them.
type Settings struct {
	ServiceDirectories string // the colon-joined, non-empty entries
	NonSecret          []Setting
	Secrets            []Setting
}

// Snapshot reads Settings as they are recorded: deprecated ROTATE_BACKUPS becomes
// PRUNE_BACKUPS, DUPLICACY_THREADS defaults to 4, newlines in RECOVERY_KIT_EXTRA_PATHS become
// colons, and a variable set but empty in the environment is kept.
func Snapshot(src Source, environ []string) (*Settings, error) {
	vals := map[string]string{}
	for _, kv := range environ {
		name, v, ok := strings.Cut(kv, "=")
		if ok && IsSetting(name) {
			vals[name] = v
		}
	}
	if _, ok := vals["PRUNE_BACKUPS"]; (!ok || vals["PRUNE_BACKUPS"] == "") && vals["ROTATE_BACKUPS"] != "" {
		vals["PRUNE_BACKUPS"] = vals["ROTATE_BACKUPS"]
	}
	delete(vals, "ROTATE_BACKUPS")
	if vals["DUPLICACY_THREADS"] == "" {
		vals["DUPLICACY_THREADS"] = "4"
	}
	if v, ok := vals["RECOVERY_KIT_EXTRA_PATHS"]; ok && v != "" {
		vals["RECOVERY_KIT_EXTRA_PATHS"] = strings.ReplaceAll(v, "\n", ":")
	}
	s := &Settings{ServiceDirectories: strings.Join(splitServiceDirectories(vals["SERVICE_DIRECTORIES"]), ":")}
	if _, ok := vals["SERVICE_DIRECTORIES"]; ok {
		vals["SERVICE_DIRECTORIES"] = s.ServiceDirectories
	}
	for name, v := range vals {
		s.NonSecret = append(s.NonSecret, Setting{name, v})
	}
	sort.Slice(s.NonSecret, func(i, j int) bool { return versionLess(s.NonSecret[i].Name, s.NonSecret[j].Name) })

	names := []string{"STORAGE_PASSWORD", "RSA_PASSPHRASE", "RECOVERY_PASSWORD", "PUSHOVER_USER_KEY", "PUSHOVER_API_TOKEN", "APPRISE_URL", "NTFY_TOKEN"}
	rotating := map[string]string{}
	for n := 1; src.Getenv(fmt.Sprintf("STORAGE_TARGET_%d_NAME", n)) != ""; n++ {
		p := fmt.Sprintf("STORAGE_TARGET_%d_", n)
		typ := src.Getenv(p + "TYPE")
		var breakglass []string
		for _, f := range Types[typ].Fields {
			if f.Rotates {
				rotating[p+f.Name] = tokenCopy(Target{Name: src.Getenv(p + "NAME")}, f)
			}
			if f.Secret {
				names = append(names, p+f.Name)
				if f.BreakGlass {
					breakglass = append(breakglass, p+"BREAKGLASS_"+f.Name)
				}
			}
		}
		names = append(names, breakglass...)
		if typ == "sftp" || typ == "sftpc" {
			names = append(names, p+"BREAKGLASS_SSH_KEY")
		}
	}
	for _, name := range names {
		v, ok, err := secretFile(src, name)
		if err != nil {
			return nil, err
		}
		if ok {
			// A token Duplicacy has refreshed since it was configured: the kit must carry the
			// current one, the original may no longer work.
			if path, rotates := rotating[name]; rotates {
				v = string(currentToken(path, []byte(v)))
			}
			s.Secrets = append(s.Secrets, Setting{name, v})
		}
	}
	return s, nil
}

// Get returns a non-secret setting's value and whether it is set.
func (s *Settings) Get(name string) (string, bool) {
	for _, kv := range s.NonSecret {
		if kv.Name == name {
			return kv.Value, true
		}
	}
	return "", false
}

// secretFile is readSecret that also tells a missing default file (ok false) from an empty one.
func secretFile(src Source, name string) (string, bool, error) {
	path := src.Getenv(name + "_FILE")
	if path == "" {
		path = filepath.Join(src.SecretsDir, strings.ToLower(name))
		if fi, err := os.Stat(path); err != nil || !fi.Mode().IsRegular() {
			return "", false, nil
		}
	}
	v, err := readSecret(src, name)
	return v, err == nil, err
}

// versionLess orders names as `sort -V` does here: runs of digits compare as numbers.
func versionLess(a, b string) bool {
	for a != "" && b != "" {
		da, db := digitRun(a), digitRun(b)
		if da > 0 && db > 0 {
			na, _ := strconv.Atoi(a[:da])
			nb, _ := strconv.Atoi(b[:db])
			if na != nb {
				return na < nb
			}
			a, b = a[da:], b[db:]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func digitRun(s string) int {
	n := 0
	for n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	return n
}

// WriteEnvAndSecrets serializes s as config-serialize.sh does: envFile holds
// SERVICE_DIRECTORIES first and then every other setting, NAME=value; secretsDir (0700)
// holds one 0600 file per secret, named in lower case, plus the key files present in keys.
func (s *Settings) WriteEnvAndSecrets(envFile, secretsDir string, keys map[string]string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "SERVICE_DIRECTORIES=%s\n", s.ServiceDirectories)
	for _, kv := range s.NonSecret {
		if kv.Name != "SERVICE_DIRECTORIES" {
			fmt.Fprintf(&b, "%s=%s\n", kv.Name, kv.Value)
		}
	}
	if err := os.WriteFile(envFile, []byte(b.String()), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(secretsDir, 0o700); err != nil {
		return err
	}
	for _, kv := range s.Secrets {
		if err := writeMode(filepath.Join(secretsDir, strings.ToLower(kv.Name)), []byte(kv.Value), 0o600); err != nil {
			return err
		}
	}
	// keys maps a source key file to its /run/secrets name; public keys are world-readable.
	for _, name := range []string{"rsa_private_key", "rsa_public_key", "ssh_private_key", "ssh_public_key"} {
		src, ok := keys[name]
		if !ok {
			continue
		}
		data, err := os.ReadFile(src)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		mode := os.FileMode(0o600)
		if strings.HasSuffix(name, "_public_key") {
			mode = 0o644
		}
		if err := writeMode(filepath.Join(secretsDir, name), data, mode); err != nil {
			return err
		}
	}
	return nil
}

func writeMode(path string, data []byte, mode os.FileMode) error {
	if err := os.WriteFile(path, data, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

// NewSettings is Settings from values (settings and secrets by name; any other name is
// ignored), with SERVICE_DIRECTORIES given as its entries.
func NewSettings(serviceDirectories []string, values map[string]string) *Settings {
	s := &Settings{ServiceDirectories: strings.Join(serviceDirectories, ":")}
	for name, v := range values {
		switch {
		case IsSetting(name) && name != "SERVICE_DIRECTORIES":
			s.NonSecret = append(s.NonSecret, Setting{name, v})
		case IsSecret(name):
			s.Secrets = append(s.Secrets, Setting{name, v})
		}
	}
	s.NonSecret = append(s.NonSecret, Setting{"SERVICE_DIRECTORIES", s.ServiceDirectories})
	sort.Slice(s.NonSecret, func(i, j int) bool { return versionLess(s.NonSecret[i].Name, s.NonSecret[j].Name) })
	sort.Slice(s.Secrets, func(i, j int) bool { return versionLess(s.Secrets[i].Name, s.Secrets[j].Name) })
	return s
}

// Secret returns a secret's value, empty when it is not set.
func (s *Settings) Secret(name string) string {
	for _, kv := range s.Secrets {
		if kv.Name == name {
			return kv.Value
		}
	}
	return ""
}
