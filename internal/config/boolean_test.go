package config

import (
	"strings"
	"testing"
	"time"
)

func TestParseBool(t *testing.T) {
	for _, value := range []string{"true", "1", "yes", "on", "TRUE", "YeS", "ON", "false", "0", "no", "off", "FALSE", "No", "OFF", "", "enabled", "2", " true "} {
		got, err := ParseBool("SETTING", value)
		want := strings.Contains("|true|1|yes|on|", "|"+strings.ToLower(value)+"|") && value != ""
		invalid := value == "enabled" || value == "2" || value == " true "
		if got != want || (err != nil) != invalid {
			t.Errorf("ParseBool(%q) = %v, %v; want %v, invalid=%v", value, got, err, want, invalid)
		}
		if invalid && !strings.Contains(err.Error(), "SETTING: invalid boolean") {
			t.Errorf("error does not identify setting: %v", err)
		}
	}
}

func TestMaintenanceBooleanValues(t *testing.T) {
	for _, name := range []string{"CHECK_BACKUPS", "PRUNE_BACKUPS", "ROTATE_BACKUPS"} {
		for _, value := range []string{"1", "yes", "ON", "false", "0", "off", "garbage"} {
			env := map[string]string{name: value}
			c, _, err := Load(Source{Getenv: func(key string) string { return env[key] }, SecretsDir: t.TempDir()}, []string{})
			if value == "garbage" {
				if err == nil || !strings.Contains(err.Error(), name) {
					t.Errorf("%s=%s: %v", name, value, err)
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			want := value == "1" || value == "yes" || value == "ON"
			got := c.PruneBackups
			if name == "CHECK_BACKUPS" {
				got = c.CheckBackups
			}
			if got != want {
				t.Errorf("%s=%s: %v", name, value, got)
			}
		}
	}
}

func TestDrillHookGracePeriod(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  time.Duration
		bad   bool
	}{{"", time.Minute, false}, {"150ms", 150 * time.Millisecond, false}, {"2m", 2 * time.Minute, false}, {"0s", 0, true}, {"-1s", 0, true}, {"bad", 0, true}} {
		got, err := (&Config{DrillHookGrace: tc.value}).DrillHookGracePeriod()
		if got != tc.want || (err != nil) != tc.bad {
			t.Errorf("grace %q = %v, %v", tc.value, got, err)
		}
	}
}
