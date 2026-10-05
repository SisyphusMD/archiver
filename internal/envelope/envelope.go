// Package envelope reads what the bash envelope feature records about the printed
// break-glass envelope: the fingerprint confirmed printed, and the fingerprint the page would
// have now (refreshed after each recovery-kit run). Neither file holds a secret.
package envelope

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SisyphusMD/archiver/internal/layout"
)

// MaxAge is how long a confirmed envelope is good for before its yearly check is due.
const MaxAge = 365 * 24 * time.Hour

// Record is one state file: a fingerprint and when it was recorded.
type Record struct {
	Fingerprint string
	At          time.Time
}

func read(path string) (Record, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Record{}, false
	}
	f := strings.Fields(string(b))
	if len(f) < 2 {
		return Record{}, false
	}
	at, err := strconv.ParseInt(f[1], 10, 64)
	if err != nil {
		return Record{}, false
	}
	return Record{Fingerprint: f[0], At: time.Unix(at, 0)}, true
}

// Status is the envelope's standing: Line for `archiver status` (empty when the recovery
// kit is not in use), Warning for healthcheck (empty unless a printed envelope is stale).
func Status(l layout.Layout, now time.Time, age func(int64, time.Time) string) (line, warning string) {
	current, haveCurrent := read(l.EnvelopeCurrent())
	confirmed, haveConfirmed := read(l.EnvelopeConfirmed())
	switch {
	case !haveConfirmed && !haveCurrent:
		return "", ""
	case !haveConfirmed:
		return "Envelope: never confirmed printed (print with 'archiver envelope', then 'archiver envelope confirm').", ""
	case haveCurrent && current.Fingerprint != confirmed.Fingerprint:
		w := "the printed break-glass envelope is out of date (a secret or storage on it changed); reprint with 'archiver envelope' and confirm"
		return fmt.Sprintf("Envelope: OUT OF DATE, printed %s; a secret or storage on it changed.", age(confirmed.At.Unix(), now)), w
	case now.Sub(confirmed.At) > MaxAge:
		w := "the break-glass envelope was confirmed over a year ago; check it is still there and readable, then 'archiver envelope confirm'"
		return fmt.Sprintf("Envelope: check due, confirmed printed %s.", age(confirmed.At.Unix(), now)), w
	}
	return fmt.Sprintf("Envelope: current, confirmed printed %s (fingerprint %s).", age(confirmed.At.Unix(), now), confirmed.Fingerprint), ""
}
