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
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/copier"
	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/hooks"
	"github.com/SisyphusMD/archiver/internal/kit"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/notify"
	"github.com/SisyphusMD/archiver/internal/proc"
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
}

var snapshotIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Run runs the pipeline and returns its exit code.
func (b *Backup) Run() int {
	b.log = &logging.Log{Dir: b.Layout.LogDir(), Basename: "archiver", ErrorTitle: "Backup Error", Stdout: b.Stdout}
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
	b.log.Notify = b.notify.Send
	for _, w := range warnings {
		b.log.Message(logging.Warning, "", w)
	}

	lock, stale, err := runlock.Acquire(b.Layout.BackupLock(), filepath.Join(b.Layout.Lock, "archiver-stop-requested"), "duplicacy", "pre-backup")
	if busy, ok := err.(*runlock.Busy); ok {
		h := busy.Holder
		fmt.Fprintf(b.Stderr, "A backup is already running (PID %d). Not starting another.\n", h.PID)
		// A refused scheduled run is a day without a backup, so it must not pass silently.
		b.notify.Send("Backup Skipped", fmt.Sprintf("A backup was not started because the previous run is still going (PID %d, stage %s, started %s).",
			h.PID, h.Stage, logging.Timestamp(h.StartedAt())))
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
		b.notify.Send("Backup Skipped", "A backup was not started because a restore into a service directory is running; it would have saved the directory half-restored.")
		return 1
	default:
		f.Close()
	}
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
	b.log.Message(logging.Info, "", "Proceeding with backup script.")
	dirs, ok := b.verifyConfig()
	if !ok {
		return 1
	}
	b.env = b.cfg.DuplicacyEnviron(b.Environ, b.Layout.SSHPrivateKey())
	b.workers = b.copyWorkersRun()
	b.refreshFailing()

	if b.stopped() {
		return b.handleStop()
	}
	return b.main(dirs)
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
	n, _ := b.cfg.BackupParallelism() // validated with the rest of the configuration
	groups := groupServices(dirs, n)
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
	if stop.Load() {
		return b.handleStop()
	}
	// The copies run from the last service in order that backed up, as they did one by one.
	lastWorking := ""
	for i, dir := range dirs {
		if ok[i] {
			lastWorking = dir
		}
	}
	b.lock.SetStage("duplicacy", "post-backup")
	if b.stopped() {
		b.log.Message(logging.Info, "", "Stop requested. Skipping storage wrap-up, invoking stop handler.")
		return b.handleStop()
	}
	if lastWorking == "" {
		b.log.Message(logging.Error, "", "No service completed a backup. Skipping storage copies.")
		b.lock.Record("completed")
		b.complete()
		return b.exitCode()
	}

	if b.workers {
		b.handOffCopies()
	} else {
		b.lock.SetStage("duplicacy", "copy")
		b.copies(lastWorking)
		if b.stopped() {
			return b.handleStop()
		}
	}
	b.recoveryKit()
	if b.stopped() {
		return b.handleStop()
	}
	b.lock.Record("completed")
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
		return false, false
	}
	log(logging.Info, fmt.Sprintf("Processing %s service.", name))
	svc := hooks.Service{Name: name, Dir: dir, SnapshotID: b.Hostname + "-" + name}
	// Checked before any hook runs: duplicacy rejects the ID at init, and by then the pre
	// hook has already stopped whatever it stops.
	if !snapshotIDPattern.MatchString(svc.SnapshotID) {
		log(logging.Error, fmt.Sprintf("Cannot back up %s: its snapshot ID '%s' may contain only letters, digits, '_' and '-' (a Duplicacy rule). Rename the directory or the host.", dir, svc.SnapshotID))
		return false, false
	}
	// Lstat: a broken symlink still means the service has not been migrated.
	if _, err := os.Lstat(filepath.Join(dir, hooks.Legacy)); err == nil {
		log(logging.Error, fmt.Sprintf("%s is not run by this pipeline, so this service's backup is skipped. Convert it with 'archiver migrate hooks'.", hooks.Legacy))
		return false, false
	}
	filters, err := hooks.ReadFilters(dir)
	if err != nil {
		log(logging.Error, fmt.Sprintf("Cannot read the filters file, so this service's backup is skipped: %v", err))
		return false, false
	}
	hasPre, err := hooks.Exists(dir, hooks.PreBackup)
	if err == nil {
		var hasPost bool
		hasPost, err = hooks.Exists(dir, hooks.PostBackup)
		if err == nil {
			return b.backupService(svc, filters, hasPre, hasPost, log)
		}
	}
	log(logging.Error, fmt.Sprintf("Cannot run this service's hooks, so its backup is skipped: %v", err))
	return false, false
}

func (b *Backup) backupService(svc hooks.Service, filters []string, hasPre, hasPost bool, log func(string, string)) (ok, stop bool) {
	state, err := os.MkdirTemp("", "archiver-hooks-")
	if err != nil {
		log(logging.Error, "Cannot create the hooks' state directory, so this service's backup is skipped: "+err.Error())
		return false, false
	}
	defer os.RemoveAll(state)
	svc.StateDir = state

	log(logging.Info, fmt.Sprintf("Starting backup for %s service.", svc.Name))
	ctx := "service:" + svc.Dir
	result := hooks.Success
	b.lock.SetStage(ctx, "pre-backup")
	preRan := false
	if hasPre {
		b.waitWhilePaused()
	}
	if hasPre && b.stopped() {
		// Stopped before the pre hook ran: there is nothing for a post hook to undo either.
		result = hooks.Stopped
	} else if hasPre {
		preRan = true
		code, err := hooks.Run(b.log, b.Environ, svc, hooks.PreBackup, "")
		if err != nil || code != 0 {
			// The service's files are in an unknown state (a half-written or stale dump).
			// Backing them up would make that the newest revision, the one auto-restore picks.
			log(logging.Error, fmt.Sprintf("Pre-backup hook failed for %s service (%s); skipping its backup so the newest revision stays the last good one.", svc.Name, exitText(code, err)))
			result = hooks.Skipped
		}
	}
	if result == hooks.Success {
		if b.stopped() {
			result = hooks.Stopped
		} else {
			b.lock.SetStage(ctx, "backup")
			result = b.primaryBackup(svc, filters, log)
		}
	}

	// The post hook undoes the pre hook (restarts what it stopped), so it runs whenever the
	// pre hook ran: after a failed pre hook, a failed backup, or a stop. Without a pre hook
	// it always runs.
	b.lock.SetStage(ctx, "post-backup")
	if hasPost && (preRan || !hasPre) {
		b.waitWhilePaused()
		code, err := hooks.Run(b.log, b.Environ, svc, hooks.PostBackup, result)
		if err != nil || code != 0 {
			log(logging.Error, fmt.Sprintf("Post-backup hook failed for %s service (%s); check that whatever its pre hook stopped is running again.", svc.Name, exitText(code, err)))
		}
	}
	if result != hooks.Success {
		return false, result == hooks.Stopped || b.stopped()
	}
	if !b.stopped() {
		b.addStorages(svc, log)
	}
	b.lock.SetStage("duplicacy", "backup")
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
	return proc.Spec{Path: b.Duplicacy, Args: args, Dir: dir, Env: b.env, Log: b.log, Service: service}
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
	if err := os.WriteFile(filepath.Join(repo, "filters"), []byte(content), 0o644); err != nil {
		log(logging.Error, fmt.Sprintf("Unable to create the Duplicacy filters file for the %s service.", svc.Name))
	}
	log(logging.Info, fmt.Sprintf("Duplicacy filters configured for %s service.", svc.Name))

	log(logging.Info, fmt.Sprintf("Starting backup to %s for %s service.", primary.Name, svc.Name))
	began := time.Now()
	p, err := b.start(b.duplicacy(svc.Dir, svc.Name, "backup", "-storage", storage, "-stats", "-threads", b.cfg.Threads))
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
				p.Wait()
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

func (b *Backup) isFailing(storage string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.failing[storage]
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
	var names []string
	for _, t := range b.cfg.Targets[1:] {
		names = append(names, t.StorageName())
	}
	failed := b.copyLegs(dir, names)
	if len(failed) == 0 || b.isSignaled() {
		return
	}
	b.log.Message(logging.Warning, "", "Retrying failed copies once: "+strings.Join(failed, " ")+".")
	for _, n := range b.copyLegs(dir, failed) {
		b.log.Message(logging.Error, n, fmt.Sprintf("Copy to %s storage failed after retry.", n))
	}
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
		b.log.Message(logging.Info, n, fmt.Sprintf("Copying backup to %s storage.", n))
		p, err := b.start(b.duplicacy(dir, n, "copy", "-from", primary, "-to", n,
			"-key", b.Layout.RSAPrivateKey(), "-threads", b.cfg.Threads, "-download-threads", b.cfg.Threads))
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
		}
	}
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
		b.lock.Record("stopped")
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
