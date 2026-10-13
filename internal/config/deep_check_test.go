package config

import (
	"strings"
	"testing"
)

func TestDeepCheckConfig(t *testing.T) {
	for _, tc := range []struct{ schedule, timeout, want string }{
		{"0 4 1 * *", "7d", ""}, {"bad", "7d", "STORAGE_TARGET_1_DEEP_CHECK"},
		{"0 0 30 2 *", "7d", "STORAGE_TARGET_1_DEEP_CHECK"}, {"", "0", "DEEP_CHECK_TIMEOUT"},
		{"", "-1h", "DEEP_CHECK_TIMEOUT"}, {"", "", ""},
	} {
		t.Run(tc.schedule+tc.timeout, func(t *testing.T) {
			c := &Config{ServiceDirectories: []string{"/data"}, Targets: []Target{{N: 1, Name: "local", Type: "local", Values: Values{"LOCAL_PATH": "/store"}, DeepCheck: tc.schedule}}, StoragePassword: "password", RSAPassphrase: "passphrase", PruneExhaustiveFrequency: "off", DeepCheckTimeout: tc.timeout}
			err := c.Validate(t.TempDir())
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
		})
	}
	if !IsSetting("DEEP_CHECK_TIMEOUT") || !IsSetting("STORAGE_TARGET_3_DEEP_CHECK") {
		t.Fatal("deep check settings not recognized")
	}
}

func TestLoadDeepCheck(t *testing.T) {
	env := map[string]string{"STORAGE_TARGET_1_NAME": "local", "STORAGE_TARGET_1_TYPE": "local", "STORAGE_TARGET_1_LOCAL_PATH": "/store", "STORAGE_TARGET_1_DEEP_CHECK": "0 4 1 * *"}
	source := Source{Getenv: func(k string) string { return env[k] }, SecretsDir: t.TempDir()}
	c, _, err := Load(source, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.Targets[0].DeepCheck != "0 4 1 * *" || c.DeepCheckTimeout != "7d" {
		t.Fatalf("bad deep check config %+v", c)
	}
	env["DEEP_CHECK_TIMEOUT"] = "48h"
	c, _, err = Load(source, nil)
	if err != nil || c.DeepCheckTimeout != "48h" {
		t.Fatalf("timeout not loaded: %+v %v", c, err)
	}
}
