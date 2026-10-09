package lockstate

import (
	"encoding/json"
	"os"
)

// ServiceResult is the last backup of one service to the primary.
type ServiceResult struct {
	LastAttempt int64  `json:"last_attempt"`
	LastSuccess int64  `json:"last_success,omitempty"`
	Result      string `json:"result"` // success, failed, skipped or stopped
	Seconds     int64  `json:"seconds,omitempty"`
}

// BackupState is each service's last backup (logs/.backup-state.json), by its absolute
// directory, which backup health reads (ADR 32).
type BackupState struct {
	Services map[string]ServiceResult `json:"services,omitempty"`
}

// ReadBackupState reads the backup state; a missing file is an empty state.
func ReadBackupState(path string) (BackupState, error) {
	var s BackupState
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

// WriteBackupState replaces the backup state atomically.
func WriteBackupState(path string, s BackupState) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
