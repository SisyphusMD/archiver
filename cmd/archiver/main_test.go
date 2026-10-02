package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/layout"
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

func TestGoPipeline(t *testing.T) {
	root, svcs := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(svcs, "app"), 0o755)
	os.MkdirAll(filepath.Join(svcs, "db"), 0o755)
	l := layout.Layout{Root: root, Lock: t.TempDir()}
	src := func(env map[string]string) config.Source {
		return config.Source{Getenv: func(k string) string { return env[k] }, SecretsDir: t.TempDir()}
	}
	base := map[string]string{"SERVICE_DIRECTORIES": svcs + "/*/"}
	if !goPipeline(l, src(base)) {
		t.Fatal("an env-native deployment with no legacy hooks must run in Go")
	}
	legacy := filepath.Join(svcs, "db", "service-backup-settings.sh")
	os.WriteFile(legacy, nil, 0o644)
	if goPipeline(l, src(base)) {
		t.Fatal("a service with service-backup-settings.sh must stay on bash until migrated")
	}
	os.WriteFile(filepath.Join(svcs, "app", "filters"), []byte("+*\n"), 0o644)
	if !goPipeline(l, src(base)) {
		t.Fatal("a partly migrated deployment must run in Go, which refuses the unmigrated service")
	}
	os.Remove(filepath.Join(svcs, "app", "filters"))
	if !goPipeline(l, src(map[string]string{"SERVICE_DIRECTORIES": svcs + "/*/", "ARCHIVER_PIPELINE": "go"})) {
		t.Fatal("ARCHIVER_PIPELINE=go must override")
	}
	os.Remove(legacy)
	os.WriteFile(l.ConfigFile(), nil, 0o600)
	if goPipeline(l, src(base)) {
		t.Fatal("a bundle deployment (config.sh) must stay on bash")
	}
	os.Remove(l.ConfigFile())
	if goPipeline(l, src(map[string]string{"SERVICE_DIRECTORIES": svcs + "/*/", "ARCHIVER_PIPELINE": "bash"})) {
		t.Fatal("ARCHIVER_PIPELINE=bash must override")
	}
}
