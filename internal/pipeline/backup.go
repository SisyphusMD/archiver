// Package pipeline runs Archiver's backup pipeline: per service, the pre-backup hook, the
// duplicacy backup, and the post-backup hook; then a copy to every secondary storage and the
// recovery kit. Stop, pause, status and healthcheck read its stages and log lines, and its
// contract behaviors are tested black-box in tests/e2e.
package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SisyphusMD/archiver/internal/checkin"
	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/copier"
	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/hooks"
	"github.com/SisyphusMD/archiver/internal/kit"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/metrics"
	"github.com/SisyphusMD/archiver/internal/notify"
	"github.com/SisyphusMD/archiver/internal/proc"
	"github.com/SisyphusMD/archiver/internal/resume"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

// Backup is one backup run's setup.
type Backup struct {
	Layout          layout.Layout
	Source          config.Source
	Environ         []string
	Hostname        string // the host part of snapshot IDs
	Stdout, Stderr  io.Writer
	Duplicacy       string   // the duplicacy binary
	RecoveryKitStep []string // the kit step's command line: `archiver recovery-kit-step`
	Signals         <-chan os.Signal
	// Probe checks a storage can be reached (ADR 34) and whether its config is there; nil
	// means kit.Probe. inUse: the storage is known to exist, so a missing one is gone.
	Probe func(t config.Target, inUse bool) (exists bool, err error)

	cfg    *config.Config
	log    *logging.Log
	lock   *runlock.Lock
	notify *notify.Notifier
	env    []string // the environment duplicacy runs with: no raw secrets, every credential
	// workers: a daemon's copy workers keep the secondaries caught up and report their
	// health, so a secondary's failure here is not this run's.
	workers   bool
	kitMarker string          // created by the recovery-kit step once the primary holds the kit
	ctx       context.Context // ended by a signal or a stop request
	cancel    context.CancelFunc

	// Services back up in parallel (BACKUP_PARALLELISM), so what they share is under mu.
	mu       sync.Mutex
	running  []*proc.Proc
	signaled bool
	failing  map[string]bool // secondaries the workers report retrying or down

	reported bool // the run's outcome was notified

	// primaryDown: the primary was found unreachable, so no further service starts and
	// the one PRIMARY DOWN notification stands for the run's failure (ADR 34).
	primaryDown atomic.Bool
	downOnce    sync.Once
	downErr     error
	// inProgress records this run on the logs volume while it lasts (ADR 46); nil without one.
	inProgress *resume.Run
	// unreachable are the secondaries found down at the start, skipped for the run: not
	// registered, not copied (ADR 34).
	unreachable map[string]bool
	// held are the secondaries whose inline copy was skipped for their copy window.
	held map[string]bool
	// share is how many services back up at once, each with its own duplicacy backup: the
	// primary's upload limit is divided among them. Copies run once, after every service.
	share int
	// primaryExisted: the first probe found the primary's config, so it must stay there.
	primaryExisted bool
	stats          map[string]backupStats // each service's backup, by directory, under mu
	kitFailed      bool                   // the recovery kit step failed: not a good run
	// halt ends only on a stop (runContext); under mu.
	halt       context.Context
	haltCancel context.CancelFunc
}

// Run runs the pipeline and returns its exit code.
func (b *Backup) Run() int {
	code := b.pipeline()
	// A run that ended early with errors, before its summary, is still one notification,
	// and its own incident: no service was backed up, which backup health counts as failing.
	if !b.reported && b.log.Reportable() > 0 {
		if b.notify == nil {
			// No configuration, so nowhere to send: the incident is still kept, for health.
			b.notify = &notify.Notifier{Incidents: b.Layout.Incidents()}
		}
		b.notify.Raise("backup-aborted", notify.Failure, "Backup Failed", b.log.Summary())
		b.checkin(false, "The backup failed before backing up any service.")
	}
	// The run's outcome in the metrics textfile at once (ADR 35), for deployments run by an
	// external scheduler with no daemon to refresh it.
	if err := metrics.WriteFile(b.Layout, os.Getenv, time.Now()); err != nil {
		b.log.Message(logging.Warning, "", "Could not write the metrics textfile: "+err.Error())
	}
	return code
}

func (b *Backup) pipeline() int {
	b.log = &logging.Log{Dir: b.Layout.LogDir(), Basename: "archiver", Stdout: b.Stdout}
	cfg, warnings, err := config.Load(b.Source, b.Environ)
	if err != nil {
		b.log.Message(logging.Error, "", err.Error())
		return 1
	}
	b.cfg = cfg
	b.notify = notify.FromConfig(cfg, b.Hostname, func(failed bool, msg string) {
		if failed {
			b.log.Unnotified(logging.Error, "", msg)
		} else {
			b.log.Unnotified(logging.Info, "", msg)
		}
	})
	b.notify.Incidents = b.Layout.Incidents()
	b.notify.WatchLog(b.log)
	for _, w := range warnings {
		b.log.Message(logging.Warning, "", w)
	}

	lock, stale, err := runlock.Acquire(b.Layout.BackupLock(), filepath.Join(b.Layout.Lock, "archiver-stop-requested"), "duplicacy", "pre-backup")
	if busy, ok := err.(*runlock.Busy); ok {
		h := busy.Holder
		fmt.Fprintf(b.Stderr, "A backup is already running (PID %d). Not starting another.\n", h.PID)
		// A refused scheduled run is a day without a backup, so it must not pass silently.
		b.notify.Raise("backup-skipped", notify.Failure, "Backup Skipped", fmt.Sprintf("A backup was not started because the previous run is still going (PID %d, stage %s, started %s).",
			h.PID, h.Stage, logging.Timestamp(h.StartedAt())))
		b.checkin(false, "A backup was not started: the previous run is still going.")
		return 1
	}
	if err != nil {
		b.log.Message(logging.Error, "", "Could not take the backup lock: "+err.Error())
		return 1
	}
	// Probed with the backup lock already held, which a restore checks after taking its own:
	// whichever starts second sees the other.
	switch f, free, err := runlock.Hold(b.Layout.RestoreLock()); {
	case err != nil:
		// Unable to tell is not the same as no restore: refuse rather than risk it.
		lock.Release()
		b.log.Message(logging.Error, "", "Could not check for a running restore, so not starting a backup: "+err.Error())
		return 1
	case !free:
		lock.Release()
		fmt.Fprintln(b.Stderr, "A restore into a service directory is running. Not starting a backup.")
		b.notify.Raise("backup-skipped", notify.Failure, "Backup Skipped", "A backup was not started because a restore into a service directory is running; it would have saved the directory half-restored.")
		b.checkin(false, "A backup was not started: a restore into a service directory is running.")
		return 1
	default:
		f.Close()
	}
	b.notify.Clear("backup-skipped", "Backups Running Again", "A backup started after one was skipped.")
	b.lock = lock
	defer b.finish()
	// A wait on a storage-creation lock ends on a signal or a stop request, like the rest.
	b.mu.Lock()
	b.ctx, b.cancel = context.WithCancel(context.Background())
	b.mu.Unlock()
	defer b.cancel()
	go func() {
		for {
			select {
			case <-b.ctx.Done():
				return
			case <-time.After(time.Second):
				// A stop ends the run as a signal does: running duplicacy and the kit step
				// end now, nothing new starts (no copy retry), and post hooks still run.
				if b.lock.StopRequested() {
					b.onSignal()
					return
				}
			}
		}
	}()

	b.log.Rotate()
	if stale {
		b.log.Message(logging.Warning, "", "Stale lock file found. Cleaned up and proceeding.")
	}
	b.log.Message(logging.Info, "", "Main backup script started.")
	prior := b.finishInterrupted()
	b.log.Message(logging.Info, "", "Proceeding with backup script.")
	dirs, ok := b.verifyConfig()
	if !ok {
		return 1
	}
	dirs = unfinishedFirst(dirs, prior)
	b.recordRun(dirs)
	b.env = b.cfg.DuplicacyEnviron(b.Environ, b.Layout.SSHPrivateKey())
	b.workers = b.copyWorkersRun()
	b.refreshFailing()

	if b.stopped() {
		return b.handleStop()
	}
	return b.main(dirs)
}

// finishInterrupted runs, before anything else, the post-backup hook of each service an
// interrupted run left between its hooks (ADR 46), with ARCHIVER_BACKUP_RESULT=interrupted,
// so whatever its pre hook stopped runs again. It returns that run's record: this run backs
// up its unfinished services first.
func (b *Backup) finishInterrupted() resume.Record {
	path := b.Layout.RunRecord("backup")
	prior, ok := resume.Read(path)
	if !ok {
		return resume.Record{}
	}
	b.inProgress, _ = resume.Begin(path, prior)
	b.log.Message(logging.Warning, "", fmt.Sprintf("The backup started %s was interrupted; it is run again now, its unfinished services first.", logging.Timestamp(prior.Started)))
	dirs := make([]string, 0, len(prior.Services))
	for dir := range prior.Services {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	// The record is on the logs volume, which someone less trusted might be able to write:
	// it only says which configured services were left between their hooks. A directory not
	// configured now is never acted on, and the snapshot ID (which names the hook state
	// directory removed below) is derived as a backup derives it, never read from the record.
	configured := map[string]bool{}
	expanded, _ := config.ExpandServiceDirectories(b.cfg.ServiceDirectories)
	for _, d := range expanded {
		if abs, err := filepath.Abs(d); err == nil {
			configured[abs] = true
		}
	}
	for _, dir := range dirs {
		if prior.Services[dir] != resume.Hooked {
			continue
		}
		name := filepath.Base(dir)
		if !configured[dir] || !config.ValidSnapshotID(name) {
			b.log.Message(logging.Warning, "", fmt.Sprintf("The interrupted backup's record names %s, which is not a configured service directory now; nothing is done there.", dir))
			_ = b.inProgress.Set(dir, resume.Pending)
			continue
		}
		svc := hooks.Service{Name: name, Dir: dir, SnapshotID: b.Hostname + "-" + name, HookDir: hooks.Hooks(b.cfg.HooksDir, dir)}
		svc.StateDir = b.Layout.HookState(svc.SnapshotID)
		if has, err := hooks.Exists(svc.HookDir, hooks.PostBackup); err != nil {
			b.log.Message(logging.Error, name, fmt.Sprintf("The interrupted backup left this service after its pre-backup hook, and its post-backup hook cannot run: %v. Check that whatever the pre hook stopped is running.", err))
		} else if has {
			_ = os.MkdirAll(svc.StateDir, 0o700)
			b.log.Message(logging.Info, name, "Running the post-backup hook the interrupted backup did not reach.")
			if code, err := hooks.Run(b.log, b.Environ, svc, hooks.PostBackup, hooks.Interrupted); err != nil || code != 0 {
				b.log.Message(logging.Error, name, fmt.Sprintf("Post-backup hook failed for %s service (%s); check that whatever its pre hook stopped is running again.", name, exitText(code, err)))
			}
		}
		_ = os.RemoveAll(svc.StateDir)
		// Recorded at once, so a crash now does not run the hook a second time.
		_ = b.inProgress.Set(dir, resume.Pending)
	}
	return prior
}

// unfinishedFirst puts the services the interrupted run did not finish first, otherwise
// keeping the configured order.
func unfinishedFirst(dirs []string, prior resume.Record) []string {
	if len(prior.Services) == 0 {
		return dirs
	}
	var first, rest []string
	for _, d := range dirs {
		abs, err := filepath.Abs(d)
		if err != nil {
			abs = d
		}
		if prior.Unfinished(abs) {
			first = append(first, d)
		} else {
			rest = append(rest, d)
		}
	}
	return append(first, rest...)
}

// recordRun records this run on the logs volume, each service pending, so an interruption
// can be finished on the next start.
func (b *Backup) recordRun(dirs []string) {
	rec := resume.Record{Services: map[string]string{}}
	for _, d := range dirs {
		if abs, err := filepath.Abs(d); err == nil {
			d = abs
		}
		rec.Services[d] = resume.Pending
	}
	if b.inProgress != nil {
		b.inProgress.Replace(rec)
		return
	}
	r, err := resume.Begin(b.Layout.RunRecord("backup"), rec)
	if err != nil {
		b.log.Message(logging.Warning, "", "Cannot record the run on the logs volume, so an interruption would not be resumed: "+err.Error())
	}
	b.inProgress = r
}

// verifyConfig validates the configuration as verify_config does, logging each step.
func (b *Backup) verifyConfig() ([]string, bool) {
	c := b.cfg
	if len(c.ServiceDirectories) == 0 {
		b.log.Message(logging.Error, "", "SERVICE_DIRECTORIES is not set. Set the SERVICE_DIRECTORIES environment variable (colon-delimited).")
		return nil, false
	}
	dirs, unmatched := config.ExpandServiceDirectories(c.ServiceDirectories)
	if len(c.Targets) > 0 {
		b.log.Message(logging.Info, "", fmt.Sprintf("%d backup targets configured.", len(c.Targets)))
	}
	if err := c.Validate(b.Source.SecretsDir); err != nil {
		b.log.Message(logging.Error, "", err.Error())
		return nil, false
	}
	b.log.Message(logging.Info, "", "All required secrets are set.")
	if c.Pushover() {
		b.log.Message(logging.Info, "", fmt.Sprintf("All required %s settings are set.", c.NotificationService))
	}
	b.log.Message(logging.Info, "", fmt.Sprintf("Maintenance settings: CHECK_BACKUPS=%t, PRUNE_BACKUPS=%t, PRUNE_KEEP=%s, PRUNE_EXHAUSTIVE_FREQUENCY=%s.",
		c.CheckBackups, c.PruneBackups, c.PruneKeep, c.PruneExhaustiveFrequency))
	for _, u := range unmatched {
		b.log.Message(logging.Error, "", fmt.Sprintf("SERVICE_DIRECTORIES entry '%s' matches no directory, so nothing there is backed up. Fix the path or the volume mount.", u))
	}
	return dirs, true
}

func (b *Backup) main(dirs []string) int {
	// The primary first: down, no service is worth starting (its pre hook would stop a
	// database for a backup that cannot happen).
	exists, err := b.probe(b.cfg.Targets[0], b.primaryInUse())
	b.primaryExisted = exists
	if err != nil {
		if b.stopped() {
			return b.handleStop() // the stop ended the probe, not an outage
		}
		b.lostPrimary(err)
		for _, dir := range dirs {
			b.skipForPrimary(dir)
		}
		b.notifyPrimaryDown()
		_ = b.lock.Record("completed")
		b.complete()
		return b.exitCode()
	}
	// Without copy workers the secondaries are registered and copied inline: one probe
	// each now, so one that is down is skipped throughout rather than retried per service.
	if !b.workers {
		b.unreachable = map[string]bool{}
		b.held = map[string]bool{}
		for _, t := range b.cfg.Targets[1:] {
			if b.outsideWindow(t) {
				continue
			}
			if _, err := b.probe(t, false); err != nil && !b.stopped() {
				b.unreachable[t.StorageName()] = true
				b.log.Message(logging.Error, t.StorageName(), fmt.Sprintf("Copy to %s storage skipped: it cannot be reached (%v). It is copied at the next run.", t.StorageName(), err))
			}
		}
	}
	b.notify.Clear("primary-down", "Primary Storage Back", fmt.Sprintf("Primary storage '%s' can be reached again.", b.cfg.Targets[0].Name))
	n, _ := b.cfg.BackupParallelism() // validated with the rest of the configuration
	groups := groupServices(dirs, n)
	b.share = min(n, len(groups))
	ok := make([]bool, len(dirs))
	var stop atomic.Bool
	var wg sync.WaitGroup
	slots := make(chan struct{}, n)
	for _, group := range groups {
		slots <- struct{}{}
		// Starting another service would run its pre hook (stopping its database, say) for
		// a backup that will never happen.
		if stop.Load() || b.stopped() {
			<-slots
			break
		}
		wg.Add(1)
		go func() {
			defer func() { <-slots; wg.Done() }()
			for _, i := range group {
				if stop.Load() || b.stopped() {
					return
				}
				if b.primaryDown.Load() {
					b.skipForPrimary(dirs[i])
					continue
				}
				var s bool
				ok[i], s = b.processService(dirs[i])
				if s {
					stop.Store(true)
					return
				}
			}
		}()
	}
	wg.Wait()
	// Sent once every service's post hook has run: what a pre hook stopped is not kept
	// down while a notification is delivered.
	b.notifyPrimaryDown()
	if stop.Load() {
		return b.handleStop()
	}
	// Copies and the kit read or write the primary: probed again first, since the last
	// service may have finished long ago. Gone, they could only fail slowly.
	if !b.primaryDown.Load() && !b.stopped() {
		if _, err := b.probe(b.cfg.Targets[0], b.primaryExisted || b.primaryInUse()); err != nil && !b.stopped() {
			b.lostPrimary(err)
			b.notifyPrimaryDown()
		}
	}
	if b.primaryDown.Load() {
		_ = b.lock.Record("completed")
		b.complete()
		return b.exitCode()
	}
	// The copies run from the last service in order that backed up, as they did one by one.
	lastWorking := ""
	for i, dir := range dirs {
		if ok[i] {
			lastWorking = dir
		}
	}
	_ = b.lock.SetStage("duplicacy", "post-backup")
	if b.stopped() {
		b.log.Message(logging.Info, "", "Stop requested. Skipping storage wrap-up, invoking stop handler.")
		return b.handleStop()
	}
	if lastWorking == "" {
		b.log.Message(logging.Error, "", "No service completed a backup. Skipping storage copies.")
		_ = b.lock.Record("completed")
		b.complete()
		return b.exitCode()
	}

	if b.workers {
		b.handOffCopies()
	} else {
		_ = b.lock.SetStage("duplicacy", "copy")
		b.copies(lastWorking)
		if b.stopped() {
			return b.handleStop()
		}
		if b.primaryDown.Load() { // lost during the copies: the kit could not be read either
			_ = b.lock.Record("completed")
			b.complete()
			return b.exitCode()
		}
	}
	b.recoveryKit()
	if b.stopped() {
		return b.handleStop()
	}
	_ = b.lock.Record("completed")
	b.complete()
	return b.exitCode()
}

// groupServices groups the indexes of dirs that must not back up at the same time, in
// order of first appearance: those with one snapshot ID (its base name: the same directory
// listed twice, or two with one name), since two backups of one snapshot ID must never run
// at once, and those that are one directory under two names (a symlink), since they share
// its repository. A group goes one directory after another; groups run in parallel. With a
// parallelism of 1 everything is one group, in the order configured.
func groupServices(dirs []string, parallelism int) [][]int {
	if parallelism <= 1 {
		all := make([]int, len(dirs))
		for i := range dirs {
			all[i] = i
		}
		return [][]int{all}
	}
	// Union-find over directories, joined by shared snapshot ID or shared real path.
	parent := make([]int, len(dirs))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	byKey := map[string]int{}
	for i, dir := range dirs {
		abs, err := filepath.Abs(dir)
		if err != nil {
			abs = filepath.Clean(dir)
		}
		keys := []string{"id:" + filepath.Base(abs)}
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			keys = append(keys, "path:"+real)
		}
		for _, k := range keys {
			if j, seen := byKey[k]; seen {
				parent[find(i)] = find(j)
			} else {
				byKey[k] = i
			}
		}
	}
	var groups [][]int
	index := map[int]int{}
	for i := range dirs {
		r := find(i)
		g, seen := index[r]
		if !seen {
			g = len(groups)
			index[r] = g
			groups = append(groups, nil)
		}
		groups[g] = append(groups[g], i)
	}
	return groups
}

func (b *Backup) exitCode() int {
	if b.log.Errors() > 0 {
		return 1
	}
	return 0
}

// processService backs up one service. ok means it backed up; stop means the run must end.
func (b *Backup) processService(dir string) (ok, stop bool) {
	// Hooks run from inside the directory, so a relative path would resolve twice.
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	name := filepath.Base(dir)
	log := func(level, msg string) { b.log.Message(level, name, msg) }
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		log(logging.Error, fmt.Sprintf("Failed to change to %s. Continuing.", dir))
		b.record(dir, hooks.Failed, time.Now())
		return false, false
	}
	log(logging.Info, fmt.Sprintf("Processing %s service.", name))
	svc := hooks.Service{Name: name, Dir: dir, SnapshotID: b.Hostname + "-" + name, HookDir: hooks.Hooks(b.cfg.HooksDir, dir)}
	// Checked before any hook runs: duplicacy rejects the ID at init, and by then the pre
	// hook has already stopped whatever it stops.
	if !config.ValidSnapshotID(svc.SnapshotID) {
		log(logging.Error, fmt.Sprintf("Cannot back up %s: its snapshot ID '%s' may contain only letters, digits, '_' and '-' (a Duplicacy rule). Rename the directory or the host.", dir, svc.SnapshotID))
		b.record(dir, hooks.Failed, time.Now())
		return false, false
	}
	// Lstat: a broken symlink still means the service has not been migrated.
	if _, err := os.Lstat(filepath.Join(dir, hooks.Legacy)); err == nil {
		log(logging.Error, fmt.Sprintf("%s is not run by this pipeline, so this service's backup is skipped. Convert it with 'archiver migrate hooks'.", hooks.Legacy))
		b.record(dir, hooks.Failed, time.Now())
		return false, false
	}
	filters, err := hooks.ReadFilters(dir)
	if err != nil {
		log(logging.Error, fmt.Sprintf("Cannot read the filters file, so this service's backup is skipped: %v", err))
		b.record(dir, hooks.Failed, time.Now())
		return false, false
	}
	if svc.HookDir != dir {
		for _, h := range []string{hooks.PreBackup, hooks.PostBackup} {
			if _, err := os.Lstat(filepath.Join(dir, h)); err == nil {
				log(logging.Warning, fmt.Sprintf("%s in the service directory is not run: with HOOKS_DIR set, hooks come from %s.", h, svc.HookDir))
			}
		}
	}
	hasPre, err := hooks.Exists(svc.HookDir, hooks.PreBackup)
	if err == nil {
		var hasPost bool
		hasPost, err = hooks.Exists(svc.HookDir, hooks.PostBackup)
		if err == nil {
			return b.backupService(svc, filters, hasPre, hasPost, log)
		}
	}
	log(logging.Error, fmt.Sprintf("Cannot run this service's hooks, so its backup is skipped: %v", err))
	b.record(dir, hooks.Failed, time.Now())
	return false, false
}

func (b *Backup) backupService(svc hooks.Service, filters []string, hasPre, hasPost bool, log func(string, string)) (ok, stop bool) {
	// On the logs volume: an interrupted run's post hook, run on the next start, still finds
	// what the pre hook left.
	state := b.Layout.HookState(svc.SnapshotID)
	_ = os.RemoveAll(state)
	if err := os.MkdirAll(state, 0o700); err != nil {
		log(logging.Error, "Cannot create the hooks' state directory, so this service's backup is skipped: "+err.Error())
		b.record(svc.Dir, hooks.Failed, time.Now())
		return false, false
	}
	defer os.RemoveAll(state)
	svc.StateDir = state

	log(logging.Info, fmt.Sprintf("Starting backup for %s service.", svc.Name))
	ctx := "service:" + svc.Dir
	result := hooks.Success
	_ = b.lock.SetStage(ctx, "pre-backup")
	preRan := false
	if hasPre {
		b.waitWhilePaused()
	}
	// The primary may have been found down while this service waited (a pause, a slot):
	// its pre hook would stop something for a backup that cannot happen.
	if b.primaryDown.Load() {
		b.skipForPrimary(svc.Dir)
		return false, false
	}
	if hasPre && b.stopped() {
		// Stopped before the pre hook ran: there is nothing for a post hook to undo either.
		result = hooks.Stopped
	} else if hasPre {
		preRan = true
		// Not refused when it cannot be written (a full logs volume): a missed backup is
		// worse than a crash in this hook going unfinished, but it is said.
		if err := b.inProgress.Set(svc.Dir, resume.Hooked); err != nil {
			log(logging.Warning, "Cannot record that the pre-backup hook runs, so a crash before the post-backup hook would not run it on the next start: "+err.Error())
		}
		code, err := hooks.Run(b.log, b.Environ, svc, hooks.PreBackup, "")
		if err != nil || code != 0 {
			// The service's files are in an unknown state (a half-written or stale dump).
			// Backing them up would make that the newest revision, the one auto-restore picks.
			log(logging.Error, fmt.Sprintf("Pre-backup hook failed for %s service (%s); skipping its backup so the newest revision stays the last good one.", svc.Name, exitText(code, err)))
			result = hooks.Skipped
		}
	}
	began := time.Now()
	if result == hooks.Success {
		if b.stopped() {
			result = hooks.Stopped
		} else {
			_ = b.lock.SetStage(ctx, "backup")
			result = b.primaryBackup(svc, filters, log)
		}
	}

	// The post hook undoes the pre hook (restarts what it stopped), so it runs whenever the
	// pre hook ran: after a failed pre hook, a failed backup, or a stop. Without a pre hook
	// it always runs.
	_ = b.lock.SetStage(ctx, "post-backup")
	if hasPost && (preRan || !hasPre) {
		b.waitWhilePaused()
		code, err := hooks.Run(b.log, b.Environ, svc, hooks.PostBackup, result)
		if err != nil || code != 0 {
			log(logging.Error, fmt.Sprintf("Post-backup hook failed for %s service (%s); check that whatever its pre hook stopped is running again.", svc.Name, exitText(code, err)))
		}
	}
	// Recorded as soon as the hooks are through, so a crash after this never runs the post
	// hook again: finished, however it went, unless a stop cut it short (then it is first
	// next time).
	if result == hooks.Stopped {
		_ = b.inProgress.Set(svc.Dir, resume.Pending)
	} else {
		_ = b.inProgress.Set(svc.Dir, resume.Done)
	}
	// A failed backup may be the primary gone mid-run: probed again, so the rest are not
	// each tried against it. After the post hook, which must not wait on the probe.
	if result == hooks.Failed && !b.isSignaled() {
		if _, err := b.probe(b.cfg.Targets[0], b.primaryExisted || b.primaryInUse()); err != nil && !b.stopped() {
			b.lostPrimary(err)
		}
	}
	// After the post hook: recording can notify, which may take a minute, and whatever the
	// pre hook stopped must not wait on it.
	b.record(svc.Dir, result, began)
	if result != hooks.Success {
		return false, result == hooks.Stopped || b.stopped()
	}
	if !b.stopped() {
		b.addStorages(svc, log)
	}
	_ = b.lock.SetStage("duplicacy", "backup")
	if b.stopped() {
		log(logging.Info, "Stop requested. Service cleanup complete, invoking stop handler.")
		return true, true
	}
	return true, false
}

func exitText(code int, err error) string {
	if err != nil {
		return err.Error()
	}
	return "exit " + strconv.Itoa(code)
}

func (b *Backup) duplicacy(dir, service string, args ...string) proc.Spec {
	return proc.Spec{Path: b.Duplicacy, Args: proc.NoScript(args...), Dir: dir, Env: b.env, Log: b.log, Service: service, Interrupt: true}
}

// run runs a duplicacy command to completion, unless a signal ends the run first.
func (b *Backup) run(s proc.Spec) int {
	p, err := b.start(s)
	if err != nil {
		b.log.Message(logging.Error, s.Service, fmt.Sprintf("Cannot run %s: %v", s.Path, err))
		return -1
	}
	code, _ := p.Wait()
	b.forget(p)
	return code
}

// runInit runs a duplicacy init or add under the storage-creation lock the copy workers
// also take: duplicacy 3.2.5 has no create-if-absent, so two creating one storage at once
// can leave it with two configurations.
func (b *Backup) runInit(storage string, s proc.Spec) int {
	release, err := runlock.Exclusive(b.ctx, b.Layout.StorageInit(storage))
	if err != nil {
		b.log.Message(logging.Error, s.Service, "Cannot take the storage-creation lock: "+err.Error())
		return -1
	}
	defer release()
	return b.run(s)
}

// waitBounded waits for p, counting only time the run is not paused toward limit (a
// paused run stays resumable); true means the limit was reached.
func (b *Backup) waitBounded(p *proc.Proc, limit time.Duration) bool {
	var active time.Duration
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for active < limit {
		select {
		case <-p.Done():
			return false
		case <-tick.C:
			if !b.lock.State().Paused() {
				active += time.Second
			}
		}
	}
	return true
}

// waitWhilePaused holds off starting a program while `archiver pause` has the run paused:
// pause stops what is running, and anything started after it must not run either. A stop
// or a signal ends the wait.
func (b *Backup) waitWhilePaused() {
	for b.lock.State().Paused() && !b.stopped() {
		time.Sleep(250 * time.Millisecond)
	}
}

func (b *Backup) start(s proc.Spec) (*proc.Proc, error) {
	b.waitWhilePaused()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.signaled {
		return nil, fmt.Errorf("the run is stopping")
	}
	p, err := proc.Start(s)
	if err == nil {
		b.running = append(b.running, p)
	}
	return p, err
}

func (b *Backup) forget(p *proc.Proc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, q := range b.running {
		if q == p {
			b.running = append(b.running[:i], b.running[i+1:]...)
			return
		}
	}
}

func (b *Backup) primaryBackup(svc hooks.Service, filters []string, log func(string, string)) string {
	primary := b.cfg.Targets[0]
	storage := primary.StorageName()
	url, err := primary.URL()
	if err != nil {
		log(logging.Error, err.Error()+".")
		return hooks.Failed
	}
	repo := filepath.Join(svc.Dir, ".duplicacy")
	if err := os.Remove(filepath.Join(repo, "preferences")); err != nil && !os.IsNotExist(err) {
		log(logging.Error, fmt.Sprintf("Error removing preferences file for the %s service.", svc.Name))
	}
	log(logging.Info, fmt.Sprintf("Initializing primary storage for %s service.", svc.Name))
	if b.runInit(storage, b.duplicacy(svc.Dir, svc.Name, "init", "-e", "-key", filepath.Join(b.Layout.Root, "keys", "public.pem"),
		"-storage-name", storage, svc.SnapshotID, url)) != 0 {
		log(logging.Error, fmt.Sprintf("Primary storage initialization failed for %s service.", svc.Name))
		if why := kit.Diagnose(b.Layout, "", primary); why != "" {
			log(logging.Error, why)
		}
	}
	if b.run(b.duplicacy(svc.Dir, svc.Name, "list", "-storage", storage)) != 0 {
		log(logging.Error, fmt.Sprintf("Primary storage verification failed for %s service.", svc.Name))
	} else {
		log(logging.Info, fmt.Sprintf("Primary storage verified for %s service.", svc.Name))
	}

	content := strings.Join(filters, "\n")
	if content != "" {
		content += "\n"
	}
	// Removed first: writing through a symlink here would truncate its target.
	os.Remove(filepath.Join(repo, "filters"))
	if err := os.WriteFile(filepath.Join(repo, "filters"), []byte(content), 0o644); err != nil { //nolint:gosec // G306: not a secret: state or notes meant to be readable
		log(logging.Error, fmt.Sprintf("Unable to create the Duplicacy filters file for the %s service.", svc.Name))
	}
	log(logging.Info, fmt.Sprintf("Duplicacy filters configured for %s service.", svc.Name))

	log(logging.Info, fmt.Sprintf("Starting backup to %s for %s service.", primary.Name, svc.Name))
	began := time.Now()
	// The output is logged and also read for metrics: the revision made, the bytes uploaded.
	args := []string{"backup", "-storage", storage, "-stats", "-threads", b.cfg.Threads}
	if primary.UploadLimit != "" {
		args = append(args, "-limit-rate", b.rate(primary.UploadLimit))
	}
	spec := b.duplicacy(svc.Dir, svc.Name, args...)
	lw := b.log.Writer(logging.Info, svc.Name)
	var out strings.Builder
	spec.Output = io.MultiWriter(lw, &out)
	defer func() {
		lw.Close()
		b.noteStats(svc.Dir, out.String())
	}()
	p, err := b.start(spec)
	if err != nil {
		log(logging.Error, fmt.Sprintf("Backup to %s failed for %s service.", primary.Name, svc.Name))
		return hooks.Failed
	}
	defer b.forget(p)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-p.Done():
			code, _ := p.Wait()
			if b.isSignaled() {
				return hooks.Stopped
			}
			if code != 0 {
				log(logging.Error, fmt.Sprintf("Backup to %s failed for %s service.", primary.Name, svc.Name))
				return hooks.Failed
			}
			log(logging.Info, fmt.Sprintf("Backup to %s completed for %s service in %s.", primary.Name, svc.Name, logging.Duration(int64(time.Since(began).Seconds()))))
			return hooks.Success
		case <-tick.C:
			if b.lock.StopRequested() {
				log(logging.Info, fmt.Sprintf("Stop requested during duplicacy backup for %s service.", svc.Name))
				p.Terminate(b.lock.State().Paused())
				_, _ = p.Wait()
				log(logging.Info, fmt.Sprintf("Duplicacy backup stopped for %s service.", svc.Name))
				return hooks.Stopped
			}
		}
	}
}

// addStorages adds every secondary storage to the service's repository, for the copies
// this run makes: copy-compatible with the primary (bit-identical) and RSA-encrypted, as
// every existing secondary was made. With copy workers the run makes no copies and never
// contacts a secondary: the workers and maintenance register storages in repositories of
// their own.
func (b *Backup) addStorages(svc hooks.Service, log func(string, string)) {
	if b.workers {
		return
	}
	primary := b.cfg.Targets[0].StorageName()
	for _, t := range b.cfg.Targets[1:] {
		b.mu.Lock()
		down := b.unreachable[t.StorageName()]
		b.mu.Unlock()
		if down || b.outsideWindow(t) {
			continue
		}
		url, err := t.URL()
		if err != nil {
			log(logging.Error, err.Error()+".")
			continue
		}
		log(logging.Info, fmt.Sprintf("Adding %s storage %s for %s service.", t.Type, t.StorageName(), svc.Name))
		spec := b.duplicacy(svc.Dir, svc.Name, "add", "-e", "-copy", primary, "-bit-identical", "-key",
			filepath.Join(b.Layout.Root, "keys", "public.pem"), t.StorageName(), svc.SnapshotID, url)
		if b.runInit(t.StorageName(), spec) != 0 {
			log(logging.Error, fmt.Sprintf("Failed to add %s storage %s for %s service.", t.Type, t.StorageName(), svc.Name))
			// Gone since the start: skipped for the rest of the run, not retried per service.
			if _, err := b.probe(t, false); err != nil && !b.stopped() {
				b.mu.Lock()
				b.unreachable[t.StorageName()] = true
				b.mu.Unlock()
				log(logging.Error, fmt.Sprintf("Copy to %s storage skipped: it cannot be reached (%v). It is copied at the next run.", t.StorageName(), err))
				continue
			}
			if why := kit.Diagnose(b.Layout, "", t); why != "" {
				log(logging.Error, why)
			}
		}
	}
}

// refreshFailing re-reads which secondaries the workers report failing; it runs before
// each step that would contact them, so an outage that began mid-backup is avoided too.
func (b *Backup) refreshFailing() {
	if !b.workers {
		return
	}
	failing := map[string]bool{}
	for name, st := range (&copier.Store{Path: b.Layout.CopyWorkersState()}).Load() {
		if st.Status == copier.Retrying || st.Status == copier.Down || st.DownSince != 0 {
			failing[name] = true
		}
	}
	b.mu.Lock()
	b.failing = failing
	b.mu.Unlock()
}

// copyWorkersRun asks the daemon whether copy workers keep the secondaries caught up.
func (b *Backup) copyWorkersRun() bool {
	if len(b.cfg.Targets) < 2 {
		return false
	}
	reply, err := daemon.Send(b.Layout.DaemonSocket(), daemon.CmdWorkers+" "+b.cfg.StorageFingerprint())
	return err == nil && reply == daemon.ReplyOK
}

// handOffCopies tells the daemon's copy workers that local changed (ADR 11), so this run
// ends with the local backup instead of waiting on slow targets. It never copies inline
// instead: a wake that timed out may still arrive, and two copiers would share a target.
// An undelivered wake is caught up by the workers' periodic check.
func (b *Backup) handOffCopies() {
	// A stop never wakes the workers it just stopped; `archiver stop` also waits for this run
	// to end before stopping them a final time.
	if b.stopped() {
		return
	}
	reply, err := daemon.Send(b.Layout.DaemonSocket(), daemon.CmdLocalChanged+" "+b.cfg.StorageFingerprint())
	if err != nil || reply != daemon.ReplyOK {
		b.log.Message(logging.Warning, "", fmt.Sprintf("Could not wake the copy workers (%v %s); they copy at their next periodic check.", err, reply))
		return
	}
	b.log.Message(logging.Info, "", "Copies to the secondary storages run in the background; follow them in copies.log or with 'archiver status'.")
}

// copies copies the primary to every secondary in parallel, from dir's repository, and
// retries a failed copy once: a copy can lose a rare race with a concurrent non-exclusive
// prune, and a fresh copy resumes where it stopped.
func (b *Backup) copies(dir string) {
	if len(b.cfg.Targets) < 2 {
		return
	}
	// However the copies end, each secondary's check-in hears how its own went.
	var final []string
	defer func() { b.targetCheckins(final) }()
	var names []string
	for _, t := range b.cfg.Targets[1:] {
		b.mu.Lock()
		down := b.unreachable[t.StorageName()] // found down earlier and reported then
		b.mu.Unlock()
		if !down && !b.outsideWindow(t) {
			names = append(names, t.StorageName())
		}
	}
	if len(names) == 0 {
		return
	}
	names = b.reachable(names)
	failed := b.copyLegs(dir, names)
	final = failed
	if len(failed) == 0 || b.isSignaled() {
		return
	}
	// One that went down since is skipped, not retried against; so is every one if the
	// primary they copy from has gone.
	if _, err := b.probe(b.cfg.Targets[0], b.primaryExisted || b.primaryInUse()); err != nil && !b.stopped() {
		b.lostPrimary(err)
		b.notifyPrimaryDown()
		return
	}
	if failed = b.reachable(failed); len(failed) == 0 {
		return
	}
	b.log.Message(logging.Warning, "", "Retrying failed copies once: "+strings.Join(failed, " ")+".")
	final = b.copyLegs(dir, failed)
	for _, n := range final {
		b.log.Message(logging.Error, n, fmt.Sprintf("Copy to %s storage failed after retry.", n))
	}
}

// targetCheckins tells each secondary's check-in URL how its inline copy went (ADR 37), as
// a copy worker would: failed if its copy failed or it could not be reached.
func (b *Backup) targetCheckins(failed []string) {
	if b.isSignaled() || b.primaryDown.Load() {
		return
	}
	for _, t := range b.cfg.Targets[1:] {
		b.mu.Lock()
		held := b.held[t.StorageName()]
		b.mu.Unlock()
		if t.CheckinURL == "" || held { // held for its copy window: nothing was done to report
			continue
		}
		b.mu.Lock()
		down := b.unreachable[t.StorageName()]
		b.mu.Unlock()
		ok := !down && !slices.Contains(failed, t.StorageName())
		msg := "copied"
		if !ok {
			msg = "copy failed"
		}
		ctx, cancel := context.WithTimeout(b.runContext(), 2*time.Minute)
		if err := checkin.Ping(ctx, t.CheckinURL, ok, msg); err != nil {
			b.log.Message(logging.Warning, t.StorageName(), fmt.Sprintf("The check-in to STORAGE_TARGET_%d_CHECKIN_URL failed: %v", t.N, err))
		}
		cancel()
	}
}

// outsideWindow reports whether t's copy window is closed now. Such a target is skipped
// for the rest of the run, without being probed or copied to, and the skip is said once.
func (b *Backup) outsideWindow(t config.Target) bool {
	w, err := config.ParseWindow(t.CopyWindow)
	b.mu.Lock()
	already := b.held[t.StorageName()]
	closed := already || (err == nil && !w.Open(time.Now()))
	if closed {
		b.held[t.StorageName()] = true
	}
	b.mu.Unlock()
	if !closed || already {
		return closed
	}
	b.log.Message(logging.Info, t.StorageName(), fmt.Sprintf("Copy to %s storage skipped: outside its copy window (%s). The first backup inside it copies everything since.", t.StorageName(), w))
	return true
}

// rate is the primary's UPLOAD_LIMIT for one of the services backing up at once, so that
// together they keep to it. Validate refuses a limit below BACKUP_PARALLELISM, so each
// share is at least 1.
func (b *Backup) rate(limit string) string {
	n, _ := strconv.Atoi(limit)
	return strconv.Itoa(max(1, n/max(1, b.share)))
}

func (b *Backup) copyLegs(dir string, names []string) (failed []string) {
	primary := b.cfg.Targets[0].StorageName()
	type leg struct {
		name  string
		p     *proc.Proc
		began time.Time
	}
	var legs []leg
	var locks []func()
	defer func() {
		for _, release := range locks {
			release()
		}
	}()
	for _, n := range names {
		// Shared with the copy workers of a daemon that started meanwhile.
		release, err := runlock.Exclusive(b.ctx, b.Layout.CopyLock(n))
		if err != nil {
			b.log.Message(logging.Warning, n, fmt.Sprintf("Copy to %s storage failed: %v.", n, err))
			failed = append(failed, n)
			continue
		}
		locks = append(locks, release)
		args := []string{"copy", "-from", primary, "-to", n, "-key", b.Layout.RSAPrivateKey(), "-threads", b.cfg.Threads, "-download-threads", b.cfg.Threads}
		var target config.Target
		for _, t := range b.cfg.Targets[1:] {
			if t.StorageName() == n {
				target = t
			}
		}
		// Checked again as each copy starts, a retry included: a lock wait can outlast the
		// window.
		if b.outsideWindow(target) {
			continue
		}
		if target.UploadLimit != "" {
			args = append(args, "-upload-limit-rate", target.UploadLimit)
		}
		b.log.Message(logging.Info, n, fmt.Sprintf("Copying backup to %s storage.", n))
		p, err := b.start(b.duplicacy(dir, n, args...))
		if err != nil {
			b.log.Message(logging.Warning, n, fmt.Sprintf("Copy to %s storage failed.", n))
			failed = append(failed, n)
			continue
		}
		legs = append(legs, leg{n, p, time.Now()})
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, l := range legs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, _ := l.p.Wait()
			b.forget(l.p)
			if code != 0 {
				b.log.Message(logging.Warning, l.name, fmt.Sprintf("Copy to %s storage failed.", l.name))
				mu.Lock()
				failed = append(failed, l.name)
				mu.Unlock()
				return
			}
			b.log.Message(logging.Info, l.name, fmt.Sprintf("Copy to %s storage completed in %s.", l.name, logging.Duration(int64(time.Since(l.began).Seconds()))))
		}()
	}
	wg.Wait()
	return failed
}

// KitDeadline bounds the recovery-kit step when copy workers keep the secondaries.
var KitDeadline = 30 * time.Minute

// recoveryKit refreshes the recovery kit through its step. Its errors were logged and
// notified there; one is counted here so the run still fails.
func (b *Backup) recoveryKit() {
	if b.cfg.RecoveryPassword == "" {
		b.log.Message(logging.Info, "", "Recovery kit not configured (no recovery_password secret); skipping.")
		return
	}
	b.refreshFailing()
	marker, merr := os.CreateTemp("", "archiver-kit-primary-")
	if merr == nil {
		marker.Close()
		os.Remove(marker.Name())
		defer os.Remove(marker.Name())
		b.kitMarker = marker.Name()
	}
	p, err := b.startPlain(b.RecoveryKitStep[0], b.RecoveryKitStep[1:]...)
	if err != nil {
		b.log.Message(logging.Error, "", "Recovery kit: cannot run its step: "+err.Error())
		return
	}
	// With copy workers, the backup must not wait indefinitely on a stalled offsite (SFTP
	// uploads have no timeout of their own): the step gets a deadline, and a kit it did not
	// finish is placed by a later run.
	var code int
	if b.workers && b.waitBounded(p, KitDeadline) {
		b.log.Message(logging.Warning, "", fmt.Sprintf("Recovery kit: still placing after %s; ending it so the backup does not wait on an offsite. A later run places it.", KitDeadline))
		p.Terminate(false)
		// Ended, the step cannot report what it found: the kit is not current everywhere.
		b.notify.Raise("kit", notify.Failure, "Recovery Kit Failed", fmt.Sprintf("The recovery kit was still being placed after %s and was ended; it is not current on every storage. A later run places it; any errors are in archiver.log.", KitDeadline))
	}
	code, _ = p.Wait()
	b.forget(p)
	if b.workers && code != 0 && code != 2 && b.kitMarker != "" {
		// The step marks when the primary holds the kit; a failure or timeout after that
		// is a secondary's, which is not this run's.
		if _, err := os.Stat(b.kitMarker); err == nil {
			code = 3
		}
	}
	switch {
	case code == 0, code == 2: // 2: placed, but not verified readable on every target; retried next run
	// 3: only secondary uploads failed, each already logged as an error, notified, and
	// retried at the next run. With copy workers, offsite trouble is reported but does not
	// fail the local backup.
	case code == 3 && b.workers:
	default:
		if !b.isSignaled() {
			b.log.AddErrors(1)
			b.kitFailed = true
		}
	}
}

func (b *Backup) probe(t config.Target, inUse bool) (bool, error) {
	if b.Probe != nil {
		return b.Probe(t, inUse)
	}
	ctx := b.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return kit.Probe(ctx, b.Layout, t, inUse)
}

// reachable keeps the secondaries named that can be reached now, reporting each that
// cannot as skipped.
func (b *Backup) reachable(names []string) []string {
	var out []string
	for _, t := range b.cfg.Targets[1:] {
		if !slices.Contains(names, t.StorageName()) {
			continue
		}
		if _, err := b.probe(t, false); err != nil && !b.stopped() {
			b.mu.Lock()
			b.unreachable[t.StorageName()] = true
			b.mu.Unlock()
			b.log.Message(logging.Error, t.StorageName(), fmt.Sprintf("Copy to %s storage skipped: it cannot be reached (%v). It is copied at the next run.", t.StorageName(), err))
			continue
		}
		out = append(out, t.StorageName())
	}
	return out
}

// checkin tells CHECKIN_URL how the run went (ADR 37). A ping that fails is a warning: the
// monitor alerts on the silence itself.
func (b *Backup) checkin(ok bool, msg string) {
	if b.cfg == nil || b.cfg.CheckinURL == "" {
		return
	}
	ctx, cancel := context.WithTimeout(b.runContext(), 2*time.Minute)
	defer cancel()
	if err := checkin.Ping(ctx, b.cfg.CheckinURL, ok, msg); err != nil {
		b.log.Message(logging.Warning, "", "The check-in to CHECKIN_URL failed: "+err.Error())
	}
}

// runContext ends only when the run is stopped, so nothing it waits on holds up a stop. It
// outlives the pipeline's own context, which also ends when the pipeline returns: a run that
// ended on its own still sends what it reports afterwards.
func (b *Backup) runContext() context.Context {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.halt == nil {
		b.halt, b.haltCancel = context.WithCancel(context.Background())
		if b.signaled {
			b.haltCancel()
		}
	}
	return b.halt
}

// primaryInUse reports whether the configured primary already holds backups: then its
// config must be there, and a missing one is the storage gone (an unmounted volume) rather
// than one for the first backup to create.
func (b *Backup) primaryInUse() bool {
	s, err := lockstate.ReadBackupState(b.Layout.BackupState())
	url, uerr := b.cfg.Targets[0].URL()
	return err == nil && uerr == nil && s.Primary != "" && s.Primary == url
}

// lostPrimary marks the primary unreachable, so no further service starts; the alert
// goes out once the services are done (notifyPrimaryDown).
func (b *Backup) lostPrimary(err error) {
	b.downOnce.Do(func() {
		b.downErr = err
		b.primaryDown.Store(true)
		b.log.Message(logging.Error, "", fmt.Sprintf("PRIMARY DOWN: storage '%s' cannot be reached (%v); no further service is backed up this run.", b.cfg.Targets[0].Name, err))
	})
}

// notifyPrimaryDown sends the run's one PRIMARY DOWN notification, if the primary was lost.
func (b *Backup) notifyPrimaryDown() {
	if !b.primaryDown.Load() {
		return
	}
	b.notify.Raise("primary-down", notify.Critical, "PRIMARY DOWN", fmt.Sprintf("Primary storage '%s' cannot be reached (%v), so no backups are being made. Archiver tries again at the next scheduled backup.", b.cfg.Targets[0].Name, b.downErr))
}

// skipForPrimary reports a service not started because the primary is down.
func (b *Backup) skipForPrimary(dir string) {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	b.log.Message(logging.Error, filepath.Base(dir), fmt.Sprintf("Skipped: primary storage '%s' is down.", b.cfg.Targets[0].Name))
	b.record(dir, hooks.Skipped, time.Now())
}

var (
	revisionLine = regexp.MustCompile(`(?m)^Backup for .* at revision (\d+) completed`)
	uploadedLine = regexp.MustCompile(`(?m)^All chunks: .* ([\d,.]+)([KMGT]?) bytes uploaded`)
)

// noteStats keeps what duplicacy's -stats said of a backup for metrics (ADR 35).
func (b *Backup) noteStats(dir, out string) {
	var st backupStats
	if m := revisionLine.FindStringSubmatch(out); m != nil {
		st.revision, _ = strconv.Atoi(m[1])
	}
	if m := uploadedLine.FindStringSubmatch(out); m != nil {
		st.uploaded = parseSize(m[1], m[2])
	}
	b.mu.Lock()
	if b.stats == nil {
		b.stats = map[string]backupStats{}
	}
	b.stats[dir] = st
	b.mu.Unlock()
}

type backupStats struct {
	revision int
	uploaded int64
}

// parseSize reads duplicacy's sizes: "3,923" with a K, M, G or T after it (powers of 1024).
func parseSize(num, unit string) int64 {
	f, err := strconv.ParseFloat(strings.ReplaceAll(num, ",", ""), 64)
	if err != nil {
		return 0
	}
	for _, u := range "KMGT" {
		if unit == "" {
			break
		}
		f *= 1024
		if string(u) == unit {
			break
		}
	}
	return int64(f)
}

// record keeps the outcome for the service in dir for backup health (ADR 32), by
// directory: two directories of one name are one snapshot ID but separate outcomes.
// Services back up in parallel, so the state file is edited under mu.
func (b *Backup) record(dir, result string, began time.Time) {
	path := b.Layout.BackupState()
	b.mu.Lock()
	s, err := lockstate.ReadBackupState(path)
	if err != nil {
		s = lockstate.BackupState{}
	}
	if s.Services == nil {
		s.Services = map[string]lockstate.ServiceResult{}
	}
	r := s.Services[dir]
	now := time.Now()
	r.LastAttempt, r.Result, r.Seconds = now.Unix(), result, int64(now.Sub(began).Seconds())
	if result == hooks.Success {
		r.LastSuccess = now.Unix()
		st := b.stats[dir]
		r.Revision, r.Uploaded = st.revision, st.uploaded
		if url, err := b.cfg.Targets[0].URL(); err == nil {
			s.Primary = url
		}
	}
	s.Services[dir] = r
	err = lockstate.WriteBackupState(path, s)
	// Notifying can take a minute; a stop or another service must not wait on it.
	b.mu.Unlock()
	// Unrecorded, an old success would stand in for this outcome: its own incident, failing.
	if err != nil {
		b.log.Message(logging.Warning, filepath.Base(dir), "Could not record the backup for backup health: "+err.Error())
		b.notify.Raise("backup-state:"+dir, notify.Failure, "Backup Health Unknown", fmt.Sprintf("Could not record backup results in %s (%v), so backup health cannot be trusted.", path, err))
		return
	}
	b.notify.Clear("backup-state:"+dir, "Backup Health Recorded", "Backup results for "+dir+" are recorded again.")
}

// startPlain runs a program with its output passed through, not logged: the recovery-kit
// step logs for itself.
func (b *Backup) startPlain(path string, args ...string) (*proc.Proc, error) {
	env := b.Environ
	if b.kitMarker != "" {
		env = append(append([]string(nil), env...), "ARCHIVER_KIT_PRIMARY_MARKER="+b.kitMarker)
	}
	b.mu.Lock()
	var names []string
	for n := range b.failing {
		names = append(names, n)
	}
	for n := range b.unreachable {
		if !b.failing[n] {
			names = append(names, n)
		}
	}
	b.mu.Unlock()
	if len(names) > 0 {
		sort.Strings(names)
		env = append(append([]string(nil), env...), "ARCHIVER_KIT_SKIP_TARGETS="+strings.Join(names, " "))
	}
	return b.start(proc.Spec{Path: path, Args: args, Env: env, Output: b.Stdout, Group: true})
}

func (b *Backup) complete() {
	s := runlock.Summarize(b.lock.State())
	took := logging.Duration(time.Now().Unix() - s.Start)
	var msg string
	switch n := b.log.Errors(); n {
	case 0:
		msg = "Completed successfully in " + took + "."
	case 1:
		msg = "Completed in " + took + " with 1 error."
	default:
		msg = fmt.Sprintf("Completed in %s with %d errors.", took, n)
	}
	fmt.Fprintln(b.Stdout, msg)
	// One notification per incident (ADR 36): a run with errors of its own raises the
	// backup incident with them, a clean one closes it. The kit's errors are the kit's own
	// incident, raised by its step, and a notification that failed is not the backup's.
	b.reported = true
	b.checkin(!b.primaryDown.Load() && b.log.Reportable() == 0 && !b.kitFailed, msg)
	if b.primaryDown.Load() {
		return // the PRIMARY DOWN notification is this run's one
	}
	// The run got as far as the services: whatever stopped an earlier one before them is over.
	b.notify.Clear("backup-aborted", "Backup Recovered", "Backups run again.")
	if b.log.Reportable() > 0 {
		b.notify.Raise("backup", notify.Failure, "Backup Failed", msg+"\n"+b.log.Summary())
		return
	}
	b.notify.Clear("backup", "Backup Recovered", "Backups complete without errors again.")
	b.notify.Send("Backup Complete", msg)
}

// stopped reports whether the run must end: `archiver stop` set the flag, or a signal came.
func (b *Backup) stopped() bool {
	b.drainSignals()
	return b.isSignaled() || b.lock.StopRequested()
}

func (b *Backup) isSignaled() bool {
	b.drainSignals()
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.signaled
}

func (b *Backup) drainSignals() {
	for {
		select {
		case <-b.Signals:
			b.onSignal()
		default:
			return
		}
	}
}

// Watch handles signals as they arrive, so a running duplicacy is ended at once rather
// than at the next check. Call it in its own goroutine.
func (b *Backup) Watch(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case <-b.Signals:
			b.onSignal()
		}
	}
}

// onSignal ends the run: duplicacy processes now (a hook is left to finish, so a post hook
// still restarts what its pre hook stopped), and nothing new starts.
func (b *Backup) onSignal() {
	b.mu.Lock()
	b.signaled = true
	if b.cancel != nil {
		b.cancel()
	}
	if b.haltCancel != nil {
		b.haltCancel()
	}
	dup := append([]*proc.Proc(nil), b.running...)
	b.mu.Unlock()
	for _, p := range dup {
		go p.Terminate(false)
	}
}

// handleStop ends a stopped run: record it, report it with
// the run's error count, and exit non-zero.
func (b *Backup) handleStop() int {
	st := b.lock.State()
	alreadyRecorded := len(st.Events) > 0 && st.Events[len(st.Events)-1].State == "stopped"
	if !alreadyRecorded {
		_ = b.lock.Record("stopped")
		s := runlock.Summarize(b.lock.State())
		took := logging.Duration(time.Now().Unix() - s.Start)
		var msg string
		switch n := b.log.Errors(); n {
		case 0:
			msg = "Stopped after " + took + " with no errors."
		case 1:
			msg = "Stopped after " + took + " with 1 error."
		default:
			msg = fmt.Sprintf("Stopped after %s with %d errors.", took, n)
		}
		fmt.Fprintln(b.Stdout, "Backup stopped. "+msg)
		b.log.Message(logging.Info, "", "Backup stopped. "+msg)
		b.notify.Send("Backup Stopped", msg)
	}
	return 1
}

// finish writes the session summary and releases the lock, whatever ended the run.
func (b *Backup) finish() {
	s := runlock.Summarize(b.lock.State())
	var status string
	switch s.EndState {
	case "completed":
		// "completed" means the run reached its end, not that every step succeeded.
		switch n := b.log.Errors(); n {
		case 0:
			status = "Backup completed successfully"
		case 1:
			status = "Backup completed with 1 error"
		default:
			status = fmt.Sprintf("Backup completed with %d errors", n)
		}
	case "stopped":
		status = "Backup stopped before completion"
	default:
		status = "Backup ended with state: " + s.EndState
	}
	b.log.Message(logging.Info, "", "Backup session summary: "+status)
	b.log.Message(logging.Info, "", "  Start time: "+logging.Timestamp(s.Start))
	b.log.Message(logging.Info, "", "  End time: "+logging.Timestamp(s.End))
	b.log.Message(logging.Info, "", "  Total time: "+logging.Duration(s.End-s.Start))
	b.log.Message(logging.Info, "", "  Pause time: "+logging.Duration(s.Paused))
	b.log.Message(logging.Info, "", "  Active time: "+logging.Duration(s.Active))
	// A stop for a container shutdown keeps the record: the run starts again with the
	// container (ADR 46). Any other end, a stop asked for included, leaves nothing to resume.
	if s.EndState == "stopped" && resume.ShuttingDown(b.Layout.ShutdownFlag()) {
		b.log.Message(logging.Info, "", "The backup stopped for a container shutdown; it runs again when the container starts.")
	} else {
		b.inProgress.End()
	}
	b.lock.Release()
	b.log.Message(logging.Info, "", "Main backup script exited.")
}

// Hostname is the host part of snapshot IDs: an inherited HOSTNAME wins over the kernel's,
// (a Kubernetes Job pins its IDs across pod names this way).
func Hostname(getenv func(string) string) string {
	if h := getenv("HOSTNAME"); h != "" {
		return h
	}
	h, _ := os.Hostname()
	return h
}
