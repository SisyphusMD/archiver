package daemon

import (
	"reflect"
	"testing"
	"time"
)

func TestNamedDrillJobs(t *testing.T) {
	vars := map[string]string{"RESTORE_DRILL_GARAGE_SCHEDULE": "0 8 * * 6#1", "RESTORE_DRILL_DB_2_SCHEDULE": "0 1 * * *"}
	var environ []string
	for k, v := range vars {
		environ = append(environ, k+"="+v)
	}
	jobs, err := Jobs(func(k string) string { return vars[k] }, time.Now(), environ)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || jobs[1].Name != "drill:GARAGE" || jobs[1].Command != "drill" || !reflect.DeepEqual(jobs[1].Args, []string{"--named", "GARAGE"}) {
		t.Fatalf("%+v", jobs)
	}
	for _, name := range []string{"DIR", "HOOK", "HOOK_TIMEOUT", "bad", "A-B", ""} {
		if NamedDrillName(name) {
			t.Errorf("accepted %q", name)
		}
	}
	for _, name := range []string{"GARAGE", "DB_2", "123", "_", "DEFAULT"} {
		if !NamedDrillName(name) {
			t.Errorf("rejected %q", name)
		}
	}
}
