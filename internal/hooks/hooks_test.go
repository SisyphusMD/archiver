package hooks

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func TestEnvironHidesSecrets(t *testing.T) {
	base := []string{"PATH=/bin", "STORAGE_PASSWORD=x", "RSA_PASSPHRASE=y", "DUPLICACY_LOCAL_PASSWORD=z",
		"DUPLICACY_B2_KEY=k", "STORAGE_TARGET_2_S3_SECRET=s", "ARCHIVER_SERVICE=spoofed", "HOSTNAME=h"}
	env := Environ(base, Service{Name: "app", Dir: "/srv/app", SnapshotID: "h-app", StateDir: "/tmp/s"}, Failed)
	want := []string{"PATH=/bin", "HOSTNAME=h", "ARCHIVER_SERVICE=app", "ARCHIVER_SERVICE_DIR=/srv/app",
		"ARCHIVER_SNAPSHOT_ID=h-app", "ARCHIVER_STATE_DIR=/tmp/s", "ARCHIVER_BACKUP_RESULT=failed"}
	if !slices.Equal(env, want) {
		t.Fatalf("got %q\nwant %q", env, want)
	}
}

func TestExistsRequiresExecutable(t *testing.T) {
	dir := t.TempDir()
	if ok, err := Exists(dir, PreBackup); ok || err != nil {
		t.Fatalf("missing hook: %v %v", ok, err)
	}
	write(t, filepath.Join(dir, PreBackup), "#!/bin/sh\n", 0o644)
	if _, err := Exists(dir, PreBackup); err == nil || !strings.Contains(err.Error(), "chmod +x") {
		t.Fatalf("non-executable hook: %v", err)
	}
	os.Chmod(filepath.Join(dir, PreBackup), 0o755)
	if ok, err := Exists(dir, PreBackup); !ok || err != nil {
		t.Fatalf("executable hook: %v %v", ok, err)
	}
}

func TestReadFilters(t *testing.T) {
	dir := t.TempDir()
	if got, _ := ReadFilters(dir); !slices.Equal(got, []string{"+*"}) {
		t.Fatalf("default = %q", got)
	}
	write(t, filepath.Join(dir, Filters), "+a/\r\n\n-*\n", 0o644)
	if got, _ := ReadFilters(dir); !slices.Equal(got, []string{"+a/", "-*"}) {
		t.Fatalf("got %q", got)
	}
}

// The shape of Cody's nas and vps settings files: filters naming the settings file, a pre
// function piping a script through log_output and warning with log_message.
const typical = `# Comment
log_message INFO "a top-level call works in bash too"
DUPLICACY_FILTERS_PATTERNS=(
  "+data/"
  "+backup.sh"
  "+restore-service.sh"
  "+service-backup-settings.sh"
  "-?*"
)
service_specific_pre_backup_function() {
  if ! (set -o pipefail; ./backup.sh 2>&1 | log_output); then
    log_message "WARNING" "Pre-backup script failed."
    return 1
  fi
}
`

func TestMigrateTypical(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	for _, tc := range []struct {
		name     string
		script   string
		wantCode int
		wantOut  string
	}{
		{"backup succeeds", "#!/bin/sh\necho dumped\n", 0, "dumped"},
		{"backup fails", "#!/bin/sh\necho broken; exit 3\n", 1, "[WARNING] Pre-backup script failed."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, Legacy), typical, 0o644)
			write(t, filepath.Join(dir, "backup.sh"), tc.script, 0o755)
			m, err := Migrate(dir, "h")
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(m.Wrote, []string{PreBackup, Filters}) || len(m.Warnings) != 0 {
				t.Fatalf("wrote %q, warnings %q", m.Wrote, m.Warnings)
			}
			if _, err := os.Stat(filepath.Join(dir, Legacy)); !os.IsNotExist(err) {
				t.Fatal("the settings file is still in place, so the Go pipeline would refuse the service")
			}
			filters, _ := ReadFilters(dir)
			want := []string{"+data/", "+backup.sh", "+restore-service.sh", "+pre-backup", "+filters", "+" + LegacyKept, "-?*"}
			if !slices.Equal(filters, want) {
				t.Fatalf("filters %q\nwant %q", filters, want)
			}
			cmd := exec.Command(filepath.Join(dir, PreBackup))
			cmd.Env = append(os.Environ(), "ARCHIVER_SERVICE=app", "ARCHIVER_SERVICE_DIR="+dir)
			out, err := cmd.CombinedOutput()
			code := 0
			if err != nil {
				code = err.(*exec.ExitError).ExitCode()
			}
			if code != tc.wantCode || !strings.Contains(string(out), tc.wantOut) {
				t.Fatalf("pre-backup exited %d with %q; want %d containing %q", code, out, tc.wantCode, tc.wantOut)
			}
		})
	}
}

func TestMigrateWarnsAboutSharedVariables(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, Legacy), `service_specific_pre_backup_function() {
  local scratch=1
  CONTAINER_WAS_RUNNING=$(echo yes)
}
service_specific_post_backup_function() {
  [ "$CONTAINER_WAS_RUNNING" = yes ] && echo start
  echo "$scratch"
}
`, 0o644)
	m, err := Migrate(dir, "h")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.Wrote, []string{PreBackup, PostBackup}) {
		t.Fatalf("wrote %q", m.Wrote)
	}
	if len(m.Warnings) != 1 || !strings.Contains(m.Warnings[0], "$CONTAINER_WAS_RUNNING") {
		t.Fatalf("warnings %q", m.Warnings)
	}
	if _, err := os.Stat(filepath.Join(dir, Filters)); !os.IsNotExist(err) {
		t.Fatal("a file with no filter array must not gain a filters file (it would change what is backed up)")
	}
}

func TestMigrateRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, Legacy), typical, 0o644)
	write(t, filepath.Join(dir, Filters), "+mine\n", 0o644)
	if _, err := Migrate(dir, "h"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("got %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, Filters)); string(b) != "+mine\n" {
		t.Fatal("an existing filters file was changed")
	}
	if m, err := Migrate(t.TempDir(), "h"); m != nil || err != nil {
		t.Fatalf("a directory without the settings file: %v %v", m, err)
	}
}

// A filter built from the service variables bash set keeps its value, with a warning.
func TestMigrateFiltersSeeServiceVariables(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	dir := filepath.Join(t.TempDir(), "db")
	os.Mkdir(dir, 0o755)
	write(t, filepath.Join(dir, Legacy), `DUPLICACY_FILTERS_PATTERNS=("+${SERVICE}.sql" "+${DUPLICACY_SNAPSHOT_ID}.txt" "-*")
`, 0o644)
	m, err := Migrate(dir, "nas")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := ReadFilters(dir); !slices.Equal(got, []string{"+db.sql", "+nas-db.txt", "-*"}) {
		t.Fatalf("filters %q", got)
	}
	if len(m.Warnings) != 1 || !strings.Contains(m.Warnings[0], "used variables") {
		t.Fatalf("warnings %q", m.Warnings)
	}
}

func TestBrokenSymlinksAreErrors(t *testing.T) {
	dir := t.TempDir()
	os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, Filters))
	os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, PreBackup))
	if f, err := ReadFilters(dir); err == nil {
		t.Fatalf("a broken filters symlink read as %q", f)
	}
	if ok, err := Exists(dir, PreBackup); err == nil {
		t.Fatalf("a broken pre-backup symlink read as present=%v", ok)
	}
}

// A migration that cannot finish leaves the directory as it found it.
func TestMigrateRollsBack(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	rename = func(string, string) error { return errors.New("device or resource busy") }
	defer func() { rename = os.Rename }()
	dir := t.TempDir()
	write(t, filepath.Join(dir, Legacy), typical, 0o644)
	if _, err := Migrate(dir, "h"); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("got %v", err)
	}
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{Legacy}) {
		t.Fatalf("left behind %q", names)
	}
	rename = os.Rename
	if _, err := Migrate(dir, "h"); err != nil {
		t.Fatalf("retry after the failure: %v", err)
	}
}

func TestWrapperLogOutputLevel(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, Legacy), `service_specific_pre_backup_function() {
  printf 'disk nearly full\nno trailing newline' | log_output WARNING
  echo plain | log_output
}
`, 0o644)
	if _, err := Migrate(dir, "h"); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(filepath.Join(dir, PreBackup)).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if string(out) != "[WARNING] disk nearly full\n[WARNING] no trailing newline\nplain\n" {
		t.Fatalf("got %q", out)
	}
}

// A settings file that stops loading after migration fails the hook rather than running a
// half-initialized function.
func TestWrapperFailsWhenSettingsFailToLoad(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, Legacy), "service_specific_pre_backup_function() { echo ran; }\n", 0o644)
	if _, err := Migrate(dir, "h"); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(filepath.Join(dir, LegacyKept), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("false\n")
	f.Close()
	out, err := exec.Command(filepath.Join(dir, PreBackup)).CombinedOutput()
	if err == nil || strings.Contains(string(out), "ran") || !strings.Contains(string(out), "[ERROR]") {
		t.Fatalf("err %v, output %q", err, out)
	}
}

func TestMigrateWarnsAboutUnrewrittenFilters(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, Legacy), `DUPLICACY_FILTERS_PATTERNS=('i:^service-backup-settings\.sh$' '-?*')
`, 0o644)
	m, err := Migrate(dir, "h")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Warnings) != 1 || !strings.Contains(m.Warnings[0], "does not rewrite") {
		t.Fatalf("warnings %q", m.Warnings)
	}
}

// A hook someone besides its owner could change is refused (ADR 45): group- or
// world-writable, in such a directory, or a link to such a file; a link to a safe file runs.
func TestUnsafeHooksRefused(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0o755)
	write := func(p string, mode os.FileMode) {
		os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755)
		os.Chmod(p, mode)
	}
	write(filepath.Join(dir, PreBackup), 0o755)
	if ok, err := Exists(dir, PreBackup); !ok || err != nil {
		t.Fatalf("a safe hook: %v %v", ok, err)
	}
	write(filepath.Join(dir, PreBackup), 0o775)
	if _, err := Exists(dir, PreBackup); err == nil || !strings.Contains(err.Error(), "chmod go-w") {
		t.Fatalf("a group-writable hook: %v", err)
	}
	write(filepath.Join(dir, PreBackup), 0o755)
	os.Chmod(dir, 0o777)
	if _, err := Exists(dir, PreBackup); err == nil {
		t.Fatal("a hook in a world-writable directory ran")
	}
	os.Chmod(dir, 0o755)
	shared := t.TempDir()
	os.Chmod(shared, 0o755)
	write(filepath.Join(shared, "hook"), 0o755)
	os.Remove(filepath.Join(dir, PreBackup))
	os.Symlink(filepath.Join(shared, "hook"), filepath.Join(dir, PreBackup))
	if ok, err := Exists(dir, PreBackup); !ok || err != nil {
		t.Fatalf("a link to a safe hook: %v %v", ok, err)
	}
	os.Chmod(shared, 0o777)
	if _, err := Exists(dir, PreBackup); err == nil {
		t.Fatal("a link into a world-writable directory ran")
	}
}

// Any directory above a hook counts: whoever can write one can swap the directories below
// it. A sticky one (like /tmp) does not, since nobody can rename another user's entry in it.
func TestUnsafeAncestorRefused(t *testing.T) {
	top := t.TempDir()
	dir := filepath.Join(top, "hooks", "app")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, PreBackup), []byte("#!/bin/sh\n"), 0o755)
	os.Chmod(filepath.Join(top, "hooks"), 0o777)
	if _, err := Exists(dir, PreBackup); err == nil || !strings.Contains(err.Error(), filepath.Join(top, "hooks")) {
		t.Fatalf("a world-writable ancestor: %v", err)
	}
	os.Chmod(filepath.Join(top, "hooks"), os.ModeSticky|0o777)
	if ok, err := Exists(dir, PreBackup); !ok || err != nil {
		t.Fatalf("a sticky ancestor: %v %v", ok, err)
	}
}

// A chain of links is checked at every hop: a link that passes through a directory others
// can write is refused even when the chain ends at a safe file.
func TestLinkChainThroughWritableDir(t *testing.T) {
	top := t.TempDir()
	safe, shared, dir := filepath.Join(top, "safe"), filepath.Join(top, "shared"), filepath.Join(top, "svc")
	for _, d := range []string{safe, shared, dir} {
		os.Mkdir(d, 0o755)
	}
	os.WriteFile(filepath.Join(safe, "hook"), []byte("#!/bin/sh\n"), 0o755)
	os.Symlink(filepath.Join(safe, "hook"), filepath.Join(shared, "link"))
	os.Symlink(filepath.Join(shared, "link"), filepath.Join(dir, PreBackup))
	if ok, err := Exists(dir, PreBackup); !ok || err != nil {
		t.Fatalf("a chain through safe directories: %v %v", ok, err)
	}
	os.Chmod(shared, 0o777)
	if _, err := Exists(dir, PreBackup); err == nil || !strings.Contains(err.Error(), shared) {
		t.Fatalf("a chain through a world-writable directory: %v", err)
	}
}

// A ".." after a symlinked directory leaves the link's target, as the kernel resolves it,
// so the file checked is the file run.
func TestDotDotAfterLinkChecksWhatRuns(t *testing.T) {
	top := t.TempDir()
	trusted, unsafeDir, dir := filepath.Join(top, "trusted"), filepath.Join(top, "unsafe"), filepath.Join(top, "svc")
	for _, d := range []string{trusted, filepath.Join(unsafeDir, "sub"), dir} {
		os.MkdirAll(d, 0o755)
	}
	os.WriteFile(filepath.Join(trusted, "hook"), []byte("#!/bin/sh\n"), 0o755)
	os.WriteFile(filepath.Join(unsafeDir, "hook"), []byte("#!/bin/sh\n"), 0o755)
	os.Chmod(filepath.Join(unsafeDir, "hook"), 0o777)
	os.Symlink(filepath.Join(unsafeDir, "sub"), filepath.Join(trusted, "link"))
	os.Symlink(trusted+"/link/../hook", filepath.Join(dir, PreBackup)) // not Join, which would clean the ".." away
	if _, err := Exists(dir, PreBackup); err == nil || !strings.Contains(err.Error(), filepath.Join(unsafeDir, "hook")) {
		t.Fatalf("checked the wrong file: %v", err)
	}
}
