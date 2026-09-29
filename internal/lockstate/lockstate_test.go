package lockstate

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func write(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// The bash pipelines write exactly this shape (lib/core/lockfile.sh).
func TestReadLock(t *testing.T) {
	p := write(t, "4242 service:/srv/app backup\n1000 running\n1100 paused\n1160 running\n1200 paused\n")
	l, ok, err := ReadLock(p)
	if err != nil || !ok {
		t.Fatalf("ReadLock = %v, %v", ok, err)
	}
	if l.PID != 4242 || l.Context != "service:/srv/app" || l.Stage != "backup" {
		t.Errorf("header = %d %q %q", l.PID, l.Context, l.Stage)
	}
	want := []Event{{1000, "running"}, {1100, "paused"}, {1160, "running"}, {1200, "paused"}}
	if !slices.Equal(l.Events, want) {
		t.Errorf("events = %v, want %v", l.Events, want)
	}
	if !l.Paused() || l.StartedAt() != 1000 {
		t.Errorf("Paused = %v, StartedAt = %d", l.Paused(), l.StartedAt())
	}
}

func TestReadLockMissing(t *testing.T) {
	_, ok, err := ReadLock(filepath.Join(t.TempDir(), "absent"))
	if ok || err != nil {
		t.Fatalf("missing lock: ok=%v err=%v", ok, err)
	}
}

func TestAlive(t *testing.T) {
	if !(Lock{PID: os.Getpid()}).Alive() {
		t.Error("this process should count as alive")
	}
	for _, pid := range []int{0, -1, 1 << 30} {
		if (Lock{PID: pid}).Alive() {
			t.Errorf("PID %d should not count as alive", pid)
		}
	}
}

func TestReadMaintenance(t *testing.T) {
	p := write(t, "local 1790539383 1790539442 1789071029\n\nbackblaze 5 6\n")
	got, err := ReadMaintenance(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []Storage{{"local", 1790539383, 1790539442, 1789071029}, {"backblaze", 5, 6, 0}}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	none, err := ReadMaintenance(filepath.Join(t.TempDir(), "absent"))
	if none != nil || err != nil {
		t.Errorf("missing record: %v, %v", none, err)
	}
}
