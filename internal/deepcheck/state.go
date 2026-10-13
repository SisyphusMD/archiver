// Package deepcheck reads and verifies a storage's chunks on its own schedule (ADR 50).
package deepcheck

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/SisyphusMD/archiver/internal/atomicfile"
	"github.com/SisyphusMD/archiver/internal/layout"
)

// Result is the last attempt on one storage. A failure remains due until a pass.
type Result struct {
	At          int64   `json:"at"`
	OK          bool    `json:"ok"`
	Seconds     float64 `json:"seconds"`
	LastSuccess int64   `json:"last_success"`
	Message     string  `json:"message,omitempty"`
}

func stateDir(l layout.Layout) string { return filepath.Join(l.LogDir(), ".deep-check-state") }

// Read returns the last attempt for each storage.
func Read(l layout.Layout) map[string]Result {
	results := map[string]Result{}
	entries, _ := os.ReadDir(stateDir(l))
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(stateDir(l), e.Name()))
		var r Result
		if err == nil && json.Unmarshal(b, &r) == nil {
			results[e.Name()[:len(e.Name())-5]] = r
		}
	}
	return results
}

func save(l layout.Layout, storage string, r Result) error {
	if err := layout.Private(stateDir(l)); err != nil {
		return err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return atomicfile.Write(filepath.Join(stateDir(l), storage+".json"), b, 0o600)
}
