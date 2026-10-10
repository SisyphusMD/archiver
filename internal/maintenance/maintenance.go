// Package maintenance implements `archiver maintenance [exhaustive]` (ADR 10): per storage,
// a check (CHECK_BACKUPS) and a prune (PRUNE_BACKUPS) with the exhaustive prune on its
// interval, independent of backups. The primary's prune leaves out revisions in use (ADR 19);
// while copy workers keep the secondaries, maintenance keeps to the primary and wakes them
// after a prune that deleted revisions (ADR 12). Lock, log, state file and messages are the
// ones status, healthcheck and stop read.
package maintenance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/damage"
	"github.com/SisyphusMD/archiver/internal/kit"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/localprune"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/notify"
	"github.com/SisyphusMD/archiver/internal/proc"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

// Run is one maintenance run.
type Run struct {
	// Probe checks a storage can be reached (ADR 34); nil means kit.Probe.
	Probe           func(config.Target) error
	Layout          layout.Layout
	Source          config.Source
	Environ         []string
	Hostname        string
	Duplicacy       string // the duplicacy binary
	ForceExhaustive bool
	Stdout, Stderr  io.Writer
	// Signals ends the run as a stop does (SIGTERM, SIGINT).
	Signals <-chan os.Signal
	Now     func() time.Time

	cfg      *config.Config
	log      *logging.Log
	notify   *notify.Notifier
	lock     *runlock.Lock
	env      []string
	repo     string
	dirs     []string          // the service directories, whose repositories may hold fossil collections
	urls     map[string]string // each storage's URL, by storage name
	stopped  bool
	pruned   bool // the primary's prune deleted revisions
	reported bool // the run's outcome was notified
}

// Execute runs maintenance and returns the exit code: 1 when it failed, had errors, or was
// stopped.
func (r *Run) Execute() int {
	code := r.execute()
	// A run that ended early with errors, before its summary, is still one notification.
	if !r.reported && r.log.Reportable() > 0 {
		r.notify.Raise("maintenance", notify.Failure, "Maintenance Failed", r.log.Summary())
	}
	return code
}

func (r *Run) execute() int {
	r.log = &logging.Log{Dir: r.Layout.LogDir(), Basename: "maintenance", Stdout: r.Stdout}
	cfg, warnings, err := config.Load(r.Source, r.Environ)
	if err != nil {
		r.log.Message(logging.Error, "", err.Error())
		return 1
	}
	r.cfg = cfg
	r.notify = notify.FromConfig(cfg, r.Hostname, func(failed bool, msg string) {
		level := logging.Info
		if failed {
			level = logging.Error
		}
		r.log.Unnotified(level, "", msg)
	})
	r.notify.Incidents = r.Layout.Incidents()
	r.notify.WatchLog(r.log)

	lock, stale, err := runlock.Acquire(r.Layout.MaintenanceLock(), r.stopFlag(), "maintenance", "starting")
	if busy, ok := err.(*runlock.Busy); ok {
		fmt.Fprintf(r.Stderr, "A maintenance run is already in progress (PID %d). Not starting another.\n", busy.Holder.PID)
		return 1
	}
	if err != nil {
		r.log.Message(logging.Error, "", "Could not take the maintenance lock: "+err.Error())
		return 1
	}
	r.lock = lock
	defer r.finish()

	r.log.Rotate()
	if stale {
		r.log.Message(logging.Warning, "", "Stale maintenance lock file found. Cleaned up and proceeding.")
	}
	r.log.Message(logging.Info, "", fmt.Sprintf("Maintenance script started (exhaustive forced: %t).", r.ForceExhaustive))
	for _, w := range warnings {
		r.log.Message(logging.Warning, "", w)
	}
	if err := cfg.Validate(r.Source.SecretsDir); err != nil {
		r.log.Message(logging.Error, "", err.Error())
		r.lock.Record("failed")
		return 1
	}
	r.log.Message(logging.Info, "", fmt.Sprintf("Maintenance settings: CHECK_BACKUPS=%t, PRUNE_BACKUPS=%t, PRUNE_KEEP=%s, PRUNE_EXHAUSTIVE_FREQUENCY=%s.",
		cfg.CheckBackups, cfg.PruneBackups, cfg.PruneKeep, cfg.PruneExhaustiveFrequency))
	if r.ForceExhaustive && !cfg.PruneBackups {
		r.log.Message(logging.Warning, "", "'exhaustive' requested but PRUNE_BACKUPS is false; no prune (exhaustive or otherwise) will run.")
	}
	r.env = cfg.DuplicacyEnviron(r.Environ, r.Layout.SSHPrivateKey())

	// check and prune are repository-context commands, run from maintenance's own repository.
	r.dirs, _ = config.ExpandServiceDirectories(cfg.ServiceDirectories)
	r.repo = RepoDir(r.Layout.LogDir())

	r.main()
	if r.log.Errors() > 0 || r.stopped {
		return 1
	}
	return 0
}

func (r *Run) stopFlag() string {
	return filepath.Join(r.Layout.Lock, "archiver-maintenance-stop-requested")
}

// stopRequested reports whether `archiver stop` or a signal asked the run to end.
func (r *Run) stopRequested() bool {
	if r.stopped {
		return true
	}
	select {
	case <-r.Signals:
		r.stopped = true
		return true
	default:
	}
	return r.lock.StopRequested()
}

func (r *Run) main() {
	cfg := r.cfg
	if !cfg.CheckBackups && !cfg.PruneBackups {
		r.log.Message(logging.Info, "", "CHECK_BACKUPS and PRUNE_BACKUPS are both false; nothing to do.")
		r.lock.Record("completed")
		return
	}

	// Copy workers maintain the secondaries themselves (check, mirror local's retention,
	// exhaustive prune: ADRs 12, 17, 18); pruning them here with -keep as well would bring
	// back the race mirroring removes.
	last := len(cfg.Targets)
	if r.workers(daemon.CmdWorkers) {
		last = 1
		if len(cfg.Targets) > 1 {
			r.log.Message(logging.Info, "", "Secondary storages are maintained by their copy workers; maintaining local only.")
		}
		if r.ForceExhaustive && cfg.PruneBackups && r.workers(daemon.CmdExhaustive) {
			r.log.Message(logging.Info, "", "Copy workers will prune their secondary storages exhaustively on their next pass.")
		}
	}

	// Probed before anything contacts them (ADR 34). The secondaries are registered through
	// the primary, so a primary that cannot be reached fails the run; an unreachable
	// secondary fails alone, in seconds, and the others are still maintained.
	if err := r.probe(cfg.Targets[0]); errors.Is(err, errStopped) {
		r.endStopped()
		return
	} else if err != nil {
		name := cfg.Targets[0].StorageName()
		r.log.Message(logging.Error, name, fmt.Sprintf("Storage %s cannot be reached (%v), and the others are registered through it: nothing is maintained this run.", name, err))
		r.lock.Record("failed")
		return
	}
	switch stopped, err := r.watched(r.prepare); {
	case stopped:
		r.endStopped()
		return
	case err != nil:
		r.log.Message(logging.Error, "", "Cannot prepare the maintenance repository: "+err.Error())
		r.lock.Record("failed")
		return
	}
	for i := 0; i < last; i++ {
		if i > 0 {
			if err := r.probe(cfg.Targets[i]); errors.Is(err, errStopped) {
				r.endStopped()
				return
			} else if err != nil {
				name := cfg.Targets[i].StorageName()
				r.log.Message(logging.Error, name, fmt.Sprintf("Storage %s cannot be reached (%v); its check and prune are skipped this run.", name, err))
				continue
			}
			// A secondary that cannot be registered fails on its own, as its check would.
			stopped, err := r.watched(func(ctx context.Context) error { return r.addSecondary(ctx, i) })
			if stopped {
				r.endStopped()
				return
			}
			if err != nil {
				name := cfg.Targets[i].StorageName()
				r.log.Message(logging.Error, name, fmt.Sprintf("Cannot maintain %s this run: %v", name, err))
				continue
			}
		}
		if r.stopRequested() || !r.storage(i) {
			r.endStopped()
			return
		}
	}
	if r.stopRequested() {
		r.endStopped()
		return
	}
	// A local prune is a change the workers mirror onto the secondaries; a run that pruned
	// nothing is not (and must not end a stop, which only a change to local does).
	if r.pruned {
		r.workers(daemon.CmdLocalChanged)
	}
	r.lock.Record("completed")
	took := logging.Duration(r.Now().Unix() - runlock.Summarize(r.lock.State()).Start)
	var msg string
	switch n := r.log.Errors(); n {
	case 0:
		msg = "Completed successfully in " + took + "."
	case 1:
		msg = "Completed in " + took + " with 1 error."
	default:
		msg = fmt.Sprintf("Completed in %s with %d errors.", took, n)
	}
	fmt.Fprintln(r.Stdout, msg)
	// One notification per incident (ADR 36): the run's errors, or its recovery.
	r.reported = true
	if r.log.Reportable() > 0 {
		r.notify.Raise("maintenance", notify.Failure, "Maintenance Failed", msg+"\n"+r.log.Summary())
		return
	}
	r.notify.Clear("maintenance", "Maintenance Recovered", "Maintenance completes without errors again.")
	r.notify.Send("Maintenance Complete", msg)
}

// watched runs f with a context a stop cancels, so a registration that hangs on a storage
// or waits for its creation lock still ends when asked; stopped reports that.
func (r *Run) watched(f func(context.Context) error) (stopped bool, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if r.stopRequested() {
					cancel()
					return
				}
			}
		}
	}()
	err = f(ctx)
	stopped = ctx.Err() != nil
	cancel()
	<-done
	return stopped, err
}

// workers sends cmd, for this run's storages, to the daemon's copy workers; true means they
// keep these storages and took it.
func (r *Run) workers(cmd string) bool {
	reply, err := daemon.Send(r.Layout.DaemonSocket(), cmd+" "+r.cfg.StorageFingerprint())
	return err == nil && reply == daemon.ReplyOK
}

func (r *Run) endStopped() {
	r.stopped = true
	r.log.Message(logging.Info, "", "Stop requested; ending maintenance early.")
	r.lock.Record("stopped")
	r.notify.Send("Maintenance Stopped", "Stopped before completing all storages.")
}

// storage checks and prunes target i; false means a stop ended it.
func (r *Run) storage(i int) bool {
	t := r.cfg.Targets[i]
	name := t.StorageName()
	state := readState(r.Layout.MaintenanceState())

	if r.cfg.CheckBackups {
		r.lock.SetStage("storage:"+name, "check")
		began := r.Now()
		// -persist carries on past damage, so every damaged revision is named, not the first.
		var out strings.Builder
		code, stopped := r.duplicacyTo(name, &out, "check", "-all", "-storage", name, "-fossils", "-resurrect", "-stats", "-persist", "-threads", r.cfg.Threads)
		switch {
		case stopped:
			return false
		case code != 0:
			msg := "Storage check failed for " + name + "."
			if d := damage.Describe(out.String()); d != "" {
				msg = "Storage check failed for " + name + "; " + d + "."
			}
			r.log.Message(logging.Error, name, msg)

		default:
			state.set(name, fieldCheck, r.Now().Unix())
			state.write(r.Layout.MaintenanceState())
			r.log.Message(logging.Info, name, fmt.Sprintf("Storage check completed for %s in %s.", name, since(began, r.Now())))
		}
	}
	if r.stopRequested() {
		return false
	}
	if !r.cfg.PruneBackups {
		return true
	}

	// Probed before the prune too: a check can take hours, and the storage go meanwhile.
	if err := r.probe(t); errors.Is(err, errStopped) {
		return false
	} else if err != nil {
		r.log.Message(logging.Error, name, fmt.Sprintf("Storage %s cannot be reached (%v); its prune is skipped this run.", name, err))
		return true
	}
	exhaustive := r.exhaustiveDue(state, name)
	if exhaustive {
		r.log.Message(logging.Info, name, fmt.Sprintf("Exhaustive prune due for %s (frequency: %s).", name, r.cfg.PruneExhaustiveFrequency))
	}
	r.lock.SetStage("storage:"+name, "prune")
	began := r.Now()
	var before map[string]bool
	listed := false
	if i == 0 {
		before, listed = r.revisions(name)
	}
	if r.stopRequested() {
		return false
	}
	var failed bool
	if i == 0 {
		// The primary's prune leaves out revisions a copy or restore is still reading (ADR 19).
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			tick := time.NewTicker(time.Second)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					if r.stopRequested() {
						r.log.Message(logging.Info, name, "Stop requested during prune.")
						cancel()
						return
					}
				}
			}
		}()
		out := r.log.Writer(logging.Info, name)
		err := localprune.Run(ctx, localprune.Options{
			Bin: r.Duplicacy, Storage: name, Keep: strings.Fields(r.cfg.PruneKeep), Threads: r.cfg.Threads,
			Exhaustive: exhaustive, InUseDir: r.Layout.InUseDir(), Out: out, Dir: r.repo, Env: r.env,
		})
		out.Close()
		stopped := ctx.Err() != nil
		cancel()
		<-done
		if stopped || r.stopped {
			return false
		}
		failed = err != nil
		if err != nil {
			r.log.Message(logging.Info, name, "Prune of local storage failed: "+err.Error())
		}
	} else {
		args := append([]string{"prune", "-all", "-storage", name}, strings.Fields(r.cfg.PruneKeep)...)
		args = append(args, "-threads", r.cfg.Threads)
		if exhaustive {
			args = append(args, "-exhaustive")
		}
		code, stopped := r.duplicacy(name, args...)
		if stopped {
			return false
		}
		failed = code != 0
	}
	if failed {
		r.log.Message(logging.Error, name, "Prune failed for "+name+" storage. Review the Duplicacy logs for details.")
		return true
	}
	now := r.Now().Unix()
	state = readState(r.Layout.MaintenanceState())
	state.set(name, fieldPrune, now)
	if exhaustive {
		state.set(name, fieldExhaustive, now)
	}
	state.write(r.Layout.MaintenanceState())
	// Copy workers mirror a local prune, and only an actual change ends a stop of theirs.
	// Only two good listings prove a deletion (a failed one proves nothing), and only a
	// revision that went missing counts: a concurrent backup's new one must not hide it.
	if listed {
		if after, ok := r.revisions(name); ok {
			for rev := range before {
				if !after[rev] {
					r.pruned = true
					break
				}
			}
		}
	}
	r.log.Message(logging.Info, name, fmt.Sprintf("Prune completed for %s storage in %s.", name, since(began, r.Now())))
	return true
}

// exhaustiveDue: forced, or PRUNE_EXHAUSTIVE_FREQUENCY has passed since the last one, less an
// hour, so a fixed daily schedule does not drift one slot later each cycle.
func (r *Run) exhaustiveDue(s state, name string) bool {
	if r.ForceExhaustive {
		return true
	}
	interval := r.cfg.ExhaustiveInterval()
	if interval == 0 {
		return false
	}
	return r.Now().Unix()-s.get(name, fieldExhaustive) >= int64((interval - time.Hour).Seconds())
}

// duplicacy runs one duplicacy command in the repository, ending it when a stop comes;
// stopped reports that.
// errStopped is a probe a stop ended: not an outage, the run's end.
var errStopped = errors.New("stopped")

// probe, as the copy workers do, takes a storage without its directory for one not created
// yet: maintenance may run before the first backup.
func (r *Run) probe(t config.Target) error {
	if r.Probe != nil {
		return r.Probe(t)
	}
	stopped, err := r.watched(func(ctx context.Context) error {
		_, err := kit.Probe(ctx, r.Layout, t, false)
		return err
	})
	if stopped {
		return errStopped
	}
	return err
}

func (r *Run) duplicacy(service string, args ...string) (code int, stopped bool) {
	return r.duplicacyTo(service, nil, args...)
}

// duplicacyTo is duplicacy with its output also copied to also.
func (r *Run) duplicacyTo(service string, also io.Writer, args ...string) (code int, stopped bool) {
	spec := proc.Spec{Path: r.Duplicacy, Args: proc.NoScript(args...), Dir: r.repo, Env: r.env, Log: r.log, Service: service}
	if also != nil {
		lw := r.log.Writer(logging.Info, service)
		defer lw.Close()
		spec.Output = io.MultiWriter(lw, also)
	}
	p, err := proc.Start(spec)
	if err != nil {
		r.log.Message(logging.Error, service, fmt.Sprintf("Cannot run %s: %v", r.Duplicacy, err))
		return -1, false
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-p.Done():
			code, _ := p.Wait()
			return code, false
		case <-tick.C:
			if r.stopRequested() {
				r.log.Message(logging.Info, service, "Stop requested during duplicacy "+args[0]+".")
				p.Terminate(false)
				p.Wait()
				return 0, true
			}
		}
	}
}

// revisions lists a storage's snapshot revisions as "id revision"; ok is false when the
// listing failed.
func (r *Run) revisions(storage string) (map[string]bool, bool) {
	var out strings.Builder
	code, err := proc.Run(proc.Spec{Path: r.Duplicacy, Args: proc.NoScript("list", "-a", "-storage", storage), Dir: r.repo, Env: r.env, Output: &out})
	if err != nil || code != 0 {
		return nil, false
	}
	revs := map[string]bool{}
	for _, line := range strings.Split(out.String(), "\n") {
		f := strings.Fields(line)
		if len(f) >= 4 && f[0] == "Snapshot" && f[2] == "revision" {
			revs[f[1]+" "+f[3]] = true
		}
	}
	return revs, true
}

func (r *Run) finish() {
	s := runlock.Summarize(r.lock.State())
	var status string
	switch s.EndState {
	case "completed":
		// "completed" means the run reached its end, not that every step succeeded.
		switch n := r.log.Errors(); n {
		case 0:
			status = "Maintenance completed successfully"
		case 1:
			status = "Maintenance completed with 1 error"
		default:
			status = fmt.Sprintf("Maintenance completed with %d errors", n)
		}
	case "stopped":
		status = "Maintenance stopped before completion"
	default:
		status = "Maintenance ended with state: " + s.EndState
	}
	r.log.Message(logging.Info, "", "Maintenance session summary: "+status)
	r.log.Message(logging.Info, "", "  Start time: "+logging.Timestamp(s.Start))
	r.log.Message(logging.Info, "", "  End time: "+logging.Timestamp(s.End))
	r.log.Message(logging.Info, "", "  Total time: "+logging.Duration(s.End-s.Start))
	r.log.Message(logging.Info, "", "  Pause time: "+logging.Duration(s.Paused))
	r.log.Message(logging.Info, "", "  Active time: "+logging.Duration(s.Active))
	r.lock.Release()
	r.log.Message(logging.Info, "", "Maintenance script exited.")
}

func since(began, now time.Time) string { return logging.Duration(int64(now.Sub(began).Seconds())) }

// The maintenance record: one line per storage, "<name> <check> <prune> <exhaustive>", each
// the epoch of its last success (0 = never). healthcheck and status read it too.
const (
	fieldCheck = iota
	fieldPrune
	fieldExhaustive
)

type state map[string][3]int64

func readState(path string) state {
	s := state{}
	b, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		var v [3]int64
		for i := 0; i < 3; i++ {
			v[i], _ = strconv.ParseInt(f[i+1], 10, 64)
		}
		s[f[0]] = v
	}
	return s
}

func (s state) get(name string, field int) int64 { return s[name][field] }

func (s state) set(name string, field int, value int64) {
	v := s[name]
	v[field] = value
	s[name] = v
}

func (s state) write(path string) error {
	names := make([]string, 0, len(s))
	for n := range s {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		v := s[n]
		fmt.Fprintf(&b, "%s %d %d %d\n", n, v[0], v[1], v[2])
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
