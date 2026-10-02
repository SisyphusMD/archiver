package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/copier"
	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/notify"
	"github.com/SisyphusMD/archiver/internal/pipeline"
)

// copyWorkers runs one worker per secondary storage (ADR 11) for a deployment on the Go
// pipeline. A deployment still on bash copies inline in its own pipeline, so it gets none:
// two copiers into one target would overlap.
type copyWorkers struct {
	mu          sync.Mutex
	owns        bool // this daemon's workers copy; decided, workers built, before the socket opens
	names       []string
	fingerprint string // the storages the workers keep (config.StorageFingerprint)
	log         *logging.Log
	store       *copier.Store
	workers     []*copier.Worker
	running     sync.WaitGroup
}

// handle answers the daemon socket.
func (w *copyWorkers) handle(cmd string) string {
	// Not held while acting: stopping slow copies must not delay other commands.
	w.mu.Lock()
	owns, workers := w.owns, append([]*copier.Worker(nil), w.workers...)
	w.mu.Unlock()
	cmd, arg, _ := strings.Cut(cmd, " ")
	if !owns {
		if cmd == daemon.CmdWorkers || cmd == daemon.CmdLocalChanged {
			return daemon.ReplyNoWorkers
		}
		return daemon.ReplyOK
	}
	// A backup run with other storage settings than the daemon's copies for itself.
	if (cmd == daemon.CmdWorkers || cmd == daemon.CmdLocalChanged) && arg != "" && arg != w.fingerprint {
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
	default:
		return "unknown command " + cmd
	}
	return daemon.ReplyOK
}

// shutdown ends every worker's copy or listing and waits for them.
func (w *copyWorkers) shutdown() {
	w.mu.Lock()
	for _, x := range w.workers {
		x.Stop()
	}
	w.mu.Unlock()
	w.running.Wait()
}

// decide settles, before the control socket opens, whether this daemon's workers own the
// copies, so no backup is ever told otherwise while they start.
func (cw *copyWorkers) decide(l layout.Layout) {
	src := config.FromEnvironment()
	if !goPipeline(l, src) {
		return
	}
	cfg, _, err := config.Load(src, os.Environ())
	if err != nil || cfg.Validate(src.SecretsDir) != nil || len(cfg.Targets) < 2 {
		return
	}
	host := pipeline.Hostname(os.Getenv)
	log := &logging.Log{Dir: l.LogDir(), Basename: "copies", ErrorTitle: "Copy Error", Stdout: os.Stdout}
	// One notifier per worker: a notifier drops a send made while it is already sending
	// (its guard against a failed notification looping), which would lose one of two
	// targets that go down or recover together.
	notifier := func() *notify.Notifier {
		return &notify.Notifier{
			Pushover: cfg.Pushover() && cfg.PushoverAPIToken != "" && cfg.PushoverUserKey != "",
			Token:    cfg.PushoverAPIToken, User: cfg.PushoverUserKey, Hostname: host,
			Logf: func(failed bool, msg string) {
				level := logging.Info
				if failed {
					level = logging.Warning
				}
				log.Message(level, "", msg)
			},
		}
	}
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
			Bin: "duplicacy", Repo: copier.RepoDir(name), Env: env, Log: log, Threads: cfg.Threads,
			PrivKey: l.RSAPrivateKey(), PubKey: filepath.Join(l.Root, "keys", "public.pem"),
			SnapshotID: host + "-archiver-copies", Primary: cfg.Targets[0], Target: t, InitLock: l.StorageInit,
		}
		w := copier.New(name, cfg.Targets[0].StorageName(), d, realClock{}, copier.Events{
			Log:    func(level, msg string) { log.Message(level, name, msg) },
			Notify: notifier().Send,
			Save:   func(s copier.State) { store.Save(s) },
		}, saved[name])
		w.CopyLock = l.CopyLock
		cw.workers = append(cw.workers, w)
	}
	cw.owns, cw.log, cw.store, cw.fingerprint = true, log, store, cfg.StorageFingerprint()
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
	cw.store.Prune(cw.names)
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

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
