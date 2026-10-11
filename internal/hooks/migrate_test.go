package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SisyphusMD/archiver/internal/layout"
)

// The generated hooks' DUPLICACY_REPO_DIR, worked out in bash, names the repository the
// backup uses for that service.
func TestWrapperRepoDirMatchesLayout(t *testing.T) {
	var line string
	for _, l := range strings.Split(wrapperHead, "\n") {
		if strings.HasPrefix(l, "DUPLICACY_REPO_DIR=") {
			line = l
		}
	}
	cmd := exec.Command("bash", "-c", line+`; printf %s "$DUPLICACY_REPO_DIR"`)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "ARCHIVER_SNAPSHOT_ID=h-app", "ARCHIVER_SERVICE_DIR=/srv/app"}
	out, err := cmd.Output()
	if want := filepath.Join(layout.Default().ServiceRepo("h-app", "/srv/app"), ".duplicacy"); err != nil || string(out) != want {
		t.Fatalf("got %q (%v), want %q", out, err, want)
	}
}
