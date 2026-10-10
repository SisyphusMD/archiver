package kit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/layout"
)

// Diagnose explains why Duplicacy could not open storage t, for its error message: it looks at
// the storage's directory through the same rclone remote the kit uses. Duplicacy itself says
// only that loading failed, and does not create a missing directory on SFTP, WebDAV or SMB.
// It returns "" when it finds nothing wrong.
func Diagnose(l layout.Layout, rclone string, t config.Target) string {
	r := &Run{Layout: l, Rclone: rclone}
	settings, dir, err := r.remote(context.Background(), t)
	if err != nil {
		return fmt.Sprintf("Storage '%s' (%s) cannot be reached: %v.", t.Name, t.Type, err)
	}
	env := append(os.Environ(), "RCLONE_CONFIG_KIT_TYPE="+settings["TYPE"])
	for k, v := range settings {
		if k != "TYPE" {
			env = append(env, "RCLONE_CONFIG_KIT_"+k+"="+v)
		}
	}
	stat := exec.Command(r.rclone(), "--config", os.DevNull, "lsjson", "--stat", "--contimeout", "15s", "--timeout", "1m", "--retries", "1",
		"KIT:"+strings.TrimSuffix(dir, "/"))
	stat.Env = env
	out, err := stat.CombinedOutput()
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(string(out))
	if i := strings.LastIndex(msg, "\n"); i >= 0 {
		msg = msg[i+1:]
	}
	if strings.Contains(msg, "not found") {
		return fmt.Sprintf("Storage '%s' (%s): %s does not exist there. Create it first: Duplicacy does not create a storage's directory.", t.Name, t.Type, dir)
	}
	return fmt.Sprintf("Storage '%s' (%s) cannot be reached: %s", t.Name, t.Type, msg)
}

// HasConfig reports whether storage t is reachable with its credentials and holds Duplicacy's
// config, read through the kit's rclone remote: nothing is written, unlike duplicacy init,
// which creates a storage that does not exist.
func HasConfig(l layout.Layout, rclone string, t config.Target) error {
	return statOn(context.Background(), l, rclone, t, "config")
}

// ErrNotFound is a storage reached that does not hold the file asked for.
var ErrNotFound = errors.New("not found")

// ErrNoStorage is a storage reached that holds no Duplicacy config (not created yet), and
// ErrNoRoot one whose directory itself is missing.
var (
	ErrNoStorage = errors.New("no Duplicacy storage")
	ErrNoRoot    = errors.New("the storage's directory does not exist")
)

// Probe reports whether storage t can be reached with its credentials, read-only and in
// seconds (ADR 34). A storage reached that holds no config yet is reachable: the first
// backup or copy creates it, its directory too where Duplicacy does that. Once a storage
// is known to exist (inUse), a missing config or directory is a storage gone, such as an
// unmounted volume's empty mountpoint. A missing directory on a type whose directory
// Duplicacy does not create (mustPreexist) can never become a storage. exists reports
// that the storage's config is there.
func Probe(ctx context.Context, l layout.Layout, t config.Target, inUse bool) (exists bool, err error) {
	err = statOn(ctx, l, "", t, "config")
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, ErrNoRoot) && mustPreexist[t.Type]:
		return false, fmt.Errorf("%w; create it first: Duplicacy does not create a %s storage's directory", err, t.Type)
	case (errors.Is(err, ErrNoStorage) || errors.Is(err, ErrNoRoot)) && !inUse:
		return false, nil
	}
	return false, err
}

var mustPreexist = map[string]bool{"sftp": true, "sftpc": true, "webdav": true, "webdav-http": true, "smb": true}

// HasFile reports whether storage t holds the file name beside its Duplicacy config (a
// recovery kit), read-only.
func HasFile(l layout.Layout, rclone string, t config.Target, name string) error {
	return statOn(context.Background(), l, rclone, t, name)
}

// statDeadline bounds a look at a storage, both requests together.
const statDeadline = 2 * time.Minute

func statOn(ctx context.Context, l layout.Layout, rclone string, t config.Target, name string) error {
	// An overall deadline, setting up the remote included (an SFTP host key scan, OneDrive's
	// token), and one low-level retry: --timeout is only an idle timeout, and a storage
	// answering 503 would otherwise be retried for minutes.
	ctx, cancel := context.WithTimeout(ctx, statDeadline)
	defer cancel()
	r := &Run{Layout: l, Rclone: rclone}
	settings, dir, err := r.remote(ctx, t)
	if err != nil {
		return err
	}
	env := append(os.Environ(), "RCLONE_CONFIG_KIT_TYPE="+settings["TYPE"])
	for k, v := range settings {
		if k != "TYPE" {
			env = append(env, "RCLONE_CONFIG_KIT_"+k+"="+v)
		}
	}
	stat := func(path string) (string, error) {
		c := exec.CommandContext(ctx, r.rclone(), "--config", os.DevNull, "lsjson", "--stat", "--contimeout", "15s", "--timeout", "1m",
			"--retries", "1", "--low-level-retries", "1", "--copy-links", "KIT:"+path) // a local config may be a link
		c.Env = env
		out, err := c.CombinedOutput()
		msg := strings.TrimSpace(string(out))
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = msg[i+1:]
		}
		return msg, err
	}
	root := strings.TrimSuffix(dir, "/")
	msg, err := stat(root + "/" + name)
	if err == nil {
		return nil
	}
	if !strings.Contains(msg, "not found") {
		return fmt.Errorf("%s", msg)
	}
	if name != "config" {
		return fmt.Errorf("%s is not at %s: %w", name, dir, ErrNotFound)
	}
	// rclone says "directory not found" for a missing file on some backends: the root is
	// looked at itself before the storage counts as gone.
	switch rmsg, rerr := stat(root); {
	case rerr == nil:
	case strings.Contains(rmsg, "not found"):
		return fmt.Errorf("%w: %s", ErrNoRoot, dir)
	default:
		return fmt.Errorf("%s", rmsg)
	}
	return fmt.Errorf("%w at %s (its config is missing)", ErrNoStorage, dir)
}
