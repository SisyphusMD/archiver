// Package hooks runs a service's executable hooks and reads its filters (ADR 20). A hook is
// any executable the container can run, started in the service directory with the
// ARCHIVER_* variables below; its exit code decides the service's outcome and its output
// goes to the log, a line starting "[ERROR] " or "[WARNING] " at that level. It never sees
// a storage secret or the RSA passphrase.
package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/proc"
)

// File names in a service directory.
const (
	PreBackup  = "pre-backup"
	PostBackup = "post-backup"
	Filters    = "filters"
	// Legacy is the sourced-bash settings file the Go pipeline does not run.
	Legacy = "service-backup-settings.sh"
	// PostRestore runs after a restore, when asked for; LegacyRestore is its old name, a
	// script run with bash when no PostRestore is present.
	PostRestore   = "post-restore"
	LegacyRestore = "restore-service.sh"
)

// Results passed to post-backup in ARCHIVER_BACKUP_RESULT.
const (
	Success = "success"
	Failed  = "failed"
	Skipped = "skipped" // the pre-backup hook failed, so nothing was backed up
	Stopped = "stopped"
)

// Service is what a hook is told about the service it runs for.
type Service struct {
	Name       string
	Dir        string
	SnapshotID string
	StateDir   string // scratch space shared by this run's pre- and post-backup hooks
	// HookDir holds the service's hooks: Dir, or HOOKS_DIR/<Name> when HOOKS_DIR is set.
	HookDir string
}

// Hooks is where a service's hooks live (ADR 45): HOOKS_DIR/<service> when hooksDir is
// set, else the service directory itself.
func Hooks(hooksDir, serviceDir string) string {
	if hooksDir == "" {
		return serviceDir
	}
	// Absolute, so the hook checked is the hook run (a hook runs in its service directory).
	abs, err := filepath.Abs(hooksDir)
	if err != nil {
		abs = hooksDir
	}
	return filepath.Join(abs, filepath.Base(serviceDir))
}

// Exists reports whether dir has the named hook. A path that exists but is not an
// executable regular file is an error, so a hook missing its execute bit is not silently
// skipped; so is one that someone other than its owner could change (Safe).
func Exists(dir, name string) (bool, error) {
	path := filepath.Join(dir, name)
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return false, nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("%s cannot be read (a broken symlink?): %v", name, err)
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm()&0o111 == 0 {
		return false, fmt.Errorf("%s exists but is not an executable file (chmod +x %s)", name, filepath.Join(dir, name))
	}
	if err := Safe(path); err != nil {
		return false, err
	}
	return true, nil
}

// Safe refuses a file that root would run although someone other than its owner could
// change it (ADR 45): the file, or any directory above it, writable by group or others,
// since whoever can write a directory can swap what is in it. A sticky directory (/tmp)
// is fine: nobody can rename another user's entry in it. A symlinked hook's target is
// checked the same way. Like sshd's StrictModes, it does not check who the owner is, so a
// service's own user may keep its hooks.
func Safe(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	hook := filepath.Base(path)
	unsafe := func(q string, mode os.FileMode) error {
		return fmt.Errorf("%s is not run because %s can be changed by users other than its owner (mode %o): run 'chmod go-w %s', or keep hooks in HOOKS_DIR", hook, q, mode.Perm(), q)
	}
	// Resolved one component at a time as the kernel does (a ".." after a symlinked
	// directory leaves its target, not the link), checking every directory an entry on the
	// way is read from: whoever can write one can swap that entry.
	cur := "/"
	rest := strings.Split(strings.TrimPrefix(abs, "/"), "/")
	for hops := 0; len(rest) > 0; {
		c := rest[0]
		rest = rest[1:]
		switch c {
		case "", ".":
			continue
		case "..":
			cur = filepath.Dir(cur)
			continue
		}
		fi, err := os.Stat(cur)
		if err != nil {
			return err
		}
		if fi.Mode().Perm()&0o022 != 0 && fi.Mode()&os.ModeSticky == 0 {
			return unsafe(cur, fi.Mode())
		}
		next := filepath.Join(cur, c)
		li, err := os.Lstat(next)
		if err != nil {
			return err
		}
		if li.Mode()&os.ModeSymlink == 0 {
			cur = next
			continue
		}
		if hops++; hops > 40 {
			return fmt.Errorf("%s is not run: too many symlinks on its path", hook)
		}
		t, err := os.Readlink(next)
		if err != nil {
			return err
		}
		if filepath.IsAbs(t) {
			cur = "/"
		}
		rest = append(strings.Split(t, "/"), rest...)
	}
	fi, err := os.Stat(cur)
	if err != nil {
		return err
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return unsafe(cur, fi.Mode())
	}
	return nil
}

// credentialVar matches the variables Duplicacy reads a storage's credentials from: its
// password, RSA passphrase, SSH key file, and every storage type's credential keys.
var credentialVar = func() *regexp.Regexp {
	keys := []string{"PASSWORD", "RSA_PASSPHRASE", "SSH_KEY_FILE"}
	for _, t := range config.Types {
		for _, f := range t.Fields {
			if f.Key != "" {
				keys = append(keys, regexp.QuoteMeta(strings.ToUpper(f.Key)))
			}
		}
	}
	return regexp.MustCompile(`^DUPLICACY_([A-Z0-9_]+_)?(` + strings.Join(keys, "|") + `)$`)
}()

// withoutSecrets is base without any secret, Duplicacy credential, or ARCHIVER_ variable.
func withoutSecrets(base []string) []string {
	var env []string
	for _, kv := range base {
		name, _, _ := strings.Cut(kv, "=")
		if config.IsSecret(name) || credentialVar.MatchString(name) || strings.HasPrefix(name, "ARCHIVER_") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// Environ is base without any secret or Duplicacy credential, plus the hook's variables.
func Environ(base []string, s Service, result string) []string {
	env := append(withoutSecrets(base),
		"ARCHIVER_SERVICE="+s.Name,
		"ARCHIVER_SERVICE_DIR="+s.Dir,
		"ARCHIVER_SNAPSHOT_ID="+s.SnapshotID,
		"ARCHIVER_STATE_DIR="+s.StateDir,
	)
	if result != "" {
		env = append(env, "ARCHIVER_BACKUP_RESULT="+result)
	}
	return env
}

// RestoreEnviron is the environment of a post-restore hook: Environ without the backup's
// variables, plus the revision restored and the storage it came from.
func RestoreEnviron(base []string, s Service, revision int, storage string) []string {
	return append(withoutSecrets(base),
		"ARCHIVER_SERVICE="+s.Name,
		"ARCHIVER_SERVICE_DIR="+s.Dir,
		"ARCHIVER_SNAPSHOT_ID="+s.SnapshotID,
		fmt.Sprintf("ARCHIVER_RESTORE_REVISION=%d", revision),
		"ARCHIVER_RESTORE_STORAGE="+storage,
	)
}

func (s Service) hookDir() string {
	if s.HookDir != "" {
		return s.HookDir
	}
	return s.Dir
}

// Run runs one hook and returns its exit code.
func Run(log *logging.Log, base []string, s Service, name, result string) (int, error) {
	return proc.Run(proc.Spec{
		Path:    filepath.Join(s.hookDir(), name),
		Dir:     s.Dir,
		Env:     Environ(base, s, result),
		Log:     log,
		Service: s.Name,
		Hook:    true,
	})
}

// ReadFilters returns the service's filter lines, or the default of backing up everything.
func ReadFilters(dir string) ([]string, error) {
	path := filepath.Join(dir, Filters)
	// Only a missing entry means "everything": a broken symlink must not drop the filters.
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return []string{"+*"}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}
