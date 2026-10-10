package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SisyphusMD/archiver/internal/checkin"
	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/copier"
	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/kit"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/notify"
	"github.com/SisyphusMD/archiver/internal/pipeline"
)

// copyWorkers runs one worker per secondary storage (ADR 11).
type copyWorkers struct {
	mu          sync.Mutex
	owns        bool // this daemon's workers copy; decided, workers built, before the socket opens
	names       []string
	fingerprint string // the storages the workers keep (config.StorageFingerprint)
	logDir      string
	repos       []string // the workers' repositories; any other in logDir is stale
	log         *logging.Log
	store       *copier.Store
	workers     []*copier.Worker
	running     sync.WaitGroup
}

// handle answers the daemon socket.
func (cw *copyWorkers) handle(cmd string) string {
	// Not held while acting: stopping slow copies must not delay other commands.
	cw.mu.Lock()
	owns, workers := cw.owns, append([]*copier.Worker(nil), cw.workers...)
	cw.mu.Unlock()
	cmd, arg, _ := strings.Cut(cmd, " ")
	if !owns {
		switch cmd {
		case daemon.CmdWorkers, daemon.CmdLocalChanged, daemon.CmdMirrorPlan, daemon.CmdAllowLarge, daemon.CmdExhaustive:
			return daemon.ReplyNoWorkers
		}
		return daemon.ReplyOK
	}
	// A backup run with other storage settings than the daemon's copies for itself.
	if (cmd == daemon.CmdWorkers || cmd == daemon.CmdLocalChanged || cmd == daemon.CmdExhaustive) && arg != "" && arg != cw.fingerprint {
		return daemon.ReplyNoWorkers
	}
	each := func(f func(*copier.Worker)) {
		for _, x := range workers {
			f(x)
		}
	}
	switch cmd {
	case daemon.CmdWorkers:
	case daemon.CmdLocalChanged:
		each((*copier.Worker).Wake)
	case daemon.CmdStop:
		each((*copier.Worker).Stop)
	case daemon.CmdPause:
		each((*copier.Worker).Pause)
	case daemon.CmdResume:
		each((*copier.Worker).Resume)
	case daemon.CmdAllowLarge:
		each((*copier.Worker).AllowLarge)
	case daemon.CmdExhaustive:
		each((*copier.Worker).ForceExhaustive)
	case daemon.CmdMirrorPlan:
		var parts []string
		for _, x := range workers {
			if !x.Upkeep.Mirror {
				parts = append(parts, x.Target+": not mirrored (PRUNE_BACKUPS is false)")
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			plan, err := x.PlanNow(ctx)
			cancel()
			if err != nil {
				parts = append(parts, fmt.Sprintf("%s: cannot plan (%v)", x.Target, err))
				continue
			}
			parts = append(parts, x.Target+": "+plan.Describe())
		}
		// One line on the wire; the CLI puts each target on its own.
		return strings.Join(parts, " | ")
	default:
		return "unknown command " + cmd
	}
	return daemon.ReplyOK
}

// shutdown ends every worker's copy or listing and waits for them.
func (cw *copyWorkers) shutdown() {
	cw.mu.Lock()
	for _, x := range cw.workers {
		x.Stop()
	}
	cw.mu.Unlock()
	cw.running.Wait()
}

// decide settles, before the control socket opens, whether this daemon's workers own the
// copies, so no backup is ever told otherwise while they start.
func (cw *copyWorkers) decide(l layout.Layout) {
	src := config.FromEnvironment()
	cfg, _, err := config.Load(src, os.Environ())
	if err != nil || cfg.Validate(src.SecretsDir) != nil || len(cfg.Targets) < 2 {
		return
	}
	host := pipeline.Hostname(os.Getenv)
	log := &logging.Log{Dir: l.LogDir(), Basename: "copies", Stdout: os.Stdout}
	// One notifier per worker, so one worker's slow send never holds up another's.
	notifier := func() *notify.Notifier {
		n := notify.FromConfig(cfg, host, func(failed bool, msg string) {
			level := logging.Info
			if failed {
				level = logging.Warning
			}
			log.Unnotified(level, "", msg)
		})
		n.Incidents = l.Incidents()
		return n
	}
	notifier().WatchLog(log)
	env := cfg.DuplicacyEnviron(os.Environ(), l.SSHPrivateKey())
	store := &copier.Store{Path: l.CopyWorkersState()}
	saved := store.Load()
	cw.mu.Lock()
	defer cw.mu.Unlock()
	// The workers exist before the socket opens, so a stop or pause it acknowledges always
	// reaches them, even one arriving before they run.
	for _, t := range cfg.Targets[1:] {
		name := t.StorageName()
		cw.names = append(cw.names, name)
		d := &copier.Duplicacy{
			Bin: "duplicacy", Repo: copier.RepoDir(l.LogDir(), cfg.Targets[0], t), Env: env, Log: log, Threads: cfg.Threads,
			PrivKey: l.RSAPrivateKey(), PubKey: filepath.Join(l.Root, "keys", "public.pem"),
			SnapshotID: host + "-archiver-copies", Primary: cfg.Targets[0], Target: t, InitLock: l.StorageInit,
			Diagnose: func(t config.Target) string { return kit.Diagnose(l, "", t) },
		}
		n := notifier()
		pings := &checkin.Latest{}
		w := copier.New(name, cfg.Targets[0].StorageName(), d, realClock{}, copier.Events{
			Log:   func(level, msg string) { log.Message(level, name, msg) },
			Raise: n.Raise,
			Clear: n.Clear,
			// In the background: a monitor that stalls must not hold up the worker, or its stop.
			Checkin: func(ok bool, msg string) {
				if t.CheckinURL == "" {
					return
				}
				pings.Send(func() {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
					defer cancel()
					if err := checkin.Ping(ctx, t.CheckinURL, ok, msg); err != nil {
						log.Message(logging.Warning, name, fmt.Sprintf("The check-in to STORAGE_TARGET_%d_CHECKIN_URL failed: %v", t.N, err))
					}
				})
			},
			Save: func(s copier.State) { _ = store.Save(s) },
		}, saved[name])
		w.CopyLock = l.CopyLock
		// The primary too: every copy reads it.
		primary := cfg.Targets[0]
		w.Probe = func(ctx context.Context) error {
			if _, err := kit.Probe(ctx, l, primary, false); err != nil {
				return fmt.Errorf("the primary %s: %w", primary.Name, err)
			}
			_, err := kit.Probe(ctx, l, t, false)
			return err
		}
		w.InUseDir = l.InUseDir()
		w.Upkeep = copier.Upkeep{
			Own:        ownIDs(cfg, host),
			Mirror:     cfg.PruneBackups,
			Exhaustive: cfg.ExhaustiveInterval(),
			Check:      cfg.TargetCheckInterval(t),
			Threads:    cfg.Threads,
		}
		cw.workers = append(cw.workers, w)
	}
	cw.owns, cw.log, cw.store, cw.fingerprint = true, log, store, cfg.StorageFingerprint()
	cw.logDir = l.LogDir()
	for _, t := range cfg.Targets[1:] {
		cw.repos = append(cw.repos, copier.RepoDir(l.LogDir(), cfg.Targets[0], t))
	}
}

// start runs the workers decide made. Each prepares its own repository in its first pass,
// so an unreachable target retries like a failed copy without holding up the others. Call
// it only once the control socket serves: without it backups cannot find the workers and
// would copy alongside them.
func (cw *copyWorkers) start(done <-chan struct{}) {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	if !cw.owns {
		return
	}
	cw.log.Rotate()
	for _, w := range cw.workers {
		cw.running.Add(1)
		go func() { defer cw.running.Done(); w.Run(done) }()
	}
	_ = cw.store.Prune(cw.names)
	// Only here, once this daemon owns the socket: a second daemon that is refused must not
	// remove the running one's repositories.
	copier.PruneRepos(cw.logDir, cw.repos)
	cw.log.Message(logging.Info, "", fmt.Sprintf("Copy workers started for %v.", cw.names))
	// One file per day, as each backup run gets one.
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(24 * time.Hour):
				cw.log.Rotate()
			}
		}
	}()
}

// ownIDs lists this deployment's snapshot IDs as the backup names them, afresh each time,
// so a service added since the daemon started is included.
func ownIDs(cfg *config.Config, host string) func() map[string]bool {
	return func() map[string]bool {
		ids := map[string]bool{}
		dirs, _ := config.ExpandServiceDirectories(cfg.ServiceDirectories)
		for _, d := range dirs {
			ids[host+"-"+filepath.Base(d)] = true
		}
		return ids
	}
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
