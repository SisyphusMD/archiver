package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/maintenance"
	"github.com/SisyphusMD/archiver/internal/pipeline"
)

// maintenanceCommand runs `archiver maintenance [exhaustive]`; ok is false for any other
// command line, which archiver.sh rejects with its usage.
func maintenanceCommand(args []string) (code int, ok bool) {
	force := false
	switch {
	case len(args) == 0:
	case len(args) == 1 && args[0] == "exhaustive":
		force = true
	default:
		return 0, false
	}
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	r := &maintenance.Run{
		Layout:          layout.Default(),
		Source:          config.FromEnvironment(),
		Environ:         os.Environ(),
		Hostname:        pipeline.Hostname(os.Getenv),
		Duplicacy:       "duplicacy",
		ForceExhaustive: force,
		Stdout:          os.Stdout,
		Stderr:          os.Stderr,
		Signals:         sigs,
		Now:             time.Now,
	}
	return r.Execute(), true
}
