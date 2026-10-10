package checkin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVariants(t *testing.T) {
	for _, c := range []struct {
		in   string
		ok   bool
		want string
	}{
		{"https://hc-ping.com/abc", true, "https://hc-ping.com/abc"},
		{"https://hc-ping.com/abc/", false, "https://hc-ping.com/abc/fail"},
		{"https://kuma.lan/api/push/tok", true, "https://kuma.lan/api/push/tok?msg=done&status=up"},
		{"https://kuma.lan/api/push/tok?ping=", false, "https://kuma.lan/api/push/tok?msg=done&ping=&status=down"},
	} {
		if got, err := variant(c.in, c.ok, "done"); err != nil || got != c.want {
			t.Errorf("%s ok=%v: %q %v, want %q", c.in, c.ok, got, err, c.want)
		}
	}
	if Valid("ftp://x") == nil || Valid("not a url") == nil || Valid("") != nil {
		t.Error("validation")
	}
}

func TestPing(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
	}))
	defer srv.Close()
	if err := Ping(context.Background(), srv.URL+"/uuid", false, ""); err != nil || len(paths) != 1 || paths[0] != "/uuid/fail" {
		t.Fatalf("%v %q", err, paths)
	}
}

// Sends run one at a time; a send waiting behind a slow one is replaced by a newer one.
func TestLatest(t *testing.T) {
	var l Latest
	release, entered := make(chan struct{}), make(chan struct{})
	got := make(chan string, 3)
	l.Send(func() { close(entered); <-release; got <- "first" })
	<-entered
	l.Send(func() { got <- "stale" })
	l.Send(func() { got <- "newest" })
	close(release)
	if a, b := <-got, <-got; a != "first" || b != "newest" {
		t.Fatalf("%s then %s", a, b)
	}
	select {
	case s := <-got:
		t.Fatalf("a replaced send ran: %s", s)
	default:
	}
}
