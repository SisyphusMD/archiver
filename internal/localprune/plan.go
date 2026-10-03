// Package localprune prunes the primary storage under its retention policy while leaving out
// revisions a running copy or restore still needs (ADR 19). Duplicacy's own dry run decides
// what the policy deletes; nothing here reimplements retention.
package localprune

import (
	"regexp"
	"sort"
	"strconv"
)

// A dry run logs each revision the policy selects; snapshot IDs contain no spaces.
var deleting = regexp.MustCompile(`Deleting snapshot (\S+) at revision (\d+)\s*$`)

// Planned reads what a `prune -dry-run` would delete, by snapshot ID.
func Planned(dryRun []string) map[string][]int {
	out := map[string][]int{}
	seen := map[string]bool{}
	for _, line := range dryRun {
		m := deleting.FindStringSubmatch(line)
		if m == nil || seen[m[1]+":"+m[2]] {
			continue
		}
		seen[m[1]+":"+m[2]] = true
		rev, _ := strconv.Atoi(m[2])
		out[m[1]] = append(out[m[1]], rev)
	}
	for id := range out {
		sort.Ints(out[id])
	}
	return out
}

// Split separates planned deletions into those to make now and those still in use, which
// wait for the next prune.
func Split(planned map[string][]int, inUse func(id string, rev int) bool) (now, held map[string][]int) {
	now, held = map[string][]int{}, map[string][]int{}
	for id, revs := range planned {
		for _, r := range revs {
			if inUse(id, r) {
				held[id] = append(held[id], r)
			} else {
				now[id] = append(now[id], r)
			}
		}
	}
	return now, held
}
