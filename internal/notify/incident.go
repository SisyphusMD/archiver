package notify

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/logging"
)

// An incident is a condition notified when it starts, again only every Repeat while it
// lasts, and once more when it clears (ADRs 33, 36): a storage down for a week is one
// alert a day, not one per retry, and its recovery is said.
//
// Delivery is at least once: the destinations a notification is owed to (Pending) are
// recorded before it is sent and struck off only once it arrives, so an outage or a crash
// mid-send means it is sent again at the next raise or clear, never that it is lost. A
// send claims the incident (Sending) so no other process sends it at the same time, and
// Gen, random per phase, tells a send's result from a later phase's: one process may clear
// while another sends.
type incident struct {
	Kind     Kind     `json:"kind"`
	Title    string   `json:"title"`
	Opened   int64    `json:"opened"`
	LastSent int64    `json:"last_sent"`
	Message  string   `json:"message,omitempty"`
	To       []string `json:"to,omitempty"`   // who the alert goes to
	Told     []string `json:"told,omitempty"` // who has received it, and so is owed its recovery
	Pending  []string `json:"pending,omitempty"`
	Sending  int64    `json:"sending,omitempty"` // when a send claimed it
	Gen      int64    `json:"gen"`
	// Recovered: the incident is over and Message is its recovery notice, still owed to
	// Pending. A raise starts a new incident.
	Recovered bool `json:"recovered,omitempty"`
	// ClearTitle and ClearMessage are a clear that came while the alert was being sent:
	// the sender delivers the recovery once the alert is out, so it never arrives first.
	ClearTitle   string `json:"clear_title,omitempty"`
	ClearMessage string `json:"clear_message,omitempty"`
}

// Raise notifies incident key unless it is already open and was notified within Repeat;
// destinations an earlier notification of it has not reached are sent it either way.
func (n *Notifier) Raise(key string, k Kind, title, message string) {
	if n == nil {
		return
	}
	if n.Incidents == "" {
		n.SendKind(k, title, message)
		return
	}
	now := n.now()
	var send *incident
	n.withIncidents(func(open map[string]incident) {
		in, ok := open[key]
		switch {
		case ok && claimed(in, now):
			// Another process is sending this incident's alert or recovery; a new message
			// must not overtake it. The next raise sends.
			return
		case !ok || in.Recovered:
			to := n.receiving(k)
			in = incident{Kind: k, Title: title, Opened: now.Unix(), Gen: rand.Int64(), //nolint:gosec // G404: an identifier, not a secret
				Message: message, To: to, Pending: to, LastSent: now.Unix()}
		case n.Repeat > 0 && now.Sub(time.Unix(in.LastSent, 0)) >= n.Repeat:
			in.Kind, in.Title, in.Gen = max(in.Kind, k), title, rand.Int64() //nolint:gosec // G404: an identifier, not a secret
			in.Message = fmt.Sprintf("Still happening, since %s: %s", time.Unix(in.Opened, 0).Format("2006-01-02 15:04"), message)
			// The repeat goes to whoever receives it now; the recovery, to everyone told.
			in.Pending, in.LastSent = n.receiving(in.Kind), now.Unix()
			in.To = append(in.To, newcomers(in.Pending, in.To)...)
		case len(in.Pending) > 0:
			// the last notification, to the destinations it has not reached
		case len(newcomers(n.receiving(in.Kind), in.To)) > 0:
			// destinations configured since: they are told of what is still going on
			added := newcomers(n.receiving(in.Kind), in.To)
			in.To, in.Pending = append(in.To, added...), added
		default:
			return
		}
		in.Sending = now.Unix()
		open[key] = in
		send = &in
	})
	if send != nil {
		n.deliver(key, *send)
	}
}

// Clear closes incident key, notifying its recovery wherever the incident went; it does
// nothing when no incident is open, apart from sending a recovery notice still owed.
func (n *Notifier) Clear(key, title, message string) {
	if n == nil || n.Incidents == "" {
		return
	}
	var send *incident
	n.withIncidents(func(open map[string]incident) {
		in, ok := open[key]
		switch {
		case !ok:
			return
		case claimed(in, n.now()) && !in.Recovered:
			// An alert still in flight must not arrive after its recovery: its sender
			// delivers the recovery when done.
			in.ClearTitle, in.ClearMessage = title, message
			open[key] = in
			return
		case claimed(in, n.now()):
			return
		case !in.Recovered:
			in = n.recovery(in, title, message)
		case len(in.Pending) == 0:
			delete(open, key)
			return
		}
		in.Sending = n.now().Unix()
		open[key] = in
		send = &in
	})
	if send != nil {
		n.deliver(key, *send)
	}
}

// recovery is incident in's recovery notice, owed to whoever its alert reached: one it
// never reached never heard of the incident, so is not told it is over.
func (n *Notifier) recovery(in incident, title, message string) incident {
	return incident{Kind: in.Kind, Title: title, Opened: in.Opened, Gen: rand.Int64(), Recovered: true, //nolint:gosec // G404: an identifier, not a secret
		Message: fmt.Sprintf("%s (after %s)", message, n.now().Sub(time.Unix(in.Opened, 0)).Round(time.Minute)),
		To:      in.Told, Pending: in.Told, Sending: n.now().Unix()}
}

// newcomers are the destinations in now that are not in before.
func newcomers(now, before []string) []string {
	var out []string
	for _, d := range now {
		if !slices.Contains(before, d) {
			out = append(out, d)
		}
	}
	return out
}

// claimLease is how long a send's claim holds off others: longer than any send takes
// (three attempts of 15 seconds per destination, with the waits between), so a claim still
// held is a send in flight, and one past it was a process that died mid-send.
const claimLease = 5 * time.Minute

// claimed reports whether another sender's claim on in still holds. One in the future is
// no claim (a forged or corrupt timestamp must not hold every alert back until then).
func claimed(in incident, now time.Time) bool {
	elapsed := now.Sub(time.Unix(in.Sending, 0))
	return in.Sending != 0 && elapsed >= 0 && elapsed < claimLease
}

func (n *Notifier) configured(name string) bool {
	for _, d := range n.Destinations {
		if d.Name() == name {
			return true
		}
	}
	return false
}

// receiving names the destinations that receive kind k.
func (n *Notifier) receiving(k Kind) []string {
	var out []string
	for _, d := range n.Destinations {
		if admits(d.On, k) {
			out = append(out, d.Name())
		}
	}
	return out
}

// deliver sends in's message to its pending destinations and strikes off those it
// reached, unless the incident has moved on meanwhile; a recovery owed to no one is done.
func (n *Notifier) deliver(key string, in incident) {
	only := map[string]bool{}
	for _, d := range in.Pending {
		only[d] = true
	}
	missed := n.sendTo(in.Kind, in.Title, in.Message, only)
	// A recipient not configured now was not sent anything: an alert stays owed to it for
	// when it is back; a recovery is dropped, as one for a destination gone for good would
	// never be done.
	for _, d := range in.Pending {
		if !n.configured(d) && !in.Recovered {
			missed = append(missed, d)
		}
	}
	var next *incident
	n.withIncidents(func(open map[string]incident) {
		now, ok := open[key]
		if !ok || now.Gen != in.Gen {
			return
		}
		if !now.Recovered {
			for _, d := range now.Pending {
				if !slices.Contains(missed, d) && !slices.Contains(now.Told, d) {
					now.Told = append(now.Told, d)
				}
			}
		}
		now.Pending, now.Sending = missed, 0
		switch {
		case now.ClearTitle != "" && !now.Recovered:
			rec := n.recovery(now, now.ClearTitle, now.ClearMessage)
			open[key], next = rec, &rec
		case now.Recovered && len(missed) == 0:
			delete(open, key)
		default:
			open[key] = now
		}
	})
	if next != nil {
		n.deliver(key, *next)
	}
}

// OpenIncidents lists the open incidents' titles by key, for status and health.
func OpenIncidents(path string) map[string]string {
	out, _ := ReadIncidents(path)
	return out
}

// ReadIncidents is OpenIncidents with the reason it could not read them; no file is no
// incidents.
func ReadIncidents(path string) (map[string]string, error) {
	out := map[string]string{}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	var open map[string]incident
	if err := json.Unmarshal(b, &open); err != nil {
		return out, err
	}
	for k, in := range open {
		if !in.Recovered {
			out[k] = in.Title
		}
	}
	return out, nil
}

// IsOpen reports whether incident key is open.
func (n *Notifier) IsOpen(key string) bool {
	if n == nil || n.Incidents == "" {
		return false
	}
	_, ok := OpenIncidents(n.Incidents)[key]
	return ok
}

func (n *Notifier) now() time.Time {
	if n.Now != nil {
		return n.Now()
	}
	return time.Now()
}

// withIncidents edits the incident state under an exclusive lock, so the backup, the
// copy workers, maintenance and drills never lose one another's changes. A state that
// cannot be read or written costs only the deduplication: the notification still goes.
func (n *Notifier) withIncidents(edit func(map[string]incident)) {
	open := map[string]incident{}
	lock, err := os.OpenFile(n.Incidents+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		edit(open)
		return
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		edit(open)
		return
	}
	if b, err := os.ReadFile(n.Incidents); err == nil {
		_ = json.Unmarshal(b, &open)
		if open == nil {
			open = map[string]incident{}
		}
	}
	edit(open)
	b, err := json.Marshal(open)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(n.Incidents), ".incidents-")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	if cerr := tmp.Close(); werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		return
	}
	if os.Rename(tmp.Name(), n.Incidents) != nil {
		os.Remove(tmp.Name())
	}
}

// WatchLog makes the log's write failures one incident, cleared when it writes again.
func (n *Notifier) WatchLog(l *logging.Log) {
	key := "log:" + l.Basename
	l.Unwritable = func(msg string) { n.Raise(key, Failure, "Log Unwritable", msg) }
	l.Writable = func() { n.Clear(key, "Log Writable Again", l.Path()+" is written again.") }
}
