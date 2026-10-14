package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNamedDrillConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "garage"), 0700); err != nil {
		t.Fatal(err)
	}
	base := map[string]string{"SERVICE_DIRECTORIES": dir + "/*/", "STORAGE_TARGET_1_NAME": "local", "STORAGE_TARGET_1_TYPE": "local", "STORAGE_TARGET_1_LOCAL_PATH": dir, "RESTORE_DRILL_GARAGE_SCHEDULE": "0 8 * * 6#1", "RESTORE_DRILL_GARAGE_SERVICES": "garage", "RESTORE_DRILL_TIMEOUT": "1s"}
	for _, tc := range []struct {
		key, value string
		bad        bool
	}{
		{"", "", false}, {"RESTORE_DRILL_GARAGE_STORAGES", "alternate", false}, {"RESTORE_DRILL_GARAGE_STORAGES", "primary", false}, {"RESTORE_DRILL_GARAGE_TIMEOUT", "7d", false},
		{"RESTORE_DRILL_GARAGE_SCHEDULE", "bad", true}, {"RESTORE_DRILL_GARAGE_SCHEDULE", "", true}, {"RESTORE_DRILL_GARAGE_SERVICES", "", true}, {"RESTORE_DRILL_GARAGE_SERVICES", "missing", true}, {"RESTORE_DRILL_GARAGE_SERVICES", "garage,", true}, {"RESTORE_DRILL_GARAGE_STORAGES", "missing", true}, {"RESTORE_DRILL_GARAGE_TIMEOUT", "0s", true}, {"RESTORE_DRILL_HOOK_SCHEDULE", "0 8 * * *", true}, {"RESTORE_DRILL_DIR_SCHEDULE", "0 8 * * *", true}, {"RESTORE_DRILL_lower_SCHEDULE", "0 8 * * *", true}, {"RESTORE_DRILL_GARAGE_HOOK_TIMEOUT", "1h", true},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			vars := map[string]string{}
			for k, v := range base {
				vars[k] = v
			}
			if tc.key != "" {
				vars[tc.key] = tc.value
			}
			var env []string
			for k, v := range vars {
				env = append(env, k+"="+v)
			}
			c, _, err := Load(Source{Getenv: func(k string) string { return vars[k] }, SecretsDir: dir}, env)
			if err == nil {
				err = c.ValidateNamedDrills()
			}
			if tc.bad {
				if err == nil || !strings.Contains(err.Error(), "invalid ") {
					t.Fatalf("expected invalid named configuration, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			d, ok := c.NamedDrill("GARAGE")
			if !ok || len(d.Services) != 1 {
				t.Fatalf("%+v", c.NamedDrills)
			}
			if tc.key != "RESTORE_DRILL_GARAGE_TIMEOUT" && d.Timeout != 24*time.Hour {
				t.Fatalf("timeout=%s", d.Timeout)
			}
			if !IsSetting("RESTORE_DRILL_GARAGE_SCHEDULE") {
				t.Fatal("named setting not captured")
			}
		})
	}
}

func TestNamedAlternateTargets(t *testing.T) {
	c := &Config{Targets: []Target{{Name: "local"}, {Name: "backblaze"}, {Name: "cowanserver"}}}
	for _, tc := range []struct {
		selection string
		want      string
		bad       string
	}{
		{"alternate", "local,backblaze,cowanserver", ""},
		{"alternate:cowanserver,backblaze", "cowanserver,backblaze", ""},
		{"alternate: backblaze , cowanserver ", "backblaze,cowanserver", ""},
		{"alternate:backblaze", "backblaze", ""},
		{"alternate:backblaze,cowanserver,backblaze", "backblaze,cowanserver,backblaze", ""},
		{"alternate:", "", "nonempty comma-separated storage names"},
		{"alternate:backblaze,", "", "nonempty comma-separated storage names"},
		{"alternate:backblaze,,cowanserver", "", "nonempty comma-separated storage names"},
		{"alternate:missing", "", "not a configured storage"},
		{"alternate:all", "", "not a configured storage"},
		{"alternate:primary", "", "not a configured storage"},
	} {
		t.Run(tc.selection, func(t *testing.T) {
			vars := map[string]string{"RESTORE_DRILL_GARAGE_SCHEDULE": "0 8 * * 6#1", "RESTORE_DRILL_GARAGE_SERVICES": "garage", "RESTORE_DRILL_GARAGE_STORAGES": tc.selection}
			drills, err := c.loadNamedDrills(Source{Getenv: func(k string) string { return vars[k] }}, []string{"RESTORE_DRILL_GARAGE_SCHEDULE=" + vars["RESTORE_DRILL_GARAGE_SCHEDULE"]})
			if tc.bad != "" {
				if err == nil || !strings.Contains(err.Error(), "invalid RESTORE_DRILL_GARAGE_STORAGES: ") || !strings.Contains(err.Error(), tc.bad) {
					t.Fatalf("error=%v, want %s", err, tc.bad)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			targets, err := drills[0].Targets(c)
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, target := range targets {
				names = append(names, target.Name)
			}
			if got := strings.Join(names, ","); got != tc.want {
				t.Fatalf("targets=%s, want %s", got, tc.want)
			}
		})
	}
}
