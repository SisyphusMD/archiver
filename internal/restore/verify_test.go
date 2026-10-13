package restore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A restored copy that matches its revision passes; each way it can differ is named.
func TestVerifyRestored(t *testing.T) {
	dir := t.TempDir()
	mtime := time.Unix(1791915317, 0)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o750)
	os.MkdirAll(filepath.Join(dir, ".duplicacy"), 0o700) // duplicacy's own, in every restore
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi\n"), 0o640)
	os.WriteFile(filepath.Join(dir, "sub", "b"), []byte("x\n"), 0o600)
	os.Symlink("a.txt", filepath.Join(dir, "link"))
	for _, p := range []string{"a.txt", "sub/b"} {
		os.Chmod(filepath.Join(dir, p), map[string]os.FileMode{"a.txt": 0o640, "sub/b": 0o600}[p])
		os.Chtimes(filepath.Join(dir, p), mtime, mtime)
	}
	os.Chmod(filepath.Join(dir, "sub"), 0o750)
	os.Chtimes(filepath.Join(dir, "sub"), mtime, mtime)
	entries := []revisionEntry{
		{Path: "a.txt", Size: 3, Time: mtime.Unix(), Mode: 0o640},
		{Path: "link", Mode: uint32(os.ModeSymlink | 0o777), Link: "a.txt"},
		{Path: "sub/", Time: mtime.Unix(), Mode: uint32(os.ModeDir | 0o750)},
		{Path: "sub/b", Size: 2, Time: mtime.Unix(), Mode: 0o600},
	}
	revision := func(es []revisionEntry) string {
		b, _ := json.Marshal(map[string]any{"id": "h-app", "revision": 3, "files": es, "chunks": []string{"c"}})
		return string(b)
	}
	files, problems, err := verifyRestored(dir, strings.NewReader(revision(entries)), false, 10)
	if err != nil || len(problems) != 0 || files != 2 {
		t.Fatalf("a matching copy: %d files, %q, %v", files, problems, err)
	}

	changed := append([]revisionEntry{}, entries...)
	changed[0].Size = 4                                                              // a.txt differs in size
	changed[1].Link = "elsewhere"                                                    // the link points elsewhere
	changed[3].Mode = 0o644                                                          // sub/b differs in mode
	changed = append(changed, revisionEntry{Path: "gone.txt", Size: 1, Mode: 0o600}) // never restored
	os.WriteFile(filepath.Join(dir, "extra"), nil, 0o600)                            // not in the revision
	_, problems, err = verifyRestored(dir, strings.NewReader(revision(changed)), false, 10)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(problems, "\n")
	for _, want := range []string{"a.txt is 3 bytes, the revision has 4", `link points to "a.txt", the revision has "elsewhere"`,
		"sub/b has mode", "gone.txt is missing", "extra was restored but the revision does not have it"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if _, problems, _ = verifyRestored(dir, strings.NewReader(revision(changed)), false, 2); len(problems) != 3 || problems[2] != "and 3 more" {
		t.Errorf("limited report: %q", problems)
	}
	if _, _, err := verifyRestored(dir, strings.NewReader(`{"id":"x"}`), false, 10); err == nil {
		t.Error("a revision without a file list passed")
	}
}

func TestVerifyRestoredMetadataAndType(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file"), []byte("x"), 0o600)
	os.Mkdir(filepath.Join(dir, "sub"), 0o700)
	st, _ := os.Lstat(filepath.Join(dir, "file"))
	sub, _ := os.Lstat(filepath.Join(dir, "sub"))
	for _, tc := range []struct {
		name   string
		entry  revisionEntry
		owners bool
		want   string
	}{
		{"mtime", revisionEntry{Path: "file", Size: 1, Mode: 0o600, Time: st.ModTime().Unix() - 1}, false, "file was modified"},
		{"directory mtime", revisionEntry{Path: "sub/", Mode: uint32(os.ModeDir | 0o700), Time: sub.ModTime().Unix() - 1}, false, "sub was modified"},
		{"type", revisionEntry{Path: "file", Mode: uint32(os.ModeDir | 0o700)}, false, "file is a file, the revision has a directory"},
		{"owner", revisionEntry{Path: "file", Size: 1, Mode: 0o600, Time: st.ModTime().Unix(), UID: -1, GID: -1}, true, "file is owned by"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(map[string]any{"files": []revisionEntry{tc.entry}})
			_, problems, err := verifyRestored(dir, strings.NewReader(string(b)), tc.owners, 10)
			if err != nil || !strings.Contains(strings.Join(problems, ";"), tc.want) {
				t.Fatalf("%v, %v", problems, err)
			}
		})
	}
}

func TestVerifyRestoredBadRecord(t *testing.T) {
	for _, record := range []string{
		`{"files":[]`, `{"files":[]} trailing`, `{"files":[],"files":[]}`,
		`{"files":[{"path":"../outside"}]}`, `{"files":[{"path":"/outside"}]}`,
		`{"files":[{"path":"same"},{"path":"same"}]}`,
	} {
		if _, _, err := verifyRestored(t.TempDir(), strings.NewReader(record), false, 10); err == nil {
			t.Errorf("accepted %s", record)
		}
	}
}
