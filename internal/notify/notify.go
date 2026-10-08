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
	// Waits are the pauses before each retry of a transient failure; nil means
	// DefaultWaits. A failure alert lost to one network blip is the alert nobody sees.
	Waits []time.Duration

	// Logf records the outcome in the pipeline's log. failed reports a send that did not
	// succeed, which the pipeline counts as an error. It must not notify (logging.Log's
	// Unnotified): a report of a failed send would fail too, and loop.
	Logf func(failed bool, msg string)

	// Sends go one at a time: services back up in parallel, and a second alert must wait
	// its turn rather than be lost.
	sendMu sync.Mutex
}

// DefaultWaits retry a transient failure twice. Pushover asks for at least 5 seconds
// before retrying a server error; with the 15-second attempt timeout a send ends within
// about a minute, which bounds how long a failing run waits on its alert.
var DefaultWaits = []time.Duration{5 * time.Second, 10 * time.Second}

// Send sends one notification, prefixed with the host and time, waiting for any send in
// progress.
func (n *Notifier) Send(title, message string) {
	if n == nil || !n.Pushover {
		return
	}
	n.sendMu.Lock()
	transient, attempts, err := n.send(title, message)
	n.sendMu.Unlock()
	if n.Logf == nil {
		return
	}
	switch {
	case err != nil && transient:
		n.Logf(true, fmt.Sprintf("Failed to send pushover notification after %d attempts (%v).", attempts, err))
	case err != nil:
		n.Logf(true, fmt.Sprintf("Failed to send pushover notification (%v). Check the Pushover secrets.", err))
	default:
		n.Logf(false, "Pushover notification sent successfully.")
	}
}

// send makes the attempts for one notification.
func (n *Notifier) send(title, message string) (transient bool, attempts int, err error) {
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
		client = &http.Client{Timeout: 15 * time.Second}
	}
	endpoint := n.URL
	if endpoint == "" {
		endpoint = PushoverURL
	}
	waits := n.Waits
	if waits == nil {
		waits = DefaultWaits
	}
	for {
		attempts++
		transient, err = post(client, endpoint, body.Encode())
		if err == nil || !transient || attempts > len(waits) {
			break
		}
		time.Sleep(waits[attempts-1])
	}
	return transient, attempts, err
}

// post makes one attempt. A network error, a rate limit or a server error is transient;
// any other refusal (bad credentials, a malformed request) would fail the same way again.
func post(client *http.Client, endpoint, form string) (transient bool, err error) {
	resp, err := client.Post(endpoint, "application/x-www-form-urlencoded", strings.NewReader(form))
	if err != nil {
		return true, err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return false, nil
	}
	return resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500, fmt.Errorf("HTTP %s", resp.Status)
}
