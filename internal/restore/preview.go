package restore

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Paths reads RESTORE_PATHS: paths inside the snapshot, separated by commas or newlines; a
// directory restores everything under it.
func Paths(v string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '\n' }) {
		if p = strings.Trim(strings.TrimSpace(p), "/"); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// patterns are duplicacy include patterns for paths, as anchored regular expressions so a
// '?' or '*' in a name is literal: a path matches itself, and everything under it when it is
// a directory. Duplicacy creates the parent directories.
func patterns(paths []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range paths {
		// Each ancestor directory itself, not what else it holds: duplicacy restores a link
		// before the files and does not create the link's parents.
		for a := filepath.Dir(p); a != "." && a != "/"; a = filepath.Dir(a) {
			if pat := "i:^" + regexp.QuoteMeta(a+"/") + "$"; !seen[pat] {
				seen[pat] = true
				out = append(out, pat)
			}
		}
		out = append(out, "i:^"+regexp.QuoteMeta(p)+"(/.*)?$")
	}
	return out
}

// selected reports whether path is one of paths or under one; with none, every path is.
func selected(path string, paths []string) bool {
	if len(paths) == 0 {
		return true
	}
	for _, p := range paths {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// entry is one file of a revision's listing.
type entry struct {
	path string
	size int64
	time string // "2006-01-02 15:04:05", as duplicacy prints it
}

// listingLine is a file line of `duplicacy list -files`: a right-aligned size, the
// modification time, a 64-character hash (spaces for an empty file), one space, the path
// exactly as stored (a leading space belongs to it).
var listingLine = regexp.MustCompile(`^ *(\d+) (\d{4}-\d\d-\d\d \d\d:\d\d:\d\d) (?:[0-9a-f]{64}| {64}) (.*)$`)

func parseListing(listing string) []entry {
	var out []entry
	for _, line := range strings.Split(listing, "\n") {
		m := listingLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		size, _ := strconv.ParseInt(m[1], 10, 64)
		out = append(out, entry{path: m[3], size: size, time: m[2]})
	}
	return out
}

// Plan is what a restore would do to its destination, judged by size and modification
// time (a hash comparison happens only in the restore itself). Conflict is a file whose size
// or time differs while overwrite is off: duplicacy hashes it, skips it when the content is
// the same, and stops the restore at the first whose content differs.
type Plan struct {
	Add, Replace, Conflict, Same, Delete []entry
}

// sameTime is duplicacy's test: modification times within a second count as the same.
func sameTime(local time.Time, listed string) bool {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", listed, time.Local)
	if err != nil {
		return false
	}
	d := local.Truncate(time.Second).Sub(t)
	return d >= -time.Second && d <= time.Second
}

func (p Plan) bytes(list []entry) int64 {
	var n int64
	for _, e := range list {
		n += e.size
	}
	return n
}

// plan compares a revision's files with what dest holds now; Delete lists what DELETE_EXTRA
// would remove.
func plan(files []entry, dest string, o Options) Plan {
	var p Plan
	in := map[string]bool{}
	for _, f := range files {
		if !selected(f.path, o.Paths) {
			continue
		}
		in[f.path] = true
		fi, err := os.Lstat(filepath.Join(dest, filepath.FromSlash(f.path)))
		switch {
		case err != nil:
			p.Add = append(p.Add, f)
		case fi.Mode().IsRegular() && fi.Size() == f.size && sameTime(fi.ModTime(), f.time):
			p.Same = append(p.Same, f)
		// Duplicacy writes an empty file by truncating, before it checks for overwrite.
		case o.Overwrite || f.size == 0:
			p.Replace = append(p.Replace, f)
		default:
			p.Conflict = append(p.Conflict, f)
		}
	}
	// Duplicacy deletes extra files only in a full restore, never with restore paths.
	if o.Delete && len(o.Paths) == 0 {
		// A destination that is itself a link is walked where it leads, as duplicacy does.
		root := dest
		if r, err := filepath.EvalSymlinks(dest); err == nil {
			root = r
		}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if d.IsDir() {
				// Duplicacy leaves every directory named .duplicacy alone, at any depth.
				if d.Name() == ".duplicacy" {
					return filepath.SkipDir
				}
				return nil
			}
			// The listing names regular files only, so a link is left out rather than guessed
			// at: one the revision holds would otherwise show as deleted.
			if d.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			if !in[rel] {
				fi, _ := d.Info()
				var size int64
				if fi != nil {
					size = fi.Size()
				}
				p.Delete = append(p.Delete, entry{path: rel, size: size})
			}
			return nil
		})
	}
	return p
}

// show prints a plan: counts and sizes, and the first paths of each change.
func (p Plan) show(out io.Writer, dest string) {
	fmt.Fprintf(out, "\nPreview of the restore into %s (regular files, by size and modification time):\n", dest)
	for _, g := range []struct {
		what string
		list []entry
	}{{"added", p.Add}, {"replaced", p.Replace}, {"deleted", p.Delete}, {"differ in size or time while overwrite is off", p.Conflict}} {
		if len(g.list) == 0 {
			continue
		}
		fmt.Fprintf(out, "  %d file(s) %s, %s:\n", len(g.list), g.what, mb(p.bytes(g.list)))
		sorted := append([]entry(nil), g.list...)
		sort.Slice(sorted, func(a, b int) bool { return sorted[a].path < sorted[b].path })
		for i, e := range sorted {
			if i == 10 {
				fmt.Fprintf(out, "      ... and %d more\n", len(sorted)-10)
				break
			}
			fmt.Fprintf(out, "      %s\n", e.path)
		}
	}
	fmt.Fprintf(out, "  %d file(s) already match.\n", len(p.Same))
	if len(p.Conflict) > 0 {
		fmt.Fprintln(out, "  Of those, any whose content also differs STOPS the restore part-way (one with the same content is skipped): turn on overwrite (OVERWRITE=1), or restore into another directory.")
	}
	if len(p.Add)+len(p.Replace)+len(p.Conflict)+len(p.Same) == 0 {
		fmt.Fprintln(out, "  Nothing in the revision matches RESTORE_PATHS.")
	}
}

func mb(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%d KB", (n+1023)/1024)
	}
}

// listFiles reads a revision's file listing from a repository connected to its storage.
func (e *Env) listFiles(repo, id string, rev int) ([]entry, error) {
	var out strings.Builder
	if err := e.duplicacy(repo, &out, "list", "-files", "-r", strconv.Itoa(rev), "-id", id).Run(); err != nil {
		return nil, fmt.Errorf("cannot list revision %d of %s: %s", rev, id, lastLine(out.String()))
	}
	return parseListing(out.String()), nil
}
