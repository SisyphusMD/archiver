package hostkeys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinSharesOneRecord(t *testing.T) {
	logs := filepath.Join(t.TempDir(), "logs")
	shared := filepath.Join(logs, ".ssh", "duplicacy_known_hosts")
	a, b := filepath.Join(t.TempDir(), ".duplicacy"), filepath.Join(t.TempDir(), ".duplicacy")
	os.MkdirAll(a, 0o700)
	os.MkdirAll(b, 0o700)
	// A repository that already pinned two servers keeps both; one pinning a different key
	// for a server the record knows is refused until one of them is corrected.
	os.WriteFile(filepath.Join(a, "known_hosts"), []byte("10.0.0.1:22 ssh-ed25519 AAAAone\n10.0.0.2:2222 ssh-ed25519 AAAAtwo\n"), 0o600)
	if err := Pin(shared, a); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(b, "known_hosts"), []byte("10.0.0.1:22 ssh-ed25519 AAAAimpostor\n10.0.0.3:22 ssh-rsa AAAAthree\n"), 0o600)
	if err := Pin(shared, b); err == nil || !strings.Contains(err.Error(), "10.0.0.1:22") {
		t.Fatalf("a repository pinning a different key was linked: %v", err)
	}
	if fi, err := os.Lstat(filepath.Join(b, "known_hosts")); err != nil || !fi.Mode().IsRegular() {
		t.Fatal("the refused repository's own record was not left in place")
	}
	os.WriteFile(filepath.Join(b, "known_hosts"), []byte("10.0.0.1:22 ssh-ed25519 AAAAone\n10.0.0.3:22 ssh-rsa AAAAthree\n"), 0o600)
	if err := Pin(shared, b); err != nil {
		t.Fatal(err)
	}
	if err := Pin(shared, b); err != nil {
		t.Fatal("a second pin of a linked repository failed:", err)
	}
	got, _ := os.ReadFile(shared)
	want := "10.0.0.1:22 ssh-ed25519 AAAAone\n10.0.0.2:2222 ssh-ed25519 AAAAtwo\n10.0.0.3:22 ssh-rsa AAAAthree\n"
	if string(got) != want {
		t.Fatalf("record:\n%s\nwant:\n%s", got, want)
	}
	for _, d := range []string{a, b} {
		if target, err := os.Readlink(filepath.Join(d, "known_hosts")); err != nil || target != shared {
			t.Errorf("%s is not linked to the record: %q %v", d, target, err)
		}
	}
	if fi, _ := os.Stat(shared); fi.Mode().Perm() != 0o600 {
		t.Errorf("record mode %o", fi.Mode().Perm())
	}
}

func TestPinRefusesWhatOthersPlanted(t *testing.T) {
	logs := filepath.Join(t.TempDir(), "logs")
	ssh := filepath.Join(logs, ".ssh")
	os.MkdirAll(ssh, 0o700)
	repo := filepath.Join(t.TempDir(), ".duplicacy")
	os.MkdirAll(repo, 0o700)
	planted := filepath.Join(t.TempDir(), "elsewhere")
	os.WriteFile(planted, []byte("10.0.0.1:22 ssh-ed25519 AAAAimpostor\n"), 0o600)
	shared := filepath.Join(ssh, "duplicacy_known_hosts")
	os.Symlink(planted, shared)
	if err := Pin(shared, repo); err == nil {
		t.Error("a record that is a link was used")
	}
	os.Remove(shared)
	os.Chmod(ssh, 0o777)
	if err := Pin(shared, repo); err == nil || !strings.Contains(err.Error(), "only Archiver can write") {
		t.Errorf("a directory others can write was used: %v", err)
	}
}

func TestSeedKeepsEveryExistingPin(t *testing.T) {
	logs := filepath.Join(t.TempDir(), "logs")
	shared := filepath.Join(logs, ".ssh", "duplicacy_known_hosts")
	dir := t.TempDir()
	write := func(name, content string, mode os.FileMode) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(content), mode)
		os.Chmod(p, mode)
		return p
	}
	old := write("old", "10.0.0.1:22 ssh-ed25519 AAAAone\n", 0o600)
	other := write("other", "10.0.0.1:22 ssh-ed25519 AAAAchanged\n10.0.0.2:22 ssh-ed25519 AAAAtwo\n", 0o600)
	open := write("open", "10.0.0.9:22 ssh-ed25519 AAAAplanted\n", 0o666)
	linked := filepath.Join(dir, "linked")
	os.Symlink(shared, linked)
	conflicts, err := Seed(shared, []string{linked, old, filepath.Join(dir, "missing"), other, open})
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 || !strings.Contains(conflicts[0], "10.0.0.1:22") {
		t.Errorf("conflicts: %q", conflicts)
	}
	got, _ := os.ReadFile(shared)
	if want := "10.0.0.1:22 ssh-ed25519 AAAAone\n10.0.0.2:22 ssh-ed25519 AAAAtwo\n"; string(got) != want {
		t.Errorf("record:\n%s\nwant:\n%s", got, want)
	}
}

func TestPinIgnoresARecordOthersCouldWrite(t *testing.T) {
	shared := filepath.Join(t.TempDir(), "logs", ".ssh", "duplicacy_known_hosts")
	repo := filepath.Join(t.TempDir(), ".duplicacy")
	os.MkdirAll(repo, 0o700)
	local := filepath.Join(repo, "known_hosts")
	os.WriteFile(local, []byte("10.0.0.9:22 ssh-ed25519 AAAAplanted\n"), 0o600)
	os.Chmod(local, 0o666)
	if err := Pin(shared, repo); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(shared); len(got) != 0 {
		t.Errorf("a record others could write added:\n%s", got)
	}
}

func TestCopyIfMissing(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "home-known_hosts"), filepath.Join(dir, "logs", ".ssh", "known_hosts")
	os.MkdirAll(filepath.Dir(filepath.Dir(dst)), 0o755)
	os.WriteFile(src, []byte("host ssh-ed25519 AAAAkit\n"), 0o600)
	if err := CopyIfMissing(src, dst); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(src, []byte("host ssh-ed25519 AAAAlater\n"), 0o600)
	if err := CopyIfMissing(src, dst); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "host ssh-ed25519 AAAAkit\n" {
		t.Errorf("got %q", got)
	}
}
