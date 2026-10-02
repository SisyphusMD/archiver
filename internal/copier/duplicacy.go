package copier

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/proc"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

// Duplicacy runs one worker's duplicacy commands from a repository of its own, which
// knows the primary and that worker's target and backs nothing up. A repository per
// target keeps one unreachable target from holding up the others, and keeps workers from
// writing one preferences file at once.
type Duplicacy struct {
	Bin        string
	Repo       string // this worker's repository directory
	Env        []string
	Log        *logging.Log
	Threads    string
	PrivKey    string
	PubKey     string
	SnapshotID string // required by init; never backed up
	Primary    config.Target
	Target     config.Target
	// InitLock names the lock held around creating one storage (layout.StorageInit), so a
	// worker never creates a storage at the same time as another worker or a backup.
	InitLock func(storage string) string

	mu       sync.Mutex
	prepared bool
}

// snapshotLine is one revision in `duplicacy list -a`.
var snapshotLine = regexp.MustCompile(`^Snapshot (\S+) revision (\d+) created at `)

// Prepare creates the repository once: the primary, then the target added as a copy of
// it with the arguments every existing secondary was made with (bit-identical, RSA). A
// failure is retried at the next pass like a failed copy.
func (d *Duplicacy) Prepare(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.prepared {
		return nil
	}
	if err := os.RemoveAll(d.Repo); err != nil {
		return err
	}
	if err := os.MkdirAll(d.Repo, 0o700); err != nil {
		return err
	}
	purl, err := d.Primary.URL()
	if err != nil {
		return err
	}
	if out, err := d.locked(ctx, d.Primary.StorageName(), func() (string, error) {
		return d.outputContext(ctx, "init", "-e", "-key", d.PubKey, "-storage-name", d.Primary.StorageName(), d.SnapshotID, purl)
	}); err != nil {
		return fmt.Errorf("preparing %s: %v: %s", d.Primary.StorageName(), err, lastLine(out))
	}
	turl, err := d.Target.URL()
	if err != nil {
		return err
	}
	if out, err := d.locked(ctx, d.Target.StorageName(), func() (string, error) {
		return d.outputContext(ctx, "add", "-e", "-copy", d.Primary.StorageName(), "-bit-identical", "-key", d.PubKey, d.Target.StorageName(), d.SnapshotID, turl)
	}); err != nil {
		return fmt.Errorf("preparing %s: %v: %s", d.Target.StorageName(), err, lastLine(out))
	}
	d.prepared = true
	return nil
}

// locked runs f under storage's creation lock.
func (d *Duplicacy) locked(ctx context.Context, storage string, f func() (string, error)) (string, error) {
	if d.InitLock != nil {
		release, err := runlock.Exclusive(ctx, d.InitLock(storage))
		if err != nil {
			return "", err
		}
		defer release()
	}
	return f()
}

func (d *Duplicacy) outputContext(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, d.Bin, args...)
	cmd.Dir, cmd.Env = d.Repo, d.Env
	out, err := cmd.CombinedOutput()
	return string(bytes.TrimSpace(out)), err
}

// Revisions lists every snapshot revision on storage. It reads only the snapshot files,
// so it is cheap even where listing chunks takes hours.
func (d *Duplicacy) Revisions(ctx context.Context, storage string) (map[string]bool, error) {
	out, err := d.outputContext(ctx, "list", "-a", "-storage", storage)
	if err != nil {
		return nil, fmt.Errorf("%v: %s", err, lastLine(out))
	}
	revs := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader([]byte(out)))
	for sc.Scan() {
		if m := snapshotLine.FindStringSubmatch(sc.Text()); m != nil {
			revs[m[1]+":"+m[2]] = true
		}
	}
	return revs, sc.Err()
}

func lastLine(s string) string {
	if i := bytes.LastIndexByte([]byte(s), '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// StartCopy copies every snapshot the target lacks from the primary.
func (d *Duplicacy) StartCopy(string) (Copy, error) {
	t := d.Target.StorageName()
	p, err := proc.Start(proc.Spec{
		Path: d.Bin, Dir: d.Repo, Env: d.Env, Log: d.Log, Service: t,
		Args: []string{"copy", "-from", d.Primary.StorageName(), "-to", t, "-key", d.PrivKey, "-threads", d.Threads, "-download-threads", d.Threads},
	})
	if err != nil {
		return nil, err
	}
	return running{p}, nil
}

type running struct{ p *proc.Proc }

func (r running) Wait() error {
	code, err := r.p.Wait()
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("duplicacy copy exited %d", code)
	}
	return nil
}
func (r running) Terminate() { r.p.Terminate(false) }
func (r running) Pause()     { r.p.Signal(syscall.SIGSTOP) }
func (r running) Resume()    { r.p.Signal(syscall.SIGCONT) }

// RepoDir is where a target's repository lives: rebuilt at every daemon start, never data.
func RepoDir(target string) string { return filepath.Join(os.TempDir(), "archiver-copies", target) }
