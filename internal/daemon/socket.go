package daemon

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/SisyphusMD/archiver/internal/runlock"
)

// Commands the daemon takes on its socket, one per connection, each answered with a line.
const (
	CmdLocalChanged = "local-changed" // a backup added revisions to the primary
	CmdWorkers      = "workers"       // whether copy workers run (ok or no-workers)
	// Either may be followed by a space and the backup's config.StorageFingerprint: workers
	// keeping other storages answer no-workers.
	CmdStop   = "stop"
	CmdPause  = "pause"
	CmdResume = "resume"
	// CmdMirrorPlan asks what each worker's next mirror pass would delete; CmdAllowLarge
	// lets each worker's next pass exceed the cap on deleting most of an ID (ADR 12).
	CmdMirrorPlan = "mirror-plan"
	CmdAllowLarge = "mirror-allow-large"
	// CmdExhaustive has each worker run an exhaustive prune on its next pass, for
	// `archiver maintenance exhaustive`.
	CmdExhaustive = "exhaustive"
)

// Replies.
const (
	ReplyOK        = "ok"
	ReplyNoWorkers = "no-workers" // the caller must copy for itself
)

// ErrRunning means another daemon already serves the socket.
var ErrRunning = errors.New("another archiver daemon is already running")

// Serve answers commands on a unix socket at path until the listener is closed. Only root
// can connect: the socket is created 0600.
func Serve(path string, handle func(cmd string) string) (net.Listener, error) {
	// Ownership is a kernel lock held for the daemon's life, taken before the socket is
	// touched: a second daemon would take over the first's controls and run its own copy
	// workers alongside the first's. Holding it makes any existing socket stale.
	own, ok, err := runlock.Hold(path + ".flock")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrRunning
	}
	os.Remove(path)
	old := syscallUmask(0o177)
	inner, err := net.Listen("unix", path)
	syscallUmask(old)
	if err != nil {
		own.Close()
		return nil, err
	}
	ln := ownedListener{inner, own}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				c.SetReadDeadline(time.Now().Add(10 * time.Second))
				line, err := bufio.NewReader(c).ReadString('\n')
				if err != nil {
					return
				}
				c.SetWriteDeadline(time.Now().Add(15 * time.Minute))
				fmt.Fprintln(c, handle(strings.TrimSpace(line)))
			}()
		}
	}()
	return ln, nil
}

// Send sends one command to the daemon at path and returns its reply. An error means no
// daemon answered.
func Send(path, cmd string) (string, error) { return SendWithin(path, cmd, 10*time.Second) }

// SendWithin is Send with its own deadline, for a command that lists storages.
func SendWithin(path, cmd string, within time.Duration) (string, error) {
	c, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return "", err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(within))
	if _, err := fmt.Fprintln(c, cmd); err != nil {
		return "", err
	}
	reply, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(reply), nil
}

// ownedListener releases daemon ownership when it is closed.
type ownedListener struct {
	net.Listener
	own *os.File
}

func (l ownedListener) Close() error {
	err := l.Listener.Close()
	l.own.Close()
	return err
}
