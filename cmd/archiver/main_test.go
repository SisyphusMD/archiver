package main

import (
	"slices"
	"testing"
)

func TestRoute(t *testing.T) {
	for _, c := range []struct {
		args       []string
		wantTarget string
		wantArgs   []string
	}{
		{nil, bashCLI, nil},
		{[]string{"backup", "--detach"}, bashCLI, []string{"backup", "--detach"}},
		{[]string{"stop", "maintenance"}, bashCLI, []string{"stop", "maintenance"}},
		{[]string{"init"}, initScript, []string{}},
		// Only a leading init is the init command; anywhere else it is an argument.
		{[]string{"help", "init"}, bashCLI, []string{"help", "init"}},
	} {
		target, args := route(c.args)
		if target != c.wantTarget || !slices.Equal(args, c.wantArgs) {
			t.Errorf("route(%q) = %s %q, want %s %q", c.args, target, args, c.wantTarget, c.wantArgs)
		}
	}
}
