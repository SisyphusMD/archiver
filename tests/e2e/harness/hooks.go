package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// PreHook is what a service's pre-backup hook does.
type PreHook int

const (
	PreSucceeds PreHook = iota
	PreFails
)

// HookSpec is a pair of hooks for one service. Both record every call; with RecordEnv
// each also saves the environment it was given.
type HookSpec struct {
	Pre       PreHook
	RecordEnv bool
}

// Hooks installs hooks into a deployment's services and reads back what they recorded.
// The 0.11 hook surface (a sourced service-backup-settings.sh defining two functions) is
// incidental (ADR 1): v1's executable hooks become a second rendering of the same spec.
type Hooks struct {
	d   *Deployment
	dir string // host side of containerHookDir
}

const containerHookDir = "/hook-records"

// NewHooks mounts a record directory under dir into d. It must be called before Start.
func NewHooks(t testing.TB, d *Deployment, dir string) *Hooks {
	t.Helper()
	rec := filepath.Join(dir, "hook-records")
	if err := os.MkdirAll(rec, 0o777); err != nil {
		t.Fatal(err)
	}
	d.Mount(rec, containerHookDir)
	return &Hooks{d: d, dir: rec}
}

// Install gives service the hooks in spec, replacing any it had. Services are bind
// mounts, so this also works on a running deployment, between backups.
func (h *Hooks) Install(t testing.TB, service string, spec HookSpec) {
	t.Helper()
	svcDir, ok := h.d.Services[service]
	if !ok {
		t.Fatalf("no service %q", service)
	}
	fn := func(phase string, fail bool) string {
		body := fmt.Sprintf("  echo '%s %s' >> %s/calls\n", service, phase, containerHookDir)
		if spec.RecordEnv {
			body += fmt.Sprintf("  env > %s/%s-%s.env\n", containerHookDir, service, phase)
		}
		if fail {
			body += "  return 7\n"
		}
		return fmt.Sprintf("service_specific_%s_backup_function() {\n%s}\n", phase, body)
	}
	script := fn("pre", spec.Pre == PreFails) + fn("post", false)
	if err := os.WriteFile(filepath.Join(svcDir, "service-backup-settings.sh"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Calls lists the hook calls so far, in order, as "<service> pre" and "<service> post".
func (h *Hooks) Calls(t testing.TB) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(h.dir, "calls"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

// Ran reports whether service's hook of phase ("pre" or "post") has been called, and for
// a post hook, only if it came after that service's pre hook.
func (h *Hooks) Ran(t testing.TB, service, phase string) bool {
	t.Helper()
	calls := h.Calls(t)
	i := slices.Index(calls, service+" "+phase)
	if i < 0 || phase == "pre" {
		return i >= 0
	}
	pre := slices.Index(calls, service+" pre")
	return pre >= 0 && pre < i
}

// Env returns the environment service's hook of phase was last called with, one
// KEY=value per line. It fails the test if that hook never recorded one.
func (h *Hooks) Env(t testing.TB, service, phase string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(h.dir, service+"-"+phase+".env"))
	if err != nil {
		t.Fatalf("%s %s hook recorded no environment: %v", service, phase, err)
	}
	return string(b)
}

// RequestStop asks the running backup to stop.
func (d *Deployment) RequestStop(t testing.TB) {
	t.Helper()
	if r := d.Archiver(t, nil, "stop"); r.Code != 0 {
		t.Fatalf("stop exited %d:\n%s", r.Code, r.Output())
	}
}

// RequestMaintenanceStop asks the running maintenance pass to stop.
func (d *Deployment) RequestMaintenanceStop(t testing.TB) {
	t.Helper()
	if r := d.Archiver(t, nil, "stop", "maintenance"); r.Code != 0 {
		t.Fatalf("stop maintenance exited %d:\n%s", r.Code, r.Output())
	}
}

// WaitForChunk polls until a local storage holds at least one chunk, which is the first
// sign on storage that a backup is uploading.
func WaitForChunk(t testing.TB, storageDir string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if names := chunkNames(storageDir); len(names) > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no chunk reached %s within %s", storageDir, timeout)
}

func chunkNames(storageDir string) []string {
	var names []string
	filepath.Walk(filepath.Join(storageDir, "chunks"), func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			names = append(names, p)
		}
		return nil
	})
	return names
}

// BreakChunkUploads makes a local storage refuse every chunk while its config and
// snapshot listing still work, so a backup to it gets past setup and fails mid-upload.
// Duplicacy stores chunks under chunks/<2 hex>/..., which cannot exist once chunks is a
// regular file. It must be called on an empty storage directory.
func BreakChunkUploads(t testing.TB, storageDir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(storageDir, "chunks"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// WriteSparse creates a file of size bytes that costs no disk but takes a backup many
// seconds to read, long enough to stop it mid-upload.
func WriteSparse(t testing.TB, path string, size int64) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Truncate(size); err != nil {
		t.Fatal(err)
	}
}
