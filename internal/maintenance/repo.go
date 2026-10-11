package maintenance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/SisyphusMD/archiver/internal/atomicfile"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/proc"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

// RepoDir is maintenance's own repository, in the logs volume beside the copy workers'.
// Duplicacy keeps a prune's pending fossil collections in the cache of the repository that
// pruned; in a service's repository they were lost to whichever service sorted first, and
// their chunks waited for the next exhaustive prune.
func RepoDir(logDir string) string { return filepath.Join(logDir, ".maintenance-repo") }

// identities records, beside the repository, which URL each storage's cache was made for.
const identities = "storages"

// prepare registers the primary and takes over the pending fossil collections service
// repositories hold for it; addSecondary registers each secondary when its turn comes, so
// one that hangs cannot hold up the primary's upkeep. Only the registration is redone each
// run: the cache must outlive it, except a storage's cache when its name now points
// elsewhere, since duplicacy keys the cache by name and trusts what it finds there.
func (r *Run) prepare(ctx context.Context) error {
	dot := filepath.Join(r.repo, ".duplicacy")
	if err := os.MkdirAll(dot, 0o700); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dot, "preferences")); err != nil && !os.IsNotExist(err) {
		return err
	}
	r.urls = map[string]string{}
	for _, t := range r.cfg.Targets {
		url, err := t.URL()
		if err != nil {
			return err
		}
		r.urls[t.StorageName()] = url
	}
	// The identities are written before any cache is, so a cache without a matching line
	// belongs to a storage removed or repointed since.
	known, err := readIdentities(filepath.Join(r.repo, identities))
	if err != nil {
		return err
	}
	caches, _ := os.ReadDir(filepath.Join(dot, "cache"))
	for _, c := range caches {
		if url, ok := r.urls[c.Name()]; !ok || known[c.Name()] != url {
			if err := os.RemoveAll(filepath.Join(dot, "cache", c.Name())); err != nil {
				return err
			}
		}
	}
	if err := writeIdentities(filepath.Join(r.repo, identities), r.urls); err != nil {
		return err
	}
	name := r.cfg.Targets[0].StorageName()
	if err := r.register(ctx, name, "init", "-e", "-key", r.pubKey(), "-storage-name", name, r.registrationID(), r.urls[name]); err != nil {
		return err
	}
	return r.adopt(name)
}

// addSecondary registers target i as a copy of the primary with the arguments every
// secondary was made with (bit-identical, RSA), so one that does not exist yet is created
// as a backup would create it.
func (r *Run) addSecondary(ctx context.Context, i int) error {
	name, primary := r.cfg.Targets[i].StorageName(), r.cfg.Targets[0].StorageName()
	if err := r.register(ctx, name, "add", "-e", "-copy", primary, "-bit-identical", "-key", r.pubKey(), name, r.registrationID(), r.urls[name]); err != nil {
		return err
	}
	return r.adopt(name)
}

func (r *Run) pubKey() string { return filepath.Join(r.Layout.Root, "keys", "public.pem") }

// registrationID is the snapshot ID init and add require; nothing is backed up under it.
func (r *Run) registrationID() string { return r.Hostname + "-archiver-maintenance" }

func (r *Run) adopt(name string) error {
	moved, err := adoptFossils(filepath.Join(r.repo, ".duplicacy"), name, r.urls[name], r.dirs)
	if moved > 0 {
		r.log.Message(logging.Info, name, fmt.Sprintf("Took over %d pending fossil collection(s) for %s from the service repositories.", moved, name))
	}
	if err != nil {
		return fmt.Errorf("taking over the fossil collections of %s: %v", name, err)
	}
	return nil
}

// register runs one init or add under the storage's creation lock, so it never creates a
// storage at the same time as a backup or a copy worker.
func (r *Run) register(ctx context.Context, storage string, args ...string) error {
	release, err := runlock.Exclusive(ctx, r.Layout.StorageInit(storage))
	if err != nil {
		return err
	}
	defer release()
	cmd := exec.CommandContext(ctx, r.Duplicacy, proc.NoScript(args...)...)
	cmd.Dir, cmd.Env = r.repo, r.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		return fmt.Errorf("registering %s: %v: %s", storage, err, lines[len(lines)-1])
	}
	return nil
}

// adoptFossils moves the fossil collections service repositories keep for storage into the
// repository dot belongs to, renumbered after its own. A service repository whose
// preferences point that name at another URL holds another storage's collections, and is
// left alone. Each collection moves rather than copies, so no prune processes one twice.
//
// Only repositories older versions kept inside service directories hold any. Whoever owns a
// service directory controls what is there, and the collections feed a root prune of the
// primary, so they are taken only through directories that are Archiver's own from
// .duplicacy down, never through a link, and only from files Archiver alone owns.
func adoptFossils(dot, storage, url string, serviceDirs []string) (int, error) {
	dst := filepath.Join(dot, "cache", storage, "fossils")
	next := maxCollection(dst) + 1
	moved := 0
	for _, d := range serviceDirs {
		n, err := adoptFrom(d, storage, url, dst, &next)
		moved += n
		if err != nil {
			return moved, err
		}
	}
	return moved, nil
}

// adoptFrom moves service directory d's collections for storage into dst, numbered from
// *next. Nothing is an error short of a failure to write dst or to remove a collection
// taken: a repository that is missing, foreign or not for this storage is skipped.
func adoptFrom(d, storage, url, dst string, next *int) (int, error) {
	dirs, err := openOwned(d, ".duplicacy")
	defer closeAll(dirs) // a refused directory is returned open too
	if err != nil {
		return 0, nil
	}
	prefs, err := readOwned(dirs[0], "preferences")
	if err != nil || !sameStorage(prefs, storage, url) {
		return 0, nil
	}
	more, err := openOwnedAt(dirs[0], "cache", storage, "fossils")
	defer closeAll(more)
	if err != nil {
		return 0, nil
	}
	src := more[len(more)-1]
	entries, err := src.ReadDir(-1)
	if err != nil {
		return 0, nil
	}
	var nums []int
	for _, e := range entries {
		if n, err := strconv.Atoi(e.Name()); err == nil && e.Type().IsRegular() {
			nums = append(nums, n)
		}
	}
	sort.Ints(nums)
	moved := 0
	for _, n := range nums {
		b, err := readOwned(src, strconv.Itoa(n))
		if err != nil {
			continue
		}
		if moved == 0 {
			if err := os.MkdirAll(dst, 0o700); err != nil {
				return moved, err
			}
		}
		if err := atomicfile.Write(filepath.Join(dst, strconv.Itoa(*next)), b, 0o600); err != nil {
			return moved, err
		}
		if err := syscall.Unlinkat(int(src.Fd()), strconv.Itoa(n)); err != nil {
			return moved, err
		}
		*next++
		moved++
	}
	return moved, nil
}

// openOwned opens dir (following links: it is configured) and then each name below it in
// turn, each without following a link and each required to be layout.Owned. The opened
// directories are returned, below dir, in order.
func openOwned(dir string, names ...string) ([]*os.File, error) {
	top, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer top.Close()
	return openOwnedAt(top, names...)
}

func openOwnedAt(parent *os.File, names ...string) ([]*os.File, error) {
	var opened []*os.File
	for _, name := range names {
		fd, err := syscall.Openat(int(parent.Fd()), name, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if err != nil {
			return opened, err
		}
		f := os.NewFile(uintptr(fd), name)
		opened = append(opened, f)
		fi, err := f.Stat()
		if err != nil {
			return opened, err
		}
		if !layout.Owned(fi) {
			return opened, fmt.Errorf("%s is not Archiver's own", name)
		}
		parent = f
	}
	return opened, nil
}

// readOwned reads the regular file name in dir without following a link, refusing one
// another user owns or that has other names (a hard link to a file elsewhere).
func readOwned(dir *os.File, name string) ([]byte, error) {
	fd, err := syscall.Openat(int(dir.Fd()), name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !fi.Mode().IsRegular() || !ok || int(st.Uid) != os.Geteuid() || st.Nlink != 1 {
		return nil, fmt.Errorf("%s is not a file of Archiver's own", name)
	}
	return io.ReadAll(f)
}

func closeAll(files []*os.File) {
	for _, f := range files {
		f.Close()
	}
}

// maxCollection is the highest collection number in dir, 0 when it has none.
func maxCollection(dir string) int {
	entries, _ := os.ReadDir(dir)
	highest := 0
	for _, e := range entries {
		if n, err := strconv.Atoi(e.Name()); err == nil && n > highest {
			highest = n
		}
	}
	return highest
}

// sameStorage reports whether the preferences register storage at url.
func sameStorage(prefs []byte, storage, url string) bool {
	var entries []struct {
		Name    string `json:"name"`
		Storage string `json:"storage"`
	}
	if json.Unmarshal(prefs, &entries) != nil {
		return false
	}
	for _, e := range entries {
		if e.Name == storage && e.Storage == url {
			return true
		}
	}
	return false
}

// readIdentities reads the identities file; a missing one (a new repository) is empty, but
// an unreadable one is an error rather than a reason to drop every cache.
func readIdentities(path string) (map[string]string, error) {
	m := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if name, url, ok := strings.Cut(line, " "); ok {
			m[name] = url
		}
	}
	return m, nil
}

// writeIdentities replaces the file whole: a torn write would read as storages repointed,
// and drop their caches with the collections in them.
func writeIdentities(path string, urls map[string]string) error {
	names := make([]string, 0, len(urls))
	for n := range urls {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "%s %s\n", n, urls[n])
	}
	return atomicfile.Write(path, []byte(b.String()), 0o600)
}
