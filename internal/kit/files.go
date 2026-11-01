package kit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/layout"
)

// ListDir lists the names directly under path, relative to t's root; a missing directory
// lists nothing. It reaches every storage type the way the kit does (ADR 24).
func ListDir(ctx context.Context, l layout.Layout, rclone string, t config.Target, path string) ([]string, error) {
	r := &Run{Layout: l, Rclone: rclone}
	env, dir, err := r.remoteEnv(ctx, t)
	if err != nil {
		return nil, err
	}
	c := exec.CommandContext(ctx, r.rclone(), "--config", os.DevNull, "lsf", "--contimeout", "15s", "--timeout", "5m",
		"--retries", "3", "KIT:"+dir+"/"+path)
	c.Env = env
	out, err := c.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "not found") {
			return nil, nil
		}
		return nil, fmt.Errorf("listing %s on '%s': %s", path, t.Name, lastLine(string(out)))
	}
	var names []string
	for _, n := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if n != "" {
			names = append(names, n)
		}
	}
	return names, nil
}

// RemoveDir deletes path and everything under it, relative to t's root. On B2 the files'
// earlier versions go too, as Duplicacy's own deletions do.
func RemoveDir(ctx context.Context, l layout.Layout, rclone string, t config.Target, path string) error {
	r := &Run{Layout: l, Rclone: rclone}
	env, dir, err := r.remoteEnv(ctx, t)
	if err != nil {
		return err
	}
	if slices.Contains(env, "RCLONE_CONFIG_KIT_TYPE=b2") {
		env = append(env, "RCLONE_CONFIG_KIT_HARD_DELETE=true")
	}
	c := exec.CommandContext(ctx, r.rclone(), "--config", os.DevNull, "purge", "--contimeout", "15s", "--timeout", "5m",
		"--retries", "3", "KIT:"+dir+"/"+path)
	c.Env = env
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("removing %s on '%s': %s", path, t.Name, lastLine(string(out)))
	}
	return nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		return s[i+1:]
	}
	return s
}
