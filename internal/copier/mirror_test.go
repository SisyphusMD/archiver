package copier

import (
	"fmt"
	"slices"
	"testing"
)

func revisions(id string, revs ...int) map[string]bool {
	m := map[string]bool{}
	for _, r := range revs {
		m[fmt.Sprintf("%s:%d", id, r)] = true
	}
	return m
}

func merge(ms ...map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, m := range ms {
		for k := range m {
			out[k] = true
		}
	}
	return out
}

func TestPlanMirror(t *testing.T) {
	for _, tc := range []struct {
		name          string
		local, target map[string]bool
		allowLarge    bool
		want          map[string][]int
		refused       []string
	}{
		{"deletes what local pruned",
			revisions("nas-app", 5, 6, 7, 8, 9, 10), revisions("nas-app", 1, 2, 5, 6, 7, 8, 9, 10),
			false, map[string][]int{"nas-app": {1, 2}}, nil},
		{"never the target's newest revision",
			revisions("nas-app", 3, 4), revisions("nas-app", 2, 3, 4, 9),
			false, map[string][]int{"nas-app": {2}}, nil},
		{"never newer than local's newest",
			revisions("nas-app", 1, 2, 3, 4), revisions("nas-app", 1, 2, 3, 4, 5, 6),
			false, map[string][]int{}, nil},
		{"an ID local lacks entirely is left alone",
			revisions("nas-other", 1), revisions("nas-app", 1, 2, 3),
			false, map[string][]int{}, nil},
		{"a host whose name extends ours is not ours",
			revisions("nas-dr-app", 2, 3), revisions("nas-dr-app", 1, 2, 3),
			false, map[string][]int{}, nil},
		{"another deployment's IDs are left alone",
			revisions("vps-app", 9), revisions("vps-app", 1, 2, 9),
			false, map[string][]int{}, nil},
		{"more than half is refused",
			revisions("nas-app", 8, 9), revisions("nas-app", 1, 2, 3, 8, 9),
			false, map[string][]int{}, []string{"nas-app"}},
		{"unless explicitly allowed",
			revisions("nas-app", 8, 9), revisions("nas-app", 1, 2, 3, 8, 9),
			true, map[string][]int{"nas-app": {1, 2, 3}}, nil},
		{"exactly half is allowed",
			revisions("nas-app", 3, 4), revisions("nas-app", 1, 2, 3, 4),
			false, map[string][]int{"nas-app": {1, 2}}, nil},
		{"IDs are planned independently",
			merge(revisions("nas-a", 2, 3, 4), revisions("nas-b", 9, 10)),
			merge(revisions("nas-a", 1, 2, 3, 4), revisions("nas-b", 1, 2, 3, 9, 10)),
			false, map[string][]int{"nas-a": {1}}, []string{"nas-b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			own := func(id string) bool { return id == "nas-app" || id == "nas-a" || id == "nas-b" || id == "nas-other" }
			p := PlanMirror(tc.local, tc.target, own, tc.allowLarge)
			if fmt.Sprint(p.Delete) != fmt.Sprint(tc.want) {
				t.Errorf("delete %v, want %v", p.Delete, tc.want)
			}
			var refused []string
			for id := range p.Refused {
				refused = append(refused, id)
			}
			slices.Sort(refused)
			if !slices.Equal(refused, tc.refused) {
				t.Errorf("refused %v, want %v", p.Refused, tc.refused)
			}
		})
	}
}
