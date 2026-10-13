package daemon

import (
	"testing"
	"time"
)

func TestDeepCheckJobs(t *testing.T) {
	env := map[string]string{"STORAGE_TARGET_1_NAME": "local", "STORAGE_TARGET_1_DEEP_CHECK": "0 4 1 * *", "STORAGE_TARGET_2_NAME": "offsite"}
	get := func(k string) string { return env[k] }
	jobs, err := Jobs(get, time.Now())
	if err != nil || len(jobs) != 1 || jobs[0].Command != "deep-check" || jobs[0].Args[0] != "local" {
		t.Fatalf("jobs %+v: %v", jobs, err)
	}
	env["STORAGE_TARGET_2_DEEP_CHECK"] = "invalid"
	if _, err := Jobs(get, time.Now()); err == nil {
		t.Fatal("invalid target schedule accepted")
	}
}
