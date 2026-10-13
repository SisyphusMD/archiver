package repository

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/hostkeys"
)

// Attach makes dir a repository of t by writing its preferences, without duplicacy init:
// init creates a storage that does not exist, and a probe must never write one. Every
// Archiver storage is encrypted.
func Attach(dir string, t config.Target, id, hostKeys string) error {
	url, err := t.URL()
	if err != nil {
		return err
	}
	prefs, _ := json.Marshal([]map[string]any{{
		"name": t.StorageName(), "id": id, "repository": "", "storage": url, "encrypted": true,
		"no_backup": true, "no_restore": false, "no_save_password": true, "keys": nil,
	}})
	if err := os.MkdirAll(filepath.Join(dir, ".duplicacy"), 0o700); err != nil {
		return err
	}
	if err := hostkeys.Pin(hostKeys, filepath.Join(dir, ".duplicacy")); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".duplicacy", "preferences"), prefs, 0o600)
}
