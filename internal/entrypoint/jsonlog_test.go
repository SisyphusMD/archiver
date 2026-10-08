package entrypoint

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestJSONLines(t *testing.T) {
	var out bytes.Buffer
	w := JSONLines("archiver", &out).(*jsonWriter)
	w.now = func() time.Time { return time.Date(2026, 10, 8, 3, 0, 0, 0, time.Local) }
	w.Write([]byte("[2026-10-08 03:00:01] [ERROR] [Service: garage] Backup to local failed: \"x\"\n--- Archiver"))
	w.Write([]byte(" Logs ---\n"))
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines: %q", len(lines), out.String())
	}
	var a, b jsonEntry
	if err := json.Unmarshal([]byte(lines[0]), &a); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal([]byte(lines[1]), &b)
	want := time.Date(2026, 10, 8, 3, 0, 1, 0, time.Local).Format(time.RFC3339)
	if a.Time != want || a.Level != "ERROR" || a.Service != "garage" || a.Log != "archiver" || a.Msg != `Backup to local failed: "x"` {
		t.Fatalf("parsed line: %+v", a)
	}
	if b.Level != "INFO" || b.Service != "" || b.Msg != "--- Archiver Logs ---" || b.Time == "" {
		t.Fatalf("other line: %+v", b)
	}
}
