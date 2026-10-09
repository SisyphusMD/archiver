package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/SisyphusMD/archiver/internal/config"
)

// payload writes the kit's contents into dir and returns their fingerprint: a hash over every
// file's path and content (and the extras' modes), so any changed, added, renamed or removed
// value changes it. It is the hash existing deployments recorded, so an upgrade re-uploads
// nothing.
func (r *Run) payload(dir string, s *config.Settings) (string, error) {
	keys := map[string]string{
		"rsa_private_key": r.Layout.RSAPrivateKey(),
		"rsa_public_key":  filepath.Join(r.Layout.Root, "keys", "public.pem"),
		"ssh_private_key": r.Layout.SSHPrivateKey(),
		"ssh_public_key":  r.Layout.SSHPrivateKey() + ".pub",
	}
	if err := s.WriteEnvAndSecrets(filepath.Join(dir, "archiver.env"), filepath.Join(dir, "secrets"), keys); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "RECREATE.txt"), []byte(r.recreateNotes(s)), 0o644); err != nil {
		return "", err
	}
	if err := copyVisible(r.deploymentDir(), filepath.Join(dir, "deployment")); err != nil {
		return "", err
	}
	if err := r.copyExtras(dir, s); err != nil {
		return "", err
	}
	return fingerprint(dir)
}

func (r *Run) deploymentDir() string { return filepath.Join(r.Layout.Root, "deployment") }

// copyVisible copies src's visible top-level entries into dst, following symlinks: single-file
// binds and Kubernetes ConfigMap volumes present the real content behind links, and a
// ConfigMap's hidden ..data/..timestamp entries would duplicate every file. No dst is left
// when nothing visible is there.
func copyVisible(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return nil
	}
	var copied bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if _, err := os.Stat(filepath.Join(src, e.Name())); err != nil {
			continue // a dangling link
		}
		if err := os.MkdirAll(dst, 0o700); err != nil {
			return err
		}
		if err := copyTree(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
		copied = true
	}
	if copied {
		return lockDown(dst)
	}
	return nil
}

// copyTree copies src to dst like cp -RL: symlinks followed (a Kubernetes secret volume is
// links into a timestamped directory), modes kept. Only regular files are copied: a device
// would copy without end, a FIFO would block the kit forever. A directory linked back into
// its own ancestry is skipped rather than followed round.
func copyTree(src, dst string) error {
	return copyTreeIn(src, dst, map[[2]uint64]bool{})
}

func copyTreeIn(src, dst string, ancestors map[[2]uint64]bool) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		id := [2]uint64{}
		if st, ok := fi.Sys().(*syscall.Stat_t); ok {
			id = [2]uint64{uint64(st.Dev), st.Ino}
		}
		if ancestors[id] {
			return nil
		}
		ancestors[id] = true
		defer delete(ancestors, id)
		if err := os.MkdirAll(dst, fi.Mode().Perm()|0o700); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyTreeIn(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()), ancestors); err != nil {
				return err
			}
		}
		return nil
	}
	if !fi.Mode().IsRegular() {
		return nil
	}
	in, err := os.OpenFile(src, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer in.Close()
	if now, err := in.Stat(); err != nil || !now.Mode().IsRegular() {
		return nil // swapped since the Stat
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// lockDown is chmod -R u+rwX,go-rwx: owner read/write (and execute where anyone could),
// nothing for anyone else.
func lockDown(root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		mode := fs.FileMode(0o600)
		if d.IsDir() || fi.Mode().Perm()&0o111 != 0 {
			mode = 0o700
		}
		return os.Chmod(p, mode)
	})
}

// copyExtras copies RECOVERY_KIT_EXTRA_PATHS into the kit's extra/, so a recovery has them
// before anything else is restored. A missing path is an error worth a notification, but the
// kit still goes out without it.
func (r *Run) copyExtras(dir string, s *config.Settings) error {
	raw, _ := s.Get("RECOVERY_KIT_EXTRA_PATHS")
	if raw == "" {
		return nil
	}
	extra := filepath.Join(dir, "extra")
	var copied bool
	for _, p := range strings.Split(raw, ":") {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			r.error(fmt.Sprintf("Recovery kit: RECOVERY_KIT_EXTRA_PATHS entry %s does not exist; the kit goes out without it.", p))
			continue
		}
		if err := os.MkdirAll(extra, 0o700); err != nil {
			return err
		}
		dest := filepath.Join(extra, filepath.Base(p))
		for n, base := 2, dest; exists(dest); n++ {
			dest = base + "." + strconv.Itoa(n)
		}
		if err := copyTree(p, dest); err != nil {
			return err
		}
		copied = true
	}
	if copied {
		return lockDown(extra)
	}
	return nil
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// fingerprint is, in dir, sha256 of
//
//	find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum
//	find extra -type f -printf '%m %p\n' | LC_ALL=C sort    (when extra/ exists)
func fingerprint(dir string) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			rel, _ := filepath.Rel(dir, p)
			files = append(files, "./"+rel)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	var modes []string
	for _, f := range files {
		// Streamed: an extra may be a large git bundle.
		sum, err := hashFile(filepath.Join(dir, f))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s  %s\n", sum, f)
		if rel := strings.TrimPrefix(f, "./"); strings.HasPrefix(rel, "extra/") {
			fi, err := os.Stat(filepath.Join(dir, f))
			if err != nil {
				return "", err
			}
			modes = append(modes, fmt.Sprintf("%o %s\n", fi.Mode().Perm(), rel))
		}
	}
	sort.Strings(modes)
	for _, m := range modes {
		io.WriteString(h, m)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// recreateNotes are the deterministic recreation notes (no timestamps: they count toward the
// fingerprint): every fact Archiver knows about how the container must be put together, so the
// kit stands alone when no manifest was mounted.
func (r *Run) recreateNotes(s *config.Settings) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	w("How to recreate this Archiver deployment (generated from the running container)")
	w("================================================================================")
	w("")
	w("1. Load archiver.env as environment variables (compose 'env_file:', or a")
	w("   Kubernetes ConfigMap via --from-env-file) and mount the secrets/ files under")
	w("   /run/secrets. Template: https://github.com/SisyphusMD/archiver/blob/main/compose.yaml")
	if hasVisible(r.deploymentDir()) {
		w("")
		w("2. deployment/ in this kit holds the manifest files that were mounted at")
		w("   %s — your actual compose/nix/k8s definition. Prefer those.", r.deploymentDir())
	}
	if v, _ := s.Get("RECOVERY_KIT_EXTRA_PATHS"); v != "" {
		w("")
		w("- extra/ in this kit holds the files this deployment chose to carry for a recovery")
		w("  (RECOVERY_KIT_EXTRA_PATHS): runbooks, scripts, repositories. Start there.")
	}
	w("")
	w("Facts this deployment depended on:")
	w("  - hostname: %s   (keep it: snapshot IDs and this kit's filename derive from it)", r.Hostname)
	for _, name := range []string{"BACKUP_SCHEDULE", "MAINTENANCE_SCHEDULE", "RESTORE_DRILL_SCHEDULE", "TZ"} {
		if v := r.getenv(name); v != "" {
			w("  - %s: %s", name, v)
		}
	}
	w("  - container paths that need host mounts:")
	sd, _ := s.Get("SERVICE_DIRECTORIES")
	for _, p := range strings.Split(sd, ":") {
		if p != "" {
			w("      %s   (data to back up)", p)
		}
	}
	for n := 1; ; n++ {
		p := "STORAGE_TARGET_" + strconv.Itoa(n) + "_"
		name, _ := s.Get(p + "NAME")
		if name == "" {
			break
		}
		if t, _ := s.Get(p + "TYPE"); t == "local" {
			path, _ := s.Get(p + "LOCAL_PATH")
			w("      %s   (local storage target '%s')", path, name)
		}
	}
	if fi, err := os.Stat(r.dockerSocket()); err == nil && fi.Mode()&fs.ModeSocket != 0 {
		w("  - /var/run/docker.sock was mounted (service pre/post hooks use docker)")
	}
	w("  - capabilities: cap_drop ALL; cap_add DAC_OVERRIDE (backup);")
	w("    plus CHOWN + FOWNER to restore files under their original ownership")
	return b.String()
}

func hasVisible(dir string) bool {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			return true
		}
	}
	return false
}

func (r *Run) dockerSocket() string {
	if r.DockerSocket != "" {
		return r.DockerSocket
	}
	return "/var/run/docker.sock"
}

func (r *Run) readme() string {
	return fmt.Sprintf(`This is the automatic recovery kit for the Archiver deployment on host '%[1]s'.

%[2]s is an encrypted snapshot of everything needed to recreate the
deployment: archiver.env (the non-secret settings), secrets/ (every secret and key file),
RECREATE.txt (recreation notes), and deployment/ (the deployment manifests, if they were
mounted). It is refreshed automatically whenever any of that changes.

To recover: download the .tar.enc file to any machine and run (it prompts for the recovery
password, which was displayed at setup and belongs in your password manager):

  openssl enc -d -aes-256-cbc -pbkdf2 -in %[2]s | tar -xvf -

Then start with RECREATE.txt, or see the 'Configuration Sources' section of the README:
https://github.com/SisyphusMD/archiver
`, r.Hostname, r.KitName())
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
