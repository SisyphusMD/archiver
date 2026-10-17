package deepcheck

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/copier"
	"github.com/SisyphusMD/archiver/internal/damage"
	"github.com/SisyphusMD/archiver/internal/inuse"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/notify"
	"github.com/SisyphusMD/archiver/internal/proc"
	"github.com/SisyphusMD/archiver/internal/repository"
	"github.com/SisyphusMD/archiver/internal/resume"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

// Run is a manual or scheduled deep check. Empty Storage selects scheduled targets.
type Run struct {
	Layout                       layout.Layout
	Source                       config.Source
	Environ                      []string
	Storage, Hostname, Duplicacy string
	Stdout, Stderr               io.Writer
	Signals                      <-chan os.Signal
	stopped                      bool
}

// RepoDir reuses the copy workers' storage identity scheme in a separate private root.
func RepoDir(logDir string, primary, target config.Target) string {
	return filepath.Join(logDir, ".deep-check-repos", filepath.Base(copier.RepoDir(logDir, primary, target)))
}

// Execute returns nonzero if any selected storage failed or was interrupted.
func (r *Run) Execute() int {
	cfg, _, err := config.Load(r.Source, r.Environ)
	if err == nil {
		err = cfg.Validate(r.Source.SecretsDir)
	}
	if err != nil {
		fmt.Fprintln(r.Stderr, err)
		return 1
	}
	selected := false
	code := 0
	for _, t := range cfg.Targets {
		if (r.Storage == "" && t.DeepCheck == "") || (r.Storage != "" && t.Name != r.Storage) {
			continue
		}
		selected = true
		if !r.one(cfg, t) {
			code = 1
			if r.stopped {
				break
			}
		}
	}
	if r.Storage != "" && !selected {
		fmt.Fprintf(r.Stderr, "%q is not a configured storage\n", r.Storage)
		return 1
	}
	return code
}

func backupRunning(l layout.Layout) bool {
	b, held, err := lockstate.ReadLock(l.BackupLock())
	return err == nil && held && b.Alive()
}

func (r *Run) one(cfg *config.Config, t config.Target) bool {
	name := t.StorageName()
	lock, _, err := runlock.Acquire(r.Layout.DeepCheckLock(name), r.Layout.DeepCheckStopFlag(name), "deep-check", name)
	if err != nil {
		fmt.Fprintln(r.Stderr, "Cannot take deep-check lock:", err)
		return false
	}
	defer lock.Release()
	log := &logging.Log{Dir: r.Layout.LogDir(), Basename: "deep-check-" + name, Stdout: r.Stdout}
	log.Rotate()
	n := notify.FromConfig(cfg, r.Hostname, func(failed bool, msg string) {
		level := logging.Info
		if failed {
			level = logging.Error
		}
		log.Unnotified(level, name, msg)
	})
	n.Incidents = r.Layout.Incidents()
	began := time.Now()
	result := Read(r.Layout)[name]
	result.At, result.OK, result.Seconds, result.Message = began.Unix(), false, 0, "interrupted; remains due"
	if err := save(r.Layout, name, result); err != nil {
		log.Message(logging.Error, name, err.Error())
		return false
	}
	if prior, pending := resume.Read(r.Layout.RunRecord("deep-check-" + name)); pending {
		log.Message(logging.Info, name, "Resuming the deep check started "+logging.Timestamp(prior.Started)+" from its saved verified chunks.")
	}
	record, err := resume.Begin(r.Layout.RunRecord("deep-check-"+name), resume.Record{Services: map[string]string{name: resume.Pending}})
	if err != nil {
		log.Message(logging.Error, name, err.Error())
		return false
	}
	timeout, _ := config.ParseInterval(cfg.DeepCheckTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	stopped, completed := false, false
	// Registration and the prune gate can wait too: cancellation must also give way then.
	finished := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-finished:
				return
			case <-ctx.Done():
				return
			case <-r.Signals:
				cancel()
				return
			case <-tick.C:
				if lock.StopRequested() || backupRunning(r.Layout) {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { close(finished); <-watcherDone }()
	if backupRunning(r.Layout) || lock.StopRequested() {
		cancel()
	}
	repo := RepoDir(r.Layout.LogDir(), cfg.Targets[0], t)
	err = layout.Private(filepath.Dir(repo))
	if err == nil {
		err = layout.Private(repo)
	}
	if err == nil {
		err = layout.Private(filepath.Join(repo, ".duplicacy"))
	}
	if err == nil {
		err = repository.Attach(repo, t, r.Hostname+"-archiver-deep-check", r.Layout.HostKeys())
	}
	var output diagnostics
	var held *inuse.Holding
	if err == nil {
		// Protect all revisions, including ones created during this check. Readers register
		// under the shared gate; prunes see this hold without delaying a backup.
		var release func()
		release, err = inuse.Gate(ctx, r.Layout.InUseDir(), name, false)
		if err == nil {
			held, err = inuse.Hold(r.Layout.InUseDir(), "deep-check", []inuse.Revision{{Storage: name, ID: "*", Rev: 0, AndNewer: true}})
			release()
			if err == nil {
				defer held.Release()
			}
		}
	}
	if err == nil && ctx.Err() == nil {
		lw := log.Writer(logging.Info, name)
		var p *proc.Proc
		err = lock.StartUnpaused(func() bool {
			for ctx.Err() == nil && lock.State().Paused() {
				select {
				case <-ctx.Done():
				case <-time.After(100 * time.Millisecond):
				}
			}
			return ctx.Err() != nil
		}, func() error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var err error
			p, err = proc.Start(proc.Spec{Path: r.Duplicacy, Dir: repo, Env: cfg.DuplicacyEnviron(r.Environ, r.Layout.SSHPrivateKey()), Output: io.MultiWriter(lw, &output), Interrupt: true, ExtraFiles: []*os.File{held.File(), lock.File()},
				Args: proc.NoScript("check", "-all", "-chunks", "-fossils", "-persist", "-storage", name, "-key", r.Layout.RSAPrivateKey(), "-threads", cfg.Threads)})
			return err
		})
		if err == nil {
			select {
			case <-p.Done():
			case <-ctx.Done():
				stopped = true
				// SIGCONT lets a paused Duplicacy handle SIGINT and save its verified chunks.
				p.Signal(syscall.SIGCONT)
				p.Terminate(false)
			}
			code, waitErr := p.Wait()
			if code >= 128 {
				stopped = true
			}
			if waitErr != nil {
				err = waitErr
			} else if code != 0 {
				err = fmt.Errorf("duplicacy exited %d", code)
			}
			if !stopped && ctx.Err() == nil {
				completed = true
				if clearErr := clearVerified(repo, name); clearErr != nil {
					err = clearErr
				}
			}
		}
		lw.Close()
	}
	if ctx.Err() != nil && !completed {
		stopped = true
	}
	result.Seconds = time.Since(began).Seconds()
	switch {
	case stopped:
		r.stopped = true
		result.Message = "interrupted (stop, timeout or backup); remains due until the next scheduled slot"
		_ = lock.Record("stopped")
		log.Message(logging.Warning, name, result.Message)
	case err != nil:
		result.Message = fmt.Sprintf("Deep check of storage %s failed: %v", name, err)
		if detail := output.describe(); detail != "" {
			result.Message += "; " + detail
		}
		log.Message(logging.Error, name, result.Message)
		report(n, name, result)
		_ = lock.Record("failed")
	default:
		result.OK, result.Message, result.LastSuccess = true, "passed", time.Now().Unix()
		log.Message(logging.Info, name, "Deep check of storage "+name+" passed.")
		report(n, name, result)
		record.End()
		_ = lock.Record("completed")
	}
	if err := save(r.Layout, name, result); err != nil {
		log.Message(logging.Error, name, err.Error())
		return false
	}
	return result.OK
}

// Only Duplicacy 3.2.5's CheckSnapshots progress file is forgotten. Its other cache
// files are unrelated to skipping verified chunks and may hold fossil collections.
func clearVerified(repo, storage string) error {
	err := os.Remove(filepath.Join(repo, ".duplicacy", "cache", storage, "verified_chunks"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func report(n *notify.Notifier, storage string, r Result) {
	key := "deep-check:" + storage
	if r.OK {
		n.Clear(key, "Deep Check Recovered", "Deep check of storage "+storage+" passed.")
	} else {
		n.Raise(key, notify.Failure, "Deep Check Failed", r.Message)
	}
}

// Keep only damage lines, bounded independently of the size of a full storage's log.
type diagnostics struct {
	pending           string
	lines             []string
	seen              map[string]bool
	chunks, revisions int
}

var damagedChunk = regexp.MustCompile(`(?i)(?:chunk |chunk: )([a-f0-9]{64})`)

func (d *diagnostics) Write(b []byte) (int, error) {
	d.pending += string(b)
	for {
		line, rest, ok := strings.Cut(d.pending, "\n")
		if !ok {
			break
		}
		d.pending = rest

		revs := damage.Revisions(line)
		key := ""
		if len(revs) > 0 && d.revisions < 3 {
			key = "revision:" + revs[0]
		} else if m := damagedChunk.FindStringSubmatch(line); m != nil && d.chunks < 3 &&
			(strings.Contains(line, "Failed") || strings.Contains(line, "does not exist") || strings.Contains(line, "hash id") || strings.Contains(line, "can't be found") || strings.Contains(line, "has a size of 0")) {
			key = "chunk:" + m[1]
		}
		if key != "" && !d.seen[key] {
			if d.seen == nil {
				d.seen = map[string]bool{}
			}
			d.seen[key] = true
			d.lines = append(d.lines, line)
			if strings.HasPrefix(key, "revision:") {
				d.revisions++
			} else {
				d.chunks++
			}
		}
	}
	if len(d.pending) > 16384 {
		d.pending = d.pending[len(d.pending)-16384:]
	}
	return len(b), nil
}
func (d *diagnostics) describe() string { return strings.Join(d.lines, "; ") }
