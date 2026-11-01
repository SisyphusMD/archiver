package forget

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/inuse"
)

type fake struct {
	files   map[string]map[string][]string // storage -> path -> entries
	removed []string
}

func (f *fake) env(t *testing.T, cfg *config.Config) Env {
	var out bytes.Buffer
	return Env{
		Ctx: context.Background(), Cfg: cfg, Hostname: "nas", InUseDir: t.TempDir(), Out: &out, In: strings.NewReader(""),
		List: func(t config.Target, path string) ([]string, error) { return f.files[t.Name][path], nil },
		Remove: func(t config.Target, path string) error {
			f.removed = append(f.removed, t.Name+":"+path)
			delete(f.files[t.Name], path)
			return nil
		},
	}
}

func setup(t *testing.T) (*config.Config, *fake) {
	root := t.TempDir()
	for _, d := range []string{"app", "db"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{
		ServiceDirectories: []string{filepath.Join(root, "*"), "/srv/unmounted"},
		Targets:            []config.Target{{N: 1, Name: "local", Type: "local"}, {N: 2, Name: "offsite", Type: "b2"}},
	}
	f := &fake{files: map[string]map[string][]string{
		"local":   {"snapshots/nas-old": {"1", "2", "7"}},
		"offsite": {"snapshots/nas-old": {"2", "7"}},
	}}
	return cfg, f
}

func TestForgetRemovesOnEveryStoragePrimaryFirst(t *testing.T) {
	cfg, f := setup(t)
	e := f.env(t, cfg)
	if rc := Run(e, []string{"old"}, true); rc != 0 {
		t.Fatalf("rc %d: %s", rc, e.Out)
	}
	if want := []string{"local:snapshots/nas-old", "offsite:snapshots/nas-old"}; !reflect.DeepEqual(f.removed, want) {
		t.Fatalf("removed %v, want %v", f.removed, want)
	}
	if !strings.Contains(e.Out.(*bytes.Buffer).String(), "maintenance exhaustive") {
		t.Fatalf("no exhaustive-prune advice: %s", e.Out)
	}
}

func TestForgetRefusesConfiguredServices(t *testing.T) {
	cfg, f := setup(t)
	// A glob match, a literal directory not mounted right now, and a whole snapshot ID.
	for _, name := range []string{"app", "unmounted", "nas-db"} {
		if rc := Run(f.env(t, cfg), []string{"old", name}, true); rc != 1 {
			t.Errorf("%s: rc %d, want 1", name, rc)
		}
	}
	if len(f.removed) != 0 {
		t.Fatalf("removed %v after a refusal", f.removed)
	}
}

func TestForgetRefusesBadIDs(t *testing.T) {
	cfg, f := setup(t)
	for _, name := range []string{"../x", "a/b", "sp ace"} {
		if rc := Run(f.env(t, cfg), []string{name}, true); rc != 2 {
			t.Errorf("%q: rc %d, want 2", name, rc)
		}
	}
}

func TestForgetNeedsConfirmation(t *testing.T) {
	cfg, f := setup(t)
	if rc := Run(f.env(t, cfg), []string{"old"}, false); rc != 1 || len(f.removed) != 0 {
		t.Fatalf("without --yes and no terminal: rc %d, removed %v", rc, f.removed)
	}
	e := f.env(t, cfg)
	e.Terminal, e.In = true, strings.NewReader("n\n")
	if rc := Run(e, []string{"old"}, false); rc != 1 || len(f.removed) != 0 {
		t.Fatalf("answered n: rc %d, removed %v", rc, f.removed)
	}
	e.In = strings.NewReader("y\n")
	if rc := Run(e, []string{"old"}, false); rc != 0 || len(f.removed) != 2 {
		t.Fatalf("answered y: rc %d, removed %v", rc, f.removed)
	}
}

func TestForgetSkipsARevisionInUse(t *testing.T) {
	cfg, f := setup(t)
	e := f.env(t, cfg)
	h, err := inuse.Hold(e.InUseDir, "copy", []inuse.Revision{{Storage: "offsite", ID: "nas-old", Rev: 7}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Release()
	if rc := Run(e, []string{"old"}, true); rc != 1 {
		t.Fatalf("rc %d, want 1", rc)
	}
	if want := []string{"local:snapshots/nas-old"}; !reflect.DeepEqual(f.removed, want) {
		t.Fatalf("removed %v, want %v", f.removed, want)
	}
}

// A revision published after the plan and read right away counts, and a failure on the
// primary leaves the secondaries alone: a copy may still be carrying the ID there.
func TestForgetRelistsAndStopsAtAFailure(t *testing.T) {
	cfg, f := setup(t)
	e := f.env(t, cfg)
	list := e.List
	listed := 0
	e.List = func(t config.Target, path string) ([]string, error) {
		listed++
		if listed == 3 { // the primary, again, under its gate
			f.files["local"][path] = append(f.files["local"][path], "8")
		}
		return list(t, path)
	}
	h, err := inuse.Hold(e.InUseDir, "restore", []inuse.Revision{{Storage: "local", ID: "nas-old", Rev: 8}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Release()
	if rc := Run(e, []string{"old"}, true); rc != 1 || len(f.removed) != 0 {
		t.Fatalf("rc %d, removed %v", rc, f.removed)
	}
}

// Nothing goes while a storage cannot be listed: a primary left out would copy the ID back.
func TestForgetNeedsEveryStorage(t *testing.T) {
	cfg, f := setup(t)
	e := f.env(t, cfg)
	list := e.List
	e.List = func(t config.Target, path string) ([]string, error) {
		if t.N == 1 {
			return nil, errors.New("unreachable")
		}
		return list(t, path)
	}
	if rc := Run(e, []string{"old"}, true); rc != 1 || len(f.removed) != 0 {
		t.Fatalf("rc %d, removed %v", rc, f.removed)
	}
}

// A service directory given as "." backs up under the working directory's name.
func TestConfiguredResolvesRelativeDirectories(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	got := Configured(&config.Config{ServiceDirectories: []string{"."}})
	if !got[filepath.Base(dir)] || got["."] {
		t.Fatalf("got %v, want %s", got, filepath.Base(dir))
	}
}

func TestForgetNothingThere(t *testing.T) {
	cfg, f := setup(t)
	if rc := Run(f.env(t, cfg), []string{"never"}, true); rc != 0 || len(f.removed) != 0 {
		t.Fatalf("rc %d, removed %v", rc, f.removed)
	}
}

func TestOrphans(t *testing.T) {
	got := Orphans([]string{"nas-app", "nas-old", "vps-old", "nas-dr-x", "nas"}, "nas", map[string]bool{"app": true})
	if want := []string{"nas-dr-x", "nas-old"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
