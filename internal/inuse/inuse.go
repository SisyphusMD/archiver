// Package inuse records which snapshot revisions running copies and restores still need,
// so a prune can leave them out (ADR 19).
//
// Each reader holds one file in the registry directory, locked for as long as the reader
// lives, listing "<storage> <snapshot id> <revision>" lines. A file nobody holds is left
// by a reader that died, and is ignored and removed. A reader lists the revisions it will
// need and registers them under a shared lock on the storage's gate; a prune holds the
// gate exclusively from reading the registry until its deletions are done, so no reader
// can list a revision the prune is about to delete without the prune seeing it.
package inuse

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Revision is one snapshot revision on one storage, or with AndNewer that revision and every
// later one. ID "*" stands for every ID the same registration does not name: a copy reads
// revisions of snapshot IDs created after it listed local.
type Revision struct {
	Storage  string
	ID       string
	Rev      int
	AndNewer bool
}

func (r Revision) line() string {
	if r.AndNewer {
		return fmt.Sprintf("%s %s %d+", r.Storage, r.ID, r.Rev)
	}
	return fmt.Sprintf("%s %s %d", r.Storage, r.ID, r.Rev)
}

func (r Revision) covers(rev int) bool { return rev == r.Rev || (r.AndNewer && rev >= r.Rev) }

// Holding is one reader's registration.
type Holding struct {
	dir, holder, path string
	f                 *os.File
}

// Hold registers revs for the life of the returned Holding. Call it with the storage's
// gate held shared (Gate), around the listing the revisions came from.
func Hold(dir, holder string, revs []Revision) (*Holding, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	h := &Holding{dir: dir, holder: holder}
	tmp, err := h.write(revs)
	if err != nil {
		return nil, err
	}
	h.path = filepath.Join(dir, strings.TrimPrefix(filepath.Base(tmp), ".tmp-"))
	if err := os.Rename(tmp, h.path); err != nil {
		h.f.Close()
		os.Remove(tmp)
		return nil, err
	}
	return h, nil
}

// write creates a locked file of revs under a name readers skip, so none sees it before it
// is complete and locked.
func (h *Holding) write(revs []Revision) (string, error) {
	f, err := os.CreateTemp(h.dir, ".tmp-"+h.holder+"-*")
	if err != nil {
		return "", err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	w := bufio.NewWriter(f)
	for _, r := range revs {
		fmt.Fprintln(w, r.line())
	}
	if err := w.Flush(); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	h.f = f
	return f.Name(), nil
}

// Narrow replaces the registered revisions, for a reader that has learned it needs fewer.
// It is atomic: a prune sees either the old set or the new one.
func (h *Holding) Narrow(revs []Revision) error {
	old := h.f
	tmp, err := h.write(revs)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, h.path); err != nil {
		h.f.Close()
		os.Remove(tmp)
		h.f = old
		return err
	}
	old.Close()
	return nil
}

// File is the locked registration file. A child process given it (exec.Cmd.ExtraFiles)
// shares the lock, so the registration stays live for as long as the child runs, even if
// this process dies first. Narrow replaces the file.
func (h *Holding) File() *os.File { return h.f }

// Release ends the registration.
func (h *Holding) Release() {
	if h == nil || h.f == nil {
		return
	}
	os.Remove(h.path)
	h.f.Close()
	h.f = nil
}

// Set is what live readers have registered on one storage.
type Set struct{ holdings [][]Revision }

// Has reports whether any reader still needs id's revision rev.
func (s *Set) Has(id string, rev int) bool {
	for _, h := range s.holdings {
		named := false
		for _, r := range h {
			if r.ID == id {
				named = true
				if r.covers(rev) {
					return true
				}
			}
		}
		if named {
			continue
		}
		for _, r := range h {
			if r.ID == "*" && r.covers(rev) {
				return true
			}
		}
	}
	return false
}

// Empty reports whether no reader registered anything on the storage.
func (s *Set) Empty() bool { return len(s.holdings) == 0 }

// Read returns what live readers registered on storage, removing files left by readers that
// died.
func Read(dir, storage string) (*Set, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return &Set{}, nil
	}
	if err != nil {
		return nil, err
	}
	set := &Set{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || e.IsDir() {
			continue
		}
		revs, err := readOne(filepath.Join(dir, e.Name()), storage)
		if err != nil {
			return nil, err
		}
		if len(revs) > 0 {
			set.holdings = append(set.holdings, revs)
		}
	}
	return set, nil
}

// readOne reads one live reader's file. A reader narrowing its registration renames a new
// file over the old one, so a file found unlocked is reopened until it is either locked (a
// live reader's, read) or unlocked and still the same file (a dead reader's, removed).
func readOne(path, storage string) ([]Revision, error) {
	for range 10 {
		f, err := os.Open(path)
		if os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB) == nil {
			fi, statErr := os.Stat(path)
			fo, _ := f.Stat()
			f.Close()
			switch {
			case os.IsNotExist(statErr):
				return nil, nil
			case statErr == nil && os.SameFile(fi, fo):
				os.Remove(path)
				return nil, nil
			}
			continue
		}
		revs, err := parse(f, storage)
		f.Close()
		return revs, err
	}
	return nil, fmt.Errorf("%s kept changing while being read", path)
}

func parse(f *os.File, storage string) ([]Revision, error) {
	var out []Revision
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 3 || fields[0] != storage {
			continue
		}
		r := Revision{Storage: fields[0], ID: fields[1]}
		rev, newer := strings.CutSuffix(fields[2], "+")
		n, err := strconv.Atoi(rev)
		if err != nil {
			continue
		}
		r.Rev, r.AndNewer = n, newer
		out = append(out, r)
	}
	return out, sc.Err()
}

// Gate locks storage's gate: shared for a reader listing and registering, exclusive for a
// prune. The returned function releases it. An exclusive wait ends with ctx.
func Gate(ctx context.Context, dir, storage string, exclusive bool) (func(), error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, ".gate-"+storage), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	for {
		if err := syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB); err == nil {
			return func() { f.Close() }, nil
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}
