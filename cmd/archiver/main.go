// Command archiver is the image's CLI and its entrypoint: every command runs here (ADR 8,
// ADR 10).
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/health"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/logview"
	"github.com/SisyphusMD/archiver/internal/status"
)

const (
	selfPath = "/usr/local/bin/archiver"
)

// ported maps each command that takes no arguments to its entry point, which returns the
// exit status; a command line with any gets the usage.
var ported = map[string]func() int{
	"status": func() int {
		if err := status.Write(os.Stdout, layout.Default(), time.Now()); err != nil {
			fmt.Fprintln(os.Stderr, "archiver:", err)
			return 1
		}
		return 0
	},
	"healthcheck": func() int {
		return health.Run(os.Stdout, layout.Default(), os.Getenv, time.Now())
	},
	"logs":             followLogs,
	"restore":          func() int { return restoreEnv().Interactive() },
	"auto-restore":     func() int { return restoreEnv().Auto() },
	"auto-restore-all": func() int { return restoreEnv().AutoAll() },
	"snapshot-exists":  func() int { return restoreEnv().SnapshotExists() },
}

func main() {
	config.PurgeRawSecrets()
	if len(os.Args) >= 2 && os.Args[1] == "entrypoint" {
		os.Exit(entrypointCommand(os.Args[2:]))
	}
	if len(os.Args) >= 2 && os.Args[1] == "daemon" {
		os.Exit(runDaemon(os.Args[2:]))
	}
	if len(os.Args) >= 2 && os.Args[1] == "backup" {
		if code, ok := backupCommand(os.Args[2:]); ok {
			os.Exit(code)
		}
	}
	if len(os.Args) >= 2 {
		if code, ok := runControl(os.Args[1], os.Args[2:]); ok {
			os.Exit(code)
		}
	}
	if len(os.Args) >= 2 && os.Args[1] == "maintenance" {
		if code, ok := maintenanceCommand(os.Args[2:]); ok {
			os.Exit(code)
		}
	}
	// Extra arguments were always ignored.
	if len(os.Args) >= 2 && os.Args[1] == "init" {
		os.Exit(initCommand())
	}
	if len(os.Args) >= 2 && os.Args[1] == "envelope" {
		if code, ok := envelopeCommand(os.Args[2:]); ok {
			os.Exit(code)
		}
	}
	if len(os.Args) >= 2 && os.Args[1] == "recovery-kit" {
		if code, ok := recoveryKitCommand(os.Args[2:]); ok {
			os.Exit(code)
		}
	}
	if len(os.Args) == 2 && os.Args[1] == "recovery-kit-step" {
		os.Exit(recoveryKitStep())
	}
	if len(os.Args) >= 2 && os.Args[1] == "mirror" {
		os.Exit(mirrorCommand(os.Args[2:]))
	}
	if len(os.Args) >= 3 && os.Args[1] == "migrate" && os.Args[2] == "hooks" {
		os.Exit(migrateHooks(os.Args[3:]))
	}
	if len(os.Args) == 2 {
		if run, ok := ported[os.Args[1]]; ok {
			os.Exit(run())
		}
	}
	os.Exit(usage(os.Args[1:]))
}

// followLogs runs the log viewer until the backup ends or a signal stops it, and then exits
// with 128 plus that signal's number, as a shell reports a command a signal ended.
func followLogs() int {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigs)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	caught := make(chan syscall.Signal, 1)
	go func() {
		if s, ok := (<-sigs).(syscall.Signal); ok {
			caught <- s
		}
		cancel()
	}()
	code := logview.Follow(ctx, os.Stdout, layout.Default(), logview.Options{})
	if code == logview.Interrupted {
		select {
		case s := <-caught:
			return 128 + int(s)
		default:
		}
	}
	return code
}

// runDaemon runs the schedules until SIGTERM or SIGINT. With --check it only validates
// them, so the entrypoint can refuse to start before anything else comes up.
func runDaemon(args []string) int {
	if len(args) == 2 && args[0] == "ctl" {
		return daemonCtl(args[1])
	}
	check := len(args) == 1 && args[0] == "--check"
	if len(args) > 0 && !check {
		fmt.Fprintln(os.Stderr, "usage: archiver daemon [--check | ctl local-changed|stop|pause|resume]")
		return 2
	}
	jobs, err := daemon.Jobs(os.Getenv, time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, "archiver:", err)
		return 1
	}
	if check {
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	l := layout.Default()
	workers := &copyWorkers{}
	workers.decide(l)
	if ln, err := daemon.Serve(l.DaemonSocket(), workers.handle); errors.Is(err, daemon.ErrRunning) {
		fmt.Fprintln(os.Stderr, "archiver daemon:", err)
		return 1
	} else if err != nil {
		fmt.Fprintln(os.Stderr, "archiver daemon: no control socket, so backups copy for themselves:", err)
	} else {
		defer func() { ln.Close(); os.Remove(l.DaemonSocket()) }()
		workers.start(ctx.Done())
		defer workers.shutdown()
	}
	daemon.Run(ctx, daemon.RealClock, os.Stdout, jobs, func(j daemon.Job) int {
		// A fresh process per run, as under cron: each run starts clean.
		cmd := exec.Command(selfPath, j.Name)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				return exit.ExitCode()
			}
			fmt.Fprintf(os.Stderr, "archiver daemon: cannot run %s: %v\n", j.Name, err)
			return 127
		}
		return 0
	})
	return 0
}

// daemonCtl sends one command to a running daemon. With no daemon it exits 1 quietly.
func daemonCtl(cmd string) int {
	// Asked whether workers keep the storages, or told local changed, the daemon must know
	// which storages the caller means: with other settings, the caller keeps its own.
	if cmd == daemon.CmdWorkers || cmd == daemon.CmdLocalChanged || cmd == daemon.CmdExhaustive {
		if cfg, _, err := config.Load(config.FromEnvironment(), nil); err == nil {
			cmd += " " + cfg.StorageFingerprint()
		}
	}
	reply, err := daemon.Send(layout.Default().DaemonSocket(), cmd)
	if err != nil {
		return 1
	}
	fmt.Println(reply)
	if reply != daemon.ReplyOK && reply != daemon.ReplyNoWorkers {
		return 1
	}
	return 0
}

// mirrorCommand runs `archiver mirror --dry-run` (what each worker's next mirror pass
// would delete) or `archiver mirror --allow-large` (let the next pass exceed the cap on
// deleting more than half of an ID, for an intended retention change; ADR 12).
func mirrorCommand(args []string) int {
	if len(args) != 1 || (args[0] != "--dry-run" && args[0] != "--allow-large") {
		fmt.Fprintln(os.Stderr, "usage: archiver mirror --dry-run | --allow-large")
		return 2
	}
	cmd := daemon.CmdMirrorPlan
	if args[0] == "--allow-large" {
		cmd = daemon.CmdAllowLarge
	}
	reply, err := daemon.SendWithin(layout.Default().DaemonSocket(), cmd, 15*time.Minute)
	if err != nil || reply == daemon.ReplyNoWorkers {
		fmt.Fprintln(os.Stderr, "Mirroring runs in the copy workers, which run under the daemon (a container with BACKUP_SCHEDULE set) when there are secondary storages.")
		return 1
	}
	if cmd == daemon.CmdAllowLarge {
		fmt.Println("The next mirror pass on each secondary may delete more than half of a snapshot ID's revisions.")
		return 0
	}
	rc := 0
	for _, line := range strings.Split(reply, " | ") {
		fmt.Println(line)
		if strings.Contains(line, ": cannot plan (") {
			rc = 1
		}
	}
	return rc
}
