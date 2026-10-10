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
	"github.com/SisyphusMD/archiver/internal/runlock"
)

// backupCommand runs `archiver backup [--detach]`; ok is false for other arguments, which
// get the usage.
func backupCommand(args []string) (int, bool) {
	// Any mix of --detach and -d.
	detach := false
	for _, a := range args {
		if a != "--detach" && a != "-d" {
			return 0, false
		}
		detach = true
	}
	l := layout.Default()
	src := config.FromEnvironment()
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
		RecoveryKitStep: []string{selfPath, "recovery-kit-step"},
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
	_ = cmd.Process.Release()
	fmt.Println("Backup started in the background (follow with 'archiver logs').")
	return 0
}

// migrateHooks runs `archiver migrate hooks [DIR...]`: every configured service directory,
// or the ones named.
func migrateHooks(dirs []string) int {
	if len(dirs) == 0 {
		cfg, _, err := config.Load(config.FromEnvironment(), nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, "archiver:", err)
			return 1
		}
		var unmatched []string
		dirs, unmatched = config.ExpandServiceDirectories(cfg.ServiceDirectories)
		for _, u := range unmatched {
			fmt.Fprintf(os.Stderr, "SERVICE_DIRECTORIES entry '%s' matches no directory; skipped.\n", u)
		}
		if len(dirs) == 0 {
			fmt.Fprintln(os.Stderr, "No service directories to migrate. Set SERVICE_DIRECTORIES, or name the directories.")
			return 1
		}
	}
	// Held throughout: a backup reading a service's files mid-migration could see neither
	// the old settings file nor the new hooks, and back up without either.
	l := layout.Default()
	lock, _, err := runlock.Acquire(l.BackupLock(), filepath.Join(l.Lock, "archiver-stop-requested"), "migrate", "hooks")
	if busy, ok := err.(*runlock.Busy); ok {
		fmt.Fprintf(os.Stderr, "A backup is running (PID %d). Migrate the hooks once it has finished.\n", busy.Holder.PID)
		return 1
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "archiver: cannot take the backup lock:", err)
		return 1
	}
	defer lock.Release()
	code, migrated := 0, 0
	for _, d := range dirs {
		m, err := hooks.Migrate(d, pipeline.Hostname(os.Getenv))
		if err != nil {
			fmt.Fprintln(os.Stderr, "archiver:", err)
			code = 1
			continue
		}
		if m == nil {
			continue
		}
		migrated++
		fmt.Printf("%s: wrote %v; kept the old file as %s.\n", d, m.Wrote, hooks.LegacyKept)
		for _, w := range m.Warnings {
			fmt.Fprintln(os.Stderr, "WARNING:", w)
		}
	}
	fmt.Printf("Migrated %d service directories.\n", migrated)
	return code
}
