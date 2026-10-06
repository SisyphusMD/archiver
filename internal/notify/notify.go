// Package notify sends notifications. Pushover is the only service today; its credentials
// travel in the request body, never on a command line.
package notify

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// PushoverURL is the Pushover message endpoint.
const PushoverURL = "https://api.pushover.net/1/messages.json"

// Notifier sends to Pushover when configured; with no service it does nothing.
type Notifier struct {
	Pushover bool
	Token    string
	User     string
	Hostname string
	URL      string // PushoverURL unless a test points it elsewhere
	Client   *http.Client
	Now      func() time.Time

	// Logf records the outcome in the pipeline's log. failed reports a send that did not
	// succeed, which the pipeline counts as an error.
	Logf func(failed bool, msg string)

	mu      sync.Mutex
	sending bool
}

// Send sends one notification, prefixed with the host and time.
// A failure while reporting a failure is not reported again, which would loop.
func (n *Notifier) Send(title, message string) {
	if n == nil || !n.Pushover {
		return
	}
	n.mu.Lock()
	if n.sending {
		n.mu.Unlock()
		return
	}
	n.sending = true
	n.mu.Unlock()
	defer func() {
		n.mu.Lock()
		n.sending = false
		n.mu.Unlock()
	}()

	now := time.Now
	if n.Now != nil {
		now = n.Now
	}
	body := url.Values{
		"token":   {n.Token},
		"user":    {n.User},
		"title":   {title},
		"message": {fmt.Sprintf("[%s] [%s] %s", n.Hostname, now().Format("2006-01-02 15:04:05"), message)},
	}
	client := n.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	endpoint := n.URL
	if endpoint == "" {
		endpoint = PushoverURL
	}
	resp, err := client.Post(endpoint, "application/x-www-form-urlencoded", strings.NewReader(body.Encode()))
	if err == nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("HTTP %s", resp.Status)
		}
	}
	if n.Logf == nil {
		return
	}
	if err != nil {
		n.Logf(true, fmt.Sprintf("Failed to send pushover notification (%v). Check the Pushover secrets.", err))
		return
	}
	n.Logf(false, "Pushover notification sent successfully.")
}
