// Package notify sends notifications to every configured destination (Pushover, Apprise,
// ntfy; ADR 36), each receiving the kinds its NOTIFY_ON setting admits. Credentials travel
// in request bodies or headers, never on a command line.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/SisyphusMD/archiver/internal/config"
)

// PushoverURL is the Pushover message endpoint.
const PushoverURL = "https://api.pushover.net/1/messages.json"

// Kind is how much a notification matters, which decides the destinations it reaches.
type Kind int

const (
	Routine Kind = iota // backup done, paused, resumed, stopped by a person
	Problem             // needs attention, nothing lost yet
	Failure             // something did not happen that should have
)

func (k Kind) String() string {
	return [...]string{"routine", "problem", "failure"}[k]
}

// KindOf classifies a notification by its title. A title not listed is a failure, so an
// unclassified notification still reaches every destination.
func KindOf(title string) Kind {
	switch title {
	case "Backup Complete", "Maintenance Complete", "Restore Drill Complete", "Backup Paused", "Backup Resumed",
		"Backup Stopped", "Maintenance Stopped":
		return Routine
	case "Mirror Refused", "Envelope Out of Date", "Envelope Check Due":
		return Problem
	}
	return Failure
}

// admits reports whether a destination set to on (NOTIFY_ON's words) receives kind k.
func admits(on string, k Kind) bool {
	switch on {
	case "everything":
		return true
	case "problems":
		return k >= Problem
	}
	return k == Failure
}

// Sink is one destination.
type Sink interface {
	// Name names it in the log ("pushover", "apprise", "ntfy").
	Name() string
	// Request builds the request that sends a notification of kind k.
	Request(k Kind, title, message string) (*http.Request, error)
	// Delivered reports whether a response means the notification arrived.
	Delivered(status int, body []byte) bool
}

// Destination is a sink and the kinds it receives.
type Destination struct {
	Sink
	On string // failures, problems or everything
}

// Notifier sends to its destinations; with none it does nothing.
type Notifier struct {
	Hostname     string
	Destinations []Destination
	Client       *http.Client
	Now          func() time.Time
	// Waits are the pauses before each retry of a transient failure; nil means
	// DefaultWaits. A failure alert lost to one network blip is the alert nobody sees.
	Waits []time.Duration

	// Logf records each outcome in the pipeline's log. failed reports a send that did not
	// succeed, which the pipeline counts as an error. It must not notify (logging.Log's
	// Unnotified): a report of a failed send would fail too, and loop.
	Logf func(failed bool, msg string)

	// Incidents is the state file that makes Raise notify once per incident (ADRs 33, 36),
	// shared by every process; empty, Raise always sends and Clear never does.
	Incidents string
	// Repeat is how often an incident still open is notified again; 0 is never.
	Repeat time.Duration

	// Sends go one at a time: services back up in parallel, and a second alert must wait
	// its turn rather than be lost.
	sendMu sync.Mutex
}

// FromConfig builds a notifier for the configured destinations.
func FromConfig(cfg *config.Config, hostname string, logf func(failed bool, msg string)) *Notifier {
	n := &Notifier{Hostname: hostname, Logf: logf, Repeat: cfg.AlertRepeat()}
	on := func(own string) string {
		if own != "" {
			return own
		}
		if cfg.NotifyOn != "" {
			return cfg.NotifyOn
		}
		return "failures"
	}
	if cfg.Pushover() && cfg.PushoverAPIToken != "" && cfg.PushoverUserKey != "" {
		n.Destinations = append(n.Destinations, Destination{&Pushover{Token: cfg.PushoverAPIToken, User: cfg.PushoverUserKey}, on(cfg.PushoverNotifyOn)})
	}
	if cfg.AppriseURL != "" {
		tags, _ := cfg.AppriseTagMap() // validated with the configuration
		n.Destinations = append(n.Destinations, Destination{&Apprise{URL: cfg.AppriseURL, Tags: tags}, on(cfg.AppriseNotifyOn)})
	}
	if cfg.NtfyURL != "" {
		n.Destinations = append(n.Destinations, Destination{&Ntfy{URL: cfg.NtfyURL, Token: cfg.NtfyToken}, on(cfg.NtfyNotifyOn)})
	}
	return n
}

// DefaultWaits retry a transient failure twice. Pushover asks for at least 5 seconds
// before retrying a server error; with the 15-second attempt timeout a send ends within
// about a minute per destination, which bounds how long a failing run waits on its alert.
var DefaultWaits = []time.Duration{5 * time.Second, 10 * time.Second}

// Send sends one notification, prefixed with the host and time, to every destination that
// receives its kind, waiting for any send in progress.
func (n *Notifier) Send(title, message string) { n.SendKind(KindOf(title), title, message) }

// SendKind is Send with the kind given rather than read from the title.
func (n *Notifier) SendKind(k Kind, title, message string) { n.sendTo(k, title, message, nil) }

// sendTo sends to the destinations receiving kind k, or, when only is not nil, to those
// it names whatever they receive now (an incident's recorded recipients), and returns the
// names of those it did not reach.
func (n *Notifier) sendTo(k Kind, title, message string, only map[string]bool) (missed []string) {
	if n == nil || len(n.Destinations) == 0 {
		return nil
	}
	now := time.Now
	if n.Now != nil {
		now = n.Now
	}
	text := fmt.Sprintf("[%s] [%s] %s", n.Hostname, now().Format("2006-01-02 15:04:05"), message)
	for _, d := range n.Destinations {
		if (only == nil && !admits(d.On, k)) || (only != nil && !only[d.Name()]) {
			continue
		}
		n.sendMu.Lock()
		transient, attempts, err := n.send(d, k, title, text)
		n.sendMu.Unlock()
		if err != nil {
			missed = append(missed, d.Name())
		}
		if n.Logf == nil {
			continue
		}
		switch {
		case err != nil && transient:
			n.Logf(true, fmt.Sprintf("Failed to send %s notification after %d attempts (%v).", d.Name(), attempts, err))
		case err != nil:
			n.Logf(true, fmt.Sprintf("Failed to send %s notification (%v). Check its settings and secrets.", d.Name(), err))
		default:
			n.Logf(false, fmt.Sprintf("%s notification sent successfully.", capitalize(d.Name())))
		}
	}
	return missed
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// send makes the attempts for one destination.
func (n *Notifier) send(d Destination, k Kind, title, text string) (transient bool, attempts int, err error) {
	client := n.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	waits := n.Waits
	if waits == nil {
		waits = DefaultWaits
	}
	for {
		attempts++
		req, rerr := d.Request(k, title, text)
		if rerr != nil {
			return false, attempts, rerr
		}
		transient, err = post(client, req, d.Delivered)
		if err == nil || !transient || attempts > len(waits) {
			break
		}
		time.Sleep(waits[attempts-1])
	}
	return transient, attempts, err
}

// post makes one attempt. A network error, a rate limit or a server error is transient;
// any other refusal (bad credentials, a malformed request) would fail the same way again.
func post(client *http.Client, req *http.Request, delivered func(int, []byte) bool) (transient bool, err error) {
	resp, err := client.Do(req)
	if err != nil {
		return true, err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	if delivered(resp.StatusCode, body) {
		return false, nil
	}
	return resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500, fmt.Errorf("HTTP %s", resp.Status)
}

// Pushover sends through Pushover's message API.
type Pushover struct {
	Token, User string
	URL         string // PushoverURL unless a test points it elsewhere
}

func (p *Pushover) Name() string { return "pushover" }

func (p *Pushover) Request(k Kind, title, message string) (*http.Request, error) {
	endpoint := p.URL
	if endpoint == "" {
		endpoint = PushoverURL
	}
	// Pushover refuses a title over 250 characters or a message over 1024, the whole
	// notification with it.
	body := url.Values{"token": {p.Token}, "user": {p.User}, "title": {clip(title, 250)}, "message": {clip(message, 1024)}}
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body.Encode()))
	if err == nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return req, err
}

func (p *Pushover) Delivered(status int, _ []byte) bool { return status == http.StatusOK }

// Apprise posts to an Apprise API server's notify endpoint (`/notify/<key>`), basic-auth
// credentials in the URL if it needs them, with the tag APPRISE_TAGS gives the kind.
type Apprise struct {
	URL  string
	Tags map[string]string // kind -> tag; a kind without one goes to "all"
}

func (a *Apprise) Name() string { return "apprise" }

func (a *Apprise) Request(k Kind, title, message string) (*http.Request, error) {
	u, err := url.Parse(a.URL)
	if err != nil {
		// url.Parse's error quotes the whole URL, basic-auth password included.
		return nil, fmt.Errorf("APPRISE_URL is not a valid URL")
	}
	payload := map[string]string{
		"title": title, "body": message,
		"type": map[Kind]string{Routine: "info", Problem: "warning", Failure: "failure"}[k],
	}
	// A stateful Apprise API key given no tag notifies only its untagged URLs: "all" is
	// what reaches every one.
	payload["tag"] = "all"
	if tag := a.Tags[k.String()]; tag != "" {
		payload["tag"] = tag
	}
	b, _ := json.Marshal(payload)
	auth := u.User
	u.User = nil
	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if auth != nil {
		pass, _ := auth.Password()
		req.SetBasicAuth(auth.Username(), pass)
	}
	return req, nil
}

// Delivered is a 200. Apprise answers 424 when any of a tag's URLs failed, without saying
// whether others took it: that is reported as a failure, and, being a 4xx, not retried, so
// the URLs that did take it never get it twice.
func (a *Apprise) Delivered(status int, _ []byte) bool { return status == http.StatusOK }

// Ntfy publishes to a topic on an ntfy server, with an access token if the topic needs
// one, at the priority the kind maps to.
type Ntfy struct {
	URL   string // server and topic
	Token string
}

func (n *Ntfy) Name() string { return "ntfy" }

func (n *Ntfy) Request(k Kind, title, message string) (*http.Request, error) {
	// ntfy turns a message over 4096 bytes into an attachment, or refuses it where
	// attachments are off.
	req, err := http.NewRequest(http.MethodPost, n.URL, strings.NewReader(clipBytes(message, 4096)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Title", title)
	req.Header.Set("Priority", map[Kind]string{Routine: "2", Problem: "3", Failure: "4"}[k])
	req.Header.Set("Tags", k.String())
	if n.Token != "" {
		req.Header.Set("Authorization", "Bearer "+n.Token)
	}
	return req, nil
}

func (n *Ntfy) Delivered(status int, _ []byte) bool { return status == http.StatusOK }

const clipped = " … (the rest is in the log)"

// clip cuts s to at most n characters, saying so.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-len([]rune(clipped))]) + clipped
}

// clipBytes cuts s to at most n bytes on a character boundary, saying so.
func clipBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n - len(clipped)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + clipped
}
