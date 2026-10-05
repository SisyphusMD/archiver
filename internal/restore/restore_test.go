package restore

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/layout"
)

func TestParseRevisions(t *testing.T) {
	listing := "Storage set to /s\nSnapshot h-app revision 1 created at 2026-01-01 00:00\n" +
		"Snapshot h-app revision 12 created at 2026-01-02 00:00\nSnapshot h-app2 revision 5 created at x\n"
	if got := parseRevisions(listing, "h-app"); !reflect.DeepEqual(got, []int{12, 1}) {
		t.Fatalf("got %v", got)
	}
}

func TestMissingCaps(t *testing.T) {
	for _, c := range []struct {
		eff  int64
		want []string
	}{{-1, nil}, {0, []string{"CHOWN", "FOWNER"}}, {capChown, []string{"FOWNER"}}, {capChown | capFowner | 1<<1, nil}} {
		if got := missingCaps(c.eff); !reflect.DeepEqual(got, c.want) {
			t.Errorf("missingCaps(%#x) = %v, want %v", c.eff, got, c.want)
		}
	}
}

// fakeDuplicacy stands in for duplicacy: a storage URL holding "down" fails init; list
// prints the revisions in <storage URL>/revs; restore writes what it restored, its flags,
// what its fd 3 is, and, when the storage has one, a restore hook. With FAKE_LOCK, restore
// then starts a "backup" (a lock held by the test process).
const fakeDuplicacy = `#!/bin/sh
case "$1" in
init)
  url=""; for a in "$@"; do url="$a"; done
  case "$url" in *down*) echo "cannot reach $url"; exit 100;; esac
  mkdir -p .duplicacy && echo "$url" > .duplicacy/url ;;
list)
  url=$(cat .duplicacy/url)
  for r in $(cat "$url/revs" 2>/dev/null); do echo "Snapshot $3 revision $r created at x"; done ;;
restore)
  url=$(cat .duplicacy/url); shift
  echo "$2 from $url" > restored.txt; echo "$*" > flags.txt
  readlink /proc/$$/fd/3 > fd3.txt 2>/dev/null
  [ -n "$FAKE_LOCK" ] && echo "$PPID backup backup" > "$FAKE_LOCK"
  [ -f "$url/hook" ] && cp "$url/hook" ./post-restore && chmod +x ./post-restore
  exit 0 ;;
esac
`

type fixture struct {
	env      *Env
	vars     map[string]string
	out, err bytes.Buffer
	root     string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{root: root, vars: map[string]string{
		"SERVICE_DIRECTORIES":         filepath.Join(root, "services") + "/*/",
		"STORAGE_TARGET_1_NAME":       "local",
		"STORAGE_TARGET_1_TYPE":       "local",
		"STORAGE_TARGET_1_LOCAL_PATH": filepath.Join(root, "store1"),
		"STORAGE_TARGET_2_NAME":       "off-site",
		"STORAGE_TARGET_2_TYPE":       "local",
		"STORAGE_TARGET_2_LOCAL_PATH": filepath.Join(root, "store2"),
	}}
	secrets := filepath.Join(root, "secrets")
	for _, d := range []string{secrets, filepath.Join(root, "store1"), filepath.Join(root, "store2"), filepath.Join(root, "lock")} {
		os.MkdirAll(d, 0o755)
	}
	os.WriteFile(filepath.Join(secrets, "storage_password"), []byte("password1"), 0o600)
	os.WriteFile(filepath.Join(secrets, "rsa_passphrase"), []byte("rp"), 0o600)
	bin := filepath.Join(root, "duplicacy")
	os.WriteFile(bin, []byte(fakeDuplicacy), 0o755)
	f.env = &Env{
		Layout:    layout.Layout{Root: root, Lock: filepath.Join(root, "lock")},
		Source:    config.Source{Getenv: func(k string) string { return f.vars[k] }, SecretsDir: secrets},
		Environ:   []string{"PATH=" + os.Getenv("PATH"), "STORAGE_PASSWORD=leak", "DUPLICACY_LOCAL_PASSWORD=leak"},
		Hostname:  "h",
		Duplicacy: bin,
		Stdin:     strings.NewReader(""),
		Stdout:    &f.out,
		Stderr:    &f.err,
		CapEff:    -1,
	}
	return f
}

func (f *fixture) revs(store int, revs string) {
	os.WriteFile(filepath.Join(f.root, "store"+strconv.Itoa(store), "revs"), []byte(revs), 0o644)
}

func (f *fixture) read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.root, path))
	if err != nil {
		t.Fatalf("%v (stdout %s, stderr %s)", err, f.out.String(), f.err.String())
	}
	return strings.TrimSpace(string(b))
}

func TestAutoFallsBackAndPins(t *testing.T) {
	f := newFixture(t)
	f.revs(2, "1 3 2")
	f.vars["SNAPSHOT_ID"], f.vars["LOCAL_DIR"] = "h-app", filepath.Join(f.root, "r")
	f.vars["STORAGE_TARGET_1_LOCAL_PATH"] = filepath.Join(f.root, "down")
	if code := f.env.Auto(); code != OK {
		t.Fatalf("exit %d: %s %s", code, f.out.String(), f.err.String())
	}
	if got := f.read(t, "r/restored.txt"); got != "3 from "+filepath.Join(f.root, "store2") {
		t.Fatalf("restored %q, want the newest revision from the secondary", got)
	}

	f.vars["STORAGE_TARGET"] = "1"
	f.vars["LOCAL_DIR"] = filepath.Join(f.root, "r2")
	if code := f.env.Auto(); code != Unreachable {
		t.Fatalf("pinned to an unreachable storage: exit %d, want %d", code, Unreachable)
	}
	f.vars["STORAGE_TARGET"], f.vars["REVISION"] = "off-site", "2"
	f.vars["OVERWRITE"], f.vars["IGNORE_OWNERSHIP"] = "1", "1"
	if code := f.env.Auto(); code != OK {
		t.Fatalf("exit %d: %s", code, f.err.String())
	}
	if got := f.read(t, "r2/restored.txt"); !strings.HasPrefix(got, "2 from") {
		t.Fatalf("restored %q, want revision 2", got)
	}
	// duplicacy holds the in-use registration itself, so it survives this process.
	if got := f.read(t, "r2/fd3.txt"); !strings.HasPrefix(got, f.env.Layout.InUseDir()+"/restore-") {
		t.Fatalf("duplicacy's fd 3 is %q, want the in-use registration", got)
	}
	if got := f.read(t, "r2/flags.txt"); !strings.Contains(got, "-overwrite -ignore-owner") {
		t.Fatalf("flags %q", got)
	}
	f.vars["REVISION"] = "9"
	if code := f.env.Auto(); code != NotFound {
		t.Fatalf("a missing revision: exit %d, want %d", code, NotFound)
	}
}

// A recovery container needs only storage settings: no service directories, and a notifier
// without its keys does not block a restore.
func TestRestoreNeedsOnlyStorageSettings(t *testing.T) {
	f := newFixture(t)
	f.revs(1, "1")
	delete(f.vars, "SERVICE_DIRECTORIES")
	f.vars["NOTIFICATION_SERVICE"] = "Pushover"
	f.vars["SNAPSHOT_ID"], f.vars["LOCAL_DIR"] = "h-app", filepath.Join(f.root, "r")
	if code := f.env.Auto(); code != OK {
		t.Fatalf("exit %d: %s", code, f.err.String())
	}
	if code := f.env.SnapshotExists(); code != OK {
		t.Fatalf("snapshot-exists: exit %d: %s", code, f.err.String())
	}
	if code := f.env.AutoAll(); code != NotFound || !strings.Contains(f.err.String(), "SERVICE_DIRECTORIES is not set") {
		t.Fatalf("auto-restore-all without SERVICE_DIRECTORIES: exit %d: %s", code, f.err.String())
	}
}

func TestMissingOwnershipCapsWarnAndRestore(t *testing.T) {
	f := newFixture(t)
	f.revs(1, "1")
	f.env.CapEff = capChown
	f.vars["SNAPSHOT_ID"], f.vars["LOCAL_DIR"] = "h-app", filepath.Join(f.root, "r")
	if code := f.env.Auto(); code != OK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(f.out.String(), "The FOWNER capability is not granted") {
		t.Fatalf("no warning: %s", f.out.String())
	}
	if got := f.read(t, "r/flags.txt"); !strings.Contains(got, "-ignore-owner") {
		t.Fatalf("flags %q, want -ignore-owner", got)
	}
}

// The hook runs only when asked, without secrets, told what was restored; its failure is
// exit 1 whatever it exited, never 2 (unreachable) or 3 (busy).
func TestRestoreHook(t *testing.T) {
	f := newFixture(t)
	f.revs(1, "4")
	os.WriteFile(filepath.Join(f.root, "store1", "hook"), []byte("#!/bin/sh\nenv > hook-env.txt\nexit ${HOOK_EXIT:-0}\n"), 0o755)
	f.vars["SNAPSHOT_ID"], f.vars["LOCAL_DIR"] = "h-app", filepath.Join(f.root, "r")
	if code := f.env.Auto(); code != OK {
		t.Fatalf("exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(f.root, "r", "hook-env.txt")); err == nil {
		t.Fatal("the hook ran without RUN_RESTORE_SERVICE")
	}
	f.vars["RUN_RESTORE_SERVICE"] = "1"
	if code := f.env.Auto(); code != OK {
		t.Fatalf("exit %d: %s", code, f.err.String())
	}
	env := f.read(t, "r/hook-env.txt") + "\n"
	for _, want := range []string{"ARCHIVER_SERVICE=r", "ARCHIVER_SNAPSHOT_ID=h-app", "ARCHIVER_RESTORE_REVISION=4", "ARCHIVER_RESTORE_STORAGE=local"} {
		if !strings.Contains(env, want+"\n") {
			t.Errorf("hook environment lacks %s", want)
		}
	}
	if strings.Contains(env, "leak") {
		t.Errorf("the hook received a secret:\n%s", env)
	}
	f.env.Environ = append(f.env.Environ, "HOOK_EXIT=3")
	if code := f.env.Auto(); code != NotFound {
		t.Fatalf("a hook exiting 3: exit %d, want %d", code, NotFound)
	}
}

func TestLegacyRestoreHookRunsWithBash(t *testing.T) {
	f := newFixture(t)
	f.revs(1, "1")
	dir := filepath.Join(f.root, "r")
	os.MkdirAll(dir, 0o755)
	// Not executable, as restore-service.sh often is not: it always ran as `bash restore-service.sh`.
	os.WriteFile(filepath.Join(dir, "restore-service.sh"), []byte("echo \"$0 in $PWD for $SNAPSHOT_ID at $LOCAL_DIR\" > ran.txt\n"), 0o644)
	f.vars["SNAPSHOT_ID"], f.vars["LOCAL_DIR"], f.vars["RUN_RESTORE_SERVICE"] = "h-app", dir, "1"
	if code := f.env.Auto(); code != OK {
		t.Fatalf("exit %d: %s", code, f.err.String())
	}
	if got := f.read(t, "r/ran.txt"); got != "restore-service.sh in "+dir+" for h-app at "+dir {
		t.Fatalf("got %q", got)
	}
}

func TestSnapshotExists(t *testing.T) {
	f := newFixture(t)
	f.vars["SNAPSHOT_ID"] = "h-app"
	if code := f.env.SnapshotExists(); code != NotFound || f.out.String() != "NOT FOUND\n" {
		t.Fatalf("exit %d, stdout %q", code, f.out.String())
	}
	f.out.Reset()
	f.revs(2, "1")
	if code := f.env.SnapshotExists(); code != OK || f.out.String() != "EXISTS\n" {
		t.Fatalf("exit %d, stdout %q", code, f.out.String())
	}
	f.out.Reset()
	f.vars["STORAGE_TARGET_1_LOCAL_PATH"], f.vars["STORAGE_TARGET_2_LOCAL_PATH"] = "/down1", "/down2"
	if code := f.env.SnapshotExists(); code != Unreachable || f.out.String() != "UNDETERMINED\n" {
		t.Fatalf("exit %d, stdout %q", code, f.out.String())
	}
}

func TestBusy(t *testing.T) {
	f := newFixture(t)
	f.vars["SNAPSHOT_ID"], f.vars["LOCAL_DIR"] = "h-app", filepath.Join(f.root, "r")
	os.WriteFile(f.env.Layout.BackupLock(), []byte(strconv.Itoa(os.Getpid())+" backup backup\n"), 0o644)
	for name, run := range map[string]func() int{"auto-restore": f.env.Auto, "auto-restore-all": f.env.AutoAll, "snapshot-exists": f.env.SnapshotExists} {
		if code := run(); code != Busy {
			t.Errorf("%s under a running backup: exit %d, want %d", name, code, Busy)
		}
	}
}

func TestAutoAll(t *testing.T) {
	f := newFixture(t)
	f.revs(1, "2")
	for _, s := range []string{"app", "web"} {
		os.MkdirAll(filepath.Join(f.root, "services", s), 0o755)
	}
	if code := f.env.AutoAll(); code != OK {
		t.Fatalf("exit %d: %s %s", code, f.out.String(), f.err.String())
	}
	for _, s := range []string{"app", "web"} {
		if got := f.read(t, "services/"+s+"/restored.txt"); !strings.HasPrefix(got, "2 from") {
			t.Fatalf("%s: %q", s, got)
		}
	}
	if !strings.Contains(f.out.String(), "restored: app web\n") {
		t.Fatalf("summary: %s", f.out.String())
	}
	f.out.Reset()
	// A backup starting once app is restored stops web from restoring under it.
	f.env.Environ = append(f.env.Environ, "FAKE_LOCK="+f.env.Layout.BackupLock())
	os.Remove(filepath.Join(f.root, "services", "web", "restored.txt"))
	if code := f.env.AutoAll(); code != NotFound || !strings.Contains(f.out.String(), "restored: app\nFAILED:   web\n") {
		t.Fatalf("exit %d: %s", code, f.out.String())
	}
	if _, err := os.Stat(filepath.Join(f.root, "services", "web", "restored.txt")); err == nil {
		t.Fatal("web restored while a backup ran")
	}
	os.Remove(f.env.Layout.BackupLock())
	f.env.Environ = f.env.Environ[:len(f.env.Environ)-1]

	f.out.Reset()
	f.vars["STORAGE_TARGET_1_LOCAL_PATH"], f.vars["STORAGE_TARGET_2_LOCAL_PATH"] = "/down1", "/down2"
	if code := f.env.AutoAll(); code != NotFound || !strings.Contains(f.out.String(), "FAILED:   app web\n") {
		t.Fatalf("exit %d: %s", code, f.out.String())
	}
}

func TestInteractive(t *testing.T) {
	f := newFixture(t)
	f.revs(2, "1 2")
	os.WriteFile(filepath.Join(f.root, "store2", "hook"), []byte("#!/bin/sh\nread answer; echo \"$answer\" > hook-ran\n"), 0o755)
	dir := filepath.Join(f.root, "r")
	// An invalid choice, then target 2; a revision not listed, then 1; customize: hash only,
	// thread count 8; then yes to the hook.
	f.env.Stdin = strings.NewReader("7\n2\nh-app\n" + dir + "\n5\n1\ny\ny\nn\nn\nn\nn\n8\ny\nfor the hook\n")
	if code := f.env.Interactive(); code != 0 {
		t.Fatalf("exit %d: %s %s", code, f.out.String(), f.err.String())
	}
	if got := f.read(t, "r/restored.txt"); got != "1 from "+filepath.Join(f.root, "store2") {
		t.Fatalf("restored %q", got)
	}
	if got := f.read(t, "r/flags.txt"); !strings.Contains(got, "-threads 8 -hash") || strings.Contains(got, "-overwrite") {
		t.Fatalf("flags %q", got)
	}
	if got := f.read(t, "r/hook-ran"); got != "for the hook" {
		t.Fatalf("the hook read %q, want the input after the answers", got)
	}
	for _, want := range []string{"Invalid choice", "Revision 5 is not one of the revisions above"} {
		if !strings.Contains(f.out.String(), want) {
			t.Errorf("output lacks %q", want)
		}
	}

	f.out.Reset()
	f.env.Stdin = strings.NewReader("")
	if code := f.env.Interactive(); code != 1 {
		t.Fatalf("no input: exit %d, want 1", code)
	}

	os.WriteFile(f.env.Layout.BackupLock(), []byte(strconv.Itoa(os.Getpid())+" backup backup\n"), 0o644)
	f.env.Stdin = strings.NewReader("2\nh-app\n" + dir + "\n1\nn\n")
	os.Remove(filepath.Join(dir, "restored.txt"))
	if code := f.env.Interactive(); code != 1 {
		t.Fatalf("under a running backup: exit %d, want 1", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "restored.txt")); err == nil {
		t.Fatal("restored while a backup ran")
	}
}
