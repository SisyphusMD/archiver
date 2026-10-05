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
}

// Exists reports whether dir has the named hook. A path that exists but is not an
// executable regular file is an error, so a hook missing its execute bit is not silently
// skipped.
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
	return true, nil
}

var credentialVar = regexp.MustCompile(`^DUPLICACY_([A-Z0-9_]+_)?(PASSWORD|RSA_PASSPHRASE|B2_ID|B2_KEY|S3_ID|S3_SECRET|SSH_KEY_FILE)$`)

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

// Run runs one hook and returns its exit code.
func Run(log *logging.Log, base []string, s Service, name, result string) (int, error) {
	return proc.Run(proc.Spec{
		Path:    filepath.Join(s.Dir, name),
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
