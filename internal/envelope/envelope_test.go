package envelope

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/layout"
)

func age(epoch int64, now time.Time) string {
	return fmt.Sprintf("%dd ago", int(now.Sub(time.Unix(epoch, 0)).Hours()/24))
}

func TestStatus(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	day := int64(86400)
	cases := []struct {
		name, confirmed, current string
		line, warning            string
	}{
		{"not in use", "", "", "", ""},
		{"never confirmed", "", "aaa 1", "never confirmed printed", ""},
		{"current", fmt.Sprintf("aaa %d", now.Unix()-10*day), "aaa 1", "current, confirmed printed 10d ago", ""},
		{"changed", fmt.Sprintf("aaa %d", now.Unix()-10*day), "bbb 1", "OUT OF DATE, printed 10d ago", "out of date"},
		{"a year old", fmt.Sprintf("aaa %d", now.Unix()-400*day), "aaa 1", "check due", "over a year ago"},
		{"confirmed, no current yet", fmt.Sprintf("aaa %d", now.Unix()-1*day), "", "current", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := layout.Layout{Root: t.TempDir(), Lock: t.TempDir()}
			os.MkdirAll(l.LogDir(), 0o755)
			if c.confirmed != "" {
				os.WriteFile(l.EnvelopeConfirmed(), []byte(c.confirmed+"\n"), 0o600)
			}
			if c.current != "" {
				os.WriteFile(l.EnvelopeCurrent(), []byte(c.current+"\n"), 0o600)
			}
			line, warning := Status(l, now, age)
			if (c.line == "") != (line == "") || !strings.Contains(line, c.line) {
				t.Errorf("line %q, want %q", line, c.line)
			}
			if (c.warning == "") != (warning == "") || !strings.Contains(warning, c.warning) {
				t.Errorf("warning %q, want %q", warning, c.warning)
			}
		})
	}
}
