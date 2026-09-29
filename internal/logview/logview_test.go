package logview

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/layout"
)

var fast = Options{Poll: 5 * time.Millisecond, FindFor: 50 * time.Millisecond, Linger: 20 * time.Millisecond}

// syncBuf is written by Follow while the test reads it.
type syncBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func setup(t *testing.T, logContent string) layout.Layout {
	t.Helper()
	l := layout.Layout{Root: t.TempDir(), Lock: t.TempDir()}
	for _, d := range []string{l.LogDir(), filepath.Dir(l.Logo())} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, l.Logo(), "LOGO\n")
	rotate(t, l, "run1.log", logContent)
	write(t, l.BackupLock(), fmt.Sprintf("%d service:/srv/app backup\n1 running\n", os.Getpid()))
	return l
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendTo(t *testing.T, p, s string) {
	t.Helper()
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString(s)
}

// rotate starts a new run's log the way the pipeline does: a new file, and archiver.log
// replaced by a symlink to it.
func rotate(t *testing.T, l layout.Layout, name, content string) {
	t.Helper()
	p := filepath.Join(l.LogDir(), name)
	write(t, p, content)
	tmp := l.BackupLog() + ".tmp"
	os.Remove(tmp)
	if err := os.Symlink(p, tmp); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, l.BackupLog()); err != nil {
		t.Fatal(err)
	}
}

func eventually(t *testing.T, buf *syncBuf, want string) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if strings.Contains(buf.String(), want) {
			return
		}
	}
	t.Fatalf("never saw %q in:\n%s", want, buf.String())
}

func start(t *testing.T, l layout.Layout, ctx context.Context) (*syncBuf, <-chan int) {
	buf := &syncBuf{}
	done := make(chan int, 1)
	go func() { done <- Follow(ctx, buf, l, fast) }()
	return buf, done
}

func TestFollowsUntilTheBackupEnds(t *testing.T) {
	var old strings.Builder
	for i := range 30 {
		fmt.Fprintf(&old, "old %d\n", i)
	}
	l := setup(t, old.String())
	buf, done := start(t, l, context.Background())

	eventually(t, buf, "old 29")
	appendTo(t, filepath.Join(l.LogDir(), "run1.log"), "new line\n")
	eventually(t, buf, "new line")
	os.Remove(l.BackupLock())

	if code := <-done; code != Done {
		t.Fatalf("exit %d, want %d", code, Done)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "LOGO\nold 20\n") || strings.Contains(out, "old 19\n") {
		t.Errorf("want the logo then the last 10 lines:\n%s", out)
	}
	if !strings.HasSuffix(out, "Backup completed. Exiting log viewer.\n") {
		t.Errorf("missing the completion line:\n%s", out)
	}
}

func TestFollowsARotatedLog(t *testing.T) {
	l := setup(t, "first run\n")
	buf, done := start(t, l, context.Background())
	eventually(t, buf, "first run")

	rotate(t, l, "run2.log", "second run\n")
	eventually(t, buf, "Log file has changed. Following the new log file...\nsecond run")
	os.Remove(l.BackupLock())
	<-done
}

func TestInterruptEndsTheViewAtOnce(t *testing.T) {
	l := setup(t, "running\n")
	ctx, cancel := context.WithCancel(context.Background())
	buf, done := start(t, l, ctx)
	eventually(t, buf, "running")

	cancel()
	select {
	case code := <-done:
		if code != Interrupted {
			t.Errorf("exit %d, want %d", code, Interrupted)
		}
	case <-time.After(time.Second):
		t.Fatal("still following a second after the interrupt")
	}
}

func TestNoBackup(t *testing.T) {
	l := setup(t, "old\n")
	os.Remove(l.BackupLock())
	buf, done := start(t, l, context.Background())
	if code := <-done; code != NoBackup || !strings.Contains(buf.String(), "No running backup found") {
		t.Errorf("exit %d:\n%s", code, buf.String())
	}
}

func TestTailOffset(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	for _, c := range []struct {
		content string
		n       int
		want    string
	}{
		{"a\nb\nc\n", 2, "b\nc\n"},
		{"a\nb\nc", 2, "b\nc"},
		{"a\nb\n", 10, "a\nb\n"},
		{"", 10, ""},
	} {
		write(t, p, c.content)
		f, _ := os.Open(p)
		off, err := tailOffset(f, c.n)
		f.Close()
		if err != nil || c.content[off:] != c.want {
			t.Errorf("tailOffset(%q, %d) = %d (%q), want %q", c.content, c.n, off, c.content[off:], c.want)
		}
	}
}
