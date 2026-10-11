// Package hostkeys keeps the SFTP host keys Duplicacy has seen in one record, so a key seen
// once is checked from then on by every repository. Duplicacy trusts an SFTP server's key on
// first sight and records it in the repository's own .duplicacy/known_hosts; a repository
// made for one restore, drill or check starts with none and so trusts whatever answers.
package hostkeys

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/SisyphusMD/archiver/internal/layout"
)

// Seed adds to the shared record the keys pinned in sources, the known_hosts files of
// repositories made before the record existed, so none of their pins is lost to a key first
// seen by another repository. It runs before any job can connect. Only files Archiver owns
// and nobody else can change are read; where two disagree the first is kept, and each
// disagreement is returned.
func Seed(shared string, sources []string) (conflicts []string, err error) {
	f, err := open(shared)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	for _, src := range sources {
		// Links are repositories already pinned to the record.
		if fi, err := os.Lstat(src); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		c, err := merge(f, src)
		if err != nil {
			return conflicts, err
		}
		conflicts = append(conflicts, c...)
	}
	return conflicts, nil
}

// Pin links prefDir's known_hosts to the shared record, first adding what a record already
// there holds for servers the shared one does not know. One that holds a different key for a
// server the record knows is refused and left in place: either could be the impostor.
func Pin(shared, prefDir string) error {
	local := filepath.Join(prefDir, "known_hosts")
	if target, err := os.Readlink(local); err == nil && target == shared {
		return nil
	}
	f, err := open(shared)
	if err != nil {
		return err
	}
	defer f.Close()
	if li, err := os.Lstat(local); err == nil && li.Mode().IsRegular() {
		conflicts, err := merge(f, local)
		if err != nil {
			return err
		}
		if len(conflicts) > 0 {
			return fmt.Errorf("%s; delete the wrong key from one of them", strings.Join(conflicts, "; "))
		}
	}
	if err := os.Remove(local); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(shared, local)
}

// open opens the shared record, creating it and its directory owner-only, and locks it: two
// merges must not both add one host. Duplicacy appends as it meets a new server
// (host-keys-append.patch), as merges do, so neither overwrites the other.
func open(shared string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(filepath.Dir(shared)), 0o755); err != nil { //nolint:gosec // G301: the logs directory holds nothing secret
		return nil, err
	}
	if err := layout.Private(filepath.Dir(shared)); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(shared, os.O_RDWR|os.O_CREATE|os.O_APPEND|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil || !trustedInfo(fi) {
		f.Close()
		return nil, fmt.Errorf("%s is not a file only Archiver can read and write", shared)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// CopyIfMissing creates dst, owner-only, from src when dst does not exist and src is a file
// only Archiver can change.
func CopyIfMissing(src, dst string) error {
	in, err := os.OpenFile(src, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil
	}
	defer in.Close()
	if fi, err := in.Stat(); err != nil || !trustedInfo(fi) {
		return nil
	}
	if err := layout.Private(filepath.Dir(dst)); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
	if os.IsExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(in, 1<<20))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

func trustedInfo(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return fi.Mode().IsRegular() && ok && st.Nlink == 1 && int(st.Uid) == os.Geteuid() && fi.Mode().Perm()&0o022 == 0
}

// merge appends to f the lines of the record at path for hosts f does not have, and names
// each host path holds a different key for. A record someone else could have written adds
// nothing: its pins could be anyone's.
func merge(f *os.File, path string) (conflicts []string, err error) {
	known := map[string]string{}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if host, key, ok := strings.Cut(strings.TrimSpace(sc.Text()), " "); ok {
			if _, seen := known[host]; !seen {
				known[host] = key // Duplicacy uses a host's first line
			}
		}
	}
	src, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	if fi, err := src.Stat(); err != nil || !trustedInfo(fi) {
		return nil, nil
	}
	var add strings.Builder
	sc = bufio.NewScanner(io.LimitReader(src, 1<<20))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		host, key, ok := strings.Cut(line, " ")
		if !ok || key == "" {
			continue
		}
		if have, seen := known[host]; seen {
			if have != key {
				conflicts = append(conflicts, fmt.Sprintf("the SFTP host key for %s in %s differs from the one in %s", host, path, f.Name()))
			}
			continue
		}
		known[host] = key
		add.WriteString(line + "\n")
	}
	if add.Len() > 0 {
		_, err = f.WriteString(add.String())
	}
	return conflicts, err
}
