// Package checkin pings a dead man's switch (ADR 37): a URL an outside monitor such as an
// Uptime Kuma push monitor or healthchecks.io expects to hear from, so silence (the container
// down, a job never run) alerts as surely as a failure.
package checkin

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client sends the pings; nil means a 15-second-timeout default.
var Client = &http.Client{Timeout: 15 * time.Second}

// waits between attempts: a monitor that missed one ping alerts at its own timeout.
var waits = []time.Duration{5 * time.Second, 15 * time.Second}

// Ping reports ok or a failure to monitor URL u, with message msg where the monitor takes
// one. A failure goes to the URL's fail variant, so the alert comes at once rather than at
// the monitor's timeout: Uptime Kuma push URLs (/api/push/) take status=down, others
// (healthchecks.io and its kind) /fail on the path.
func Ping(ctx context.Context, u string, ok bool, msg string) error {
	if u == "" {
		return nil
	}
	target, err := variant(u, ok, msg)
	if err != nil {
		return err
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return err
		}
		resp, err := Client.Do(req)
		// The URL carries the monitor's token: an error never names it.
		if ue, isURL := err.(*url.Error); isURL {
			err = ue.Err
		}
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 300 {
				return nil
			}
			err = fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		if attempt >= len(waits) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waits[attempt]):
		}
	}
}

func variant(raw string, ok bool, msg string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("not an http(s) URL")
	}
	if strings.Contains(u.Path, "/api/push/") {
		q := u.Query()
		if ok {
			q.Set("status", "up")
		} else {
			q.Set("status", "down")
		}
		if msg != "" {
			q.Set("msg", msg)
		}
		u.RawQuery = q.Encode()
		return u.String(), nil
	}
	if !ok {
		u.Path = strings.TrimSuffix(u.Path, "/") + "/fail"
	}
	return u.String(), nil
}

// Valid reports whether u can be a check-in URL; empty is off.
func Valid(u string) error {
	if u == "" {
		return nil
	}
	_, err := variant(u, true, "")
	return err
}

// Latest sends one target's pings one at a time, in order, without blocking the caller:
// while one is in flight only the newest waiting status is kept, so an older status never
// arrives after a newer one.
type Latest struct {
	mu   sync.Mutex
	next *func()
	busy bool
}

// Send queues send, replacing any send still waiting.
func (l *Latest) Send(send func()) {
	l.mu.Lock()
	l.next = &send
	if l.busy {
		l.mu.Unlock()
		return
	}
	l.busy = true
	l.mu.Unlock()
	go func() {
		for {
			l.mu.Lock()
			next := l.next
			l.next = nil
			if next == nil {
				l.busy = false
				l.mu.Unlock()
				return
			}
			l.mu.Unlock()
			(*next)()
		}
	}()
}
