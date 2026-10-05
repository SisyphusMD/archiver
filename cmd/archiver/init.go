package main

import (
	"os"
	"time"

	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/setup"
)

// initCommand runs `archiver init`.
func initCommand() int {
	l := layout.Default()
	return (&setup.Init{
		KeysDir:  l.Root + "/keys",
		SetupDir: l.Root + "/setup",
		In:       os.Stdin,
		Out:      os.Stdout,
		Now:      time.Now,
		Hide:     setup.HideTerminal(os.Stdin),
	}).Run()
}
