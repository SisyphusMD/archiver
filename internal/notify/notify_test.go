package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
	n := &Notifier{Hostname: "nas", Destinations: pushoverTo(srv.URL),
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
	n := &Notifier{Destinations: pushoverTo(srv.URL), Logf: func(failed bool, msg string) {
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
		{"bad credentials", []int{400}, 1, true, "Failed to send pushover notification (HTTP 400 Bad Request). Check its settings and secrets."},
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
			n := &Notifier{Destinations: pushoverTo(srv.URL), Waits: []time.Duration{time.Millisecond, time.Millisecond},
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
	n := &Notifier{Destinations: pushoverTo(srv.URL), Waits: []time.Duration{time.Millisecond},
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
	n := &Notifier{Destinations: pushoverTo(srv.URL), Logf: func(bool, string) {}}
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

func pushoverTo(url string) []Destination {
	return []Destination{{&Pushover{Token: "tok", User: "usr", URL: url}, "everything"}}
}

// Each destination gets the kinds its setting admits; a title not classified is a failure.
func TestKindsReachDestinations(t *testing.T) {
	var mu sync.Mutex
	got := map[string][]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		mu.Lock()
		got[r.URL.Path] = append(got[r.URL.Path], r.Form.Get("title"))
		mu.Unlock()
	}))
	defer srv.Close()
	n := &Notifier{Destinations: []Destination{
		{&Pushover{URL: srv.URL + "/failures"}, "failures"},
		{&Pushover{URL: srv.URL + "/problems"}, "problems"},
		{&Pushover{URL: srv.URL + "/everything"}, "everything"},
	}}
	for _, title := range []string{"Backup Complete", "Mirror Refused", "Storage Down", "Something New"} {
		n.Send(title, "m")
	}
	want := map[string][]string{
		"/failures":   {"Storage Down", "Something New"},
		"/problems":   {"Mirror Refused", "Storage Down", "Something New"},
		"/everything": {"Backup Complete", "Mirror Refused", "Storage Down", "Something New"},
	}
	for path, titles := range want {
		if strings.Join(got[path], ",") != strings.Join(titles, ",") {
			t.Errorf("%s got %v, want %v", path, got[path], titles)
		}
	}
}

// Apprise: JSON with the kind's tag and type, basic auth from the URL, never in the URL
// sent; a partial delivery (424 "Sent") counts as delivered and is not retried.
func TestApprise(t *testing.T) {
	calls := 0
	var body map[string]string
	var user, pass, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		user, pass, _ = r.BasicAuth()
		path = r.URL.Path
		json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusFailedDependency)
		w.Write([]byte(`{"error":"partial","details":[["Sent"],["Failed"]]}`))
	}))
	defer srv.Close()
	u := strings.Replace(srv.URL, "http://", "http://me:secret@", 1) + "/notify/archiver"
	var msg string
	n := &Notifier{Hostname: "nas", Destinations: []Destination{{&Apprise{URL: u, Tags: map[string]string{"failure": "critical"}}, "failures"}},
		Waits: []time.Duration{time.Millisecond}, Logf: func(_ bool, m string) { msg = m }}
	n.Send("Storage Down", "offsite unreachable")
	if calls != 1 || user != "me" || pass != "secret" || path != "/notify/archiver" {
		t.Fatalf("calls %d auth %q/%q path %q", calls, user, pass, path)
	}
	if body["tag"] != "critical" || body["type"] != "failure" || body["title"] != "Storage Down" || !strings.HasSuffix(body["body"], "offsite unreachable") {
		t.Fatalf("body %v", body)
	}
	if msg != "Apprise notification sent successfully." {
		t.Fatalf("log %q", msg)
	}
	// Without a tag for the kind, "all": an Apprise API key given no tag notifies only its
	// untagged URLs.
	body = nil
	n.Destinations[0].On = "everything"
	n.Send("Backup Complete", "done")
	if body["tag"] != "all" || body["type"] != "info" {
		t.Fatalf("body without a mapped tag %v", body)
	}
}

// ntfy: the message as the body, title and priority in headers, the token as a bearer.
func TestNtfy(t *testing.T) {
	var title, prio, auth, bodyText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		title, prio, auth = r.Header.Get("Title"), r.Header.Get("Priority"), r.Header.Get("Authorization")
		b := new(strings.Builder)
		io.Copy(b, r.Body)
		bodyText = b.String()
	}))
	defer srv.Close()
	n := &Notifier{Hostname: "nas", Destinations: []Destination{{&Ntfy{URL: srv.URL + "/archiver", Token: "tk_1"}, "everything"}}}
	n.Send("Mirror Refused", "too many deletions")
	if title != "Mirror Refused" || prio != "3" || auth != "Bearer tk_1" || !strings.HasSuffix(bodyText, "too many deletions") {
		t.Fatalf("title %q prio %q auth %q body %q", title, prio, auth, bodyText)
	}
}

// A malformed APPRISE_URL is reported without the password inside it.
func TestAppriseBadURLHidesPassword(t *testing.T) {
	var msg string
	n := &Notifier{Destinations: []Destination{{&Apprise{URL: "http://me:s3cret@apprise:80x/notify"}, "everything"}},
		Logf: func(_ bool, m string) { msg = m }}
	n.Send("Storage Down", "x")
	if msg == "" || strings.Contains(msg, "s3cret") {
		t.Fatalf("log %q", msg)
	}
}
