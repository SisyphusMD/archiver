package kit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/layout"
)

// A missing directory explains a failure only where Duplicacy does not create one.
func TestDiagnoseBlamesAMissingDirectoryOnlyWhereItMustExist(t *testing.T) {
	root := t.TempDir()
	rclone := filepath.Join(root, "rclone")
	os.WriteFile(rclone, []byte(`#!/bin/sh
[ "$1" = obscure ] && { echo obscured; exit 0; }
echo "ERROR : error listing: directory not found" >&2
exit 3
`), 0o755)
	l := layout.Layout{Root: root, Lock: filepath.Join(root, "lock")}
	smb := config.Target{N: 1, Name: "nas", Type: "smb",
		Values: config.Values{"SMB_HOST": "h", "SMB_USER": "u", "SMB_SHARE": "s", "SMB_PATH": "p", "SMB_PASSWORD": "pw"}}
	if why := Diagnose(l, rclone, smb); !strings.Contains(why, "Create it first") {
		t.Errorf("smb: %q", why)
	}
	fabric := config.Target{N: 1, Name: "ff", Type: "fabric", Values: config.Values{"FABRIC_ENDPOINT": "f", "FABRIC_PATH": "p", "FABRIC_TOKEN": "t"}}
	if why := Diagnose(l, rclone, fabric); !strings.Contains(why, "Create it first") {
		t.Errorf("fabric: %q", why)
	}
	gcd := config.Target{N: 1, Name: "drive", Type: "gcd", Values: config.Values{"GCD_PATH": "p", "GCD_TOKEN": "{}"}}
	if why := Diagnose(l, rclone, gcd); why != "" {
		t.Errorf("gcd: %q", why)
	}
}
