// Package kit implements the recovery kit (ADR 10): the configuration as provided (settings,
// every secret, the keys), recreation notes, any mounted deployment manifests and the
// RECOVERY_KIT_EXTRA_PATHS files, encrypted with the recovery password into one file placed
// beside the backups on every storage. One reachable storage and the password recover
// everything: `openssl enc -d -aes-256-cbc -pbkdf2 -in <kit> | tar -xvf -`. Nothing reads the
// kit back, so it is never a dependency of a run.
//
// The kit is re-placed only when its content changes, when a storage lacks it, or when
// forced. The state file and fingerprint are the ones existing deployments recorded, so an
// upgrade re-places nothing.
package kit

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/logging"
)

// Exit codes of a kit run.
const (
	OK              = 0
	Failed          = 1 // the primary failed, or the kit could not be built
	Unverified      = 2 // placed everywhere, but a placement is less readable than the storage's own files
	SecondaryFailed = 3 // on the primary, but a secondary upload failed
)

// stateVersion versions the placement scheme inside the recorded fingerprint: bumping it
// re-places every kit once, content unchanged.
const stateVersion = "5"

// Run is one kit refresh.
type Run struct {
	Layout   layout.Layout
	Source   config.Source
	Environ  []string
	Hostname string
	Force    bool
	// Skip names (sanitized) the secondaries a copy worker reports failing: the backup must not
	// wait on them. Unrecorded, they get the kit on a later run.
	Skip []string
	// PrimaryMarker, when set, is created once the primary holds the current kit, so the
	// backup can tell a later failure or timeout is a secondary's.
	PrimaryMarker string
	// EnvelopeCheck runs the envelope check; nil skips it.
	EnvelopeCheck func() error
	Log           *logging.Log
	DockerSocket  string // tests point it elsewhere

	cfg *config.Config
}

func (r *Run) getenv(name string) string { return r.Source.Getenv(name) }

// KitName is the kit's file name, after the same host as the snapshot IDs: an inherited
// HOSTNAME wins over the kernel's, or every Kubernetes Job run would leave a new kit behind.
func (r *Run) KitName() string    { return "archiver-recovery-kit-" + r.Hostname + ".tar.enc" }
func (r *Run) readmeName() string { return "archiver-recovery-kit-" + r.Hostname + ".README.txt" }

func (r *Run) statePath() string { return filepath.Join(r.Layout.LogDir(), ".recovery-kit-state") }

func (r *Run) info(msg string)    { r.Log.Message(logging.Info, "", msg) }
func (r *Run) warning(msg string) { r.Log.Message(logging.Warning, "", msg) }
func (r *Run) error(msg string)   { r.Log.Message(logging.Error, "", msg) }

// ValidatePassword reports why a recovery password cannot protect the kit, if it cannot.
func ValidatePassword(recovery, storage string) error {
	if len(recovery) < 8 {
		return fmt.Errorf("RECOVERY_PASSWORD must be at least 8 characters; use a long generated password (it is the only thing protecting the recovery kit at rest).")
	}
	if recovery == storage {
		return fmt.Errorf("RECOVERY_PASSWORD must differ from STORAGE_PASSWORD: the recovery kit contains the storage password, so protecting it with the same value defeats the kit.")
	}
	return nil
}

// Execute refreshes the kit where needed and returns OK, Failed, Unverified or
// SecondaryFailed. Failed and unverified storages stay out of the state record, so the next
// run retries them.
func (r *Run) Execute() int {
	cfg, _, err := config.Load(r.Source, r.Environ)
	if err != nil {
		r.error(err.Error())
		return Failed
	}
	r.cfg = cfg
	if cfg.RecoveryPassword == "" {
		r.info("Recovery kit not configured (no recovery_password secret); skipping.")
		return OK
	}
	if err := ValidatePassword(cfg.RecoveryPassword, cfg.StoragePassword); err != nil {
		r.error(err.Error())
		return Failed
	}
	settings, err := config.Snapshot(r.Source, r.Environ)
	if err != nil {
		r.error(err.Error())
		return Failed
	}
	work, err := os.MkdirTemp("", "archiver-recovery-kit.")
	if err != nil {
		r.error("Recovery kit: mktemp failed.")
		return Failed
	}
	defer os.RemoveAll(work)
	os.Chmod(work, 0o700)
	fp, err := r.payload(work, settings)
	if err != nil {
		r.error("Recovery kit: payload serialization failed: " + err.Error())
		return Failed
	}
	fp = "v" + stateVersion + ":" + fp

	// What a printed envelope would say now: status compares it with what was confirmed.
	if r.EnvelopeCheck != nil {
		if err := r.EnvelopeCheck(); err != nil {
			r.warning("Envelope: could not check the printed envelope against the configuration.")
		}
	}

	recorded := r.readState(fp)
	var pending []config.Target
	var placed []string
	for _, t := range cfg.Targets {
		name := t.StorageName()
		if slices.Contains(r.Skip, name) {
			r.warning(fmt.Sprintf("Recovery kit: skipping %s, which its copy worker reports failing; it is placed after the storage recovers.", name))
			continue
		}
		if !r.Force && slices.Contains(recorded, name) {
			placed = append(placed, name)
		} else {
			pending = append(pending, t)
		}
	}
	if len(pending) == 0 || pending[0].N != 1 {
		r.primaryHasKit()
	}
	if len(pending) == 0 {
		r.info("Recovery kit is up to date on all storage targets.")
		return OK
	}

	kit, readme := filepath.Join(work, r.KitName()), filepath.Join(work, r.readmeName())
	if err := r.encrypt(work, kit); err != nil {
		r.error("Recovery kit: encryption failed: " + err.Error())
		return Failed
	}
	if err := os.WriteFile(readme, []byte(r.readme()), 0o644); err != nil {
		r.error("Recovery kit: " + err.Error())
		return Failed
	}

	failures, unverified, primaryFailed := 0, 0, false
	for _, t := range pending {
		name := t.StorageName()
		switch r.upload(t, kit, readme) {
		case OK:
			if t.N == 1 {
				r.primaryHasKit()
			}
			r.info(fmt.Sprintf("Recovery kit updated on storage '%s' (%s).", name, r.KitName()))
			placed = append(placed, name)
		case Unverified:
			r.info(fmt.Sprintf("Recovery kit placed on storage '%s' but not verified readable; re-placing on the next run.", name))
			unverified++
		default:
			r.error(fmt.Sprintf("Recovery-kit upload to storage '%s' failed; will retry on the next run.", name))
			failures++
			primaryFailed = primaryFailed || t.N == 1
		}
	}
	r.writeState(fp, placed)
	switch {
	case primaryFailed:
		return Failed
	case failures > 0:
		return SecondaryFailed
	case unverified > 0:
		return Unverified
	}
	return OK
}

func (r *Run) primaryHasKit() {
	if r.PrimaryMarker != "" {
		os.WriteFile(r.PrimaryMarker, nil, 0o600)
	}
}

// readState returns the storages recorded as holding the kit with fingerprint fp: line 1 of
// the state file is a fingerprint, the following lines storage names.
func (r *Run) readState(fp string) []string {
	b, err := os.ReadFile(r.statePath())
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) == 0 || lines[0] != fp {
		return nil
	}
	return lines[1:]
}

func (r *Run) writeState(fp string, names []string) {
	content := fp + "\n"
	for _, n := range names {
		content += n + "\n"
	}
	if len(names) == 0 {
		content += "\n" // the recorded format: an empty line for no storages
	}
	if err := os.WriteFile(r.statePath(), []byte(content), 0o600); err != nil {
		r.warning("Recovery kit: could not record where the kit is placed: " + err.Error())
		return
	}
	os.Chmod(r.statePath(), 0o600)
}

// encrypt writes the payload as one tar, encrypted the way stock `openssl enc -d -aes-256-cbc
// -pbkdf2` reverses with only the password. The password reaches openssl on fd 3, never argv.
func (r *Run) encrypt(work, out string) error {
	members := []string{"archiver.env", "secrets", "RECREATE.txt"}
	for _, m := range []string{"deployment", "extra"} {
		if fi, err := os.Stat(filepath.Join(work, m)); err == nil && fi.IsDir() {
			members = append(members, m)
		}
	}
	tar := exec.Command("tar", append([]string{"-C", work, "-cf", "-"}, members...)...)
	enc := exec.Command("openssl", "enc", "-aes-256-cbc", "-pbkdf2", "-pass", "fd:3", "-out", out)
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	defer pr.Close()
	enc.ExtraFiles = []*os.File{pr}
	var tarErr, encErr bytes.Buffer
	tar.Stderr, enc.Stderr = &tarErr, &encErr
	pipe, err := tar.StdoutPipe()
	if err != nil {
		pw.Close()
		return err
	}
	enc.Stdin = pipe
	if err := tar.Start(); err != nil {
		pw.Close()
		return err
	}
	if err := enc.Start(); err != nil {
		pw.Close()
		tar.Process.Kill()
		tar.Wait()
		return err
	}
	io.WriteString(pw, r.cfg.RecoveryPassword+"\n")
	pw.Close()
	errEnc := enc.Wait()
	errTar := tar.Wait()
	if errTar != nil {
		return fmt.Errorf("tar: %v: %s", errTar, strings.TrimSpace(tarErr.String()))
	}
	if errEnc != nil {
		return fmt.Errorf("openssl: %v: %s", errEnc, strings.TrimSpace(encErr.String()))
	}
	return nil
}
