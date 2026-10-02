package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMessageAndRotate(t *testing.T) {
	dir := t.TempDir()
	clock := time.Date(2026, 10, 1, 9, 5, 7, 0, time.Local)
	var out bytes.Buffer
	var notes []string
	l := &Log{Dir: dir, Basename: "archiver", ErrorTitle: "Backup Error", Stdout: &out, Now: func() time.Time { return clock },
		Notify: func(title, msg string) { notes = append(notes, title+": "+msg) }}
	old := filepath.Join(dir, "prior_logs", "archiver-2026-09-01_000000.log")
	os.MkdirAll(filepath.Dir(old), 0o755)
	os.WriteFile(old, nil, 0o644)
	os.Chtimes(old, clock.Add(-9*24*time.Hour), clock.Add(-9*24*time.Hour))
	recent := filepath.Join(dir, "prior_logs", "archiver-2026-09-25_000000.log")
	os.WriteFile(recent, nil, 0o644)
	os.Chtimes(recent, clock.Add(-6*24*time.Hour), clock.Add(-6*24*time.Hour))

	l.Rotate()
	l.Message(Info, "app", "hello")
	l.Message(Warning, "", "careful")
	l.Message(Error, "app", "broke")

	target, err := os.Readlink(l.Path())
	if err != nil || target != "prior_logs/archiver-2026-10-01_090507.log" {
		t.Fatalf("symlink -> %q, %v", target, err)
	}
	b, _ := os.ReadFile(l.Path())
	for _, want := range []string{
		"[2026-10-01 09:05:07] [INFO] [Service: app] hello\n",
		"[2026-10-01 09:05:07] [WARNING] [Service: archiver] careful\n",
		"[2026-10-01 09:05:07] [ERROR] [Service: app] broke\n",
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("log lacks %q:\n%s", want, b)
		}
	}
	if !strings.Contains(out.String(), "careful") || !strings.Contains(out.String(), "broke") || strings.Contains(out.String(), "hello") {
		t.Errorf("stdout should carry only warnings and errors:\n%s", out.String())
	}
	if l.Errors() != 1 || len(notes) != 1 || notes[0] != "Backup Error: [app] broke" {
		t.Errorf("errors %d, notes %q", l.Errors(), notes)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("a log older than eight days was kept")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Error("a six-day-old log was deleted")
	}
}

func TestDuration(t *testing.T) {
	for in, want := range map[int64]string{
		0: "0 seconds", 1: "1 second", 61: "1 minute and 1 second", 3600: "1 hour",
		90061: "1 day, 1 hour, 1 minute, and 1 second", 172800: "2 days",
	} {
		if got := Duration(in); got != want {
			t.Errorf("Duration(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestWriterLogsLines(t *testing.T) {
	dir := t.TempDir()
	l := &Log{Dir: dir, Basename: "archiver"}
	w := l.Writer(Info, "svc")
	w.Write([]byte("one\ntw"))
	w.Write([]byte("o\nthree"))
	w.Close()
	b, _ := os.ReadFile(l.Path())
	if n := strings.Count(string(b), "[Service: svc]"); n != 3 || !strings.Contains(string(b), "] two\n") || !strings.Contains(string(b), "] three\n") {
		t.Fatalf("got:\n%s", b)
	}
}

func TestHookLinesLevels(t *testing.T) {
	dir := t.TempDir()
	l := &Log{Dir: dir, Basename: "archiver"}
	l.HookLines("db", strings.NewReader("dumping\n[WARNING] slow\n[ERROR] dump failed\n[INFO] stays as written\n"))
	b, _ := os.ReadFile(l.Path())
	for _, want := range []string{"[INFO] [Service: db] dumping", "[WARNING] [Service: db] slow", "[ERROR] [Service: db] dump failed", "[INFO] [Service: db] [INFO] stays as written"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("log lacks %q:\n%s", want, b)
		}
	}
	if l.Errors() != 1 {
		t.Errorf("errors = %d, want the [ERROR] line counted", l.Errors())
	}
}
