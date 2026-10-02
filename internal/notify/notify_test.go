package notify

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSend(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		got = append(got, r.Form.Get("token")+"|"+r.Form.Get("user")+"|"+r.Form.Get("title")+"|"+r.Form.Get("message"))
	}))
	defer srv.Close()
	var logs []string
	n := &Notifier{Pushover: true, Token: "tok", User: "usr", Hostname: "nas", URL: srv.URL,
		Now:  func() time.Time { return time.Date(2026, 10, 1, 8, 0, 0, 0, time.Local) },
		Logf: func(failed bool, msg string) { logs = append(logs, msg) }}
	n.Send("Backup Complete", "done")
	if len(got) != 1 || got[0] != "tok|usr|Backup Complete|[nas] [2026-10-01 08:00:00] done" {
		t.Fatalf("server got %q", got)
	}
	if len(logs) != 1 || logs[0] != "Pushover notification sent successfully." {
		t.Fatalf("logs %q", logs)
	}
	(&Notifier{}).Send("x", "y") // not configured: nothing to do
}

// A failed send is reported once, and the error that report raises is not sent again.
func TestFailureDoesNotLoop(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	var n *Notifier
	failures := 0
	n = &Notifier{Pushover: true, URL: srv.URL, Logf: func(failed bool, msg string) {
		if failed {
			failures++
			n.Send("Backup Error", msg) // what logging an error does
		}
	}}
	n.Send("Backup Error", "boom")
	if calls != 1 || failures != 1 {
		t.Fatalf("calls %d failures %d", calls, failures)
	}
}
