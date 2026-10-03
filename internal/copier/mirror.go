package copier

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// MirrorPlan is what one mirror pass deletes from a target (ADRs 12, 18): the revisions
// local has pruned, under rails that keep a bug or a broken local from emptying a target.
type MirrorPlan struct {
	Delete  map[string][]int  // snapshot ID -> revisions to delete, ascending
	Refused map[string]string // snapshot ID -> why its deletions were refused
}

// Deletions is the number of revisions the plan deletes.
func (p MirrorPlan) Deletions() int {
	n := 0
	for _, revs := range p.Delete {
		n += len(revs)
	}
	return n
}

// PlanMirror works out a mirror pass from both storages' revision lists ("id:revision",
// as Runner.Revisions returns them). Only IDs own accepts (this deployment's own snapshot
// IDs, exactly; a hostname prefix would also match another host's "<prefix>-dr-...") are
// touched. For each, it deletes the target's revisions that local no longer
// has, except:
//   - nothing when local has no revision of the ID at all (local may be broken or empty);
//   - never the ID's newest revision on the target;
//   - never a revision newer than local's newest (local cannot have pruned it);
//   - nothing when that would delete more than half the ID's target revisions, unless
//     allowLarge (a shorter retention policy does this once, on purpose).
func PlanMirror(local, target map[string]bool, own func(id string) bool, allowLarge bool) MirrorPlan {
	plan := MirrorPlan{Delete: map[string][]int{}, Refused: map[string]string{}}
	lrevs, trevs := byID(local), byID(target)
	for id, tr := range trevs {
		if !own(id) {
			continue
		}
		lr := lrevs[id]
		if len(lr) == 0 {
			continue
		}
		have := map[int]bool{}
		for _, r := range lr {
			have[r] = true
		}
		localNewest, targetNewest := lr[len(lr)-1], tr[len(tr)-1]
		var del []int
		for _, r := range tr {
			if !have[r] && r != targetNewest && r < localNewest {
				del = append(del, r)
			}
		}
		if len(del) == 0 {
			continue
		}
		if 2*len(del) > len(tr) && !allowLarge {
			plan.Refused[id] = fmt.Sprintf("would delete %d of %d revisions", len(del), len(tr))
			continue
		}
		plan.Delete[id] = del
	}
	return plan
}

// byID groups "id:revision" entries into sorted revisions per ID.
func byID(revs map[string]bool) map[string][]int {
	out := map[string][]int{}
	for k := range revs {
		i := strings.LastIndexByte(k, ':')
		if i < 0 {
			continue
		}
		n, err := strconv.Atoi(k[i+1:])
		if err != nil {
			continue
		}
		out[k[:i]] = append(out[k[:i]], n)
	}
	for id := range out {
		sort.Ints(out[id])
	}
	return out
}
