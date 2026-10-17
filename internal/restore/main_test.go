package restore

import (
	"os"
	"testing"

	"github.com/SisyphusMD/archiver/internal/hooks"
)

// The test binary stands in for archiver when a restore hook runs under its guard.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == hooks.GuardCommand {
		os.Exit(hooks.Guard(os.Args[2:]))
	}
	os.Exit(m.Run())
}
