package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/envelope"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/pipeline"
)

// envelopePage builds the envelope from the configuration as provided.
func envelopePage(l layout.Layout, src config.Source) (*envelope.Page, error) {
	s, err := config.Snapshot(src, os.Environ())
	if err != nil {
		return nil, err
	}
	return envelope.Build(envelope.Source{Settings: s, Hostname: pipeline.Hostname(os.Getenv), SSHKeyFile: l.SSHPrivateKey()}), nil
}

// envelopeCheck is the recovery kit's envelope check: it records what the page would say now
// and warns and notifies once when a confirmed envelope goes stale.
func envelopeCheck(l layout.Layout, src config.Source, log *logging.Log, notify func(title, msg string)) error {
	p, err := envelopePage(l, src)
	if err != nil {
		return err
	}
	stale, err := envelope.Check(l, p, time.Now())
	if err != nil || stale == nil {
		return err
	}
	log.Message(logging.Warning, "", stale.Log)
	if notify != nil {
		notify(stale.Title, stale.Notification)
	}
	return nil
}

// envelopeCommand runs `archiver envelope [DIR]` and `archiver envelope confirm`; ok is false
// for any other command line, which gets the usage.
func envelopeCommand(args []string) (code int, ok bool) {
	if len(args) > 1 {
		return 0, false
	}
	l := layout.Default()
	src := config.FromEnvironment()
	cfg, _, err := config.Load(src, os.Environ())
	if err == nil {
		err = cfg.ValidateStorage(src.SecretsDir)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "archiver:", err)
		return 1, true
	}
	if cfg.RecoveryPassword == "" {
		fmt.Fprintf(os.Stderr, "The envelope carries the recovery kit's password, and the recovery kit is not configured. Provide the recovery password at %s/recovery_password (or point RECOVERY_PASSWORD_FILE at it).\n", src.SecretsDir)
		return 1, true
	}
	p, err := envelopePage(l, src)
	if err != nil {
		fmt.Fprintln(os.Stderr, "archiver:", err)
		return 1, true
	}
	if len(args) == 1 && args[0] == "confirm" {
		fp, err := envelope.Confirm(l, p, time.Now())
		switch {
		case errors.Is(err, envelope.ErrNeverWritten):
			fmt.Fprintln(os.Stderr, "No envelope has been written yet: run 'archiver envelope', print it, then confirm.")
		case errors.Is(err, envelope.ErrChanged):
			fmt.Fprintln(os.Stderr, "The configuration changed after the envelope was written, so the printed page is already out of date. Run 'archiver envelope' again, print the new page, then confirm.")
		case err != nil:
			fmt.Fprintln(os.Stderr, "Could not record the envelope as printed.")
		default:
			fmt.Printf("Recorded envelope %s as printed. 'archiver status' will say when it goes out of date.\n", fp)
			return 0, true
		}
		return 1, true
	}
	dir := filepath.Join(l.Root, "envelope")
	if len(args) == 1 {
		dir = args[0]
	}
	w, err := envelope.Write(l, p, pipeline.Hostname(os.Getenv), dir, time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not write the envelope to %s.\n", dir)
		return 1, true
	}
	if w.PDFSkipped {
		fmt.Fprintln(os.Stderr, "The PDF was not written: some text on the page (a password or credential) has characters the PDF's fonts cannot show. Print the HTML page instead.")
	}
	fmt.Printf("Wrote the break-glass envelope (fingerprint %s):\n", w.Fingerprint)
	if !w.PDFSkipped {
		fmt.Println("  " + w.PDF)
	}
	fmt.Printf("  %s   (the same page, for printing from a browser)\n", w.HTML)
	fmt.Print(`
They hold your recovery password and storage credentials in PLAINTEXT. Print one, then:
  1. archiver envelope confirm
  2. delete both files (and any copies, e.g. after 'docker cp').
`)
	return 0, true
}
