package localprune

import (
	"fmt"
	"testing"
)

func TestPlanned(t *testing.T) {
	out := []string{
		"Storage set to /backup-store",
		"2026-10-03 05:34:58.015 INFO SNAPSHOT_DELETE Deleting snapshot nas-app at revision 7",
		"Deleting snapshot nas-app at revision 3",
		"Deleting snapshot vps-db at revision 12",
		"Found unreferenced chunk 4f2a",
		"Deleting snapshot nas-app at revision 3", // logged again by an exhaustive pass
		"The snapshot nas-app at revision 9 has been removed",
		"No snapshot to delete",
	}
	if got := fmt.Sprint(Planned(out)); got != "map[nas-app:[3 7] vps-db:[12]]" {
		t.Fatalf("got %s", got)
	}
	if got := Planned([]string{"Storage set to /x", "No snapshot to delete"}); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestSplit(t *testing.T) {
	planned := map[string][]int{"nas-app": {3, 7, 8}, "vps-db": {12}}
	cases := []struct {
		name      string
		inUse     map[string][]int
		now, held string
	}{
		{"nothing in use", nil, "map[nas-app:[3 7 8] vps-db:[12]]", "map[]"},
		{"one revision in use", map[string][]int{"nas-app": {7}}, "map[nas-app:[3 8] vps-db:[12]]", "map[nas-app:[7]]"},
		{"an ID entirely in use", map[string][]int{"vps-db": {12, 13}}, "map[nas-app:[3 7 8]]", "map[vps-db:[12]]"},
		{"in use but not planned changes nothing", map[string][]int{"nas-app": {5}, "other": {1}}, "map[nas-app:[3 7 8] vps-db:[12]]", "map[]"},
	}
	for _, tc := range cases {
		now, held := Split(planned, func(id string, rev int) bool {
			for _, r := range tc.inUse[id] {
				if r == rev {
					return true
				}
			}
			return false
		})
		if fmt.Sprint(now) != tc.now || fmt.Sprint(held) != tc.held {
			t.Errorf("%s: now %v held %v", tc.name, now, held)
		}
	}
}
