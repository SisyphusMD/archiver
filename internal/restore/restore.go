// Package restore implements `archiver restore`, `auto-restore`, `auto-restore-all` and
// `snapshot-exists` (ADR 10). The non-interactive commands keep the 0.11 interface that
// deployments and restore drills script against: SNAPSHOT_ID, LOCAL_DIR, REVISION,
// STORAGE_TARGET, the restore toggles, RUN_RESTORE_SERVICE, and exit codes 0 (done),
// 1 (not found or failed), 2 (unreachable or invalid environment) and 3 (a backup is running).
package restore

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/hooks"
	"github.com/SisyphusMD/archiver/internal/inuse"
	"github.com/SisyphusMD/archiver/internal/kit"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/proc"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

// Exit codes of the non-interactive commands.
const (
	OK          = 0
	NotFound    = 1 // also a failed restore or restore hook
	Unreachable = 2 // also an invalid environment
	Busy        = 3
)

// Env is what a restore command runs with.
type Env struct {
	Layout    layout.Layout
	Source    config.Source
	Environ   []string
	Hostname  string
	Duplicacy string // the duplicacy binary
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	// CapEff is the effective capability set (from /proc/self/status), or -1 if unknown.
	CapEff int64
	// AfterRestore runs on the restored directory before its restore hook; the command
	// uses it to migrate a restored service-backup-settings.sh. Nil does nothing.
	AfterRestore func(dir string)

	cfg  *config.Config
	held *os.File // the restore lock, while a restore into a service directory runs
}

func (e *Env) getenv(name string) string { return e.Source.Getenv(name) }

// load reads the configuration and validates what reaching the storages needs, reporting a
// problem on stderr. A restore in a bare recovery container needs no service directories
// or notifier.
func (e *Env) load() bool {
	cfg, warnings, err := config.Load(e.Source, e.Environ)
	for _, w := range warnings {
		fmt.Fprintln(e.Stderr, "WARNING:", w)
	}
	if err == nil {
		err = cfg.ValidateStorage(e.Source.SecretsDir)
	}
	if err != nil {
		fmt.Fprintln(e.Stderr, "ERROR:", err)
		return false
	}
	e.cfg = cfg
	return true
}

// backupRunning reports whether a backup holds its lock.
func (e *Env) backupRunning() bool {
	l, ok, _ := lockstate.ReadLock(e.Layout.BackupLock())
	return ok && l.Alive()
}

// guard keeps backups out while dir, already created, is restored, when dir is, holds or lies
// in a configured service directory: a backup then would save it half-restored, and both rewrite its
// repository. The restore lock is taken before the backup lock is checked and a backup
// takes its own before probing this one, so whichever starts second sees the other. The
// lock lasts until release, through the restore hook. Restores elsewhere only check that no
// backup is running.
func (e *Env) guard(dir string) error {
	if e.held != nil {
		return nil
	}
	if !e.inServiceDir(dir) {
		// No lock to take, but a restore still never starts during a backup.
		if e.backupRunning() {
			return errBackupRunning
		}
		return nil
	}
	f, ok, err := runlock.Hold(e.Layout.RestoreLock())
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("another restore into a service directory is running")
	}
	if e.backupRunning() {
		f.Close()
		return errBackupRunning
	}
	e.held = f
	return nil
}

var errBackupRunning = errors.New("a backup is running; restore once it has finished (or stop it with 'archiver stop backup')")

// release ends the guard, at the end of the command.
func (e *Env) release() {
	if e.held != nil {
		e.held.Close()
		e.held = nil
	}
}

// inServiceDir reports whether dir, which exists, is, lies in or holds a service directory:
// one of the directories SERVICE_DIRECTORIES expands to, the backup's own expansion, with
// symlinks resolved on both sides. A pattern also counts when dir holds where it points,
// for a service directory the restore will create; a pattern component this cannot match
// exactly (a bracket class) counts as matching, keeping backups out rather than in.
func (e *Env) inServiceDir(dir string) bool {
	if e.cfg == nil {
		return false
	}
	d, err := filepath.Abs(dir)
	if err != nil {
		return true // cannot tell: keep backups out
	}
	d = resolved(d)
	dirs, _ := config.ExpandServiceDirectories(e.cfg.ServiceDirectories)
	for _, sd := range dirs {
		if abs, err := filepath.Abs(sd); err == nil && overlaps(d, resolved(abs)) {
			return true
		}
	}
	for _, pattern := range e.cfg.ServiceDirectories {
		abs, err := filepath.Abs(pattern)
		if err != nil {
			return true
		}
		if overlaps(d, abs) {
			return true
		}
		// Each leading part that exists, found as the backup finds it and with symlinks
		// resolved, followed by the rest of the pattern.
		c := components(abs)
		for k := 1; k < len(c); k++ {
			found, _ := config.ExpandServiceDirectories([]string{"/" + strings.Join(c[:k], "/")})
			for _, m := range found {
				if overlaps(d, filepath.Join(append([]string{resolved(m)}, c[k:]...)...)) {
					return true
				}
			}
		}
	}
	return false
}

// overlaps reports whether dir and a path pattern matches are the same, or one holds the
// other: every component they both have matches.
func overlaps(dir, pattern string) bool {
	dc, pc := components(dir), components(pattern)
	for i := 0; i < len(dc) && i < len(pc); i++ {
		if strings.Contains(pc[i], "[") {
			continue
		}
		if ok, err := filepath.Match(pc[i], dc[i]); err != nil || !ok {
			return false
		}
	}
	return true
}

func components(p string) []string {
	var c []string
	for _, s := range strings.Split(filepath.Clean(p), "/") {
		if s != "" {
			c = append(c, s)
		}
	}
	return c
}

// resolved resolves the symlinks of p's longest existing ancestor, for a path that may not
// exist yet.
func resolved(p string) string {
	rest := ""
	for cur := p; ; cur = filepath.Dir(cur) {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(r, rest)
		}
		if cur == filepath.Dir(cur) {
			return p
		}
		rest = filepath.Join(filepath.Base(cur), rest)
	}
}

// pinnedTarget resolves STORAGE_TARGET, a storage name or number.
func (e *Env) pinnedTarget(v string) (config.Target, bool) {
	if n, err := strconv.Atoi(v); err == nil {
		for _, t := range e.cfg.Targets {
			if t.N == n {
				return t, true
			}
		}
		return config.Target{}, false
	}
	for _, t := range e.cfg.Targets {
		if t.Name == v {
			return t, true
		}
	}
	return config.Target{}, false
}

func (e *Env) duplicacy(dir string, out io.Writer, args ...string) *exec.Cmd {
	cmd := exec.Command(e.Duplicacy, proc.NoScript(args...)...)
	cmd.Dir = dir
	cmd.Env = e.cfg.DuplicacyEnviron(e.Environ, e.Layout.SSHPrivateKey())
	cmd.Stdout, cmd.Stderr = out, out
	e.inherit(cmd)
	return cmd
}

// inherit passes the restore lock to a duplicacy child, so one that outlives a killed
// restore still keeps backups out until it ends.
func (e *Env) inherit(cmd *exec.Cmd) {
	if e.held != nil {
		cmd.ExtraFiles = append(cmd.ExtraFiles, e.held)
	}
}

// connect makes dir a duplicacy repository of snapshot id on t, replacing any it was.
func (e *Env) connect(dir string, t config.Target, id string, out io.Writer) error {
	if t.Type == "sftp" || t.Type == "sftpc" {
		for _, k := range []string{e.Layout.SSHPrivateKey(), e.Layout.SSHPrivateKey() + ".pub"} {
			if _, err := os.Stat(k); err != nil {
				return fmt.Errorf("missing SSH key file %s for the SFTP storage '%s'; restore the keys directory first", k, t.Name)
			}
		}
	}
	url, err := t.URL()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(dir, ".duplicacy")); err != nil {
		return err
	}
	pub := filepath.Join(e.Layout.Root, "keys", "public.pem")
	if err := e.duplicacy(dir, out, "init", "-e", "-key", pub, "-storage-name", t.StorageName(), id, url).Run(); err != nil {
		if why := kit.Diagnose(e.Layout, "", t); why != "" {
			return fmt.Errorf("duplicacy %s storage initialization failed for '%s': %s", t.Type, t.StorageName(), why)
		}
		return fmt.Errorf("duplicacy %s storage initialization failed for '%s'", t.Type, t.StorageName())
	}
	return nil
}

// revisions lists id's revisions on the repository's storage, newest first.
func (e *Env) revisions(dir, id string) ([]int, error) {
	var out strings.Builder
	if err := e.duplicacy(dir, &out, "list", "-id", id).Run(); err != nil {
		fmt.Fprint(e.Stderr, out.String())
		return nil, err
	}
	return parseRevisions(out.String(), id), nil
}

func parseRevisions(listing, id string) []int {
	var revs []int
	for _, line := range strings.Split(listing, "\n") {
		f := strings.Fields(line)
		if len(f) >= 4 && f[0] == "Snapshot" && f[1] == id && f[2] == "revision" {
			if n, err := strconv.Atoi(f[3]); err == nil {
				revs = append(revs, n)
			}
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(revs)))
	return revs
}

// Options are duplicacy restore's settings.
type Options struct {
	Hash, Overwrite, Delete, IgnoreOwner, Persist bool
	Threads                                       string
}

func (o Options) args() []string {
	var a []string
	for _, f := range []struct {
		on   bool
		flag string
	}{{o.Hash, "-hash"}, {o.Overwrite, "-overwrite"}, {o.Delete, "-delete"}, {o.IgnoreOwner, "-ignore-owner"}, {o.Persist, "-persist"}} {
		if f.on {
			a = append(a, f.flag)
		}
	}
	return a
}

// Capability bits restore needs to recreate ownership.
const (
	capChown  = 1 << 0
	capFowner = 1 << 3
)

// missingCaps names the capabilities, of CHOWN and FOWNER, that capEff lacks.
func missingCaps(capEff int64) []string {
	if capEff < 0 {
		return nil
	}
	var m []string
	if capEff&capChown == 0 {
		m = append(m, "CHOWN")
	}
	if capEff&capFowner == 0 {
		m = append(m, "FOWNER")
	}
	return m
}

// restore restores revision rev of id from t into dir, already connected. A restore is never
// refused for missing ownership capabilities: without them it restores without ownership.
func (e *Env) restore(dir string, t config.Target, id string, rev int, o Options, out io.Writer) error {
	if !o.IgnoreOwner {
		if m := missingCaps(e.CapEff); len(m) > 0 {
			fmt.Fprintf(out, "[WARN] The %s capability is not granted, so this restore cannot preserve original ownership: restored files will be owned by root. Add CHOWN and FOWNER to the container's cap_add to keep ownership, or set IGNORE_OWNERSHIP=1 to silence this warning.\n", strings.Join(m, " "))
			o.IgnoreOwner = true
		}
	}
	for _, l := range linkedDirs(dir) {
		fmt.Fprintf(out, "[WARN] '%s' is a link to a directory outside '%s' ('%s'); Duplicacy restores through it, so files under it land there. Remove or move the link first if that is not intended.\n", l[0], dir, l[1])
	}
	// Registered in use, so no prune deletes the revision meanwhile (ADR 19).
	release, err := inuse.Gate(context.Background(), e.Layout.InUseDir(), t.StorageName(), false)
	if err != nil {
		return err
	}
	h, err := inuse.Hold(e.Layout.InUseDir(), "restore", []inuse.Revision{{Storage: t.StorageName(), ID: id, Rev: rev}})
	release()
	if err != nil {
		return err
	}
	defer h.Release()
	args := append([]string{"restore", "-r", strconv.Itoa(rev), "-key", e.Layout.RSAPrivateKey(), "-stats", "-threads", o.Threads}, o.args()...)
	cmd := e.duplicacy(dir, out, args...)
	// duplicacy shares the registration's lock, so a restore that outlives this process
	// (killed mid-restore) still keeps its revision from being pruned.
	cmd.ExtraFiles = append(cmd.ExtraFiles, h.File())
	return cmd.Run()
}

// postRestore runs dir's restore hook, if it has one, and returns its exit code.
func (e *Env) postRestore(dir, id string, rev int, t config.Target, stdin io.Reader, out io.Writer) (int, error) {
	s := hooks.Service{Name: filepath.Base(dir), Dir: dir, SnapshotID: id}
	env := hooks.RestoreEnviron(e.Environ, s, rev, t.Name)
	var cmd *exec.Cmd
	hookDir := e.hookDir(dir)
	ok, err := hooks.Exists(hookDir, hooks.PostRestore)
	if err != nil {
		return 0, err
	}
	legacy := !ok && isFile(filepath.Join(hookDir, hooks.LegacyRestore))
	if legacy {
		if err := hooks.Safe(filepath.Join(hookDir, hooks.LegacyRestore)); err != nil {
			return 0, err
		}
	}
	switch {
	case ok:
		fmt.Fprintf(out, "Running %s...\n", hooks.PostRestore)
		cmd = exec.Command(filepath.Join(hookDir, hooks.PostRestore))
	case legacy:
		fmt.Fprintf(out, "Running %s...\n", hooks.LegacyRestore)
		script := hooks.LegacyRestore // what it always ran as, its $0, when in the directory
		if hookDir != dir {
			script = filepath.Join(hookDir, hooks.LegacyRestore)
		}
		cmd = exec.Command("bash", script)
		// What it always saw: the restore's variables, for this service (auto-restore-all
		// runs one restore per service), last so they override any the caller set.
		env = append(env, "SNAPSHOT_ID="+id, "LOCAL_DIR="+dir)
	default:
		fmt.Fprintf(out, "No %s or %s in '%s'; nothing to run.\n", hooks.PostRestore, hooks.LegacyRestore, hookDir)
		return 0, nil
	}
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, out, out
	// The hook does not inherit the restore lock: a service it starts in the background
	// would hold it, and keep backups out, for as long as that service runs.
	// A reader stdin is copied by a goroutine that may sit blocked on a terminal after the
	// hook exits; don't wait for it long.
	cmd.WaitDelay = time.Second
	err = cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	return 0, err
}

func (e *Env) hasRestoreHook(dir string) bool {
	hd := e.hookDir(dir)
	return isFile(filepath.Join(hd, hooks.PostRestore)) || isFile(filepath.Join(hd, hooks.LegacyRestore))
}

// hookDir is where dir's restore hook lives: HOOKS_DIR/<name> when set (ADR 45).
func (e *Env) hookDir(dir string) string {
	if e.cfg == nil {
		return dir
	}
	return hooks.Hooks(e.cfg.HooksDir, dir)
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// prepare creates dir if needed and returns its absolute path.
func prepare(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", err
	}
	return abs, nil
}

// yes reads a y/N answer; anything but y or Y, or the end of input, is no.
func yes(in *bufio.Reader) bool {
	line, _ := in.ReadString('\n')
	line = strings.TrimSpace(line)
	return line == "y" || line == "Y"
}

// linkedDirs lists the symlinks under dir that resolve to a directory outside it, each with
// its target, stopping after 20: Duplicacy writes a snapshot's files through such a link
// rather than replacing it, so whoever could write dir chose where they go.
func linkedDirs(dir string) [][2]string {
	var found [][2]string
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil
	}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if len(found) >= 20 {
			return filepath.SkipAll
		}
		if err != nil {
			return nil
		}
		if d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		target, err := filepath.EvalSymlinks(p)
		if err != nil {
			return nil
		}
		if fi, err := os.Stat(target); err != nil || !fi.IsDir() {
			return nil
		}
		if rel, err := filepath.Rel(root, target); err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
			return nil
		}
		found = append(found, [2]string{p, target})
		return nil
	})
	return found
}
