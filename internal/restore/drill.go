package restore

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/hooks"
	"github.com/SisyphusMD/archiver/internal/inuse"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/notify"
	"github.com/SisyphusMD/archiver/internal/proc"
	"github.com/SisyphusMD/archiver/internal/resume"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

// DrillOptions narrow a drill to one service and/or one storage (`archiver drill [service]
// [storage]`); empty means the configured selection.
type DrillOptions struct {
	Service, Storage, Named string
	Now                     func() time.Time
}

// drillRun is one drill in progress.
type drillRun struct {
	e        *Env
	log      *logging.Log
	lock     *runlock.Lock
	state    lockstate.DrillState
	allState lockstate.DrillState
	named    config.NamedDrill
	now      func() time.Time
	root     string
	run      *resume.Run // this drill's record and the directories it restores into (ADR 46)
	// restored: the drill got as far as restoring, so its errors are its restores' own
	// incidents.
	restored bool
	// dirs maps a service name to its directory, for its drill hook.
	dirs map[string]string
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
	if o.Named != "" {
		var ok bool
		d.named, ok = cfg.NamedDrill(o.Named)
		if !ok {
			log.Message(logging.Error, "", "Unknown named restore drill: "+o.Named)
			return 1
		}
		if o.Service != "" || o.Storage != "" {
			log.Message(logging.Error, "", "A named drill cannot override its services or storages.")
			return 1
		}
	}
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
			n.Raise(d.incident("", ""), notify.Failure, "Restore Drill Failed", d.message(log.Summary()))
		}
	}()

	lock, stale, err := runlock.Acquire(e.Layout.DrillLock(), e.Layout.DrillStopFlag(), d.incident("", ""), "starting")
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
			if isWorkRepo(work) {
				if err := os.RemoveAll(work); err != nil {
					log.Message(logging.Warning, "", fmt.Sprintf("Could not delete %s, an interrupted drill's repository: %v", work, err))
				}
				continue
			}
			// Judged against the configured drill directory, never the one the record (on
			// the logs volume) names.
			if !e.leftover(work, d.root) {
				log.Message(logging.Warning, "", fmt.Sprintf("Not deleting %s, recorded as an interrupted drill's copy: it is no longer a drill directory of its own (it lies in a service directory, or is not one directly in the drill directory).", work))
				continue
			}
			if err := os.RemoveAll(work); err != nil {
				log.Message(logging.Warning, "", fmt.Sprintf("Could not delete %s, an interrupted drill's copy: %v", work, err))
			}
		}
	}
	d.run, _ = resume.Begin(e.Layout.RunRecord("drill"), resume.Record{Drill: o.Named})
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
	limit, _ := cfg.DrillTimeout()
	if o.Named != "" {
		limit = d.named.Timeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
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
		for lock.State().Paused() && !lock.StopRequested() && ctx.Err() == nil {
			time.Sleep(time.Second)
		}
	}
	defer func() { e.ctx, e.beforeRun = nil, nil }()
	d.state, err = lockstate.ReadDrillState(e.Layout.DrillState())
	if err != nil {
		log.Message(logging.Warning, "", "Unreadable drill state, starting afresh: "+err.Error())
	}
	if o.Named != "" {
		d.allState = d.state
		d.state = lockstate.DrillState{}
		if saved := d.allState.Named[o.Named]; saved != nil {
			d.state = *saved
		}
	}
	if d.state.Pending == nil {
		d.state.Pending = map[string][]string{}
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
	log.Message(logging.Info, "", fmt.Sprintf("Restore drill started (%s): %d storage(s), drill directory %s.", drillName(o.Named), len(targets), d.root))
	d.restored = true
	n.Clear(d.incident("", ""), "Restore Drills Running Again", d.message("Restore drills start again."))

	count, _ := cfg.DrillCount()
	if o.Named != "" {
		count = 0
	}
	tried, failed, stopped := 0, 0, false
	started := now()
	for _, t := range targets {
		if o.Named != "" {
			d.state.Pending[t.StorageName()] = slices.DeleteFunc(d.state.Pending[t.StorageName()], func(name string) bool { return !slices.Contains(services, name) })
		}
		picked := d.pick(t, services, count, o.Service != "" || o.Named != "")
		if err := d.save(); err != nil {
			d.log.Message(logging.Error, "", "Could not record pending drill selections: "+err.Error())
			return 1
		}
		for _, name := range picked {
			if d.stopRequested() {
				stopped = true
				break
			}
			prior := d.state.Results[t.StorageName()][name]
			if d.state.Results[t.StorageName()] == nil {
				d.state.Results[t.StorageName()] = map[string]lockstate.DrillResult{}
			}
			d.state.Results[t.StorageName()][name] = lockstate.DrillResult{At: now().Unix(), LastSuccess: prior.LastSuccess, Interrupted: true, Message: "interrupted; remains due"}
			if err := d.save(); err != nil {
				d.log.Message(logging.Error, "", "Could not record pending drill result: "+err.Error())
				return 1
			}
			r := d.one(t, name)
			d.state.Results[t.StorageName()][name] = r
			key := d.incident(t.StorageName(), name)
			if !r.Interrupted && (r.OK || (r.Skipped && !n.IsOpen(key))) {
				d.state.Pending[t.StorageName()] = slices.DeleteFunc(d.state.Pending[t.StorageName()], func(p string) bool { return p == name })
			}
			writeErr := d.save()
			if !r.Skipped && !r.Interrupted {
				tried++
				if !r.OK {
					failed++
					n.Raise(key, notify.Failure, "Restore Drill Failed", d.message(fmt.Sprintf("Restore drill of %s-%s from '%s' failed: %s", e.Hostname, name, t.Name, r.Message)))
				} else {
					n.Clear(key, "Restore Drill Passing", d.message(fmt.Sprintf("A restore drill of %s-%s from '%s' passes again.", e.Hostname, name, t.Name)))
				}
			}
			if writeErr != nil {
				d.log.Message(logging.Error, "", "Could not record the drill: "+writeErr.Error())
				return 1
			}
			if r.Interrupted || lock.StopRequested() || ctx.Err() != nil {
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
				key := d.incident(t.StorageName(), name)
				r := d.state.Results[t.StorageName()][name]
				if _, ok := open[key]; ok && (r.At < started.Unix() || r.Skipped) {
					n.Raise(key, notify.Failure, "Restore Drill Failed",
						d.message(fmt.Sprintf("No restore drill of %s-%s from '%s' has passed since one failed (the last: %s).", e.Hostname, name, t.Name, r.Message)))
				}
			}
		}
	}

	if d.named.Alternating() && !stopped && failed == 0 && len(targets) > 0 && len(d.state.Pending[targets[0].StorageName()]) == 0 {
		d.state.NextStorage = (d.state.NextStorage + 1) % len(d.state.StorageOrder)
		d.state.Storage = ""
	}

	unresolved := false
	if d.named.Name != "" {
		for _, t := range targets {
			if len(d.state.Pending[t.StorageName()]) > 0 {
				unresolved = true
			}
		}
	}
	end := now().Unix()
	d.state.LastRun, d.state.LastFailed = end, failed > 0 || stopped || unresolved
	if !d.state.LastFailed && tried > 0 {
		d.state.LastPass = end
	}
	if err := d.save(); err != nil {
		log.Message(logging.Error, "", "Could not record the drill: "+err.Error())
		return 1
	}
	switch {
	case stopped:
		log.Message(logging.Info, "", "Restore drill stopped (stop requested or "+d.timeoutVariable()+" exceeded).")
		_ = lock.Record("stopped")
		return 1
	case failed > 0:
		log.Unnotified(logging.Error, "", fmt.Sprintf("Restore drill finished: %d of %d restore(s) failed.", failed, tried))
		_ = lock.Record("failed")
		return 1
	case unresolved:
		log.Unnotified(logging.Error, "", "Named restore drill remains due: a previously failed service has not passed.")
		_ = lock.Record("failed")
		return 1
	case tried == 0:
		log.Message(logging.Warning, "", "Restore drill finished without restoring anything (every service skipped).")
	default:
		log.Message(logging.Info, "", fmt.Sprintf("Restore drill finished: %d restore(s) verified.", tried))
		n.Send("Restore Drill Complete", d.message(fmt.Sprintf("%d restore(s) verified.", tried)))
	}
	_ = lock.Record("completed")
	return 0
}

// failedEarly records a drill that failed before restoring anything, so the healthcheck
// sees the failure rather than a drill that never ran.
func (d *drillRun) failedEarly() {
	d.state.LastRun, d.state.LastFailed = d.now().Unix(), true
	_ = d.save()
	_ = d.lock.Record("failed")
}

// selection is the services (sorted names, exclusions dropped) and storages a drill covers.
func (d *drillRun) selection(o DrillOptions) ([]string, []config.Target, error) {
	cfg := d.e.cfg
	dirs, _ := config.ExpandServiceDirectories(cfg.ServiceDirectories)
	var services []string
	d.dirs = map[string]string{}
	for _, dir := range dirs {
		name := filepath.Base(dir)
		if _, ok := d.dirs[name]; !ok {
			d.dirs[name] = dir
		}
		if o.Service != "" {
			if name == o.Service {
				services = append(services, name)
			}
			continue
		}
		if (o.Named != "" && slices.Contains(d.named.Services, name)) || (o.Named == "" && !cfg.DrillExcluded(name)) {
			services = append(services, name)
		}
	}
	sort.Strings(services)
	if o.Service != "" && len(services) == 0 {
		return nil, nil, fmt.Errorf("'%s' is not a configured service (SERVICE_DIRECTORIES).", o.Service)
	}
	targets, err := cfg.DrillTargets()
	if o.Named != "" {
		targets, err = d.named.Targets(cfg)
	}
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
	if d.named.Alternating() && len(targets) > 0 {
		var order []string
		for _, target := range targets {
			order = append(order, target.StorageName())
		}
		if !slices.Equal(d.state.StorageOrder, order) {
			d.state.NextStorage = 0
			d.state.StorageOrder = order
		}
		index := d.state.NextStorage % len(targets)
		if index < 0 {
			index = 0
		}
		if targets[index].StorageName() != d.state.Storage {
			for i, t := range targets {
				if t.StorageName() == d.state.Storage {
					index = i
					break
				}
			}
		}
		d.state.NextStorage = index
		targets = targets[index : index+1]
		d.state.Storage = targets[0].StorageName()
	}
	return services, targets, nil
}

// pick is the services drilled on t this run: all of them for count 0 or a named service,
// else unfinished selections first, then the next count in rotation.
func (d *drillRun) pick(t config.Target, services []string, count int, named bool) []string {
	if d.state.Pending == nil {
		d.state.Pending = map[string][]string{}
	}
	storage := t.StorageName()
	pending := d.state.Pending[storage]
	if !named {
		pending = slices.DeleteFunc(pending, func(name string) bool { return !slices.Contains(services, name) })
		d.state.Pending[storage] = pending
	}
	if !named && len(pending) > 0 {
		d.state.Pending[storage] = pending
		return slices.Clone(pending)
	}
	var out []string
	if named || count == 0 || count >= len(services) {
		out = slices.Clone(services)
	} else {
		start := d.state.Rotation[storage] % len(services)
		for i := 0; i < count; i++ {
			out = append(out, services[(start+i)%len(services)])
		}
		d.state.Rotation[storage] = (start + count) % len(services)
	}
	for _, name := range out {
		if !slices.Contains(d.state.Pending[storage], name) {
			d.state.Pending[storage] = append(d.state.Pending[storage], name)
		}
	}
	return out
}

func (d *drillRun) stopRequested() bool {
	for d.lock.State().Paused() && !d.lock.StopRequested() && d.e.context().Err() == nil {
		time.Sleep(time.Second)
	}
	return d.lock.StopRequested() || d.e.context().Err() != nil
}

// one drills service name on t: its newest revision restored into a scratch directory,
// every chunk verified by duplicacy as it downloads and every entry checked against the
// revision's record. Its drill hook runs before the copy is deleted.
func (d *drillRun) one(t config.Target, name string) (r lockstate.DrillResult) {
	started := d.now()
	r = lockstate.DrillResult{At: started.Unix(), LastSuccess: d.state.Results[t.StorageName()][name].LastSuccess}
	defer func() {
		r.Seconds = d.now().Sub(started).Seconds()
		if r.OK {
			r.LastSuccess = d.now().Unix()
		}
	}()
	id := d.e.Hostname + "-" + name
	out := d.log.Writer(logging.Info, name)
	defer out.Close()
	interrupt := func() lockstate.DrillResult {
		r.Interrupted, r.Message = true, "stop requested or "+d.timeoutVariable()+" exceeded"
		return r
	}
	fail := func(msg string) lockstate.DrillResult {
		// Whatever a stop interrupted fails; that is the stop, not a broken backup.
		if d.lock.StopRequested() || d.e.context().Err() != nil {
			return interrupt()
		}
		r.Message = msg
		d.log.Message(logging.Error, name, fmt.Sprintf("Restore drill of %s from '%s' failed: %s", id, t.Name, msg))
		return r
	}
	skip := func(msg string) lockstate.DrillResult {
		if d.lock.StopRequested() || d.e.context().Err() != nil {
			return interrupt()
		}
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
	_ = d.lock.SetStage(d.incident("", ""), t.StorageName()+"/"+name)

	repo, err := workRepo()
	if err != nil {
		return fail(err.Error())
	}
	// Recorded like the copy, so a drill killed now has it deleted by the next.
	_ = d.run.Set(repo, resume.Pending)
	defer func() {
		if os.RemoveAll(repo) == nil {
			d.run.Forget(repo)
		}
	}()
	if err := d.e.connect(repo, work, t, id, out); err != nil {
		return fail(err.Error())
	}
	// The revision is chosen and registered in use under the storage's gate, so no prune
	// can delete it between being picked and being restored (ADR 19).
	release, err := inuse.Gate(d.e.context(), d.e.Layout.InUseDir(), t.StorageName(), false)
	if err != nil {
		return fail(err.Error())
	}
	if d.stopRequested() {
		release()
		return interrupt()
	}
	revs, err := d.e.revisions(repo, id)
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
	// The revision's own record of every entry, kept beside the repository: the restored
	// copy is checked against it.
	record := filepath.Join(repo, "revision.json")
	if err := d.e.catRevision(repo, record, id, r.Revision); err != nil {
		return fail(fmt.Sprintf("cannot read the record of revision %d: %v", r.Revision, err))
	}
	want, size, err := revisionSize(d.e.context(), record)
	if err != nil {
		return fail(fmt.Sprintf("cannot read the record of revision %d: %v", r.Revision, err))
	}
	if free, ok := freeBytes(d.root); ok && free < size+size/20+64<<20 {
		return skip(fmt.Sprintf("revision %d needs %d MB but %s has %d MB free", r.Revision, size>>20+1, d.root, free>>20))
	}
	if d.stopRequested() {
		return interrupt()
	}
	d.log.Message(logging.Info, name, fmt.Sprintf("Drilling %s revision %d from '%s' (%d files).", id, r.Revision, t.Name, want))
	if err := d.e.restore(repo, work, t, id, r.Revision, Options{IgnoreOwner: true, Threads: d.e.cfg.Threads}, out); err != nil {
		return fail(fmt.Sprintf("restoring revision %d: %v", r.Revision, err))
	}
	f, err := os.Open(record)
	if err != nil {
		return fail(err.Error())
	}
	got, problems, err := verifyRestored(d.e.context(), work, skipToJSON(f), false, 10)
	f.Close()
	if err != nil {
		return fail(err.Error())
	}
	if len(problems) > 0 {
		return fail(fmt.Sprintf("revision %d restored differently from its record: %s", r.Revision, strings.Join(problems, "; ")))
	}
	if msg := d.hook(t, name, id, r.Revision, work); msg != "" {
		return fail(msg)
	}
	if d.stopRequested() {
		return fail("stop requested")
	}
	r.OK, r.Files = true, got
	d.log.Message(logging.Info, name, fmt.Sprintf("Restore drill of %s revision %d from '%s' passed (%d files).", id, r.Revision, t.Name, got))
	return r
}

func freeBytes(dir string) (int64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, false
	}
	return int64(st.Bavail) * int64(st.Bsize), true //nolint:gosec // G115: filesystem sizes and byte values fit
}

// hook runs the service's drill hook, if it has one, in the restored copy (ADR 48), and
// returns why the drill fails, or "" when it passes or there is no hook.
func (d *drillRun) hook(t config.Target, name, id string, rev int, work string) string {
	dir := d.dirs[name]
	if dir == "" {
		return ""
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "its drill hook cannot run: " + err.Error()
	}
	hookDir := hooks.Hooks(d.e.cfg.HooksDir, dir)
	ok, err := hooks.ExistsFor(hookDir, dir, hooks.Drill)
	if err != nil {
		return "its drill hook cannot run: " + err.Error()
	}
	if !ok {
		return ""
	}
	limit, err := d.e.cfg.DrillHookTimeout()
	if err != nil {
		return err.Error()
	}
	svc := hooks.Service{Name: name, Dir: dir, SnapshotID: id, HookDir: hookDir}
	env := append(hooks.RestoreEnviron(d.e.Environ, svc, rev, t.Name), "ARCHIVER_DRILL_DIR="+work)
	d.log.Message(logging.Info, name, fmt.Sprintf("Running the drill hook on revision %d from '%s'.", rev, t.Name))
	if d.e.beforeRun != nil {
		d.e.beforeRun()
	}
	if d.lock.StopRequested() || d.e.context().Err() != nil {
		return "stop requested"
	}
	p, err := proc.Start(proc.Spec{Path: filepath.Join(hookDir, hooks.Drill), Dir: work, Env: env,
		Log: d.log, Service: name, Hook: true, Group: true})
	if err != nil {
		return "its drill hook cannot run: " + err.Error()
	}
	// A hook can exit while its background children still use the restored copy.
	defer func() {
		p.Terminate(true)
		<-p.Done()
	}()
	remaining, checked := limit, time.Now()
	paused := d.lock.State().Paused()
	timer := time.NewTimer(limit)
	defer timer.Stop()
	poll := time.NewTicker(50 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-p.Done():
			code, err := p.Wait()
			if err != nil || code != 0 {
				return fmt.Sprintf("its drill hook failed on revision %d (%s)", rev, exitText(code, err))
			}
			return ""
		case <-timer.C:
		case <-poll.C:
		}
		if d.lock.StopRequested() || d.e.context().Err() != nil {
			return "stop requested"
		}
		now, wasPaused := time.Now(), paused
		paused = d.lock.State().Paused()
		// Frozen children cannot use their time budget. Treat a pause transition's
		// polling interval as paused too, rather than expiring during the transition.
		if !wasPaused && !paused {
			remaining -= now.Sub(checked)
		}
		checked = now
		if paused {
			timer.Stop()
			continue
		}
		if remaining <= 0 {
			return fmt.Sprintf("its drill hook did not finish within %s", limit)
		}
		timer.Reset(remaining)
	}
}

func exitText(code int, err error) string {
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("exit %d", code)
}

// catRevision writes the record of revision rev of id (`duplicacy cat` with no file) to
// path; stderr is kept separately so it cannot corrupt the record.
func (e *Env) catRevision(repo, path, id string, rev int) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd := e.duplicacy(repo, f, "cat", "-r", strconv.Itoa(rev), "-id", id)
	cmd.Stderr = &stderr
	err = cmd.Run()
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("%v %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// revisionSize is the number of regular files the record at path lists, and their total size.
func revisionSize(ctx context.Context, path string) (files int, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	err = revisionEntries(skipToJSON(f), func(e revisionEntry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if os.FileMode(e.Mode).IsRegular() {
			files++
			size += e.Size
		}
		return nil
	})
	return files, size, err
}

// skipToJSON skips any line duplicacy printed before the record's opening brace.
func skipToJSON(r io.Reader) io.Reader {
	br := bufio.NewReader(r)
	for {
		line, err := br.Peek(1)
		if err != nil || line[0] == '{' {
			return br
		}
		if _, err := br.ReadString('\n'); err != nil {
			return br
		}
	}
}

func (d *drillRun) incident(storage, service string) string {
	key := "drill"
	if d.named.Name != "" {
		key += ":" + d.named.Name
	}
	if storage != "" {
		key += ":" + storage + ":" + service
	}
	return key
}

func (d *drillRun) save() error {
	if d.named.Name == "" {
		return lockstate.WriteDrillState(d.e.Layout.DrillState(), d.state)
	}
	if d.allState.Named == nil {
		d.allState.Named = map[string]*lockstate.DrillState{}
	}
	d.allState.Named[d.named.Name] = &d.state
	return lockstate.WriteDrillState(d.e.Layout.DrillState(), d.allState)
}

func drillName(name string) string {
	if name == "" {
		return "default"
	}
	return name
}

func (d *drillRun) timeoutVariable() string {
	if d.named.Name != "" {
		return "RESTORE_DRILL_" + d.named.Name + "_TIMEOUT"
	}
	return "RESTORE_DRILL_TIMEOUT"
}

func (d *drillRun) message(message string) string {
	if d.named.Name != "" {
		return "Named drill " + d.named.Name + ": " + message
	}
	return message
}
