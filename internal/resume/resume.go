// Package resume records runs in progress on the logs volume, so a run that a crash, a kill
// or a container restart ended is finished on the next start (ADR 46): the post-backup hook
// of each service left between its hooks runs, and the run starts again at once.
package resume

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/SisyphusMD/archiver/internal/atomicfile"
)

// A service's stage in a backup.
const (
	Pending = "pending"
	Hooked  = "hooked" // its pre-backup hook ran and its post-backup hook has not
	Done    = "done"
)

// Record is one run in progress.
type Record struct {
	Started int64 `json:"started"`
	// Services are a backup's service directories and their stages, or the directories a
	// drill restored into.
	Services map[string]string `json:"services,omitempty"`
}

// Unfinished reports whether the service at dir did not finish in the run recorded.
func (r Record) Unfinished(dir string) bool {
	s, ok := r.Services[dir]
	return ok && s != Done
}

// Read returns the run recorded at path; ok is false when there is none.
func Read(path string) (Record, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Record{}, false
	}
	var r Record
	if json.Unmarshal(b, &r) != nil {
		return Record{}, false
	}
	return r, true
}

// Run is a recorded run in progress. Its methods are safe for concurrent use by the
// services of one backup.
type Run struct {
	path string
	mu   sync.Mutex
	rec  Record
}

// Begin records a run starting now.
func Begin(path string, rec Record) (*Run, error) {
	if rec.Started == 0 {
		rec.Started = time.Now().Unix()
	}
	r := &Run{path: path, rec: rec}
	return r, r.write()
}

// Replace records rec in place of what the run recorded.
func (r *Run) Replace(rec Record) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rec.Started = r.rec.Started
	r.rec = rec
	_ = r.write()
}

// Set records a service's stage; an error means it is not on disk.
func (r *Run) Set(dir, stage string) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rec.Services == nil {
		r.rec.Services = map[string]string{}
	}
	r.rec.Services[dir] = stage
	return r.write()
}

// Forget drops a service (or a drill's directory) from the record.
func (r *Run) Forget(key string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rec.Services, key)
	_ = r.write()
}

// ShuttingDown reports whether the container is stopping (the entrypoint's `archiver stop
// --shutdown` left flag): a run that stops then keeps its record and starts again with the
// container.
func ShuttingDown(flag string) bool {
	_, err := os.Stat(flag)
	return err == nil
}

// End removes the record: the run ended, however it went, and nothing is left to resume.
func (r *Run) End() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.Remove(r.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = os.WriteFile(r.path, nil, 0o600) // an empty record reads as none
	}
	// Synced, so a power cut now does not bring back a run that already ended.
	syncDir(filepath.Dir(r.path))
}

func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}

// write saves the record whole and synced (atomicfile), so a crash mid-write leaves the
// previous one and a power cut after a pre-backup hook stopped something still finds the
// hook recorded. Called under r.mu, or before r is shared.
func (r *Run) write() error {
	b, err := json.Marshal(r.rec)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil { //nolint:gosec // G301: the logs directory, readable as it always was
		return err
	}
	return atomicfile.Write(r.path, b, 0o600)
}
