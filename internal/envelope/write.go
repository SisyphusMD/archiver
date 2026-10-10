package envelope

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/SisyphusMD/archiver/internal/layout"
)

const htmlHead = `<!doctype html>
<html><head><meta charset="utf-8"><title>Break-glass envelope</title>
<style>
@page { size: letter; margin: 14mm; }
body { font: 10pt/1.35 Helvetica, Arial, sans-serif; color: #000; background: #fff; max-width: 190mm; margin: 0 auto; }
h1 { font-size: 16pt; margin: 0 0 2pt; }
h2 { font-size: 11pt; margin: 10pt 0 3pt; }
.sub { font-size: 8.5pt; color: #333; margin: 0 0 6pt; }
pre { font: 8.5pt/1.25 "Courier New", Courier, monospace; margin: 1pt 0; white-space: pre-wrap; word-break: break-all; }
.warn { font-weight: bold; border: 1.5pt solid #000; padding: 2pt 4pt; margin: 3pt 0; }
.qr { float: right; margin: 0 0 4pt 10pt; }
.qr svg { width: 38mm; height: 38mm; }
h2, hr, .notes { clear: both; }
hr { border: 0; border-top: 0.75pt solid #000; margin: 8pt 0 2pt; }
.notes div { border-bottom: 0.75pt solid #000; height: 7mm; }
</style></head><body>
`

var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

// HTML is the page for printing from a browser, QR codes as inline SVG.
func (p *Page) HTML() ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(htmlHead)
	tags := map[string]string{"title": "<h1>%s</h1>\n", "sub": "<p class=\"sub\">%s</p>\n", "h": "<h2>%s</h2>\n",
		"p": "<p>%s</p>\n", "m": "<pre>%s</pre>\n", "cmd": "<pre>%s</pre>\n", "warn": "<p class=\"warn\">%s</p>\n"}
	for _, e := range p.Elements {
		switch e.Kind {
		case "qr":
			cmd := exec.Command("qrencode", "-t", "SVG", "-m", "1", "-o", "-")
			cmd.Stdin = strings.NewReader(e.Text)
			svg, err := cmd.Output()
			if err != nil {
				return nil, err
			}
			if strings.HasPrefix(string(svg), "<?xml") {
				if i := bytes.IndexByte(svg, '\n'); i >= 0 {
					svg = svg[i+1:]
				}
			}
			b.WriteString(`<div class="qr">`)
			b.Write(svg)
			b.WriteString("</div>\n")
		case "rule":
			b.WriteString("<hr>\n")
		case "notes":
			i := strings.LastIndex(e.Text, "|")
			label, count := e.Text[:i], e.Text[i+1:]
			fmt.Fprintf(&b, `<p class="notes"><b>%s</b></p><div class="notes">`, htmlEscaper.Replace(label))
			n, _ := strconv.Atoi(count)
			for range n {
				b.WriteString("<div></div>")
			}
			b.WriteString("</div>\n")
		default:
			if t, ok := tags[e.Kind]; ok {
				fmt.Fprintf(&b, t, htmlEscaper.Replace(e.Text))
			}
		}
	}
	b.WriteString("</body></html>\n")
	return b.Bytes(), nil
}

// State files: what `archiver envelope` last wrote, what was confirmed printed, and what the
// page would say now; each "<fingerprint> <epoch>". The notified file holds the reason last
// notified. None holds a secret.
func writtenFile(l layout.Layout) string  { return filepath.Join(l.LogDir(), ".envelope-written") }
func notifiedFile(l layout.Layout) string { return filepath.Join(l.LogDir(), ".envelope-notified") }

func writeRecord(path, fp string, at time.Time) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(fmt.Sprintf("%s %d\n", fp, at.Unix())), 0o644); err != nil { //nolint:gosec // G306: not a secret: state or notes meant to be readable
		return err
	}
	return os.Rename(tmp, path)
}

// Written is what Write produced: the fingerprint, and the files (no PDF when the page has
// text its fonts cannot show).
type Written struct {
	Fingerprint, HTML, PDF string
	PDFSkipped             bool
}

// Write writes dir/envelope-<host>.html and .pdf and records the fingerprint as written. The
// files are created private and moved into place: rewriting an existing file would keep its
// permissions, which could leave the secrets readable.
func Write(l layout.Layout, p *Page, host, dir string, now time.Time) (Written, error) {
	fp := p.Fingerprint()
	for i := range p.Elements {
		if p.Elements[i].Kind == "sub" {
			p.Elements[i].Text = fmt.Sprintf("Printed %s. Envelope fingerprint %s; 'archiver status' shows the current one.", now.Format("2006-01-02"), fp)
			break
		}
	}
	w := Written{Fingerprint: fp, HTML: filepath.Join(dir, "envelope-"+host+".html"), PDF: filepath.Join(dir, "envelope-"+host+".pdf")}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return w, err
	}
	html, err := p.HTML()
	if err != nil {
		return w, err
	}
	if err := writePrivate(w.HTML, html); err != nil {
		return w, err
	}
	pdf, err := p.PDF()
	switch {
	case errors.Is(err, ErrUnprintable):
		os.Remove(w.PDF)
		w.PDFSkipped = true
	case err != nil:
		return w, err
	default:
		if err := writePrivate(w.PDF, pdf); err != nil {
			return w, err
		}
	}
	return w, writeRecord(writtenFile(l), fp, now)
}

func writePrivate(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".envelope.")
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		os.Remove(f.Name())
		return err
	}
	return nil
}

// Confirm errors.
var (
	ErrNeverWritten = errors.New("no envelope has been written")
	ErrChanged      = errors.New("the configuration changed after the envelope was written")
)

// Confirm records the page Write last wrote as printed now: never the configuration as it is
// at confirm time, which may have changed since the page was printed.
func Confirm(l layout.Layout, p *Page, now time.Time) (string, error) {
	written, ok := read(writtenFile(l))
	if !ok {
		return "", ErrNeverWritten
	}
	if p.Fingerprint() != written.Fingerprint {
		return "", ErrChanged
	}
	if err := writeRecord(l.EnvelopeConfirmed(), written.Fingerprint, now); err != nil {
		return "", err
	}
	if err := writeRecord(l.EnvelopeCurrent(), written.Fingerprint, now); err != nil {
		return "", err
	}
	os.Remove(notifiedFile(l))
	return written.Fingerprint, nil
}

// Stale is why a confirmed envelope needs attention, for one warning and notification.
type Stale struct{ Title, Log, Notification string }

// Check records what the page would say now (for status) and returns, once per change, why a
// confirmed envelope no longer matches or is due its yearly check. Nothing is checked until
// an envelope has been confirmed printed.
func Check(l layout.Layout, p *Page, now time.Time) (*Stale, error) {
	fp := p.Fingerprint()
	if err := writeRecord(l.EnvelopeCurrent(), fp, now); err != nil {
		return nil, err
	}
	confirmed, ok := read(l.EnvelopeConfirmed())
	if !ok {
		return nil, nil
	}
	var reason string
	switch {
	case confirmed.Fingerprint != fp:
		reason = "changed:" + fp
	case now.Unix()-confirmed.At.Unix() > int64(MaxAge/time.Second):
		reason = "aged:" + strconv.FormatInt(confirmed.At.Unix(), 10)
	default:
		return nil, nil
	}
	if b, _ := os.ReadFile(notifiedFile(l)); strings.TrimSpace(string(b)) == reason {
		return nil, nil
	}
	if err := os.WriteFile(notifiedFile(l), []byte(reason+"\n"), 0o644); err != nil { //nolint:gosec // G306: not a secret: state or notes meant to be readable
		return nil, err
	}
	if strings.HasPrefix(reason, "changed:") {
		return &Stale{"Envelope Out of Date",
			"Envelope: the printed break-glass envelope is out of date (a secret or storage on it changed). Print a new one with 'archiver envelope', then run 'archiver envelope confirm'.",
			"The printed break-glass envelope no longer matches the configuration. Print a new one with 'archiver envelope' and confirm it."}, nil
	}
	return &Stale{"Envelope Check Due",
		"Envelope: the printed break-glass envelope was confirmed over 365 days ago. Check the paper is still there and readable, reprint if needed, and run 'archiver envelope confirm'.",
		"The break-glass envelope was confirmed over a year ago. Check it is still there and readable, then run 'archiver envelope confirm'."}, nil
}

// Compared is a page measured against the printed envelope, read-only.
type Compared struct {
	Confirmed   bool      // an envelope has been confirmed printed
	Matches     bool      // it says what this page says
	ConfirmedAt time.Time // when it was confirmed
	Due         bool      // older than MaxAge: its yearly check is due
}

// Compare measures p against the confirmed envelope without recording anything.
func Compare(l layout.Layout, p *Page, now time.Time) Compared {
	c, ok := read(l.EnvelopeConfirmed())
	if !ok {
		return Compared{}
	}
	return Compared{Confirmed: true, Matches: c.Fingerprint == p.Fingerprint(), ConfirmedAt: c.At,
		Due: now.Sub(c.At) > MaxAge}
}
