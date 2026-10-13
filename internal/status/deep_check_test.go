package status

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/runlock"
)

func TestDeepCheckStatus(t *testing.T) {
	root := t.TempDir()
	l := layout.Layout{Root: root, Lock: filepath.Join(root, "locks")}
	dir := filepath.Join(l.LogDir(), ".deep-check-state")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(l.Lock, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "local.json"), []byte(`{"at":100,"ok":true,"seconds":2,"last_success":102}`), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(110, 0)
	get := func(string) string { return "" }
	var out bytes.Buffer
	if err := Write(&out, l, get, now); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Deep check of local: passed") {
		t.Fatal(out.String())
	}
	if r := Take(l, get, now).DeepChecks["local"]; r.At != 100 || !r.OK || r.LastSuccess != 102 {
		t.Fatalf("bad JSON status %+v", r)
	}
	lock, _, err := runlock.Acquire(l.DeepCheckLock("local"), l.DeepCheckStopFlag("local"), "deep-check", "local")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	out.Reset()
	if err := Write(&out, l, get, now); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Deep check of local: running") {
		t.Fatal(out.String())
	}
}
