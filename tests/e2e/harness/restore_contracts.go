package harness

import (
	"io/fs"
	"path/filepath"
	"testing"
)

// SetFilters makes a service back up exactly the paths Duplicacy's include/exclude
// patterns select, written in the form image reads them. The patterns keep Duplicacy's
// syntax (first match wins, "+"/"-" prefix, trailing "/" for directories), which is the
// contract; how they reach archiver is not. Whatever file carries them is itself excluded,
// so the service's backed-up set is decided by patterns alone.
func SetFilters(t testing.TB, image, serviceDir string, patterns []string) {
	t.Helper()
	var all []string
	for _, f := range filterFiles(t, image) {
		all = append(all, "-"+f)
	}
	writeServiceFiles(t, image, serviceDir, serviceFiles{filters: append(all, patterns...)})
}

// Files keeps only the non-directory entries: which directories a restore recreates
// around the selected files is Duplicacy's business, not the filter contract.
func (tr Tree) Files() Tree {
	out := Tree{}
	for p, e := range tr {
		if !e.Mode.IsDir() {
			out[p] = e
		}
	}
	return out
}

// Content drops ownership and permission bits, keeping each path's type, bytes, and
// symlink target: what a restore must deliver even when it cannot recreate owners.
func (tr Tree) Content() Tree {
	out := Tree{}
	for p, e := range tr {
		out[p] = Entry{Mode: e.Mode.Type(), SHA256: e.SHA256, Target: e.Target}
	}
	return out
}

// ModTimes maps every regular file under root to its mtime in whole seconds, the
// precision Duplicacy stores.
func ModTimes(t testing.TB, root string) map[string]int64 {
	t.Helper()
	times := map[string]int64{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".duplicacy" {
			return filepath.SkipDir
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		times[filepath.ToSlash(rel)] = info.ModTime().Unix()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return times
}
