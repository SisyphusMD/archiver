package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/layout"
)

// fakeDuplicacy records each command and the directory it ran in. FAKE_INIT_FAIL makes
// init fail.
const fakeDuplicacy = `#!/bin/sh
echo "$(pwd) $*" >> "$FAKE_CALLS"
case "$2" in
init) [ -n "$FAKE_INIT_FAIL" ] && { echo "init failed"; exit 1; }
  mkdir -p .duplicacy && echo "$*" > .duplicacy/preferences ;;
backup) echo "Backup for /x at revision 1 completed" ;;
esac
exit 0
`

// newBackup makes a backup of one service, app, in root, to a local storage there.
func newBackup(t *testing.T, root string, initFails bool) (b *Backup, svc, calls string) {
	t.Helper()
	svc = filepath.Join(root, "services", "app")
	secrets := filepath.Join(root, "secrets")
	for _, d := range []string{svc, secrets, filepath.Join(root, "store"), filepath.Join(root, "lock"), filepath.Join(root, "logs")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(svc, "file.txt"), []byte("data"), 0o644)
	os.WriteFile(filepath.Join(secrets, "storage_password"), []byte("password1"), 0o600)
	os.WriteFile(filepath.Join(secrets, "rsa_passphrase"), []byte("rp"), 0o600)
	bin := filepath.Join(root, "duplicacy")
	os.WriteFile(bin, []byte(fakeDuplicacy), 0o755)
	calls = filepath.Join(root, "calls")
	vars := map[string]string{
		"SERVICE_DIRECTORIES":         filepath.Join(root, "services") + "/*/",
		"STORAGE_TARGET_1_NAME":       "local",
		"STORAGE_TARGET_1_TYPE":       "local",
		"STORAGE_TARGET_1_LOCAL_PATH": filepath.Join(root, "store"),
	}
	environ := []string{"PATH=" + os.Getenv("PATH"), "FAKE_CALLS=" + calls}
	if initFails {
		environ = append(environ, "FAKE_INIT_FAIL=1")
	}
	b = &Backup{
		Layout:    layout.Layout{Root: root, Lock: filepath.Join(root, "lock")},
		Source:    config.Source{Getenv: func(k string) string { return vars[k] }, SecretsDir: secrets},
		Environ:   environ,
		Hostname:  "h",
		Stdout:    &strings.Builder{},
		Stderr:    &strings.Builder{},
		Duplicacy: bin,
		Probe:     func(config.Target, bool) (bool, error) { return true, nil },
	}
	return b, svc, calls
}

// Every duplicacy command of a backup runs from Archiver's own repository, which init
// points at the service directory: a .duplicacy the service's owner planted there (here a
// link to preferences that would back up something else) is never read or written.
func TestBackupRunsFromItsOwnRepository(t *testing.T) {
	root := t.TempDir()
	b, svc, calls := newBackup(t, root, false)
	planted := filepath.Join(root, "planted")
	os.MkdirAll(planted, 0o755)
	os.WriteFile(filepath.Join(planted, "preferences"), []byte(`[{"name":"local","repository":"/etc"}]`), 0o644)
	os.Symlink(planted, filepath.Join(svc, ".duplicacy"))
	if code := b.Run(); code != 0 {
		t.Fatalf("exit %d", code)
	}
	work := b.Layout.ServiceRepo("h-app", svc)
	got, _ := os.ReadFile(calls)
	lines := strings.Split(strings.TrimSpace(string(got)), "\n")
	var cmds []string
	for _, l := range lines {
		dir, cmd, _ := strings.Cut(l, " ")
		if dir != work {
			t.Errorf("ran in %s, not %s: %s", dir, work, cmd)
		}
		cmds = append(cmds, strings.Fields(cmd)[1])
	}
	if strings.Join(cmds, " ") != "init list backup" {
		t.Fatalf("commands %v", cmds)
	}
	if !strings.Contains(lines[0], " -repository "+svc+" h-app ") {
		t.Fatalf("init does not point the repository at the service: %s", lines[0])
	}
	if b, _ := os.ReadFile(filepath.Join(planted, "preferences")); !strings.Contains(string(b), "/etc") {
		t.Fatalf("the planted preferences were changed: %s", b)
	}
	if entries, _ := os.ReadDir(planted); len(entries) != 1 {
		t.Fatalf("something was written through the planted link: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(work, ".duplicacy", "filters")); err != nil {
		t.Fatalf("filters not written to the repository: %v", err)
	}
	if fi, err := os.Lstat(work); err != nil || !layout.Owned(fi) || fi.Mode().Perm() != 0o700 {
		t.Fatalf("repository %v, %v; want an owner-only directory", fi, err)
	}
}

// A failed init fails the service: what preferences are there, this run did not write.
func TestFailedInitFailsTheService(t *testing.T) {
	b, _, calls := newBackup(t, t.TempDir(), true)
	if code := b.Run(); code == 0 {
		t.Fatal("a backup whose init failed exited 0")
	}
	got, _ := os.ReadFile(calls)
	if strings.Contains(string(got), " backup ") {
		t.Fatalf("backed up after a failed init:\n%s", got)
	}
}

// A repository someone else could have changed is refused, not used.
func TestForeignRepositoryRefused(t *testing.T) {
	for name, tamper := range map[string]func(work string){
		"group-writable": func(work string) { os.Chmod(work, 0o770) },
		"a link": func(work string) {
			other := work + "-elsewhere"
			os.Rename(work, other)
			os.Symlink(other, work)
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			b, svc, calls := newBackup(t, root, false)
			if code := b.Run(); code != 0 {
				t.Fatalf("first run: exit %d", code)
			}
			os.Remove(calls)
			tamper(b.Layout.ServiceRepo("h-app", svc))
			b, _, _ = newBackup(t, root, false)
			if code := b.Run(); code == 0 {
				t.Fatal("a backup through a tampered repository exited 0")
			}
			if got, _ := os.ReadFile(calls); len(got) > 0 {
				t.Fatalf("duplicacy ran:\n%s", got)
			}
			if log, _ := os.ReadFile(b.Layout.BackupLog()); !strings.Contains(string(log), "Cannot prepare the Duplicacy repository") {
				t.Fatalf("the refusal is not logged:\n%s", log)
			}
		})
	}
}

// Two service directories with one basename share a snapshot ID, but not a repository: a
// resume point one leaves must never be taken up by the other's backup.
func TestSameBasenameDirectoriesHaveTheirOwnRepositories(t *testing.T) {
	root := t.TempDir()
	b, svc, calls := newBackup(t, root, false)
	other := filepath.Join(root, "elsewhere", "app")
	os.MkdirAll(other, 0o755)
	src := b.Source
	b.Source.Getenv = func(k string) string {
		if k == "SERVICE_DIRECTORIES" {
			return src.Getenv(k) + ":" + other
		}
		return src.Getenv(k)
	}
	if code := b.Run(); code != 0 {
		t.Fatalf("exit %d", code)
	}
	got, _ := os.ReadFile(calls)
	backups := map[string]bool{}
	for _, l := range strings.Split(strings.TrimSpace(string(got)), "\n") {
		if dir, cmd, _ := strings.Cut(l, " "); strings.Fields(cmd)[1] == "backup" {
			backups[dir] = true
		}
	}
	a, c := b.Layout.ServiceRepo("h-app", svc), b.Layout.ServiceRepo("h-app", other)
	if a == c || !backups[a] || !backups[c] || len(backups) != 2 {
		t.Fatalf("backups ran in %v, want one each in %s and %s", backups, a, c)
	}
}
