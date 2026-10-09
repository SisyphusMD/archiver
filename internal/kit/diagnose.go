package kit

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/layout"
)

// Diagnose explains why Duplicacy could not open storage t, for its error message: it looks at
// the storage's directory through the same rclone remote the kit uses. Duplicacy itself says
// only that loading failed, and does not create a missing directory on SFTP, WebDAV or SMB.
// It returns "" when it finds nothing wrong.
func Diagnose(l layout.Layout, rclone string, t config.Target) string {
	r := &Run{Layout: l, Rclone: rclone}
	settings, dir, err := r.remote(t)
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
	return statOn(l, rclone, t, "config")
}

// ErrNotFound is a storage reached that does not hold the file asked for.
var ErrNotFound = errors.New("not found")

// HasFile reports whether storage t holds the file name beside its Duplicacy config (a
// recovery kit), read-only.
func HasFile(l layout.Layout, rclone string, t config.Target, name string) error {
	return statOn(l, rclone, t, name)
}

func statOn(l layout.Layout, rclone string, t config.Target, name string) error {
	r := &Run{Layout: l, Rclone: rclone}
	settings, dir, err := r.remote(t)
	if err != nil {
		return err
	}
	env := append(os.Environ(), "RCLONE_CONFIG_KIT_TYPE="+settings["TYPE"])
	for k, v := range settings {
		if k != "TYPE" {
			env = append(env, "RCLONE_CONFIG_KIT_"+k+"="+v)
		}
	}
	stat := exec.Command(r.rclone(), "--config", os.DevNull, "lsjson", "--stat", "--contimeout", "15s", "--timeout", "1m", "--retries", "1",
		"KIT:"+strings.TrimSuffix(dir, "/")+"/"+name)
	stat.Env = env
	if out, err := stat.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = msg[i+1:]
		}
		if strings.Contains(msg, "not found") {
			if name == "config" {
				return fmt.Errorf("no Duplicacy storage at %s (its config is missing)", dir)
			}
			return fmt.Errorf("%s is not at %s: %w", name, dir, ErrNotFound)
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}
