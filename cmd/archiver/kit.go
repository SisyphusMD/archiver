package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/kit"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/notify"
	"github.com/SisyphusMD/archiver/internal/pipeline"
)

// kitRun is a kit refresh logging to the backup log, its errors notified as the backup's.
func kitRun(src config.Source) (*kit.Run, *notify.Notifier) {
	l := layout.Default()
	host := pipeline.Hostname(os.Getenv)
	log := &logging.Log{Dir: l.LogDir(), Basename: "archiver", Stdout: os.Stdout}
	var n *notify.Notifier
	if cfg, _, err := config.Load(src, os.Environ()); err == nil {
		n = notify.FromConfig(cfg, host, nil)
		n.Incidents = l.Incidents()
		n.WatchLog(log)
	}
	r := &kit.Run{
		Layout:   l,
		Source:   src,
		Environ:  os.Environ(),
		Hostname: host,
		Log:      log,
	}
	r.EnvelopeCheck = func() error { return envelopeCheck(l, src, log, n.Send) }
	return r, n
}

// kitExecute runs the kit as one incident (ADR 36): its errors notify together, and a kit
// current everywhere again says so.
func kitExecute(r *kit.Run, n *notify.Notifier) int {
	code := r.Execute()
	switch {
	case r.Log.Reportable() > 0:
		n.Raise("kit", notify.Failure, "Recovery Kit Failed", r.Log.Summary())
	// A kit still missing from a storage skipped as down stays an incident, so it repeats
	// and retries; one is not opened for it, since the storage's own alert says why.
	case len(r.Skip) > 0:
		if n.IsOpen("kit") {
			n.Raise("kit", notify.Failure, "Recovery Kit Failed", "The recovery kit is not current on "+strings.Join(r.Skip, ", ")+": the storage is down, and the kit is placed when it is back.")
		}
	case code == kit.Unverified:
		if n.IsOpen("kit") {
			n.Raise("kit", notify.Failure, "Recovery Kit Failed", "The recovery kit was placed but could not be verified readable on every storage; it is placed again next run. See archiver.log.")
		}
	case code == kit.OK:
		n.Clear("kit", "Recovery Kit Recovered", "The recovery kit is current on every storage again.")
	}
	return code
}

// recoveryKitCommand runs `archiver recovery-kit [force]`; ok is false for any other command
// line, which gets the usage.
func recoveryKitCommand(args []string) (code int, ok bool) {
	force := false
	switch {
	case len(args) == 0:
	case len(args) == 1 && args[0] == "force":
		force = true
	default:
		return 0, false
	}
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
		fmt.Fprintf(os.Stderr, "The recovery kit is not configured. Provide the recovery password at %s/recovery_password (or point RECOVERY_PASSWORD_FILE at it).\n", src.SecretsDir)
		return 1, true
	}
	r, n := kitRun(src)
	r.Force = force
	switch kitExecute(r, n) {
	case kit.OK:
		fmt.Printf("Recovery kit complete: %s is current on all storage targets.\n", r.KitName())
		return 0, true
	case kit.Unverified:
		fmt.Fprintln(os.Stderr, "Recovery kit placed, but at least one target could not be verified readable by a mirror/backup user; see the log. It will be re-placed on the next run.")
	default:
		fmt.Fprintln(os.Stderr, "Recovery kit finished with errors; see the log. Failed targets will be retried on the next run.")
	}
	return 1, true
}

// recoveryKitStep is the backup pipeline's kit step, run as its own process so the backup can
// bound and end it: exit 0, 1 (errors, each logged and notified), 2 (placed but not verified
// readable everywhere) or 3 (only secondary uploads failed).
func recoveryKitStep() int {
	r, n := kitRun(config.FromEnvironment())
	r.Skip = strings.Fields(os.Getenv("ARCHIVER_KIT_SKIP_TARGETS"))
	r.PrimaryMarker = os.Getenv("ARCHIVER_KIT_PRIMARY_MARKER")
	return kitExecute(r, n)
}
