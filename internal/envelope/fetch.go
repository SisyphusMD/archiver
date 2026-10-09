package envelope

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/SisyphusMD/archiver/internal/config"
)

// Obscure is rclone's reversible encoding of a password, which its remotes expect; tests
// stand in their own.
var Obscure = func(pass string) (string, error) {
	cmd := exec.Command("rclone", "obscure", "-")
	cmd.Stdin = strings.NewReader(pass)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("rclone obscure: %v", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// backends are rclone's names for the remote types whose name differs from its TYPE setting.
var backends = map[string]string{"google cloud storage": "gcs"}

// fetchCommand is a single shell command that downloads the kit from a storage of type typ
// with values v, needing nothing but rclone (an on-the-fly remote, no config file). It is
// empty for types reached otherwise (a local disk, SFTP) or whose credential cannot be
// written down (a token that changes as it is used).
func fetchCommand(typ string, v config.Values, kit string) (string, error) {
	t, ok := config.Types[typ]
	if !ok || t.Remote == nil || typ == "local" || typ == "sftp" || typ == "sftpc" {
		return "", nil
	}
	for _, f := range t.Fields {
		if f.Rotates {
			return "", nil
		}
	}
	settings, dir := t.Remote(v)
	if p, ok := settings["PASS"]; ok {
		obscured, err := Obscure(p)
		if err != nil {
			return "", err
		}
		settings["PASS"] = obscured
	}
	backend := settings["TYPE"]
	if b, ok := backends[backend]; ok {
		backend = b
	}
	delete(settings, "TYPE")
	for k, val := range settings {
		// A credentials file as downloaded spans lines; a printed command must be one line.
		var compact bytes.Buffer
		if strings.HasPrefix(strings.TrimSpace(val), "{") && json.Compact(&compact, []byte(val)) == nil {
			settings[k] = compact.String()
		}
	}
	keys := make([]string, 0, len(settings))
	for k, val := range settings {
		if val != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	// The credentials go in RCLONE_CONFIG_* variables, not in the remote string on the
	// command line, where any user on the machine could read them from the process list.
	var b strings.Builder
	b.WriteString("RCLONE_CONFIG_KIT_TYPE=" + backend)
	for _, k := range keys {
		fmt.Fprintf(&b, " RCLONE_CONFIG_KIT_%s=%s", k, shellQuote(settings[k]))
	}
	b.WriteString(" rclone --config /dev/null copyto " + shellQuote("kit:"+strings.TrimSuffix(dir, "/")+"/"+kit) + " " + kit)
	return b.String(), nil
}

// shellQuote single-quotes s for a POSIX shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
