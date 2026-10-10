package restore

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/inuse"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/notify"
	"github.com/SisyphusMD/archiver/internal/resume"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

// DrillOptions narrow a drill to one service and/or one storage (`archiver drill [service]
// [storage]`); empty means the configured selection.
type DrillOptions struct {
	Service, Storage string
	Now              func() time.Time
}

// drillRun is one drill in progress.
type drillRun struct {
	e     *Env
	log   *logging.Log
	lock  *runlock.Lock
	state lockstate.DrillState
	now   func() time.Time
	root  string
	run   *resume.Run // this drill's record and the directories it restores into (ADR 46)
	// restored: the drill got as far as restoring, so its errors are its restores' own
	// incidents.
	restored bool
}

// leftover reports whether work, recorded by an interrupted drill, is still safe to delete
// as its copy: a real directory named as a drill makes them, directly in the drill directory
// it recorded, and nowhere in a service's data, which a remount or a changed symlink since
// could have put there.
func (e *Env) leftover(work, root string) bool {
	if root == "" || filepath.Dir(work) != filepath.Clean(root) || !strings.HasPrefix(filepath.Base(work), "drill-") {
		return false
	}
	fi, err := os.Lstat(work)
	if err != nil || !fi.IsDir() {
		return false
	}
	// The mount table lists mounts by their resolved paths.
	real, err := filepath.EvalSymlinks(work)
	if err != nil || mountedUnder(real, "/proc/self/mountinfo") {
		return false
	}
	return !e.inServiceDir(work) && !e.inServiceDir(root)
}

// mountedUnder reports whether anything is mounted at dir or below it: a bind mount there is
// someone's data whatever its path says. A mount table that cannot be read counts as yes.
func mountedUnder(dir, mountinfo string) bool {
	b, err := os.ReadFile(mountinfo)
	if err != nil {
		return true
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		// Field 5 is the mount point, with space, tab, newline and backslash octal-escaped.
		mp := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(f[4])
		if mp == dir || strings.HasPrefix(mp, dir+"/") {
			return true
		}
	}
	return false
}

// Drill restores services' newest revisions into the drill directory, checks them, and
// deletes the copies (ADR 28). It returns 1 when a restore failed or the drill was stopped.
func (e *Env) Drill(o DrillOptions) int {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	log := &logging.Log{Dir: e.Layout.LogDir(), Basename: "drill", Stdout: e.Stdout, Now: now}
	cfg, warnings, err := config.Load(e.Source, e.Environ)
	if err == nil {
		err = cfg.Validate(e.Source.SecretsDir)
	}
	if err != nil {
		log.Message(logging.Error, "", err.Error())
		return 1
	}
	e.cfg = cfg
	d := &drillRun{}
	n := notify.FromConfig(cfg, e.Hostname, func(failed bool, msg string) {
		level := logging.Info
		if failed {
			level = logging.Error
		}
		log.Unnotified(level, "", msg)
	})
	n.Incidents = e.Layout.Incidents()
	n.WatchLog(log)
	// What fails before any restore (the selection, the drill directory) is the drill's own
	// incident; each restore's is its service's on its storage.
	defer func() {
		if log.Reportable() > 0 && !d.restored {
			n.Raise("drill", notify.Failure, "Restore Drill Failed", log.Summary())
		}
	}()

	lock, stale, err := runlock.Acquire(e.Layout.DrillLock(), e.Layout.DrillStopFlag(), "drill", "starting")
	if busy, ok := err.(*runlock.Busy); ok {
		fmt.Fprintf(e.Stderr, "A restore drill is already running (PID %d). Not starting another.\n", busy.Holder.PID)
		return 1
	}
	if err != nil {
		log.Message(logging.Error, "", "Could not take the drill lock: "+err.Error())
		return 1
	}
	defer lock.Release()
	log.Rotate()
	if stale {
		log.Message(logging.Warning, "", "Stale drill lock file found. Cleaned up and proceeding.")
	}
	for _, w := range warnings {
		log.Message(logging.Warning, "", w)
	}

	d.e, d.log, d.lock, d.now, d.root = e, log, lock, now, cfg.DrillDirectory()
	// An interrupted drill left its copies behind: deleted, exactly the directories it
	// recorded creating, before this drill takes its place.
	if prior, ok := resume.Read(e.Layout.RunRecord("drill")); ok {
		log.Message(logging.Warning, "", fmt.Sprintf("The restore drill started %s was interrupted; deleting its copies, and this drill takes its place.", logging.Timestamp(prior.Started)))
		for work := range prior.Services {
			if !e.leftover(work, prior.Dir) {
				log.Message(logging.Warning, "", fmt.Sprintf("Not deleting %s, recorded as an interrupted drill's copy: it is no longer a drill directory of its own (it lies in a service directory, or is not one directly in the drill directory).", work))
				continue
			}
			if err := os.RemoveAll(work); err != nil {
				log.Message(logging.Warning, "", fmt.Sprintf("Could not delete %s, an interrupted drill's copy: %v", work, err))
			}
		}
	}
	d.run, _ = resume.Begin(e.Layout.RunRecord("drill"), resume.Record{Dir: d.root})
	defer func() {
		if lock.StopRequested() && resume.ShuttingDown(e.Layout.ShutdownFlag()) {
			log.Message(logging.Info, "", "The restore drill stopped for a container shutdown; it runs again when the container starts.")
			return
		}
		d.run.End()
	}()
	// A stop ends whatever the drill waits on (a lock a backup or prune holds) or runs, and a
	// pause holds back every program it would start: with nothing running yet, there would
	// be no process for the stop or pause to reach.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for ctx.Err() == nil {
			if lock.StopRequested() {
				cancel()
				return
			}
			time.Sleep(time.Second)
		}
	}()
	e.ctx = ctx
	e.beforeRun = func() {
		for lock.State().Paused() && !lock.StopRequested() {
			time.Sleep(time.Second)
		}
	}
	defer func() { e.ctx, e.beforeRun = nil, nil }()
	d.state, err = lockstate.ReadDrillState(e.Layout.DrillState())
	if err != nil {
		log.Message(logging.Warning, "", "Unreadable drill state, starting afresh: "+err.Error())
	}
	if d.state.Rotation == nil {
		d.state.Rotation = map[string]int{}
	}
	if d.state.Results == nil {
		d.state.Results = map[string]map[string]lockstate.DrillResult{}
	}

	services, targets, err := d.selection(o)
	if err != nil {
		log.Message(logging.Error, "", err.Error())
		d.failedEarly()
		return 1
	}
	if err := os.MkdirAll(d.root, 0o700); err != nil {
		log.Message(logging.Error, "", fmt.Sprintf("Cannot create the drill directory %s: %v", d.root, err))
		d.failedEarly()
		return 1
	}
	// A drill writes and deletes in its directory, so it must never be, hold or lie in a
	// service's data.
	if e.inServiceDir(d.root) {
		log.Message(logging.Error, "", fmt.Sprintf("RESTORE_DRILL_DIR %s is, holds or lies in a service directory; point it at scratch space of its own.", d.root))
		d.failedEarly()
		return 1
	}
	log.Message(logging.Info, "", fmt.Sprintf("Restore drill started: %d storage(s), drill directory %s.", len(targets), d.root))
	d.restored = true
	n.Clear("drill", "Restore Drills Running Again", "Restore drills start again.")

	count, _ := cfg.DrillCount()
	tried, failed, stopped := 0, 0, false
	started := now()
	for _, t := range targets {
		for _, name := range d.pick(t, services, count, o.Service != "") {
			if d.stopRequested() {
				stopped = true
				break
			}
			r := d.one(t, name)
			if d.state.Results[t.StorageName()] == nil {
				d.state.Results[t.StorageName()] = map[string]lockstate.DrillResult{}
			}
			d.state.Results[t.StorageName()][name] = r
			key := "drill:" + t.StorageName() + ":" + name
			if !r.Skipped {
				tried++
				if !r.OK {
					failed++
					n.Raise(key, notify.Failure, "Restore Drill Failed", fmt.Sprintf("Restore drill of %s-%s from '%s' failed: %s", e.Hostname, name, t.Name, r.Message))
				} else {
					n.Clear(key, "Restore Drill Passing", fmt.Sprintf("A restore drill of %s-%s from '%s' passes again.", e.Hostname, name, t.Name))
				}
			}
			if lock.StopRequested() {
				stopped = true
				break
			}
		}
		if stopped {
			break
		}
	}

	// A failure stays an incident until its service passes a drill, however far off its
	// turn in the rotation is and whether later attempts were skipped: raised each run, it
	// repeats on its own interval.
	if !stopped {
		open := notify.OpenIncidents(n.Incidents)
		for _, t := range targets {
			for _, name := range services {
				key := "drill:" + t.StorageName() + ":" + name
				r := d.state.Results[t.StorageName()][name]
				if _, ok := open[key]; ok && (r.At < started.Unix() || r.Skipped) {
					n.Raise(key, notify.Failure, "Restore Drill Failed",
						fmt.Sprintf("No restore drill of %s-%s from '%s' has passed since one failed (the last: %s).", e.Hostname, name, t.Name, r.Message))
				}
			}
		}
	}

	end := now().Unix()
	d.state.LastRun, d.state.LastFailed = end, failed > 0
	if failed == 0 && tried > 0 && !stopped {
		d.state.LastPass = end
	}
	if err := lockstate.WriteDrillState(e.Layout.DrillState(), d.state); err != nil {
		log.Message(logging.Warning, "", "Could not record the drill: "+err.Error())
	}
	switch {
	case stopped:
		log.Message(logging.Info, "", "Restore drill stopped.")
		_ = lock.Record("stopped")
		return 1
	case failed > 0:
		log.Unnotified(logging.Error, "", fmt.Sprintf("Restore drill finished: %d of %d restore(s) failed.", failed, tried))
		_ = lock.Record("failed")
		return 1
	case tried == 0:
		log.Message(logging.Warning, "", "Restore drill finished without restoring anything (every service skipped).")
	default:
		log.Message(logging.Info, "", fmt.Sprintf("Restore drill finished: %d restore(s) verified.", tried))
		n.Send("Restore Drill Complete", fmt.Sprintf("%d restore(s) verified.", tried))
	}
	_ = lock.Record("completed")
	return 0
}

// failedEarly records a drill that failed before restoring anything, so the healthcheck
// sees the failure rather than a drill that never ran.
func (d *drillRun) failedEarly() {
	d.state.LastRun, d.state.LastFailed = d.now().Unix(), true
	_ = lockstate.WriteDrillState(d.e.Layout.DrillState(), d.state)
	_ = d.lock.Record("failed")
}

// selection is the services (sorted names, exclusions dropped) and storages a drill covers.
func (d *drillRun) selection(o DrillOptions) ([]string, []config.Target, error) {
	cfg := d.e.cfg
	dirs, _ := config.ExpandServiceDirectories(cfg.ServiceDirectories)
	var services []string
	for _, dir := range dirs {
		name := filepath.Base(dir)
		if o.Service != "" {
			if name == o.Service {
				services = append(services, name)
			}
			continue
		}
		if !cfg.DrillExcluded(name) {
			services = append(services, name)
		}
	}
	sort.Strings(services)
	if o.Service != "" && len(services) == 0 {
		return nil, nil, fmt.Errorf("'%s' is not a configured service (SERVICE_DIRECTORIES).", o.Service)
	}
	targets, err := cfg.DrillTargets()
	if err != nil {
		return nil, nil, err
	}
	if o.Storage != "" {
		var only []config.Target
		for _, t := range cfg.Targets {
			if t.Name == o.Storage {
				only = append(only, t)
			}
		}
		if len(only) == 0 {
			return nil, nil, fmt.Errorf("'%s' is not a configured storage (STORAGE_TARGET_N_NAME).", o.Storage)
		}
		targets = only
	}
	return services, targets, nil
}

// pick is the services drilled on t this run: all of them for count 0 or a named service,
// else the next count in rotation, so each is drilled in turn across runs.
func (d *drillRun) pick(t config.Target, services []string, count int, named bool) []string {
	if named || count == 0 || count >= len(services) {
		return services
	}
	start := d.state.Rotation[t.StorageName()] % len(services)
	var out []string
	for i := 0; i < count; i++ {
		out = append(out, services[(start+i)%len(services)])
	}
	d.state.Rotation[t.StorageName()] = (start + count) % len(services)
	return out
}

func (d *drillRun) stopRequested() bool {
	for d.lock.State().Paused() && !d.lock.StopRequested() {
		time.Sleep(time.Second)
	}
	return d.lock.StopRequested()
}

// listedFile is one file line of `duplicacy list -files`: size (right-aligned, so smaller
// ones are padded with spaces), date, time, then the hash (blank for an empty file) and path.
var listedFile = regexp.MustCompile(`(?m)^ *\d+ \d{4}-\d\d-\d\d \d\d:\d\d:\d\d .*$`)
var listSize = regexp.MustCompile(`(?m)^Total size: (\d+)`)

// one drills service name on t: its newest revision restored into a scratch directory,
// every chunk verified by duplicacy as it downloads, the file count checked against the
// revision's listing. Restore hooks never run, and the copy is deleted afterwards.
func (d *drillRun) one(t config.Target, name string) lockstate.DrillResult {
	r := lockstate.DrillResult{At: d.now().Unix()}
	id := d.e.Hostname + "-" + name
	out := d.log.Writer(logging.Info, name)
	defer out.Close()
	fail := func(msg string) lockstate.DrillResult {
		// Whatever a stop interrupted fails; that is the stop, not a broken backup.
		if d.lock.StopRequested() {
			r.Skipped, r.Message = true, "stop requested"
			return r
		}
		r.Message = msg
		d.log.Message(logging.Error, name, fmt.Sprintf("Restore drill of %s from '%s' failed: %s", id, t.Name, msg))
		return r
	}
	skip := func(msg string) lockstate.DrillResult {
		r.Skipped, r.Message = true, msg
		d.log.Message(logging.Warning, name, fmt.Sprintf("Restore drill of %s from '%s' skipped: %s", id, t.Name, msg))
		return r
	}
	// A directory of its own, never one that already exists: only what the drill created
	// is ever deleted.
	work, err := os.MkdirTemp(d.root, "drill-"+t.StorageName()+"-"+name+"-")
	if err != nil {
		return fail(err.Error())
	}
	_ = d.run.Set(work, resume.Pending)
	defer func() {
		if os.RemoveAll(work) == nil {
			d.run.Forget(work)
		}
	}()
	_ = d.lock.SetStage("drill", t.StorageName()+"/"+name)

	if err := d.e.connect(work, t, id, out); err != nil {
		return fail(err.Error())
	}
	// The revision is chosen and registered in use under the storage's gate, so no prune
	// can delete it between being picked and being restored (ADR 19).
	release, err := inuse.Gate(d.e.context(), d.e.Layout.InUseDir(), t.StorageName(), false)
	if err != nil {
		if d.lock.StopRequested() {
			return skip("stop requested")
		}
		return fail(err.Error())
	}
	if d.stopRequested() {
		release()
		return skip("stop requested")
	}
	revs, err := d.e.revisions(work, id)
	if err != nil {
		release()
		return fail("cannot list its revisions")
	}
	if len(revs) == 0 {
		release()
		return skip("no revision on this storage yet")
	}
	r.Revision = revs[0]
	held, err := inuse.Hold(d.e.Layout.InUseDir(), "drill", []inuse.Revision{{Storage: t.StorageName(), ID: id, Rev: r.Revision}})
	release()
	if err != nil {
		return fail(err.Error())
	}
	defer held.Release()
	var listing strings.Builder
	if err := d.e.duplicacy(work, &listing, "list", "-files", "-r", strconv.Itoa(r.Revision), "-id", id).Run(); err != nil {
		return fail(fmt.Sprintf("cannot list the files of revision %d", r.Revision))
	}
	// The files listed are counted: duplicacy's "Files:" summary is missing for a revision
	// without files, and for one whose files are all empty.
	want := len(listedFile.FindAllString(listing.String(), -1))
	if s := listSize.FindStringSubmatch(listing.String()); s != nil {
		size, _ := strconv.ParseInt(s[1], 10, 64)
		if free, ok := freeBytes(d.root); ok && free < size+size/20+64<<20 {
			return skip(fmt.Sprintf("revision %d needs %d MB but %s has %d MB free", r.Revision, size>>20+1, d.root, free>>20))
		}
	}
	if d.stopRequested() {
		return skip("stop requested")
	}
	d.log.Message(logging.Info, name, fmt.Sprintf("Drilling %s revision %d from '%s' (%d files).", id, r.Revision, t.Name, want))
	if err := d.e.restore(work, t, id, r.Revision, Options{IgnoreOwner: true, Threads: d.e.cfg.Threads}, out); err != nil {
		return fail(fmt.Sprintf("restoring revision %d: %v", r.Revision, err))
	}
	got, err := countFiles(work)
	if err != nil {
		return fail(err.Error())
	}
	if got != want {
		return fail(fmt.Sprintf("revision %d restored %d files, its listing has %d", r.Revision, got, want))
	}
	r.OK, r.Files = true, got
	d.log.Message(logging.Info, name, fmt.Sprintf("Restore drill of %s revision %d from '%s' passed (%d files).", id, r.Revision, t.Name, got))
	return r
}

// countFiles counts the regular files under dir, its .duplicacy repository left out: the
// number duplicacy's listing reports.
func countFiles(dir string) (int, error) {
	n := 0
	err := filepath.WalkDir(dir, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if de.IsDir() && p == filepath.Join(dir, ".duplicacy") {
			return filepath.SkipDir
		}
		if de.Type().IsRegular() {
			n++
		}
		return nil
	})
	return n, err
}

func freeBytes(dir string) (int64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, false
	}
	return int64(st.Bavail) * int64(st.Bsize), true //nolint:gosec // G115: filesystem sizes and byte values fit
}
