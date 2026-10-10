// Package doctor implements `archiver doctor` (ADR 30): one read-only pass over the
// configuration and secrets, every storage, the container, and how fresh the backups, copies,
// kit, envelope and drills are, each line OK, WARN or FAIL with what to do. Nothing is
// written to a storage; a test notification is sent only with --notify.
package doctor

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/copier"
	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/envelope"
	"github.com/SisyphusMD/archiver/internal/hooks"
	"github.com/SisyphusMD/archiver/internal/kit"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/notify"
	"github.com/SisyphusMD/archiver/internal/restore"
	"github.com/SisyphusMD/archiver/internal/status"
)

// Env is what doctor reads and runs.
type Env struct {
	Layout    layout.Layout
	Source    config.Source
	Environ   []string
	Hostname  string
	Duplicacy string
	Rclone    string // "" for rclone on PATH
	Out       io.Writer
	CapEff    int64 // the effective capability set, or -1 when unknown
	Notify    bool  // --notify: send a test notification to every destination
	Now       func() time.Time
	// Probe lists a storage's snapshots (restore.Env.Snapshots) and HasConfig checks one
	// exists (kit.HasConfig, the default); tests stand in their own.
	Probe     func(config.Target) (map[string]restore.SnapshotInfo, error)
	HasConfig func(config.Target) error
	HasFile   func(config.Target, string) error // kit.HasFile by default
}

type report struct {
	w           io.Writer
	fails, warn int
}

func (r *report) section(name string)    { fmt.Fprintf(r.w, "\n== %s ==\n", name) }
func (r *report) ok(f string, a ...any)  { fmt.Fprintf(r.w, "OK    "+f+"\n", a...) }
func (r *report) wrn(f string, a ...any) { r.warn++; fmt.Fprintf(r.w, "WARN  "+f+"\n", a...) }
func (r *report) bad(f string, a ...any) { r.fails++; fmt.Fprintf(r.w, "FAIL  "+f+"\n", a...) }

// Run writes the report and returns 1 when anything failed, else 0.
func Run(e Env) int {
	if e.Now == nil {
		e.Now = time.Now
	}
	r := &report{w: e.Out}
	now := e.Now()

	r.section("Configuration and secrets")
	cfg, warnings, err := config.Load(e.Source, e.Environ)
	if err != nil {
		r.bad("The configuration cannot be read: %v", err)
		return r.summary()
	}
	for _, w := range warnings {
		r.wrn("%s", w)
	}
	if err := cfg.Validate(e.Source.SecretsDir); err != nil {
		r.bad("%v", err)
	} else {
		r.ok("Settings and required secrets are valid (%d storage(s))", len(cfg.Targets))
	}
	if jobs, err := daemon.Jobs(e.Source.Getenv, now); err != nil {
		r.bad("%v: the container refuses to start", err)
	} else if len(jobs) > 0 {
		var names []string
		for _, j := range jobs {
			names = append(names, j.Env+"="+j.Spec)
		}
		r.ok("Schedules: %s", strings.Join(names, ", "))
	}
	e.keys(r, cfg)
	e.kitPassword(r, cfg)
	e.notifier(r, cfg)

	r.section("Storages")
	snaps := e.storages(r, cfg)

	r.section("Container")
	services := e.container(r, cfg)

	r.section("Freshness")
	e.freshness(r, cfg, services, snaps, now)

	return r.summary()
}

func (r *report) summary() int {
	fmt.Fprintf(r.w, "\n%d failure(s), %d warning(s).\n", r.fails, r.warn)
	if r.fails > 0 {
		return 1
	}
	return 0
}

// keys checks that the RSA private key decrypts with the passphrase and matches the public
// key, the pair every revision's chunks are encrypted to. The passphrase reaches openssl on
// fd 3, never its command line.
func (e Env) keys(r *report, cfg *config.Config) {
	priv, pub := e.Layout.RSAPrivateKey(), filepath.Join(e.Layout.Root, "keys", "public.pem")
	if cfg.RSAPassphrase == "" {
		return // reported by the validation above
	}
	derived, err := openssl(cfg.RSAPassphrase, "rsa", "-in", priv, "-passin", "fd:3", "-pubout")
	if err != nil {
		r.bad("The RSA private key %s does not decrypt with rsa_passphrase: %v", priv, err)
		return
	}
	want, err := os.ReadFile(pub)
	switch {
	case err != nil:
		r.bad("The RSA public key %s cannot be read: %v", pub, err)
	case !bytes.Equal(bytes.TrimSpace(derived), bytes.TrimSpace(want)):
		r.bad("The RSA public key %s is not the private key's: new revisions would be encrypted to a key you cannot decrypt", pub)
	default:
		r.ok("The RSA key pair decrypts with rsa_passphrase and its halves match")
	}
}

func openssl(pass string, args ...string) ([]byte, error) {
	rd, wr, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("openssl", args...)
	cmd.ExtraFiles = []*os.File{rd}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		rd.Close()
		wr.Close()
		return nil, err
	}
	rd.Close()
	_, _ = io.WriteString(wr, pass+"\n")
	wr.Close()
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

func (e Env) kitPassword(r *report, cfg *config.Config) {
	if cfg.RecoveryPassword == "" {
		r.wrn("No recovery_password secret: no recovery kit is written, so a disaster recovery needs the keys and settings from elsewhere")
		return
	}
	if err := kit.ValidatePassword(cfg.RecoveryPassword, cfg.StoragePassword); err != nil {
		r.bad("%v", err)
		return
	}
	r.ok("The recovery kit password is set")
}

func (e Env) notifier(r *report, cfg *config.Config) {
	var results []string
	failed := false
	n := notify.FromConfig(cfg, e.Hostname, func(f bool, msg string) {
		failed = failed || f
		results = append(results, msg)
	})
	if len(n.Destinations) == 0 {
		r.wrn("No notification destination is configured: failures are only in the logs")
		return
	}
	var names []string
	for _, d := range n.Destinations {
		names = append(names, d.Name()+" ("+d.On+")")
	}
	if !e.Notify {
		r.ok("Notifications go to %s; 'archiver doctor --notify' sends a test", strings.Join(names, ", "))
		return
	}
	for i := range n.Destinations {
		n.Destinations[i].On = "everything" // a test goes everywhere, whatever each receives
	}
	n.Send("Archiver Doctor Test", "A test notification from 'archiver doctor --notify'.")
	if failed {
		r.bad("Test notification: %s", strings.Join(results, "; "))
	} else {
		r.ok("Test notification sent to %s", strings.Join(names, ", "))
	}
}

// storages reaches each storage read-only: its Duplicacy config through rclone, then its
// snapshot listing, which needs the storage password. It returns the listings by storage.
func (e Env) storages(r *report, cfg *config.Config) map[string]map[string]restore.SnapshotInfo {
	out := map[string]map[string]restore.SnapshotInfo{}
	for _, t := range cfg.Targets {
		role := "secondary"
		if t.N == 1 {
			role = "primary"
		}
		has := e.HasConfig
		if has == nil {
			has = func(t config.Target) error { return kit.HasConfig(e.Layout, e.Rclone, t) }
		}
		if err := has(t); err != nil {
			r.bad("Storage '%s' (%s, %s) cannot be read: %v", t.Name, t.Type, role, err)
			continue
		}
		s, err := e.Probe(t)
		var keyErr *restore.KeyError
		if errors.As(err, &keyErr) {
			r.bad("Storage '%s' (%s, %s) opens, but %v", t.Name, t.Type, role, err)
			continue
		}
		var unchecked *restore.KeyUnchecked
		if errors.As(err, &unchecked) {
			out[t.StorageName()] = s
			r.wrn("Storage '%s' (%s, %s) opens with the configured password, but %v", t.Name, t.Type, role, err)
			continue
		}
		if err != nil {
			r.bad("Storage '%s' (%s, %s) is reachable but cannot be opened with the configured password: %v", t.Name, t.Type, role, err)
			continue
		}
		out[t.StorageName()] = s
		r.ok("Storage '%s' (%s, %s) is reachable, opens with the configured password, and its data decrypts with the RSA key (%d snapshot IDs)", t.Name, t.Type, role, len(s))
	}
	return out
}

// container checks what the container gives Archiver, returning the service names.
func (e Env) container(r *report, cfg *config.Config) []string {
	const capChown, capDAC, capFowner = 1 << 0, 1 << 1, 1 << 3
	switch {
	case e.CapEff < 0:
		r.wrn("The container's capabilities could not be read")
	case e.CapEff&capDAC == 0:
		r.bad("DAC_OVERRIDE is not granted: files other users own cannot be backed up (add it to cap_add)")
	case e.CapEff&capChown == 0 || e.CapEff&capFowner == 0:
		r.wrn("CHOWN or FOWNER is not granted: restores cannot keep owners and modes (add both to cap_add)")
	default:
		r.ok("Capabilities: DAC_OVERRIDE, CHOWN and FOWNER are granted")
	}

	logs := e.Layout.LogDir()
	if mounted(logs) {
		r.ok("The logs directory %s is a mounted volume", logs)
	} else {
		r.wrn("The logs directory %s is not a mounted volume: state (copy workers, kit, drills, fossil collections) is lost when the container is recreated", logs)
	}
	if mb, ok := freeMB(logs); ok && mb < 100 {
		r.bad("Only %d MB free for logs", mb)
	}
	if spec := e.Source.Getenv("RESTORE_DRILL_SCHEDULE"); spec != "" {
		dir := cfg.DrillDirectory()
		probe := dir
		for probe != "/" {
			if _, err := os.Stat(probe); err == nil {
				break
			}
			probe = filepath.Dir(probe)
		}
		if mb, ok := freeMB(probe); ok {
			r.ok("Restore drills restore into %s (%d MB free); a service larger than that is skipped", dir, mb)
		}
	}

	dirs, unmatched := config.ExpandServiceDirectories(cfg.ServiceDirectories)
	for _, u := range unmatched {
		r.bad("SERVICE_DIRECTORIES pattern %s matches no directory: nothing under it is backed up", u)
	}
	var names []string
	for _, d := range dirs {
		name := filepath.Base(d)
		names = append(names, name)
		if id := e.Hostname + "-" + name; !config.ValidSnapshotID(id) {
			r.bad("%s: its snapshot ID '%s' may contain only letters, digits, '_' and '-' (a Duplicacy rule), so it is never backed up; rename the directory or the host", name, id)
		}
		if _, err := os.Lstat(filepath.Join(d, hooks.Legacy)); err == nil {
			r.bad("%s still has %s, which this release does not run: its backups are skipped until 'archiver migrate hooks' converts it", name, hooks.Legacy)
		}
		hd := hooks.Hooks(cfg.HooksDir, d)
		for _, h := range []string{hooks.PreBackup, hooks.PostBackup, hooks.PostRestore} {
			if _, err := hooks.Exists(hd, h); err != nil {
				r.bad("%s: %v", name, err)
			}
		}
		// Without post-restore, a restore runs restore-service.sh through bash: no execute bit
		// needed, but the same check on who can change it.
		legacy := filepath.Join(hd, hooks.LegacyRestore)
		if _, err := os.Lstat(filepath.Join(hd, hooks.PostRestore)); os.IsNotExist(err) {
			if _, err := os.Lstat(legacy); err == nil {
				if err := hooks.Safe(legacy); err != nil {
					r.bad("%s: %v", name, err)
				}
			}
		}
	}
	if len(dirs) > 0 {
		r.ok("%d service directories found: %s", len(dirs), strings.Join(names, ", "))
	}
	return names
}

// freshness reports how current the backups, copies, kit, envelope and drills are.
func (e Env) freshness(r *report, cfg *config.Config, services []string, snaps map[string]map[string]restore.SnapshotInfo, now time.Time) {
	every, scheduled := daemon.Interval(e.Source.Getenv("BACKUP_SCHEDULE"), now)
	for _, t := range cfg.Targets {
		s, ok := snaps[t.StorageName()]
		if !ok {
			continue // reported unreachable above
		}
		for _, name := range services {
			id := e.Hostname + "-" + name
			info, ok := s[id]
			switch {
			case !ok:
				r.wrn("%s has no revision on '%s' yet", id, t.Name)
			case scheduled && now.Sub(info.Created) > 2*every:
				r.wrn("%s's newest revision on '%s' is %d, from %s (more than twice BACKUP_SCHEDULE's interval)", id, t.Name, info.Revision, status.Age(info.Created.Unix(), now))
			case t.N == 1:
				r.ok("%s: newest revision %d on '%s', %s", id, info.Revision, t.Name, status.Age(info.Created.Unix(), now))
			default:
				if p, ok := snaps[cfg.Targets[0].StorageName()][id]; ok && info.Revision < p.Revision {
					r.wrn("%s on '%s' is at revision %d, behind the primary's %d: its copies have not caught up", id, t.Name, info.Revision, p.Revision)
				}
			}
		}
	}

	states := (&copier.Store{Path: e.Layout.CopyWorkersState()}).Load()
	names := make([]string, 0, len(states))
	for n := range states {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		s := states[n]
		switch {
		case s.Status == copier.Down:
			r.bad("Copies to %s: DOWN since %s: %s", n, status.Age(s.DownSince, now), s.LastError)
		case s.Status == copier.Retrying:
			r.wrn("Copies to %s: retrying, failing since %s: %s", n, status.Age(s.FailingSince, now), s.LastError)
		case s.CheckFailed != "":
			r.wrn("Copies to %s: the last check failed: %s", n, s.CheckFailed)
		default:
			r.ok("Copies to %s: %s", n, s.Status)
		}
	}

	k := &kit.Run{Layout: e.Layout, Source: e.Source, Environ: e.Environ, Hostname: e.Hostname,
		Log: &logging.Log{Dir: os.TempDir(), Basename: "doctor-kit", Stdout: io.Discard}}
	if placed, configured, err := k.CurrentOn(); err != nil {
		r.wrn("Could not tell where the current recovery kit is: %v", err)
	} else if configured {
		has := e.HasFile
		if has == nil {
			has = func(t config.Target, name string) error { return kit.HasFile(e.Layout, e.Rclone, t, name) }
		}
		var missing, vanished, unverified []string
		for _, t := range cfg.Targets {
			recorded := false
			for _, p := range placed {
				recorded = recorded || p == t.StorageName()
			}
			switch {
			case !recorded:
				missing = append(missing, t.Name)
			default:
				// Recorded is not enough: the file is looked for on the storage itself.
				err := has(t, k.KitName())
				switch {
				case errors.Is(err, kit.ErrNotFound):
					vanished = append(vanished, t.Name)
				case err != nil:
					unverified = append(unverified, fmt.Sprintf("%s (%v)", t.Name, err))
				}
			}
		}
		if len(missing) > 0 {
			r.wrn("The recovery kit for this configuration is not yet on: %s (the next backup places it; 'archiver recovery-kit' now)", strings.Join(missing, ", "))
		}
		if len(vanished) > 0 {
			r.wrn("The recovery kit recorded as placed is gone from: %s ('archiver recovery-kit force' places it again; a normal run trusts its record)", strings.Join(vanished, ", "))
		}
		if len(unverified) > 0 {
			r.wrn("Could not check the recovery kit is still on: %s", strings.Join(unverified, "; "))
		}
		if len(missing)+len(vanished)+len(unverified) == 0 {
			r.ok("The recovery kit for this configuration is on every storage")
		}
	}

	if s, err := config.Snapshot(e.Source, e.Environ); err == nil {
		page := envelope.Build(envelope.Source{Settings: s, Hostname: e.Hostname, SSHKeyFile: e.Layout.SSHPrivateKey()})
		switch c := envelope.Compare(e.Layout, page, now); {
		case !c.Confirmed:
		case !c.Matches:
			r.wrn("The printed envelope (confirmed %s) no longer matches the configuration: print a new one ('archiver envelope', then 'archiver envelope confirm')", status.Age(c.ConfirmedAt.Unix(), now))
		case c.Due:
			r.wrn("The printed envelope was confirmed %s: its yearly check is due", status.Age(c.ConfirmedAt.Unix(), now))
		default:
			r.ok("The printed envelope matches the configuration (confirmed %s)", status.Age(c.ConfirmedAt.Unix(), now))
		}
	}

	e.drills(r, now)
}

// drills applies the healthcheck's drill rules (ADR 28): a failed last drill, or none
// passing within twice the schedule's interval, counted from scheduling until one passes.
func (e Env) drills(r *report, now time.Time) {
	spec := e.Source.Getenv("RESTORE_DRILL_SCHEDULE")
	ds, err := lockstate.ReadDrillState(e.Layout.DrillState())
	if err != nil {
		r.wrn("The restore drill state is unreadable: %v", err)
		return
	}
	if ds.LastFailed {
		r.wrn("The last restore drill failed (%s); see 'archiver status'", status.Age(ds.LastRun, now))
	}
	if spec == "" {
		if ds.LastRun > 0 && !ds.LastFailed {
			r.ok("Last restore drill %s (no RESTORE_DRILL_SCHEDULE)", status.Age(ds.LastRun, now))
		}
		return
	}
	since := ds.LastPass
	if since == 0 {
		since = ds.Scheduled
	}
	if since == 0 {
		since = ds.LastRun
	}
	every, ok := daemon.Interval(spec, now)
	switch {
	case ok && since > 0 && now.Sub(time.Unix(since, 0)) > 2*every:
		r.wrn("No restore drill has passed since %s (more than twice RESTORE_DRILL_SCHEDULE's interval)", status.Age(since, now))
	case ds.LastPass > 0 && !ds.LastFailed:
		r.ok("Last passing restore drill %s", status.Age(ds.LastPass, now))
	case ds.LastRun == 0:
		r.ok("Restore drills scheduled (%s); none has run yet", spec)
	}
}

// mounted reports whether dir is on a mount of its own or under one other than the
// container's root, from the mount table: a bind mount can share its parent's device.
func mounted(dir string) bool {
	b, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false
	}
	return underMount(dir, string(b))
}

func underMount(dir, mountinfo string) bool {
	dir = filepath.Clean(dir)
	for _, line := range strings.Split(mountinfo, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 || f[4] == "/" {
			continue
		}
		// Mount points escape spaces and the like as octal: \040.
		mp := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(f[4])
		if dir == mp || strings.HasPrefix(dir, mp+"/") {
			return true
		}
	}
	return false
}

func freeMB(dir string) (int64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, false
	}
	return int64(st.Bavail) * int64(st.Bsize) >> 20, true //nolint:gosec // G115: filesystem sizes and byte values fit
}
