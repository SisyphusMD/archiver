package notify

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/SisyphusMD/archiver/internal/logging"
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

// Wired as the commands wire it (the log notifies through the notifier, the notifier
// reports through Unnotified), a failed send is logged and counted once and not sent again.
func TestFailureDoesNotLoop(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	log := &logging.Log{Dir: t.TempDir(), Basename: "archiver", ErrorTitle: "Backup Error"}
	n := &Notifier{Pushover: true, URL: srv.URL, Logf: func(failed bool, msg string) {
		if failed {
			log.Unnotified(logging.Error, "", msg)
		}
	}}
	log.Notify = n.Send
	log.Message(logging.Error, "app", "boom")
	if calls != 1 || log.Errors() != 2 {
		t.Fatalf("sends %d, errors %d (want 1 send; the error and the failed send logged)", calls, log.Errors())
	}
}

// A transient failure is retried; a refusal that would fail the same way again is not.
func TestRetries(t *testing.T) {
	cases := []struct {
		name     string
		statuses []int // one per attempt; the last repeats
		calls    int
		failed   bool
		msg      string
	}{
		{"server error then success", []int{503, 200}, 2, false, "Pushover notification sent successfully."},
		{"rate limited then success", []int{429, 200}, 2, false, "Pushover notification sent successfully."},
		{"server error throughout", []int{500}, 3, true, "Failed to send pushover notification after 3 attempts (HTTP 500 Internal Server Error)."},
		{"bad credentials", []int{400}, 1, true, "Failed to send pushover notification (HTTP 400 Bad Request). Check the Pushover secrets."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.statuses[min(calls, len(c.statuses)-1)])
				calls++
			}))
			defer srv.Close()
			var failed bool
			var msg string
			n := &Notifier{Pushover: true, URL: srv.URL, Waits: []time.Duration{time.Millisecond, time.Millisecond},
				Logf: func(f bool, m string) { failed, msg = f, m }}
			n.Send("Backup Error", "boom")
			if calls != c.calls || failed != c.failed || msg != c.msg {
				t.Fatalf("calls %d failed %v msg %q", calls, failed, msg)
			}
		})
	}
}

// A connection that never answers is a network error, retried like a server error.
func TestRetriesNetworkError(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
	}))
	defer srv.Close()
	var msg string
	n := &Notifier{Pushover: true, URL: srv.URL, Waits: []time.Duration{time.Millisecond},
		Logf: func(_ bool, m string) { msg = m }}
	n.Send("Backup Error", "boom")
	if calls != 2 || msg != "Pushover notification sent successfully." {
		t.Fatalf("calls %d msg %q", calls, msg)
	}
}

// Two alerts sent at once both arrive: the second waits for the first instead of being
// dropped as if it were a failure report looping.
func TestConcurrentSendsBothArrive(t *testing.T) {
	var mu sync.Mutex
	got := 0
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		mu.Lock()
		got++
		mu.Unlock()
	}))
	defer srv.Close()
	n := &Notifier{Pushover: true, URL: srv.URL, Logf: func(bool, string) {}}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); n.Send("Backup Error", "boom") }()
	}
	time.Sleep(100 * time.Millisecond) // both inside Send, one holding the turn
	close(release)
	wg.Wait()
	if got != 2 {
		t.Fatalf("server got %d of 2 concurrent alerts", got)
	}
}
