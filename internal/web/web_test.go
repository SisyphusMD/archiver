package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
)

func TestPage(t *testing.T) {
	root := t.TempDir()
	l := layout.Layout{Root: root, Lock: filepath.Join(root, "lock")}
	os.MkdirAll(l.LogDir(), 0o755)
	os.MkdirAll(filepath.Join(root, "srv", "app"), 0o755)
	lockstate.WriteBackupState(l.BackupState(), lockstate.BackupState{Services: map[string]lockstate.ServiceResult{
		filepath.Join(root, "srv", "app"): {LastAttempt: 1, LastSuccess: 1, Result: "success", Revision: 3}}})
	os.WriteFile(filepath.Join(l.LogDir(), "archiver.log"), []byte("line one\n<script>alert(1)</script>\n"), 0o644)
	env := map[string]string{"SERVICE_DIRECTORIES": filepath.Join(root, "srv") + "/*/"}
	h := Handler(l, func(k string) string { return env[k] })

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/?log=../../etc/passwd", nil))
	body := rec.Body.String()
	for _, want := range []string{"Backup health", ">OK<", "<td>app</td>", "line one", "&lt;script&gt;alert(1)&lt;/script&gt;"} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(body, "<script>") {
		t.Error("a log line was not escaped")
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Error("no CSP")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST answered %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/status.json", nil))
	if !strings.Contains(rec.Body.String(), `"backup_health"`) {
		t.Error("no JSON")
	}
}

func TestRedact(t *testing.T) {
	in := `Failed to send apprise notification (Post "http://user:pw@apprise:8000/notify/abc123": dial tcp: refused)`
	got := redact(in)
	if strings.Contains(got, "abc123") || strings.Contains(got, "pw@") || !strings.Contains(got, "/notify/***") || !strings.Contains(got, "http://***@apprise") {
		t.Fatalf("%s", got)
	}
}
