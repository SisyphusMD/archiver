package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// Entry is what a restore must reproduce for one path.
type Entry struct {
	Mode   fs.FileMode
	UID    uint32
	GID    uint32
	SHA256 string // regular files
	Target string // symlinks
}

// Tree maps slash-separated relative paths to entries.
type Tree map[string]Entry

// Snapshot records every path under root except the ones archiver itself creates there.
func Snapshot(t testing.TB, root string) Tree {
	t.Helper()
	tree := Tree{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}
		if rel == ".duplicacy" || strings.HasPrefix(rel, ".duplicacy"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		st := info.Sys().(*syscall.Stat_t)
		e := Entry{Mode: info.Mode(), UID: st.Uid, GID: st.Gid}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			e.Target, err = os.Readlink(path)
		case info.Mode().IsRegular():
			e.SHA256, err = fileHash(path)
		}
		tree[filepath.ToSlash(rel)] = e
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// Diff reports every difference between want and got, or nothing when they match.
func (tr Tree) Diff(got Tree) []string {
	var diffs []string
	for p, w := range tr {
		g, ok := got[p]
		switch {
		case !ok:
			diffs = append(diffs, "missing "+p)
		case g != w:
			diffs = append(diffs, fmt.Sprintf("%s: tr %s, got %s", p, w, g))
		}
	}
	for p := range got {
		if _, ok := tr[p]; !ok {
			diffs = append(diffs, "unexpected "+p)
		}
	}
	sort.Strings(diffs)
	return diffs
}

func (e Entry) String() string {
	s := e.Mode.String() + " " + strconv.Itoa(int(e.UID)) + ":" + strconv.Itoa(int(e.GID))
	if e.SHA256 != "" {
		s += " " + e.SHA256[:12]
	}
	if e.Target != "" {
		s += " -> " + e.Target
	}
	return s
}

func fileHash(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// ObjectNames lists every file under storageDir/sub by relative path, sorted. Duplicacy
// names a chunk after its ID, so two storages hold the same chunks exactly when these
// lists match, even though each encrypts a chunk's bytes with its own random IV.
func ObjectNames(t testing.TB, storageDir, sub string) []string {
	t.Helper()
	root := filepath.Join(storageDir, sub)
	var names []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	return names
}

// Revisions lists the revision numbers of a snapshot ID on a local storage. Duplicacy
// stores revision n of id as snapshots/<id>/<n>, a layout every duplicacy version reads.
func Revisions(t testing.TB, storageDir, snapshotID string) []int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(storageDir, "snapshots", snapshotID))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var revs []int
	for _, e := range entries {
		if n, err := strconv.Atoi(e.Name()); err == nil {
			revs = append(revs, n)
		}
	}
	sort.Ints(revs)
	return revs
}
