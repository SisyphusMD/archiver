package setup

import (
	"os"
	"os/signal"
	"syscall"
	"unsafe"
)

// HideTerminal is a Hide for f when it is a terminal, else nil. An interrupt while input is
// hidden restores the echo before the process exits, or the user's shell is left without it.
func HideTerminal(f *os.File) func() (restore func()) {
	var t syscall.Termios
	if ioctl(f.Fd(), syscall.TCGETS, &t) != nil {
		return nil
	}
	return func() func() {
		saved, noEcho := t, t
		noEcho.Lflag &^= syscall.ECHO
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		done := make(chan struct{})
		go func() {
			select {
			case s := <-sigs:
				_ = ioctl(f.Fd(), syscall.TCSETS, &saved)
				os.Stdout.WriteString("\n")
				os.Exit(128 + int(s.(syscall.Signal)))
			case <-done:
			}
		}()
		_ = ioctl(f.Fd(), syscall.TCSETS, &noEcho)
		return func() {
			signal.Stop(sigs)
			close(done)
			_ = ioctl(f.Fd(), syscall.TCSETS, &saved)
		}
	}
}

func ioctl(fd uintptr, req uintptr, t *syscall.Termios) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(unsafe.Pointer(t))); errno != 0 {
		return errno
	}
	return nil
}
