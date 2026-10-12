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

// A settings file whose pre-backup function switches the filter list (to a snapshot it
// takes) cannot be carried by the filters file, which holds the starting list: migration
// says so, naming the function that does it, and says nothing for a file that does not.
func TestMigrateWarnsWhenFiltersChangeAtRuntime(t *testing.T) {
	migrate := func(settings string) []string {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, Legacy), []byte(settings), 0o644); err != nil {
			t.Fatal(err)
		}
		m, err := Migrate(dir, "host")
		if err != nil {
			t.Fatal(err)
		}
		return m.Warnings
	}
	runtime := migrate(`DUPLICACY_FILTERS_PATTERNS=("+data/live/*" "-*")
use_snapshot() { DUPLICACY_FILTERS_PATTERNS=("+data/pit/*" "-*"); }
service_specific_pre_backup_function() { use_snapshot; }
`)
	if len(runtime) != 1 || !strings.Contains(runtime[0], "use_snapshot changes DUPLICACY_FILTERS_PATTERNS") {
		t.Errorf("warnings: %q", runtime)
	}
	static := migrate(`DUPLICACY_FILTERS_PATTERNS=("+data/*" "-*")
service_specific_pre_backup_function() { echo dump; }
`)
	if len(static) != 0 {
		t.Errorf("a static list warned: %q", static)
	}
}
