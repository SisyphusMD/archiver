// Package damage reads which revisions a duplicacy check found damaged.
package damage

import (
	"regexp"
	"strings"
)

// A revision with missing chunks, or whose own metadata no longer loads (an altered
// chunk), as duplicacy 3.2.5's check -persist reports them.
var line = regexp.MustCompile(`(?m)^(?:Some chunks referenced by snapshot|Failed to load chunks for snapshot) (\S+) at revision (\d+)\b`)

// Revisions lists the damaged revisions in a check's output as "ID revision N", in the
// order reported, each once.
func Revisions(output string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range line.FindAllStringSubmatch(output, -1) {
		r := m[1] + " revision " + m[2]
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}

// Describe is the failure text for a check: the damaged revisions when the output names
// them.
func Describe(output string) string {
	revs := Revisions(output)
	if len(revs) == 0 {
		return ""
	}
	return "damaged revisions: " + strings.Join(revs, ", ")
}
