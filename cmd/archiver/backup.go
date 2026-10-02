package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/hooks"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/pipeline"
	"github.com/SisyphusMD/archiver/internal/runlock"
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
	// Any mix of --detach and -d, as archiver.sh accepts; anything else is bash's to reject.
	detach := false
	for _, a := range args {
		if a != "--detach" && a != "-d" {
			return 0, false
		}
		detach = true
	}
	l := layout.Default()
	src := config.FromEnvironment()
	if !goPipeline(l, src) {
		// The daemon decided ownership with its own environment; a bash run forced here
		// copies inline and would be a second copier into each target.
		// Bash copies without the workers' copy locks, so it is refused whenever workers run,
		// whatever its storage settings.
		if reply, err := daemon.Send(l.DaemonSocket(), daemon.CmdWorkers); err == nil && reply == daemon.ReplyOK {
			fmt.Fprintln(os.Stderr, "Copy workers keep this deployment's secondary storages, and the bash pipeline would copy alongside them. Run the backup without ARCHIVER_PIPELINE=bash.")
			return 1, true
		}
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

// migrateHooks runs `archiver migrate hooks [DIR...]`: every configured service directory,
// or the ones named.
func migrateHooks(dirs []string) int {
	// Migrated services run only on the Go pipeline; converting them for a deployment that
	// must stay on bash would leave every one of their backups refused.
	if _, err := os.Stat(layout.Default().ConfigFile()); err == nil {
		fmt.Fprintln(os.Stderr, "This deployment still uses a bundle (config.sh), which runs the bash pipeline. Migrate the bundle first ('archiver migrate'), then the hooks.")
		return 1
	}
	if os.Getenv("ARCHIVER_PIPELINE") == "bash" {
		fmt.Fprintln(os.Stderr, "ARCHIVER_PIPELINE=bash keeps this deployment on the bash pipeline, which does not run migrated hooks. Unset it before migrating.")
		return 1
	}
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
