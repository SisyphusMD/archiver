package health

import (
	"syscall"
	"unsafe"
)

type syscallTimeval struct{ Sec, Usec int64 }

const (
	atFDCWD           = -100
	atSymlinkNoFollow = 0x100
)

// lutimes sets a symlink's own timestamps, which os.Chtimes cannot (it follows the link).
func lutimes(path string, tv []syscallTimeval) error {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return err
	}
	ts := [2]syscall.Timespec{
		{Sec: tv[0].Sec, Nsec: tv[0].Usec * 1000},
		{Sec: tv[1].Sec, Nsec: tv[1].Usec * 1000},
	}
	fd := atFDCWD
	_, _, errno := syscall.Syscall6(syscall.SYS_UTIMENSAT, uintptr(fd),
		uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&ts[0])), atSymlinkNoFollow, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
