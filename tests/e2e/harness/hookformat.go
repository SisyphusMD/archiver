package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The hook surface is incidental (ADR 1): 0.11 sources a service-backup-settings.sh
// defining two functions and a filter array; images labelled io.archiver.hooks=executable
// run executable pre-backup and post-backup files and read a plain filters file (ADR 20).
// Tests state what a hook does; this renders it in the form the image runs.

var (
	formatMu sync.Mutex
	formats  = map[string]bool{}
)

// ExecutableHooks reports whether image runs executable hooks.
func ExecutableHooks(t testing.TB, image string) bool {
	t.Helper()
	formatMu.Lock()
	defer formatMu.Unlock()
	if v, ok := formats[image]; ok {
		return v
	}
	r := docker(t, "image", "inspect", "-f", `{{index .Config.Labels "io.archiver.hooks"}}`, image)
	if r.Code != 0 {
		t.Fatalf("inspect %s: %s", image, r.Output())
	}
	v := strings.TrimSpace(r.Stdout) == "executable"
	formats[image] = v
	return v
}

// serviceFiles is what a service carries: hook bodies (shell, run as a function so return
// and exit both work) and filter patterns. A nil field is left out.
type serviceFiles struct {
	pre, post *string
	filters   []string
}

// writeServiceFiles replaces dir's hooks and filters with files, in image's form.
func writeServiceFiles(t testing.TB, image, dir string, f serviceFiles) {
	t.Helper()
	if ExecutableHooks(t, image) {
		for _, h := range []struct {
			name string
			body *string
		}{{"pre-backup", f.pre}, {"post-backup", f.post}} {
			path := filepath.Join(dir, h.name)
			if h.body == nil {
				os.Remove(path)
				continue
			}
			src := "#!/bin/bash\nhook() {\n" + *h.body + "\n}\nhook\n"
			if err := os.WriteFile(path, []byte(src), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		path := filepath.Join(dir, "filters")
		if f.filters == nil {
			os.Remove(path)
			return
		}
		if err := os.WriteFile(path, []byte(strings.Join(f.filters, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	var b strings.Builder
	if f.filters != nil {
		b.WriteString("DUPLICACY_FILTERS_PATTERNS=(\n")
		for _, p := range f.filters {
			if strings.Contains(p, "'") {
				t.Fatalf("filter pattern %q: single quotes are not supported", p)
			}
			b.WriteString("  '" + p + "'\n")
		}
		b.WriteString(")\n")
	}
	for _, h := range []struct {
		phase string
		body  *string
	}{{"pre", f.pre}, {"post", f.post}} {
		if h.body != nil {
			fmt.Fprintf(&b, "service_specific_%s_backup_function() {\n%s\n}\n", h.phase, *h.body)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "service-backup-settings.sh"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// filterFiles are the files a service's filters live in, which a filter test excludes so
// the backed-up set is decided by its patterns alone.
func filterFiles(t testing.TB, image string) []string {
	if ExecutableHooks(t, image) {
		return []string{"filters"}
	}
	return []string{"service-backup-settings.sh"}
}
