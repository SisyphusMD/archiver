package web

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/deepcheck"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/status"
)

func TestPage(t *testing.T) {
	root := t.TempDir()
	l := layout.Layout{Root: root, Lock: filepath.Join(root, "lock")}
	os.MkdirAll(l.LogDir(), 0o755)
	os.MkdirAll(filepath.Join(root, "srv", "app"), 0o755)
	lockstate.WriteBackupState(l.BackupState(), lockstate.BackupState{Services: map[string]lockstate.ServiceResult{
		filepath.Join(root, "srv", "app"): {LastAttempt: 1, LastSuccess: 1, Result: "success", Revision: 3}}})
	os.WriteFile(filepath.Join(l.LogDir(), "archiver.log"), []byte("line one\n<script>alert(1)</script>\n"), 0o644)
	// httptest addresses requests to example.com.
	env := map[string]string{"SERVICE_DIRECTORIES": filepath.Join(root, "srv") + "/*/", "WEB_ALLOWED_HOSTS": "example.com"}
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
	// A name the deployment did not list (a rebound attacker's) gets nothing.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/status.json", nil)
	req.Host = "attacker.example:8470"
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMisdirectedRequest || strings.Contains(rec.Body.String(), "backup_health") {
		t.Errorf("an unknown name was answered: %d", rec.Code)
	}
}

func TestRedact(t *testing.T) {
	in := `Failed to send apprise notification (Post "http://user:pw@apprise:8000/notify/abc123": dial tcp: refused)`
	got := redact(in)
	if strings.Contains(got, "abc123") || strings.Contains(got, "pw@") || !strings.Contains(got, "/notify/***") || !strings.Contains(got, "http://***@apprise") {
		t.Fatalf("%s", got)
	}
}

func TestPageDrillResults(t *testing.T) {
	result := lockstate.DrillResult{At: 1, OK: true, Revision: 7}
	named := map[string]*lockstate.DrillState{
		"GARAGE": {Results: map[string]map[string]lockstate.DrillResult{"offsite": {"db": result}}},
		"EMPTY":  {}, "NIL": nil,
	}
	for _, withDefault := range []bool{false, true} {
		t.Run(fmt.Sprint(withDefault), func(t *testing.T) {
			s := status.Snapshot{NamedDrills: named, Drill: status.Run{Running: true, Paused: true, Context: "drill:GARAGE"}}
			if withDefault {
				s.Drills = map[string]map[string]lockstate.DrillResult{"local": {"app": result}}
			}
			var out bytes.Buffer
			if err := page.Execute(&out, view{S: s, Now: time.Unix(10, 0)}); err != nil {
				t.Fatal(err)
			}
			body := out.String()
			for _, want := range []string{"Restore drill GARAGE: running, paused", "<td>GARAGE</td><td>offsite</td><td>db</td>", "passed, revision 7"} {
				if !strings.Contains(body, want) {
					t.Errorf("page lacks %q", want)
				}
			}
			if strings.Contains(body, "No drills") {
				t.Error("named results shown as no drills")
			}
			if withDefault && !strings.Contains(body, "<td>default</td><td>local</td><td>app</td>") {
				t.Error("default results missing alongside named results")
			}
		})
	}
}

func TestPageDeepChecksAndKitDrills(t *testing.T) {
	s := status.Snapshot{
		Drill: status.Run{Running: true, Context: "kit-drill:offsite"},
		DeepChecks: map[string]deepcheck.Result{
			"deep-pass": {At: 1, OK: true, LastSuccess: 1, Seconds: 2},
			"deep-fail": {At: 1, Message: "damaged <chunk>"},
		},
		KitDrills: map[string]lockstate.KitDrillResult{
			"kit-pass": {At: 1, OK: true, LastSuccess: 1, Seconds: 3, SignInRequired: true},
			"":         {At: 1, Message: "kit <failure>"},
		},
	}
	var out bytes.Buffer
	if err := page.Execute(&out, view{S: s, Now: time.Unix(10, 0)}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Kit drill: running", "Deep checks", "<td>deep-pass</td>", "<td>deep-fail</td>", "passed", "FAILED: damaged &lt;chunk&gt;", "Kit drills", "<td>kit-pass</td>", "<td>run</td>", "FAILED: kit &lt;failure&gt;", "Provider sign-in is left to a person.", "2s", "3s"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("page lacks %q", want)
		}
	}
}
