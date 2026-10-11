package layout

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// Owned reports whether fi, from Lstat, is a real directory of this process's user that
// nobody else can write.
func Owned(fi fs.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return fi.IsDir() && fi.Mode().Perm()&0o022 == 0 && ok && int(st.Uid) == os.Geteuid()
}

// Private creates dir owner-only when it is missing, then requires it to be Owned: a link,
// or a directory someone else made or can write, is refused rather than used.
func Private(dir string) error {
	if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
		return err
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !Owned(fi) {
		return fmt.Errorf("%s is not a directory only Archiver can write (a link, another owner, or group/other write)", dir)
	}
	return nil
}
