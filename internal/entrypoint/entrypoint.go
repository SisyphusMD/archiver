// Package entrypoint is the container's start: it refuses a bundle-era configuration, puts
// the mounted keys in place, clears lock state a previous container left, forwards the logs
// to stdout for `docker logs`, and runs the scheduler or waits for manual commands. `init`
// `run <command>` and `migrate hooks` are one-shot modes. A SIGTERM stops whatever runs, gracefully, before
// the container exits.
package entrypoint

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/hooks"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

// Env is what the entrypoint runs with; tests point it elsewhere.
type Env struct {
	Layout     layout.Layout
	Getenv     func(string) string
	SecretsDir string
	Stdout     io.Writer
	Stderr     io.Writer
	// Self is the archiver binary, run for each step that is a command of its own.
	Self string
}

func (e *Env) secret(name, def string) string {
	if v := e.Getenv(name); v != "" {
		return v
	}
	return filepath.Join(e.SecretsDir, def)
}

// RefuseBundle reports the first bundle-era thing found: a deployment still configured by a
// bundle must convert first (ADRs 4, 22), since starting without it would back up nothing, or
// with half a configuration.
func (e *Env) RefuseBundle() string {
	found := ""
	for _, p := range []string{
		filepath.Join(e.Layout.Root, "bundle", "bundle.tar.enc"),
		filepath.Join(e.Layout.Root, "config.sh"),
		e.secret("BUNDLE_PASSWORD_FILE", "bundle_password"),
	} {
		if _, err := os.Lstat(p); err == nil {
			found = p
		}
	}
	if e.Getenv("BUNDLE_PASSWORD") != "" {
		found = "BUNDLE_PASSWORD in the environment"
	}
	return found
}

// BundleHelp is what a refused bundle deployment is told to do.
const BundleHelp = `This release no longer reads bundles (bundle.tar.enc, config.sh). Convert yours once with the
0.11 image, which writes the same configuration as env-native materials:

  docker run --rm \
    -v ./archiver-bundle:/opt/archiver/bundle:ro \
    -v ./secrets/bundle_password:/run/secrets/bundle_password:ro \
    -v ./archiver-migrate:/opt/archiver/migrate \
    ghcr.io/sisyphusmd/archiver:0.11 run migrate

Then load archiver-migrate/archiver.env as environment variables and the files in
archiver-migrate/secrets/ under /run/secrets, remove the bundle mount and the bundle_password
secret, and start this release again. The files hold your secrets in plaintext: move them
into your secret store and delete them. See docs/upgrading.md, "Upgrading from a bundle".
`

// PlaceKeys copies the mounted key files into the keys directory with their modes; a missing
// optional key (SSH, for a deployment without SFTP) is skipped. It fails when the RSA pair,
// which every deployment needs, is not there.
func (e *Env) PlaceKeys() error {
	keys := filepath.Join(e.Layout.Root, "keys")
	if err := os.MkdirAll(keys, 0o700); err != nil {
		return err
	}
	for _, k := range []struct {
		env, def, dst string
		mode          os.FileMode
	}{
		{"RSA_PRIVATE_KEY_FILE", "rsa_private_key", "private.pem", 0o600},
		{"RSA_PUBLIC_KEY_FILE", "rsa_public_key", "public.pem", 0o644},
		{"SSH_PRIVATE_KEY_FILE", "ssh_private_key", "id_ed25519", 0o600},
		// The SFTP restore path needs the public half too.
		{"SSH_PUBLIC_KEY_FILE", "ssh_public_key", "id_ed25519.pub", 0o644},
	} {
		src := e.secret(k.env, k.def)
		fi, err := os.Stat(src)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		dst := filepath.Join(keys, k.dst)
		if err := os.WriteFile(dst, data, k.mode); err != nil { //nolint:gosec // G703: a path from the layout or configuration, not from untrusted input
			return err
		}
		if err := os.Chmod(dst, k.mode); err != nil {
			return err
		}
	}
	for _, f := range []string{"private.pem", "public.pem"} {
		if _, err := os.Stat(filepath.Join(keys, f)); err != nil {
			return fmt.Errorf("no RSA key files found.\nMount the RSA keypair at %s and %s\n(or point RSA_PRIVATE_KEY_FILE / RSA_PUBLIC_KEY_FILE at them).",
				filepath.Join(e.SecretsDir, "rsa_private_key"), filepath.Join(e.SecretsDir, "rsa_public_key"))
		}
	}
	return nil
}

// ClearLocks removes lock and stop-flag state a previous container left in /var/lock, which
// survives `docker restart`. A fresh PID namespace cannot hold a live prior holder, so this is
// safe, and it keeps a recycled PID from faking a running backup (every scheduled one then
// refused) and a leftover stop flag from ending the first backup.
func (e *Env) ClearLocks() {
	for _, name := range []string{"archiver-main.lock", "archiver-stop-requested", "archiver-maintenance.lock",
		"archiver-maintenance-stop-requested", "archiver-main.lock.tmp", "archiver-maintenance.lock.tmp",
		"archiver-drill.lock", "archiver-drill-stop-requested", "archiver-drill.lock.tmp"} {
		os.Remove(filepath.Join(e.Layout.Lock, name))
	}
}

// Warnings are configuration problems worth saying at start; fatal ones end the start.
func (e *Env) Warnings() (fatal string, warnings []string) {
	// An ignored schedule rename would mean no scheduled backups and nobody noticing.
	if e.Getenv("CRON_SCHEDULE") != "" {
		return "CRON_SCHEDULE was renamed to BACKUP_SCHEDULE. Rename the environment variable and restart.", nil
	}
	if e.Getenv("ROTATE_BACKUPS") != "" {
		warnings = append(warnings, "WARNING: ROTATE_BACKUPS is deprecated; rename it to PRUNE_BACKUPS (still honored for now).")
	}
	// Check and prune run only on MAINTENANCE_SCHEDULE (or a manual `archiver maintenance`).
	if f := e.Getenv("LOG_FORMAT"); f != "" && f != "text" && f != "json" {
		warnings = append(warnings, fmt.Sprintf("WARNING: LOG_FORMAT='%s' is not text or json; using text.", f))
	}
	if e.Getenv("BACKUP_SCHEDULE") != "" && e.Getenv("MAINTENANCE_SCHEDULE") == "" {
		warnings = append(warnings,
			"WARNING: MAINTENANCE_SCHEDULE is not set: storage check and prune will never run automatically.",
			`         Set MAINTENANCE_SCHEDULE (e.g. "0 13 * * *") or run 'archiver maintenance' yourself.`)
	}
	return "", warnings
}

// RunCommands are the commands `run` accepts.
var RunCommands = []string{"auto-restore", "auto-restore-all", "snapshot-exists", "healthcheck", "backup", "maintenance"}

// Follow prints path's new lines to w as they are written, from the end of the file when it
// first appears, after a banner, through rotations (as `tail -F -n 0`), until stop closes.
func Follow(path, banner string, w io.Writer, stop <-chan struct{}) {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	wait := func() bool {
		select {
		case <-stop:
			return false
		case <-tick.C:
			return true
		}
	}
	var f *os.File
	for f == nil {
		if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() {
			if f, err = os.Open(path); err == nil {
				_, _ = f.Seek(0, io.SeekEnd)
				break
			}
		}
		if !wait() {
			return
		}
	}
	fmt.Fprintf(w, "--- %s ---\n", banner)
	buf := make([]byte, 64<<10)
	var partial []byte
	for {
		for {
			n, _ := f.Read(buf)
			if n == 0 {
				break
			}
			data := append(partial, buf[:n]...)
			if i := strings.LastIndexByte(string(data), '\n'); i >= 0 {
				_, _ = w.Write(data[:i+1])
				partial = append([]byte(nil), data[i+1:]...)
			} else {
				partial = data
			}
		}
		if !wait() {
			f.Close()
			return
		}
		// Rotated (a new file at the path) or truncated: read the new one from its start.
		cur, err := f.Stat()
		now, err2 := os.Stat(path)
		if err == nil && err2 == nil && (!os.SameFile(cur, now) || now.Size() < offset(f)) {
			if nf, err := os.Open(path); err == nil {
				f.Close()
				f, partial = nf, nil
			}
		}
	}
}

func offset(f *os.File) int64 {
	o, _ := f.Seek(0, io.SeekCurrent)
	return o
}

// LocksClear reports whether no backup, maintenance or drill holds its lock.
func (e *Env) LocksClear() bool {
	for _, name := range []string{"archiver-main.lock", "archiver-maintenance.lock", "archiver-drill.lock"} {
		if _, err := os.Stat(filepath.Join(e.Layout.Lock, name)); err == nil {
			return false
		}
	}
	// A copy worker holds its storage's copy lock while its duplicacy runs, saving its state
	// after a stop included; the daemon is signalled only once they are free.
	copies, _ := filepath.Glob(filepath.Join(e.Layout.Lock, "archiver-copy-*.flock"))
	for _, p := range copies {
		f, free, err := runlock.Hold(p)
		if err == nil && !free {
			return false
		}
		if f != nil {
			f.Close()
		}
	}
	return true
}

// Exec replaces this process with `archiver args...`.
func (e *Env) Exec(args ...string) error {
	return syscall.Exec(e.Self, append([]string{e.Self}, args...), os.Environ())
}

// LegacyServices lists the configured service directories that still hold the sourced
// service-backup-settings.sh, which this release does not run (ADR 20).
func LegacyServices(src config.Source) []string {
	cfg, _, err := config.Load(src, nil)
	if err != nil {
		return nil // the commands report a configuration problem themselves
	}
	dirs, _ := config.ExpandServiceDirectories(cfg.ServiceDirectories)
	var legacy []string
	for _, d := range dirs {
		if _, err := os.Lstat(filepath.Join(d, hooks.Legacy)); err == nil {
			legacy = append(legacy, d)
		}
	}
	return legacy
}

// LegacyHelp is what a deployment with legacy settings files is told to do.
func LegacyHelp(dirs []string) string {
	return fmt.Sprintf(`ERROR: service-backup-settings.sh, which this release does not run, is still in: %s.
Convert it once, with this container's mounts and environment:

  docker compose run --rm archiver migrate hooks

(or the same 'docker run' as this container with 'migrate hooks' as its command), then start
again. The hooks keep calling your existing functions. See docs/hooks.md, "Upgrading from
service-backup-settings.sh".
`, strings.Join(dirs, ", "))
}
