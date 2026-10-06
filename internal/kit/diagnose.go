package kit

import (
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
