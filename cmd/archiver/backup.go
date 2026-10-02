package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/hooks"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/pipeline"
)

const recoveryKitStep = "/opt/archiver/lib/scripts/recovery-kit-step.sh"

// goPipeline reports whether this deployment's backups run in Go. A bundle (config.sh), or
// services that all still carry service-backup-settings.sh, stay on the bash pipeline until
// migrated. ARCHIVER_PIPELINE=bash or =go overrides the choice.
func goPipeline(l layout.Layout, src config.Source) bool {
	switch src.Getenv("ARCHIVER_PIPELINE") {
	case "bash":
		return false
	case "go":
		return true
	}
	if _, err := os.Stat(l.ConfigFile()); err == nil {
		return false
	}
	cfg, _, err := config.Load(src, nil)
	if err != nil {
		return true // the Go pipeline reports the configuration error
	}
	dirs, _ := config.ExpandServiceDirectories(cfg.ServiceDirectories)
	legacy, executable := false, false
	for _, d := range dirs {
		if exists(filepath.Join(d, hooks.Legacy)) {
			legacy = true
		}
		for _, f := range []string{hooks.PreBackup, hooks.PostBackup, hooks.Filters} {
			if exists(filepath.Join(d, f)) {
				executable = true
			}
		}
	}
	// A partly migrated deployment runs in Go, which refuses the unmigrated services
	// loudly; bash would run their hooks but silently ignore the migrated ones'.
	return !legacy || executable
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// backupCommand runs `archiver backup [--detach]` in Go, or returns false to leave the
// command line to bash (other arguments, or a deployment not on the Go pipeline).
func backupCommand(args []string) (int, bool) {
	detach := false
	switch {
	case len(args) == 0:
	case len(args) == 1 && (args[0] == "--detach" || args[0] == "-d"):
		detach = true
	default:
		return 0, false
	}
	l := layout.Default()
	src := config.FromEnvironment()
	if !goPipeline(l, src) {
		return 0, false
	}
	if detach {
		return detachBackup(l), true
	}
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	b := &pipeline.Backup{
		Layout:          l,
		Source:          src,
		Environ:         os.Environ(),
		Hostname:        pipeline.Hostname(os.Getenv),
		Stdout:          os.Stdout,
		Stderr:          os.Stderr,
		Duplicacy:       "duplicacy",
		RecoveryKitStep: recoveryKitStep,
		Signals:         sigs,
	}
	stop := make(chan struct{})
	go b.Watch(stop)
	code := b.Run()
	close(stop)
	return code, true
}

// detachBackup starts the backup in its own session with its output discarded, refusing
// first, visibly, if one is running: the detached run's own refusal would go nowhere.
func detachBackup(l layout.Layout) int {
	if h, ok, _ := lockstate.ReadLock(l.BackupLock()); ok && h.Alive() {
		fmt.Fprintf(os.Stderr, "A backup is already running (PID %d). Not starting another.\n", h.PID)
		return 1
	}
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "archiver:", err)
		return 1
	}
	defer devnull.Close()
	cmd := exec.Command(selfPath, "backup")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, devnull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "archiver: cannot start the backup:", err)
		return 1
	}
	cmd.Process.Release()
	fmt.Println("Backup started in the background (follow with 'archiver logs').")
	return 0
}
