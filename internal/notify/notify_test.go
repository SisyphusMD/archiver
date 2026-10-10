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
	"unicode/utf8"

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

// Wired as the commands wire it (a run notifies its summary, the notifier reports through
// Unnotified), a failed send is logged and counted once and never joins a summary.
func TestFailureDoesNotLoop(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	log := &logging.Log{Dir: t.TempDir(), Basename: "archiver"}
	n := &Notifier{Destinations: pushoverTo(srv.URL), Logf: func(failed bool, msg string) {
		if failed {
			log.Unnotified(logging.Error, "", msg)
		}
	}}
	log.Message(logging.Error, "app", "boom")
	n.Raise("backup", Failure, "Backup Failed", log.Summary())
	if calls != 1 || log.Errors() != 2 || log.Summary() != "[app] boom" {
		t.Fatalf("sends %d, errors %d, summary %q (want 1 send; the error and the failed send logged)", calls, log.Errors(), log.Summary())
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
// sent; a 424 (some URL failed) is a failure, and not retried, so no URL gets it twice.
func TestApprise(t *testing.T) {
	calls := 0
	var body map[string]string
	var user, pass, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		user, pass, _ = r.BasicAuth()
		path = r.URL.Path
		json.NewDecoder(r.Body).Decode(&body)
		if body["type"] == "failure" {
			w.WriteHeader(http.StatusFailedDependency)
			w.Write([]byte(`{"error":"One or more notification could not be sent"}`))
		}
	}))
	defer srv.Close()
	u := strings.Replace(srv.URL, "http://", "http://me:secret@", 1) + "/notify/archiver"
	var msg string
	var failed bool
	n := &Notifier{Hostname: "nas", Destinations: []Destination{{&Apprise{URL: u, Tags: map[string]string{"failure": "critical"}}, "failures"}},
		Waits: []time.Duration{time.Millisecond}, Logf: func(f bool, m string) { failed, msg = f, m }}
	n.Send("Storage Down", "offsite unreachable")
	if calls != 1 || user != "me" || pass != "secret" || path != "/notify/archiver" {
		t.Fatalf("calls %d auth %q/%q path %q", calls, user, pass, path)
	}
	if body["tag"] != "critical" || body["type"] != "failure" || body["title"] != "Storage Down" || !strings.HasSuffix(body["body"], "offsite unreachable") {
		t.Fatalf("body %v", body)
	}
	if !failed || !strings.Contains(msg, "424") {
		t.Fatalf("a 424 was not reported as a failure: %v %q", failed, msg)
	}
	// Without a tag for the kind, "all": an Apprise API key given no tag notifies only its
	// untagged URLs.
	body = nil
	n.Destinations[0].On = "everything"
	n.Send("Backup Complete", "done")
	if body["tag"] != "all" || body["type"] != "info" || failed || msg != "Apprise notification sent successfully." {
		t.Fatalf("body without a mapped tag %v, log %v %q", body, failed, msg)
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

// An incident notifies when raised, not again while open until the repeat interval has
// passed, and once on clearing; a clear with nothing open says nothing. A repeat of 0 is
// never.
func TestIncidents(t *testing.T) {
	var titles, messages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		titles = append(titles, r.Form.Get("title"))
		messages = append(messages, r.Form.Get("message"))
	}))
	defer srv.Close()
	clock := time.Date(2026, 10, 1, 8, 0, 0, 0, time.Local)
	state := t.TempDir() + "/incidents.json"
	n := &Notifier{Hostname: "nas", Destinations: pushoverTo(srv.URL), Incidents: state, Repeat: 24 * time.Hour,
		Now: func() time.Time { return clock }}
	n.Clear("copy:offsite", "Storage Recovered", "nothing was down")
	n.Raise("copy:offsite", Failure, "Storage Down", "refused")
	clock = clock.Add(time.Hour)
	n.Raise("copy:offsite", Failure, "Storage Down", "refused")
	n.Raise("backup", Failure, "Backup Failed", "boom")
	if got := OpenIncidents(state); len(got) != 2 || got["copy:offsite"] != "Storage Down" {
		t.Fatalf("open %v", got)
	}
	clock = clock.Add(23 * time.Hour)
	n.Raise("copy:offsite", Failure, "Storage Down", "refused")
	n.Clear("copy:offsite", "Storage Recovered", "caught up")
	n.Clear("copy:offsite", "Storage Recovered", "caught up")
	want := "Storage Down|Backup Failed|Storage Down|Storage Recovered"
	if strings.Join(titles, "|") != want {
		t.Fatalf("sent %q, want %q", titles, want)
	}
	if !strings.Contains(messages[2], "Still happening, since 2026-10-01 08:00: refused") || !strings.Contains(messages[3], "caught up (after 24h0m0s)") {
		t.Fatalf("messages %q", messages)
	}

	never := &Notifier{Destinations: pushoverTo(srv.URL), Incidents: t.TempDir() + "/i.json", Now: func() time.Time { return clock }}
	titles = nil
	never.Raise("kit", Failure, "Recovery Kit Failed", "x")
	clock = clock.Add(30 * 24 * time.Hour)
	never.Raise("kit", Failure, "Recovery Kit Failed", "x")
	if len(titles) != 1 {
		t.Fatalf("a repeat of 0 repeated: %q", titles)
	}
}

// A notification an outage kept from a destination is tried again at the next raise,
// interval or not; once it arrives the incident is quiet again.
func TestIncidentRetriesMissedDestinations(t *testing.T) {
	down := true
	sent := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		sent++
	}))
	defer srv.Close()
	n := &Notifier{Destinations: pushoverTo(srv.URL), Incidents: t.TempDir() + "/i.json", Repeat: 24 * time.Hour}
	n.Raise("backup", Failure, "Backup Failed", "boom")
	down = false
	n.Raise("backup", Failure, "Backup Failed", "boom")
	n.Raise("backup", Failure, "Backup Failed", "boom")
	if sent != 1 {
		t.Fatalf("sent %d after the destination came back, want exactly 1", sent)
	}
	// A send another process claimed is in flight: not sent again until its lease ends.
	clock := time.Now()
	n.Now = func() time.Time { return clock }
	n.withIncidents(func(open map[string]incident) {
		in := open["backup"]
		in.Pending, in.Sending = []string{"pushover"}, clock.Unix()
		open["backup"] = in
	})
	n.Raise("backup", Failure, "Backup Failed", "boom")
	if sent != 1 {
		t.Fatal("a claimed send was sent again")
	}
	clock = clock.Add(claimLease)
	n.Raise("backup", Failure, "Backup Failed", "boom")
	if sent != 2 {
		t.Fatalf("sent %d: a send whose claim lapsed (its process died) must be sent again", sent)
	}
}

// A recovery notice an outage kept from a destination is tried again at the next clear,
// and only there.
func TestRecoveryRetriedWhenMissed(t *testing.T) {
	down := false
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		r.ParseForm()
		got = append(got, r.Form.Get("title"))
	}))
	defer srv.Close()
	state := t.TempDir() + "/i.json"
	n := &Notifier{Destinations: pushoverTo(srv.URL), Incidents: state, Repeat: 24 * time.Hour}
	n.Raise("backup", Failure, "Backup Failed", "boom")
	down = true
	n.Clear("backup", "Backup Recovered", "fine")
	if len(OpenIncidents(state)) != 0 {
		t.Fatal("a recovered incident still reads as open")
	}
	down = false
	n.Clear("backup", "Backup Recovered", "fine")
	n.Clear("backup", "Backup Recovered", "fine")
	if strings.Join(got, "|") != "Backup Failed|Backup Recovered" {
		t.Fatalf("sent %q", got)
	}
}

// Pushover's limits are kept: a long summary is cut, saying so, rather than refused.
func TestPushoverClipsLongMessages(t *testing.T) {
	var msg string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		msg = r.Form.Get("message")
	}))
	defer srv.Close()
	(&Notifier{Destinations: pushoverTo(srv.URL)}).Send("Backup Failed", strings.Repeat("é", 3000))
	if n := len([]rune(msg)); n != 1024 || !strings.HasSuffix(msg, "(the rest is in the log)") {
		t.Fatalf("message of %d characters: ...%s", n, msg[len(msg)-40:])
	}
	if got := clipBytes(strings.Repeat("é", 3000), 4096); len(got) > 4096 || !utf8.ValidString(got) {
		t.Fatalf("clipBytes gave %d bytes, valid %v", len(got), utf8.ValidString(got))
	}
}

// A clear while the alert is still being sent is delivered after it by its sender; a
// destination configured while an incident is open is told of it.
func TestClearDuringSendAndNewDestinations(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		got = append(got, r.Form.Get("title"))
	}))
	defer srv.Close()
	state := t.TempDir() + "/i.json"
	quiet := &Notifier{Incidents: state, Repeat: 0}
	quiet.Raise("backup", Failure, "Backup Failed", "boom")
	n := &Notifier{Destinations: pushoverTo(srv.URL), Incidents: state, Repeat: 0}
	n.Raise("backup", Failure, "Backup Failed", "boom")
	n.Raise("backup", Failure, "Backup Failed", "boom")
	if strings.Join(got, "|") != "Backup Failed" {
		t.Fatalf("a newly configured destination: sent %q", got)
	}
	// Mid-send: the claim is held; the clear is queued, and the sender delivers it.
	var in incident
	n.withIncidents(func(open map[string]incident) {
		in = open["backup"]
		in.Pending, in.Sending = []string{"pushover"}, time.Now().Unix()
		open["backup"] = in
	})
	n.Clear("backup", "Backup Recovered", "fine")
	n.deliver("backup", in)
	if strings.Join(got, "|") != "Backup Failed|Backup Failed|Backup Recovered" || len(OpenIncidents(state)) != 0 {
		t.Fatalf("sent %q, open %v", got, OpenIncidents(state))
	}
}

// An alert owed to a destination missing from the configuration stays owed until it is
// back; a recovery owed to one is dropped.
func TestPendingForUnconfiguredDestination(t *testing.T) {
	sent := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sent++ }))
	defer srv.Close()
	state := t.TempDir() + "/i.json"
	n := &Notifier{Destinations: pushoverTo(srv.URL), Incidents: state}
	n.withIncidents(func(open map[string]incident) {
		open["backup"] = incident{Kind: Failure, Title: "Backup Failed", Message: "boom", To: []string{"pushover"}, Pending: []string{"pushover"}, Gen: 1}
	})
	none := &Notifier{Incidents: state}
	none.Raise("backup", Failure, "Backup Failed", "boom")
	n.Raise("backup", Failure, "Backup Failed", "boom")
	if sent != 1 {
		t.Fatalf("sent %d: the alert must wait for its destination, then go once", sent)
	}
	n.Clear("backup", "Backup Recovered", "fine")
	if sent != 2 || len(OpenIncidents(state)) != 0 {
		t.Fatalf("sent %d, open %v", sent, OpenIncidents(state))
	}
}

// A critical notification reaches every failures destination at a higher priority.
func TestCriticalPriority(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		got = append(got, r.Form.Get("priority"))
	}))
	defer srv.Close()
	n := &Notifier{Destinations: pushoverTo(srv.URL)}
	n.SendKind(Critical, "PRIMARY DOWN", "x")
	n.SendKind(Failure, "Backup Failed", "x")
	if strings.Join(got, "|") != "1|" {
		t.Fatalf("priorities %q", got)
	}
	if req, _ := (&Ntfy{URL: "http://x/t"}).Request(Critical, "t", "m"); req.Header.Get("Priority") != "5" {
		t.Fatal("ntfy priority for critical")
	}
}

// A failed send never names the URL it went to: an Apprise URL carries its key.
func TestSendFailureHidesURL(t *testing.T) {
	var logged []string
	n := &Notifier{Destinations: []Destination{{&Apprise{URL: "http://127.0.0.1:1/notify/secret-key-123"}, "failures"}},
		Waits: []time.Duration{}, Logf: func(failed bool, msg string) { logged = append(logged, msg) }}
	n.Send("Backup Failed", "x")
	if len(logged) != 1 || strings.Contains(logged[0], "secret-key-123") || strings.Contains(logged[0], "/notify/") {
		t.Fatalf("logged %q", logged)
	}
}

// A claim holds for its lease and no longer, and one stamped in the future holds nothing.
func TestClaimedIgnoresFutureClaims(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	for _, c := range []struct {
		sending int64
		want    bool
	}{
		{0, false},
		{now.Unix() - 60, true},
		{now.Unix() - int64(claimLease/time.Second) - 1, false},
		{now.Unix() + 3600, false},
		{now.Unix() + 100*365*86400, false},
	} {
		if got := claimed(incident{Sending: c.sending}, now); got != c.want {
			t.Errorf("sending %d: claimed %v", c.sending, got)
		}
	}
}
