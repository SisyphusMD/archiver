package restore

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// revisionEntry is one entry of a revision, as `duplicacy cat` prints it: a directory's
// path ends in a slash, and mode holds Go's os.FileMode bits.
type revisionEntry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Time int64  `json:"time"`
	Mode uint32 `json:"mode"`
	UID  int    `json:"uid"`
	GID  int    `json:"gid"`
	Link string `json:"link"`
}

// revisionEntries calls fn for each entry of the revision r holds (the output of
// `duplicacy cat -r N` with no file), reading it as a stream: a revision can list millions.
func revisionEntries(r io.Reader, fn func(revisionEntry) error) error {
	dec := json.NewDecoder(r)
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return fmt.Errorf("the revision is not a JSON object")
	}
	found := false
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		if t != "files" {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return err
			}
			continue
		}
		if found {
			return fmt.Errorf("the revision lists files more than once")
		}
		found = true
		if t, err := dec.Token(); err != nil || t != json.Delim('[') {
			return fmt.Errorf("the revision's files are not a list")
		}
		for dec.More() {
			var e revisionEntry
			if err := dec.Decode(&e); err != nil {
				return err
			}
			if err := fn(e); err != nil {
				return err
			}
		}
		if _, err = dec.Token(); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("the revision lists no files")
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("the revision has trailing data")
	}
	return nil
}

// verifyRestored checks the copy in dir against the revision r records: every entry is
// there with its type, and its size, modification time and mode (a link's target instead),
// and its owner and group when owners is set; nothing else is there but the top-level
// .duplicacy directory Duplicacy makes in a restore's destination. Duplicacy has already compared
// each file's content with its recorded hash. It returns the number of regular files and
// the mismatches found, at most limit of them described.
func verifyRestored(dir string, r io.Reader, owners bool, limit int) (files int, problems []string, err error) {
	seen := map[string]bool{}
	mismatches := 0
	report := func(format string, a ...any) {
		mismatches++
		if len(problems) < limit {
			problems = append(problems, fmt.Sprintf(format, a...))
		}
	}
	const kind = os.ModeDir | os.ModeSymlink | os.ModeNamedPipe | os.ModeSocket | os.ModeDevice | os.ModeCharDevice
	const perm = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	err = revisionEntries(r, func(e revisionEntry) error {
		rel := strings.TrimSuffix(e.Path, "/")
		if rel == "." || !filepath.IsLocal(rel) || filepath.Clean(rel) != rel {
			return fmt.Errorf("invalid revision path %q", e.Path)
		}
		if seen[rel] {
			return fmt.Errorf("duplicate revision path %q", e.Path)
		}
		seen[rel] = true
		want := os.FileMode(e.Mode)
		fi, err := os.Lstat(filepath.Join(dir, rel))
		if err != nil {
			report("%s is missing", rel)
			return nil
		}
		if fi.Mode()&kind != want&kind {
			report("%s is %s, the revision has %s", rel, typeName(fi.Mode()), typeName(want))
			return nil
		}
		switch {
		case want&os.ModeSymlink != 0:
			if target, err := os.Readlink(filepath.Join(dir, rel)); err != nil || target != e.Link {
				report("%s points to %q, the revision has %q", rel, target, e.Link)
			}
		case want.IsRegular():
			files++
			if fi.Size() != e.Size {
				report("%s is %d bytes, the revision has %d", rel, fi.Size(), e.Size)
			}
		}
		if want&os.ModeSymlink == 0 && fi.ModTime().Unix() != e.Time {
			report("%s was modified at %d, the revision has %d", rel, fi.ModTime().Unix(), e.Time)
		}
		if want&os.ModeSymlink == 0 && fi.Mode()&perm != want&perm {
			report("%s has mode %v, the revision has %v", rel, fi.Mode()&perm, want&perm)
		}
		if owners {
			if st, ok := fi.Sys().(*syscall.Stat_t); ok && (int(st.Uid) != e.UID || int(st.Gid) != e.GID) {
				report("%s is owned by %d:%d, the revision has %d:%d", rel, st.Uid, st.Gid, e.UID, e.GID)
			}
		}
		return nil
	})
	if err != nil {
		return files, problems, fmt.Errorf("reading the revision: %w", err)
	}
	err = filepath.WalkDir(dir, func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if rel == "." {
			return nil
		}
		if rel == ".duplicacy" && de.IsDir() {
			return filepath.SkipDir
		}
		if !seen[rel] {
			report("%s was restored but the revision does not have it", rel)
		}
		return nil
	})
	if err != nil {
		return files, problems, err
	}
	if extra := mismatches - len(problems); extra > 0 {
		problems = append(problems, fmt.Sprintf("and %d more", extra))
	}
	return files, problems, nil
}

func typeName(m os.FileMode) string {
	switch {
	case m&os.ModeDir != 0:
		return "a directory"
	case m&os.ModeSymlink != 0:
		return "a link"
	case m.IsRegular():
		return "a file"
	default:
		return "a special file"
	}
}
