package metrics

import (
	"strings"
	"testing"

	"github.com/SisyphusMD/archiver/internal/deepcheck"
	"github.com/SisyphusMD/archiver/internal/status"
)

func TestDeepCheckMetrics(t *testing.T) {
	out := Render(status.Snapshot{DeepChecks: map[string]deepcheck.Result{"local": {At: 10, OK: false, Seconds: 2.5, LastSuccess: 8}}})
	for _, want := range []string{
		`archiver_deep_check_last_timestamp_seconds{storage="local"} 10`,
		`archiver_deep_check_last_ok{storage="local"} 0`,
		`archiver_deep_check_last_duration_seconds{storage="local"} 2.5`,
		`archiver_deep_check_last_success_timestamp_seconds{storage="local"} 8`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s", want)
		}
	}
}
