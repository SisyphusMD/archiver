// Package atomicfile writes a file whole: to a temporary file of its own beside it, created
// with a random name (never one that could already exist, a planted link included), synced,
// and renamed into place, so a reader never sees half of it and a crash leaves the old one.
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write replaces path with data, with mode as the file's permissions.
func Write(path string, data []byte, mode os.FileMode) error {
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	f, err := os.CreateTemp(dir, "."+base+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	err = f.Chmod(mode)
	if err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
