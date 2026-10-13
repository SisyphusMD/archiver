package main

import (
	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/deepcheck"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/metrics"
	"github.com/SisyphusMD/archiver/internal/pipeline"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func deepCheckCommand(args []string) int {
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigs)
	l := layout.Default()
	defer func() { _ = metrics.WriteFile(l, os.Getenv, time.Now()) }()
	r := &deepcheck.Run{Layout: l, Source: config.FromEnvironment(), Environ: os.Environ(), Hostname: pipeline.Hostname(os.Getenv), Duplicacy: "duplicacy", Stdout: os.Stdout, Stderr: os.Stderr, Signals: sigs}
	if len(args) == 1 {
		r.Storage = args[0]
	}
	return r.Execute()
}
