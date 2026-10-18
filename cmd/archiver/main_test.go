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

func TestFlagGuard(t *testing.T) {
	for _, c := range []struct {
		args    []string
		code    int
		handled bool
	}{
		{[]string{"drill", "--help"}, 0, true},
		{[]string{"backup", "-h"}, 0, true},
		{[]string{"drill", "--named", "--help"}, 0, true},
		{[]string{"drill", "--verbose"}, 2, true},
		{[]string{"drill", "garage", "-x"}, 2, true},
		{[]string{"kit-drill", "--all"}, 2, true},
		{[]string{"deep-check", "-f"}, 2, true},
		{[]string{"drill", "--named", "GARAGE"}, 0, false},
		{[]string{"drill", "garage", "backblaze"}, 0, false},
		{[]string{"backup", "--detach"}, 0, false},
		{[]string{"drill"}, 0, false},
		{[]string{"help"}, 0, false},
	} {
		code, handled := flagGuard(c.args)
		if code != c.code || handled != c.handled {
			t.Errorf("flagGuard(%q) = %d, %v, want %d, %v", c.args, code, handled, c.code, c.handled)
		}
	}
}
