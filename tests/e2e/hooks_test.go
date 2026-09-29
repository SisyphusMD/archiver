package e2e

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/tests/e2e/harness"
)

// hookFixture is a deployment of the image under test with one primary and one copy
// storage, whose services each hold one small file. Hooks are installed by the test.
type hookFixture struct {
	d       *harness.Deployment
	hooks   *harness.Hooks
	primary string
	copyDir string
}

func newHookFixture(t *testing.T, services ...string) hookFixture {
	t.Helper()
	image := os.Getenv("ARCHIVER_IMAGE")
	if image == "" {
		t.Fatal("ARCHIVER_IMAGE is unset")
	}
	work := harness.WorkDir(t)
	f := hookFixture{primary: filepath.Join(work, "storage-primary"), copyDir: filepath.Join(work, "storage-copy")}
	keysDir := filepath.Join(work, "keys")
	for _, d := range []string{f.primary, f.copyDir, keysDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	svcs := map[string]string{}
	for _, s := range services {
		dir := filepath.Join(work, "svc", s)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte(s+" data\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		svcs[s] = dir
	}
	f.d = &harness.Deployment{
		Image:            image,
		Hostname:         "e2e-hooks",
		Services:         svcs,
		Storages:         []harness.Storage{{Name: "primary", Dir: f.primary}, {Name: "offsite", Dir: f.copyDir}},
		Keys:             harness.NewKeys(t, image, keysDir),
		RecoveryPassword: "e2e-recovery-password",
	}
	f.hooks = harness.NewHooks(t, f.d, work)
	return f
}

func (f hookFixture) revisions(t *testing.T, service string) []int {
	t.Helper()
	return harness.Revisions(t, f.primary, f.d.SnapshotID(service))
}

// TestHooksPostRunsAfterFailedBackup: the post hook undoes the pre hook (restarts what it
// stopped), so a backup that fails mid-upload must still be followed by it.
func TestHooksPostRunsAfterFailedBackup(t *testing.T) {
	f := newHookFixture(t, "app")
	harness.BreakChunkUploads(t, f.primary)
	f.hooks.Install(t, "app", harness.HookSpec{Pre: harness.PreSucceeds})
	f.d.Start(t)

	r := f.d.Backup(t)
	if got := f.revisions(t, "app"); len(got) > 0 {
		t.Fatalf("the backup was meant to fail, but it wrote revisions %v (exit %d)", got, r.Code)
	}
	if !f.hooks.Ran(t, "app", "pre") {
		t.Fatalf("the pre hook never ran; calls %v\n%s", f.hooks.Calls(t), r.Output())
	}
	if !f.hooks.Ran(t, "app", "post") {
		t.Errorf("the post hook did not run after the failed backup; calls %v", f.hooks.Calls(t))
	}
}

// TestHooksPostRunsAfterStoppedBackup: a stop that lands while a service is uploading must
// still run that service's post hook before the run ends.
func TestHooksPostRunsAfterStoppedBackup(t *testing.T) {
	f := newHookFixture(t, "app")
	// Many seconds of reading, so the stop lands long before the backup could finish.
	harness.WriteSparse(t, filepath.Join(f.d.Services["app"], "big.img"), 16<<30)
	f.hooks.Install(t, "app", harness.HookSpec{Pre: harness.PreSucceeds})
	f.d.Start(t)

	done := f.d.StartBackup(t)
	harness.WaitForChunk(t, f.primary, 2*time.Minute)
	f.d.RequestStop(t)

	r := done.Wait(t, 2*time.Minute)
	if got := f.revisions(t, "app"); len(got) > 0 {
		t.Fatalf("the stop was meant to land mid-upload, but the backup wrote revisions %v (exit %d)", got, r.Code)
	}
	if !f.hooks.Ran(t, "app", "post") {
		t.Errorf("the post hook did not run after the stopped backup; calls %v\n%s", f.hooks.Calls(t), r.Output())
	}
}

// TestHooksPreFailureSkipsOnlyThatService: a failed pre hook leaves its service's files in
// an unknown state (a half-written dump), so that service gets no new revision (its newest
// stays the last good one), the services around it still back up, and the run fails.
func TestHooksPreFailureSkipsOnlyThatService(t *testing.T) {
	f := newHookFixture(t, "a", "b", "c")
	for _, s := range []string{"a", "b", "c"} {
		f.hooks.Install(t, s, harness.HookSpec{Pre: harness.PreSucceeds})
	}
	f.d.Start(t)
	if r := f.d.Backup(t); r.Code != 0 {
		t.Fatalf("first backup, all hooks succeeding, exited %d:\n%s", r.Code, r.Output())
	}

	f.hooks.Install(t, "b", harness.HookSpec{Pre: harness.PreFails})
	r := f.d.Backup(t)
	if r.Code == 0 {
		t.Errorf("the backup exited 0 although b's pre hook failed:\n%s", r.Output())
	}
	for s, want := range map[string][]int{"a": {1, 2}, "b": {1}, "c": {1, 2}} {
		if got := f.revisions(t, s); !slices.Equal(got, want) {
			t.Errorf("revisions of %s = %v, want %v", s, got, want)
		}
	}
}

// TestHooksEnvironmentHoldsNoSecrets: hooks are user code, often shelling out to other
// containers, so no secret may reach their environment. Two services and two storages
// cover a hook running after the pipeline has already talked to storage.
func TestHooksEnvironmentHoldsNoSecrets(t *testing.T) {
	f := newHookFixture(t, "a", "b")
	for _, s := range []string{"a", "b"} {
		f.hooks.Install(t, s, harness.HookSpec{Pre: harness.PreSucceeds, RecordEnv: true})
	}
	f.d.Start(t)
	if r := f.d.Backup(t); r.Code != 0 {
		t.Fatalf("backup exited %d:\n%s", r.Code, r.Output())
	}

	priv, err := os.ReadFile(f.d.Keys.PrivatePath)
	if err != nil {
		t.Fatal(err)
	}
	// One full base64 line of the key body is enough to recognise it anywhere.
	var keyLine string
	for _, l := range strings.Split(string(priv), "\n") {
		if len(l) >= 64 && !strings.Contains(l, ":") {
			keyLine = l
			break
		}
	}
	if keyLine == "" {
		t.Fatalf("no base64 body line in %s", f.d.Keys.PrivatePath)
	}
	secrets := map[string]string{
		"storage password":  harness.StoragePassword,
		"RSA passphrase":    f.d.Keys.Passphrase,
		"recovery password": f.d.RecoveryPassword,
		"RSA private key":   keyLine,
	}
	for _, s := range []string{"a", "b"} {
		for _, phase := range []string{"pre", "post"} {
			env := f.hooks.Env(t, s, phase)
			for what, secret := range secrets {
				for _, line := range strings.Split(env, "\n") {
					if strings.Contains(line, secret) {
						name, _, _ := strings.Cut(line, "=")
						t.Errorf("%s %s hook's environment holds the %s (in %s)", s, phase, what, name)
					}
				}
			}
		}
	}
}
