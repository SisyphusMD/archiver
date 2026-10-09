package logging

import (
	"bytes"
	"fmt"
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
	l := &Log{Dir: dir, Basename: "archiver", Stdout: &out, Now: func() time.Time { return clock }}
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
	if l.Errors() != 1 || l.Summary() != "[app] broke" {
		t.Errorf("errors %d, summary %q", l.Errors(), l.Summary())
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

// Two runs starting in the same second each get their own file.
func TestRotateSameSecond(t *testing.T) {
	dir := t.TempDir()
	clock := time.Date(2026, 10, 2, 12, 0, 0, 0, time.Local)
	l := &Log{Dir: dir, Basename: "archiver", Now: func() time.Time { return clock }}
	l.Rotate()
	l.Message(Error, "", "first run failed")
	l.Rotate()
	b, _ := os.ReadFile(l.Path())
	if strings.Contains(string(b), "first run failed") {
		t.Fatalf("the second run's log carries the first run's error:\n%s", b)
	}
	if target, _ := os.Readlink(l.Path()); target != "prior_logs/archiver-2026-10-02_120000-1.log" {
		t.Fatalf("symlink -> %q", target)
	}
}

// An unwritable log notifies once, however many lines fail to be written.
func TestUnwritableLogNotifiesOnce(t *testing.T) {
	var sent []string
	l := &Log{Dir: filepath.Join(t.TempDir(), "missing"), Basename: "archiver",
		Unwritable: func(msg string) { sent = append(sent, msg) }}
	l.Unnotified(Info, "", "a notification's outcome")
	l.Message(Info, "", "one")
	l.Message(Info, "", "two")
	if len(sent) != 1 || !strings.Contains(sent[0], "Cannot write") || l.Errors() != 3 {
		t.Fatalf("sent %q, errors %d", sent, l.Errors())
	}
}

// A run's summary carries its first error lines and counts the rest; a notification's own
// failure is left out.
func TestSummary(t *testing.T) {
	l := &Log{Dir: t.TempDir(), Basename: "archiver"}
	l.Unnotified(Error, "", "pushover failed")
	for i := range 12 {
		l.Message(Error, "app", fmt.Sprintf("failure %d", i))
	}
	s := l.Summary()
	if !strings.HasPrefix(s, "[app] failure 0\n") || !strings.Contains(s, "[app] failure 9\n") || strings.Contains(s, "failure 10") ||
		!strings.HasSuffix(s, "... and 2 more in archiver.log.") || strings.Contains(s, "pushover") {
		t.Fatalf("summary:\n%s", s)
	}
}

// Writable is told at the run's first line written and again after a failure clears.
func TestWritableAfterFailure(t *testing.T) {
	dir := t.TempDir()
	var events []string
	l := &Log{Dir: dir, Basename: "archiver",
		Unwritable: func(string) { events = append(events, "unwritable") },
		Writable:   func() { events = append(events, "writable") }}
	l.Message(Info, "", "one")
	l.Message(Info, "", "two")
	// A directory where the log file belongs fails every write, root's included.
	os.Remove(l.Path())
	os.Mkdir(l.Path(), 0o700)
	l.Message(Info, "", "three")
	l.Message(Info, "", "three again")
	os.Remove(l.Path())
	l.Message(Info, "", "four")
	l.Message(Info, "", "five")
	if strings.Join(events, " ") != "writable unwritable writable" {
		t.Fatalf("events %q", events)
	}
}
