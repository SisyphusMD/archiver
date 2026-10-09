package main

import (
	"fmt"
	"os"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/notify"
	"github.com/SisyphusMD/archiver/internal/pipeline"
)

// notifyTest runs `archiver notify test`: one message to every destination, whatever it
// receives, saying what that is, so each can be checked by eye (ADR 36).
func notifyTest() int {
	cfg, _, err := config.Load(config.FromEnvironment(), os.Environ())
	if err != nil {
		fmt.Fprintln(os.Stderr, "archiver:", err)
		return 1
	}
	failed := false
	logf := func(f bool, msg string) {
		failed = failed || f
		fmt.Println(msg)
	}
	host := pipeline.Hostname(os.Getenv)
	all := notify.FromConfig(cfg, host, logf)
	if len(all.Destinations) == 0 {
		fmt.Println("No notification destination is configured (PUSHOVER_*, APPRISE_URL, NTFY_URL): failures are only in the logs.")
		return 1
	}
	repeat := "never repeats"
	if d := cfg.AlertRepeat(); d > 0 {
		repeat = "repeats every " + d.String()
	}
	for _, d := range all.Destinations {
		one := &notify.Notifier{Hostname: host, Logf: logf, Destinations: []notify.Destination{{Sink: d.Sink, On: "everything"}}}
		one.SendKind(notify.Failure, "Archiver Test", fmt.Sprintf("A test from 'archiver notify test'. This destination receives %s; an ongoing failure %s.", receives(d.On), repeat))
	}
	if failed {
		return 1
	}
	return 0
}

func receives(on string) string {
	switch on {
	case "everything":
		return "everything (NOTIFY_ON=everything): failures, problems and routine events"
	case "problems":
		return "failures and problems (NOTIFY_ON=problems)"
	}
	return "failures only (NOTIFY_ON=failures)"
}
