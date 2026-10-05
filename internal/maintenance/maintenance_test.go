package maintenance

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
)

// The record keeps each storage's other fields when one changes, and reads back what bash
// wrote ("<name> <check> <prune> <exhaustive>").
func TestState(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".maintenance-state")
	os.WriteFile(p, []byte("local 10 20 30\noffsite 1 2 3\n"), 0o600)
	s := readState(p)
	s.set("local", fieldPrune, 99)
	s.set("new", fieldCheck, 5)
	if err := s.write(p); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "local 10 99 30\nnew 5 0 0\noffsite 1 2 3\n" {
		t.Fatalf("got %q", b)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
}

func TestExhaustiveDue(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	day := int64(86400)
	r := &Run{cfg: &config.Config{PruneExhaustiveFrequency: "monthly"}, Now: func() time.Time { return now }}
	s := state{}
	if !r.exhaustiveDue(s, "local") {
		t.Error("never run: due")
	}
	s.set("local", fieldExhaustive, now.Unix()-29*day)
	if r.exhaustiveDue(s, "local") {
		t.Error("29 days after a monthly exhaustive: not due")
	}
	s.set("local", fieldExhaustive, now.Unix()-30*day+1800)
	if !r.exhaustiveDue(s, "local") {
		t.Error("within the hour of grace: due")
	}
	r.ForceExhaustive = true
	s.set("local", fieldExhaustive, now.Unix())
	if !r.exhaustiveDue(s, "local") {
		t.Error("forced: due")
	}
	r.ForceExhaustive = false
	r.cfg.PruneExhaustiveFrequency = "off"
	s.set("local", fieldExhaustive, 0)
	if r.exhaustiveDue(s, "local") {
		t.Error("off: never due")
	}
}
