package main

import "testing"

func TestUsageExitCodes(t *testing.T) {
	for _, c := range []struct {
		args []string
		want int
	}{
		{nil, 1},
		{[]string{"help"}, 0},
		{[]string{"nope"}, 1},
		{[]string{"start"}, 1},
		{[]string{"bundle", "export"}, 2},
		{[]string{"migrate"}, 2},
		{[]string{"backup", "prune"}, 1},
		{[]string{"stop", "backup", "all"}, 1},
		{[]string{"status", "x"}, 1},
	} {
		if got := usage(c.args); got != c.want {
			t.Errorf("usage(%q) = %d, want %d", c.args, got, c.want)
		}
	}
}
