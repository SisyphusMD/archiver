package harness

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// Run is an archiver command started in the container and not yet finished.
type Run struct {
	cmd  *exec.Cmd
	out  bytes.Buffer
	errb bytes.Buffer
	done chan struct{}
	res  Result
}

// StartArchiver starts `archiver <args>` in the container without waiting for it, so a
// test can act while it runs. The command is killed if the test ends first.
func (d *Deployment) StartArchiver(t testing.TB, env map[string]string, args ...string) *Run {
	t.Helper()
	full := []string{"exec"}
	for k, v := range env {
		full = append(full, "-e", k+"="+v)
	}
	full = append(append(full, d.name, "archiver"), args...)
	r := &Run{cmd: exec.Command("docker", full...), done: make(chan struct{})}
	r.cmd.Stdout, r.cmd.Stderr = &r.out, &r.errb
	if err := r.cmd.Start(); err != nil {
		t.Fatalf("docker %s: %v", strings.Join(full, " "), err)
	}
	go func() {
		err := r.cmd.Wait()
		r.res = Result{Stdout: r.out.String(), Stderr: r.errb.String()}
		if ee, ok := err.(*exec.ExitError); ok {
			r.res.Code = ee.ExitCode()
		} else if err != nil {
			r.res.Code, r.res.Stderr = -1, r.res.Stderr+err.Error()
		}
		close(r.done)
	}()
	t.Cleanup(func() {
		if !r.Done() {
			r.cmd.Process.Kill()
			<-r.done
		}
	})
	return r
}

// Done reports whether the command has finished.
func (r *Run) Done() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

// Wait waits up to timeout for the command and returns how it ended.
func (r *Run) Wait(t testing.TB, timeout time.Duration) Result {
	t.Helper()
	if r.Done() {
		return r.res
	}
	select {
	case <-r.done:
		return r.res
	case <-time.After(timeout):
		r.cmd.Process.Kill()
		<-r.done
		t.Fatalf("%v still running after %v:\n%s", r.cmd.Args, timeout, r.res.Output())
		return Result{}
	}
}

// The methods below are the 0.11 command surface for runs; their names and flags are
// incidental (ADR 1) and change here, once, when v1 renames them.

// Backup runs one backup to completion.
func (d *Deployment) Backup(t testing.TB) Result {
	t.Helper()
	return d.Archiver(t, nil, "backup")
}

// StartBackup starts a backup in the foreground of a background exec, so its exit code is
// still observed.
func (d *Deployment) StartBackup(t testing.TB) *Run {
	t.Helper()
	return d.StartArchiver(t, nil, "backup")
}

// Maintenance runs one maintenance pass (check and prune) to completion.
func (d *Deployment) Maintenance(t testing.TB) Result {
	t.Helper()
	return d.Archiver(t, nil, "maintenance")
}

// StartMaintenance starts a maintenance pass without waiting for it.
func (d *Deployment) StartMaintenance(t testing.TB) *Run {
	t.Helper()
	return d.StartArchiver(t, nil, "maintenance")
}

// Pause suspends the running backup.
func (d *Deployment) Pause(t testing.TB) {
	t.Helper()
	if r := d.Archiver(t, nil, "pause"); r.Code != 0 {
		t.Fatalf("pause exited %d:\n%s", r.Code, r.Output())
	}
}

// Resume continues a paused backup.
func (d *Deployment) Resume(t testing.TB) {
	t.Helper()
	if r := d.Archiver(t, nil, "resume"); r.Code != 0 {
		t.Fatalf("resume exited %d:\n%s", r.Code, r.Output())
	}
}

// Exhaustive is how often maintenance lists the whole storage to collect orphans.
type Exhaustive string

const (
	ExhaustiveOff   Exhaustive = "off"
	ExhaustiveDaily Exhaustive = "daily"
)

// MaintenanceConfig says what a maintenance pass does. The retention policy stays the
// default, which keeps every revision a test can make within a day.
type MaintenanceConfig struct {
	Check      bool
	Prune      bool
	Exhaustive Exhaustive
}

// SetMaintenance configures maintenance. It must be called before Start.
func (d *Deployment) SetMaintenance(m MaintenanceConfig) {
	d.setEnv("CHECK_BACKUPS", fmt.Sprint(m.Check))
	d.setEnv("PRUNE_BACKUPS", fmt.Sprint(m.Prune))
	d.setEnv("PRUNE_EXHAUSTIVE_FREQUENCY", string(m.Exhaustive))
}

// SingleThreaded makes duplicacy use one upload thread, which stretches a large backup
// long enough to act on while it runs. It must be called before Start.
func (d *Deployment) SingleThreaded() {
	d.setEnv("DUPLICACY_THREADS", "1")
}

func (d *Deployment) setEnv(k, v string) {
	if d.Extra == nil {
		d.Extra = map[string]string{}
	}
	d.Extra[k] = v
}

// Gate holds every backup of one service in its pre-backup hook until opened, so a test
// can keep a run in progress for as long as it needs without racing it.
type Gate struct {
	dir string
}

// GatePreHook installs a pre-backup hook on service that records each entry and then
// waits for the gate. The gate starts closed. It must be called before Start. The hook is
// written in the form d's image runs.
func (d *Deployment) GatePreHook(t testing.TB, service string) *Gate {
	t.Helper()
	dir, err := os.MkdirTemp(filepath.Dir(d.Keys.PrivatePath), "gate-")
	if err != nil {
		t.Fatal(err)
	}
	const inContainer = "/e2e-gate"
	d.Mount(dir, inContainer)
	hook := `  echo entered >> ` + inContainer + `/entered
  while [ ! -e ` + inContainer + `/open ]; do sleep 0.2; done`
	writeServiceFiles(t, d.Image, d.Services[service], serviceFiles{pre: &hook})
	return &Gate{dir: dir}
}

// GatePostRestore installs a post-restore hook on service that records each entry and
// then waits for the gate, so a test can keep a restore in progress. It must be called
// before Start; the hook is executable, the form v1 runs.
func (d *Deployment) GatePostRestore(t testing.TB, service string) *Gate {
	t.Helper()
	dir, err := os.MkdirTemp(filepath.Dir(d.Keys.PrivatePath), "gate-")
	if err != nil {
		t.Fatal(err)
	}
	const inContainer = "/e2e-restore-gate"
	d.Mount(dir, inContainer)
	src := "#!/bin/bash\necho entered >> " + inContainer + "/entered\nwhile [ ! -e " + inContainer + "/open ]; do sleep 0.2; done\n"
	if err := os.WriteFile(filepath.Join(d.Services[service], "post-restore"), []byte(src), 0o755); err != nil {
		t.Fatal(err)
	}
	return &Gate{dir: dir}
}

// ServiceDir is service's directory inside the container.
func (d *Deployment) ServiceDir(service string) string { return containerServiceDir(service) }

// Entered is how many runs have reached the hook so far.
func (g *Gate) Entered(t testing.TB) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(g.dir, "entered"))
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(b), "\n")
}

// Open lets waiting and future runs through.
func (g *Gate) Open(t testing.TB) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(g.dir, "open"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Close holds future runs again.
func (g *Gate) Close(t testing.TB) {
	t.Helper()
	if err := os.Remove(filepath.Join(g.dir, "open")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

// DuplicacyStates returns the ps state of every duplicacy process in the container. It
// asks the Docker host (docker top), so it needs nothing from the image under test.
func (d *Deployment) DuplicacyStates(t testing.TB) []string {
	t.Helper()
	r := docker(t, "top", d.name, "-eo", "pid,stat,comm")
	if r.Code != 0 {
		t.Fatalf("docker top: %s", r.Output())
	}
	var states []string
	for _, line := range strings.Split(r.Stdout, "\n")[1:] {
		f := strings.Fields(line)
		if len(f) == 3 && f[2] == "duplicacy" {
			states = append(states, f[1])
		}
	}
	return states
}

// Poll calls cond until it returns true, failing the test with what after timeout.
func Poll(t testing.TB, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting for %s", timeout, what)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Hold checks cond repeatedly for the whole of d and fails the test with what the first
// time it is false. It is for asserting that something does NOT happen over a window.
func Hold(t testing.TB, d time.Duration, what string, cond func() bool) {
	t.Helper()
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if !cond() {
			t.Fatalf("%s stopped holding", what)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Fingerprint summarizes every file under dir by path and size, so two calls differ
// exactly when something was written, renamed, or removed there. A missing dir is empty.
func Fingerprint(t testing.TB, dir string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		lines = append(lines, fmt.Sprintf("%s %d", rel, info.Size()))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// ChunkCount is how many chunk files a local storage holds, or 0 before it has any.
func ChunkCount(t testing.TB, storageDir string) int {
	t.Helper()
	s := Fingerprint(t, filepath.Join(storageDir, "chunks"))
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// PlantOrphanChunk writes a chunk no snapshot references into a local storage, named the
// way duplicacy names chunks (chunks/<2 hex>/<62 hex>). Only an exhaustive prune can find
// it; a plain prune looks only at chunks of the revisions it deletes. It returns the
// chunk's path.
func PlantOrphanChunk(t testing.TB, storageDir string) string {
	t.Helper()
	id := make([]byte, 32)
	body := make([]byte, 1024)
	rand.Read(id)
	rand.Read(body)
	h := hex.EncodeToString(id)
	p := filepath.Join(storageDir, "chunks", h[:2], h[2:])
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// ChunkState is where a chunk stands in duplicacy's two-step collection.
type ChunkState string

const (
	ChunkLive   ChunkState = "live"
	ChunkFossil ChunkState = "fossil" // renamed <chunk>.fsl by a prune, deletable later
	ChunkGone   ChunkState = "gone"
)

// StateOf reports the state of the chunk at path (as PlantOrphanChunk returned it).
// The .fsl suffix is duplicacy's storage format, which every version reads.
func StateOf(t testing.TB, path string) ChunkState {
	t.Helper()
	for _, c := range []struct {
		p string
		s ChunkState
	}{{path, ChunkLive}, {path + ".fsl", ChunkFossil}} {
		if _, err := os.Stat(c.p); err == nil {
			return c.s
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	return ChunkGone
}

// LargestChunk returns the path of the biggest chunk file on a local storage. With a
// multi-megabyte random file in the backup, that is one of its data chunks, never
// snapshot metadata, which duplicacy keeps small.
func LargestChunk(t testing.TB, storageDir string) string {
	t.Helper()
	var best string
	var size int64 = -1
	err := filepath.WalkDir(filepath.Join(storageDir, "chunks"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > size {
			best, size = p, info.Size()
		}
		return nil
	})
	if err != nil || best == "" {
		t.Fatalf("no chunks on %s: %v", storageDir, err)
	}
	return best
}
