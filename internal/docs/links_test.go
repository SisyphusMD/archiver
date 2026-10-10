// Package docs holds no code: its test keeps the documentation's links working (ADR 43).
package docs

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

const root = "../.."

var (
	heading = regexp.MustCompile(`^#{1,6} (.*)$`)
	link    = regexp.MustCompile(`\]\(([^)\s]+)\)`)
)

// slug is the anchor GitHub and Forgejo give a heading: lower case, punctuation dropped,
// spaces as hyphens.
func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// lines are a Markdown file's lines outside fenced code.
func lines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	fence := false
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
			continue
		}
		if !fence {
			out = append(out, l)
		}
	}
	return out
}

func anchors(t *testing.T, path string) map[string]bool {
	out, seen := map[string]bool{}, map[string]int{}
	for _, l := range lines(t, path) {
		if m := heading.FindStringSubmatch(l); m != nil {
			s := slug(m[1])
			if n := seen[s]; n > 0 {
				out[s+"-"+string(rune('0'+n))] = true
			} else {
				out[s] = true
			}
			seen[s]++
		}
	}
	return out
}

// Every relative link in README.md and docs/ names a file that exists, and every anchor a
// heading of the file it points into.
func TestLinks(t *testing.T) {
	files := []string{filepath.Join(root, "README.md")}
	filepath.WalkDir(filepath.Join(root, "docs"), func(p string, d os.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(p, ".md") {
			files = append(files, p)
		}
		return nil
	})
	cache := map[string]map[string]bool{}
	for _, f := range files {
		for _, l := range lines(t, f) {
			for _, m := range link.FindAllStringSubmatch(l, -1) {
				target := m[1]
				if regexp.MustCompile(`^[a-z]+:`).MatchString(target) {
					continue
				}
				path, frag, _ := strings.Cut(target, "#")
				dest := f
				if path != "" {
					dest = filepath.Join(filepath.Dir(f), path)
				}
				if _, err := os.Stat(dest); err != nil {
					t.Errorf("%s links to %s, which does not exist", f, target)
					continue
				}
				if frag == "" || !strings.HasSuffix(dest, ".md") {
					continue
				}
				if cache[dest] == nil {
					cache[dest] = anchors(t, dest)
				}
				if !cache[dest][frag] {
					t.Errorf("%s links to %s, which has no heading for #%s", f, target, frag)
				}
			}
		}
	}
}
