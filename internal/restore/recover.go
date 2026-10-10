package restore

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/entrypoint"
	"github.com/SisyphusMD/archiver/internal/kit"
)

// RecoverOptions are `archiver recover`'s inputs.
type RecoverOptions struct {
	Kit      string // the downloaded kit file
	Password string // the recovery password
	Out      string // where the recovered configuration and secrets are written
	Yes      bool   // restore without asking (a person at a terminal is asked)
	Hook     bool   // run each service's restore hook after its files
	// Migrate makes the AfterRestore step for the recovered configuration and host: a
	// restored legacy settings file must be migrated with what the kit says, not the
	// recovery container's empty environment.
	Migrate func(src config.Source, host string) func(dir string)
}

// Recover is the guided disaster recovery (ADR 29): it decrypts the recovery kit, writes the
// configuration and secrets it holds to o.Out, puts the keys in place, checks every storage,
// and restores every service this host backed up into its configured directory, saying at
// each step what worked and what to do next.
func (e *Env) Recover(o RecoverOptions) int {
	out := e.Stdout
	step := func(n int, what string) { fmt.Fprintf(out, "\n[%d/5] %s\n", n, what) }
	fail := func(format string, a ...any) int {
		fmt.Fprintf(e.Stderr, "ERROR: "+format+"\n", a...)
		return 1
	}

	// Duplicacy runs in other directories and reads some credentials as file paths.
	if abs, err := filepath.Abs(o.Out); err == nil {
		o.Out = abs
	}
	step(1, "Decrypting the recovery kit "+o.Kit)
	stage, err := os.MkdirTemp("", "archiver-recover-")
	if err != nil {
		return fail("%v", err)
	}
	defer os.RemoveAll(stage)
	_ = os.Chmod(stage, 0o700) //nolint:gosec // G302: deliberate mode: owner-only, or a log or lock that is not secret
	if err := decryptKit(o.Kit, o.Password, stage); err != nil {
		return fail("the kit does not open with that password (or is not a recovery kit): %v", err)
	}
	if _, err := os.Stat(filepath.Join(stage, "archiver.env")); err != nil {
		return fail("the kit holds no archiver.env: it is from a release before env-native configuration; recover it by hand (its README)")
	}
	fmt.Fprintln(out, "  OK")

	step(2, "Writing the configuration and secrets to "+o.Out)
	// The directory is marked with the kit's contents hash before anything is copied: a
	// recovery of this same kit, declined or cut short, runs again and finishes the copy;
	// anything else there (another kit, someone's files) is never overwritten.
	id, err := payloadHash(stage)
	if err != nil {
		return fail("%v", err)
	}
	mark := filepath.Join(o.Out, ".archiver-recovery")
	if entries, err := os.ReadDir(o.Out); err == nil && len(entries) > 0 {
		have, _ := os.ReadFile(mark)
		if strings.TrimSpace(string(have)) != id {
			return fail("%s is not empty and holds no recovery of this kit; point the recovery at an empty directory so nothing there is overwritten", o.Out)
		}
		fmt.Fprintln(out, "  Recovered here from this kit before; finishing and reusing it.")
	}
	if err := os.MkdirAll(o.Out, 0o700); err != nil {
		return fail("%v", err)
	}
	// The secrets land here in plaintext: the directory is made the recovery's alone before
	// anything is written, and nothing in it is followed (see copyPrivate).
	if fi, err := os.Lstat(o.Out); err != nil || !fi.IsDir() {
		return fail("%s must be a directory, not a link to one", o.Out)
	}
	if err := os.Chmod(o.Out, 0o700); err != nil { //nolint:gosec // G302: deliberate mode: owner-only, or a log or lock that is not secret
		return fail("%v", err)
	}
	if err := writePrivate(mark, 0o600, strings.NewReader(id+"\n")); err != nil {
		return fail("%v", err)
	}
	for _, name := range []string{"archiver.env", "secrets", "RECREATE.txt", "deployment", "extra"} {
		if _, err := os.Stat(filepath.Join(stage, name)); err != nil {
			continue
		}
		if err := copyPrivate(filepath.Join(stage, name), filepath.Join(o.Out, name)); err != nil {
			return fail("writing %s: %v", name, err)
		}
	}
	env, err := readEnvFile(filepath.Join(o.Out, "archiver.env"))
	if err != nil {
		return fail("%v", err)
	}
	host := recordedHostname(filepath.Join(o.Out, "RECREATE.txt"))
	recorded, haveList := recordedServices(filepath.Join(o.Out, "RECREATE.txt"))
	if host == "" {
		host = e.Hostname
	}
	fmt.Fprintf(out, "  OK: archiver.env, secrets/ (owner-only) and RECREATE.txt. The deployment was host '%s'.\n", host)

	// Hooks run only with --hook; without it, a HOOKS_DIR not yet mounted on the new host
	// must not stop the data coming back.
	if !o.Hook {
		delete(env, "HOOKS_DIR")
	}
	step(3, "Putting the keys in place")
	src := config.Source{Getenv: func(k string) string { return env[k] }, SecretsDir: filepath.Join(o.Out, "secrets")}
	keys := &entrypoint.Env{Layout: e.Layout, Getenv: src.Getenv, SecretsDir: src.SecretsDir}
	if err := keys.PlaceKeys(); err != nil {
		return fail("%v", err)
	}
	fmt.Fprintln(out, "  OK")

	step(4, "Checking the storages")
	e.Source, e.Hostname, e.cfg = src, host, nil
	// Restore hooks see the recovered deployment's settings, as they did on the old host
	// (secrets are kept from them as always).
	for k, v := range env {
		e.Environ = append(e.Environ, k+"="+v)
	}
	if o.Migrate != nil {
		e.AfterRestore = o.Migrate(src, host)
	}
	if !e.load() {
		return 1
	}
	// Every readable storage counts: a secondary may hold a service the primary lost, or the
	// other way round. Each snapshot is restored from wherever its newest revision is.
	listing := map[string]SnapshotInfo{}
	from := map[string]string{}
	read := 0
	for _, t := range e.cfg.Targets {
		s, err := e.Snapshots(t)
		var unchecked *KeyUnchecked
		switch {
		case errors.As(err, &unchecked):
			fmt.Fprintf(out, "  '%s' (%s): OK, %d snapshot IDs (%v; the restore itself proves the key)\n", t.Name, t.Type, len(s), err)
		case err != nil:
			fmt.Fprintf(out, "  '%s' (%s): unreachable or unreadable: %v\n", t.Name, t.Type, err)
			continue
		default:
			fmt.Fprintf(out, "  '%s' (%s): OK, %d snapshot IDs\n", t.Name, t.Type, len(s))
		}
		read++
		for id, info := range s {
			if cur, ok := listing[id]; !ok || info.Revision > cur.Revision {
				listing[id], from[id] = info, t.Name
			}
		}
	}
	if read == 0 {
		return fail("no storage could be read; check the network, the credentials in %s/secrets, and the storage settings in archiver.env", o.Out)
	}

	step(5, "Restoring every service")
	var plan []placement
	var unplaced, missing []string
	if haveList {
		plan, missing = placeRecorded(recorded, listing, host)
	} else {
		fmt.Fprintln(out, "  (The kit predates its list of services: they are found from the snapshots; a name that may be another host's is left for you.)")
		plan, unplaced = placeServices(listing, host, e.cfg.ServiceDirectories)
		missing = missingLiterals(listing, host, e.cfg.ServiceDirectories)
	}
	for _, name := range missing {
		fmt.Fprintf(out, "  %s-%s: configured, but no readable storage holds a snapshot of it\n", host, name)
	}
	if len(plan) == 0 {
		return fail("no storage holds a snapshot of host '%s' that SERVICE_DIRECTORIES places", host)
	}
	for _, p := range plan {
		fmt.Fprintf(out, "  %s -> %s (newest revision %d, on '%s')\n", p.id, p.dir, listing[p.id].Revision, from[p.id])
	}
	for _, id := range unplaced {
		fmt.Fprintf(out, "  %s: no SERVICE_DIRECTORIES entry places it; restore it yourself with auto-restore (SNAPSHOT_ID=%s)\n", id, id)
	}
	if !o.Yes && isTerminal(e.Stdin) {
		fmt.Fprint(out, "Restore these now? Each directory receives its newest revision. (y/N): ")
		if !yes(bufio.NewReader(e.Stdin)) {
			fmt.Fprintln(out, "Nothing restored. The configuration and keys are in place; run 'archiver recover' again, or auto-restore per service.")
			return 0
		}
	}
	defer e.release()
	var failed []string
	for _, p := range plan {
		fmt.Fprintf(out, "\n=== %s -> %s ===\n", p.id, p.dir)
		req, code := e.request(p.id, p.dir)
		req.hook = o.Hook
		// The revision planned, not whatever is newest where it ends up coming from: a
		// storage lost since the check must fail the service, not restore an older copy.
		req.revision = strconv.Itoa(listing[p.id].Revision)
		// The storage holding the newest revision first, then the rest as usual.
		for i, t := range req.targets {
			if t.Name == from[p.id] && i > 0 {
				req.targets = append([]config.Target{t}, append(append([]config.Target{}, req.targets[:i]...), req.targets[i+1:]...)...)
				break
			}
		}
		if code == OK {
			code = e.run(req)
		}
		if code != OK {
			failed = append(failed, p.id)
		}
	}
	fmt.Fprintln(out)
	// An unplaced snapshot or a missing one is a service not recovered: the run is
	// incomplete, not done.
	failed = append(failed, unplaced...)
	for _, name := range missing {
		failed = append(failed, host+"-"+name)
	}
	if len(failed) > 0 {
		total := len(plan) + len(unplaced) + len(missing)
		fmt.Fprintf(out, "Restored %d of %d services; NOT RESTORED: %s. Restore each with auto-restore once the cause is fixed.\n", total-len(failed), total, strings.Join(failed, " "))
		return 1
	}
	fmt.Fprintf(out, "All %d services restored.\n\nNext: recreate the deployment with %s/archiver.env as its environment and %s/secrets as /run/secrets,\nwith the hostname '%s' (or HOSTNAME=%s) so new backups continue the same snapshots. See RECREATE.txt.\nThe secrets are plaintext: move them into your secret store and delete %s.\n",
		len(plan), o.Out, o.Out, host, host, o.Out)
	return 0
}

// decryptKit opens a recovery kit exactly as its README does by hand (stock openssl, the
// password on fd 3, never the command line) and unpacks it into dir.
func decryptKit(kit, password, dir string) error {
	rd, wr, err := os.Pipe()
	if err != nil {
		return err
	}
	dec := exec.Command("openssl", "enc", "-d", "-aes-256-cbc", "-pbkdf2", "-pass", "fd:3", "-in", kit)
	dec.ExtraFiles = []*os.File{rd}
	untar := exec.Command("tar", "-xf", "-", "-C", dir)
	var decErr, tarErr bytes.Buffer
	dec.Stderr, untar.Stderr = &decErr, &tarErr
	pipe, err := dec.StdoutPipe()
	if err != nil {
		return err
	}
	untar.Stdin = pipe
	if err := dec.Start(); err != nil {
		rd.Close()
		wr.Close()
		return err
	}
	rd.Close()
	_, _ = io.WriteString(wr, password+"\n")
	wr.Close()
	if err := untar.Start(); err != nil {
		_ = dec.Process.Kill()
		_ = dec.Wait()
		return err
	}
	decErr2 := dec.Wait()
	tarErr2 := untar.Wait()
	switch {
	case decErr2 != nil:
		return fmt.Errorf("openssl: %s", strings.TrimSpace(decErr.String()))
	case tarErr2 != nil:
		return fmt.Errorf("tar: %s", strings.TrimSpace(tarErr.String()))
	}
	return nil
}

// copyPrivate copies a file or a directory tree with owner-only modes (scripts keep their
// execute bit): the kit's secrets are plaintext once out of it.
func copyPrivate(src, dst string) error {
	return filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if fi.IsDir() {
			return privateDir(target)
		}
		if !fi.Mode().IsRegular() {
			return nil
		}
		if err := privateDir(filepath.Dir(target)); err != nil {
			return err
		}
		// Owner-only, keeping the owner's execute bit: the kit's extras carry scripts.
		mode := os.FileMode(0o600)
		if fi.Mode()&0o100 != 0 {
			mode = 0o700
		}
		in, err := os.Open(p) //nolint:gosec // G122: walks a private directory of its own
		if err != nil {
			return err
		}
		defer in.Close()
		return writePrivate(target, mode, in)
	})
}

// privateDir makes dir owner-only, refusing a link where a directory belongs.
func privateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if fi, err := os.Lstat(dir); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s is a link, not a directory", dir)
	}
	return os.Chmod(dir, 0o700) //nolint:gosec // G302: deliberate mode: owner-only, or a log or lock that is not secret
}

// writePrivate writes a new file at target, never through whatever was there: an entry
// left from an earlier run (or put there before the directory was locked) is removed, and
// the file is created exclusively without following links.
func writePrivate(target string, mode os.FileMode, r io.Reader) error {
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Chmod(target, mode)
}

// payloadHash identifies a kit's contents: every file's path and bytes.
func payloadHash(dir string) (string, error) {
	h := sha256.New()
	err := filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil || !fi.Mode().IsRegular() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		f, err := os.Open(p) //nolint:gosec // G122: walks a private directory of its own
		if err != nil {
			return err
		}
		defer f.Close()
		fmt.Fprintf(h, "%s\x00%d\x00", rel, fi.Size())
		_, err = io.Copy(h, f)
		return err
	})
	return hex.EncodeToString(h.Sum(nil)), err
}

// readEnvFile reads the kit's archiver.env: KEY=VALUE lines, values as written.
func readEnvFile(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	env := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok && k != "" && !strings.HasPrefix(k, "#") {
			env[k] = v
		}
	}
	return env, nil
}

var recordedHost = regexp.MustCompile(`(?m)^\s*- hostname: (\S+)`)

// recordedHostname is the hostname RECREATE.txt records, or "".
func recordedHostname(path string) string {
	b, _ := os.ReadFile(path)
	if m := recordedHost.FindSubmatch(b); m != nil {
		return string(m[1])
	}
	return ""
}

type placement struct{ id, dir string }

// placeServices, for a kit without its list of services, maps the host's snapshot IDs to
// directories from SERVICE_DIRECTORIES, which
// on a fresh host may not exist yet: a pattern whose last part matches the service name
// (/srv/*/) places it beside its siblings, a literal path whose name is the service's places
// it there. unplaced are the host's IDs no pattern places.
func placeServices(listing map[string]SnapshotInfo, host string, patterns []string) (plan []placement, unplaced []string) {
	prefix := host + "-"
	var ids []string
	for id := range listing {
		if strings.HasPrefix(id, prefix) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		name := strings.TrimPrefix(id, prefix)
		// Without the kit's list, "nas-home-app" may be host nas's home-app or host
		// nas-home's app: a hyphenated name only a glob would place is left for a person.
		if strings.Contains(name, "-") && !literal(name, patterns) {
			unplaced = append(unplaced, id)
			continue
		}
		dir := dirFor(name, patterns)
		if dir == "" {
			unplaced = append(unplaced, id)
			continue
		}
		plan = append(plan, placement{id, dir})
	}
	return plan, unplaced
}

// literal reports whether a SERVICE_DIRECTORIES entry names service name exactly.
func literal(name string, patterns []string) bool {
	for _, p := range patterns {
		if filepath.Base(filepath.Clean(p)) == name {
			return true
		}
	}
	return false
}

// recordedServices is the service directories RECREATE.txt lists; ok is false for a kit
// made before it listed them.
func recordedServices(path string) (dirs []string, ok bool) {
	b, _ := os.ReadFile(path)
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		if l != kit.ServicesHeader {
			continue
		}
		for _, d := range lines[i+1:] {
			if !strings.HasPrefix(d, "      ") || strings.TrimSpace(d) == "" {
				break
			}
			dirs = append(dirs, strings.TrimPrefix(d, "      "))
		}
		return dirs, true
	}
	return nil, false
}

// dirFor is where SERVICE_DIRECTORIES puts service name: beside its siblings under a glob
// (/srv/*/), or at a literal path of that name; "" when nothing does.
func dirFor(name string, patterns []string) string {
	// A path whose last part is the name itself is taken literally first. With glob
	// characters in it, only when its parent exists as written (a directory named with
	// them, which backups take literally): /srv/tenant[12]/app may instead be a glob over
	// tenants, which a fresh host cannot tell apart, so that is left for a person.
	for _, p := range patterns {
		clean := filepath.Clean(p)
		if filepath.Base(clean) != name {
			continue
		}
		if !config.HasMeta(clean) {
			return config.Unescape(clean)
		}
		if fi, err := os.Stat(filepath.Dir(clean)); err == nil && fi.IsDir() {
			return clean
		}
	}
	// The matcher backups expand SERVICE_DIRECTORIES with, so every service a backup found
	// under a pattern is placed under it again. Two globs that both take the name could each
	// have been its home, so that is left for a person.
	found := ""
	for _, p := range patterns {
		clean := filepath.Clean(p)
		base, parent := filepath.Base(clean), filepath.Dir(clean)
		if config.MatchName(base, name) && !config.HasMeta(parent) {
			dir := filepath.Join(config.Unescape(parent), name)
			if found != "" && found != dir {
				return ""
			}
			found = dir
		}
	}
	return found
}

// placeRecorded plans the services the kit lists, each back into its own directory;
// missing have no snapshot on any readable storage.
func placeRecorded(dirs []string, listing map[string]SnapshotInfo, host string) (plan []placement, missing []string) {
	for _, dir := range dirs {
		name := filepath.Base(dir)
		id := host + "-" + name
		if _, ok := listing[id]; !ok {
			missing = append(missing, name)
			continue
		}
		plan = append(plan, placement{id, dir})
	}
	return plan, missing
}

// missingLiterals are the services named by literal SERVICE_DIRECTORIES paths that no
// readable storage holds, for a kit without its list of services.
func missingLiterals(listing map[string]SnapshotInfo, host string, patterns []string) []string {
	var out []string
	for _, p := range patterns {
		clean := filepath.Clean(p)
		if strings.ContainsAny(clean, "*?[") {
			continue
		}
		name := filepath.Base(clean)
		if _, ok := listing[host+"-"+name]; !ok {
			out = append(out, name)
		}
	}
	return out
}
