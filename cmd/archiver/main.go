// Command archiver is the image's CLI, midway through replacing the bash implementation
// (ADR 8). Commands already ported run here; every other command line runs the bash program
// that implements it, which replaces this process, so signals, the terminal, and the exit
// status pass straight through as if the bash program had been called directly.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/health"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/logview"
	"github.com/SisyphusMD/archiver/internal/status"
)

const (
	bashCLI    = "/opt/archiver/archiver.sh"
	initScript = "/opt/archiver/lib/scripts/init.sh"
	selfPath   = "/usr/local/bin/archiver"
)

// ported maps each command implemented in Go to its entry point, which returns the exit
// status. They take no arguments; a command line with any is left to archiver.sh, so its
// usage errors stay exactly as they were.
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
	"logs": followLogs,
}

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "daemon" {
		os.Exit(runDaemon(os.Args[2:]))
	}
	if len(os.Args) >= 2 && os.Args[1] == "backup" {
		if code, ok := backupCommand(os.Args[2:]); ok {
			os.Exit(code)
		}
	}
	if len(os.Args) >= 3 && os.Args[1] == "migrate" && os.Args[2] == "hooks" {
		os.Exit(migrateHooks(os.Args[3:]))
	}
	if len(os.Args) == 2 {
		if run, ok := ported[os.Args[1]]; ok {
			os.Exit(run())
		}
	}
	target, args := route(os.Args[1:])
	argv := append([]string{target}, args...)
	err := syscall.Exec(target, argv, os.Environ())
	// Exec only returns on failure.
	fmt.Fprintf(os.Stderr, "archiver: cannot run %s: %v\n", target, err)
	os.Exit(127)
}

// route picks the bash program that implements a command line and the arguments it gets.
// init has no archiver.sh verb: the entrypoint used to run its script directly, and now
// comes through here like every other caller.
func route(args []string) (string, []string) {
	if len(args) > 0 && args[0] == "init" {
		return initScript, args[1:]
	}
	return bashCLI, args
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
		// A fresh process per run, as under cron: the bash pipeline keeps its own state.
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

// daemonCtl sends one command to a running daemon. With no daemon it exits 1 quietly:
// the bash stop, pause and resume call it whether or not one runs.
func daemonCtl(cmd string) int {
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
