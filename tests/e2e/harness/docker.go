// Package harness drives archiver images as black boxes: it starts containers, runs
// commands in them, and inspects what lands on storage and in restore targets. It never
// reaches into archiver internals, so the same tests can judge the bash and Go images.
package harness

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Result is one finished command.
type Result struct {
	Stdout string
	Stderr string
	Code   int
}

// Output is stdout and stderr together, for failure messages.
func (r Result) Output() string { return r.Stdout + r.Stderr }

func docker(t testing.TB, args ...string) Result {
	t.Helper()
	cmd := exec.Command("docker", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("docker %s: %v", strings.Join(args, " "), err)
		}
		code = ee.ExitCode()
	}
	return Result{Stdout: out.String(), Stderr: errb.String(), Code: code}
}

// WorkDir returns a fresh directory that means the same path inside the runner and on the
// Docker host, so it can be bind-mounted into archiver containers. It is removed after
// the test unless the test failed, when it is kept for inspection.
func WorkDir(t testing.TB) string {
	t.Helper()
	root := os.Getenv("ARCHIVER_E2E_WORK")
	if root == "" {
		t.Fatal("ARCHIVER_E2E_WORK is unset; run through tests/e2e/run.sh")
	}
	dir := filepath.Join(root, sanitize(t.Name())+"-"+randomSuffix())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("kept work dir %s", dir)
			return
		}
		os.RemoveAll(dir)
	})
	return dir
}

func randomSuffix() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			return r
		}
		return '-'
	}, s)
}
