package lockstate

import (
	"encoding/json"
	"os"

	"github.com/SisyphusMD/archiver/internal/atomicfile"
)

// DrillResult is the last drill of one service on one storage.
type DrillResult struct {
	At       int64  `json:"at"`
	Revision int    `json:"revision,omitempty"`
	Files    int    `json:"files,omitempty"`
	OK       bool   `json:"ok"`
	Skipped  bool   `json:"skipped,omitempty"`
	Message  string `json:"message,omitempty"`
}

// DrillState is what drills remember between runs (logs/.drill-state.json).
type DrillState struct {
	// Rotation is, per storage, where the next drill's services start in the sorted list.
	Rotation map[string]int `json:"rotation,omitempty"`
	// Results are the last drill of each service, by storage then service.
	Results map[string]map[string]DrillResult `json:"results,omitempty"`
	// LastRun is when the last drill ended and whether it failed; LastPass is when one last
	// ended with every service it tried restored. The healthcheck reads both (ADR 28).
	LastRun    int64 `json:"last_run,omitempty"`
	LastFailed bool  `json:"last_failed,omitempty"`
	LastPass   int64 `json:"last_pass,omitempty"`
	// Scheduled is when the daemon first ran with drills scheduled: the healthcheck's start
	// for "none has passed in twice the interval" until a drill has run at all.
	Scheduled int64 `json:"scheduled,omitempty"`
}

// ReadDrillState reads the drill state; a missing file is an empty state.
func ReadDrillState(path string) (DrillState, error) {
	var s DrillState
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

// WriteDrillState replaces the drill state atomically.
func WriteDrillState(path string, s DrillState) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(path, append(b, '\n'), 0o644)
}
