// Command archiver is the image's CLI and its entrypoint: every command runs here (ADR 8,
// ADR 10).
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/SisyphusMD/archiver/internal/backuphealth"
	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/doctor"
	"github.com/SisyphusMD/archiver/internal/health"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/lockstate"
	"github.com/SisyphusMD/archiver/internal/logview"
	"github.com/SisyphusMD/archiver/internal/restore"
	"github.com/SisyphusMD/archiver/internal/setup"
	"github.com/SisyphusMD/archiver/internal/status"
)

const (
	selfPath = "/usr/local/bin/archiver"
)

// ported maps each command that takes no arguments to its entry point, which returns the
// exit status; a command line with any gets the usage.
var ported = map[string]func() int{
	"status": func() int {
		if err := status.Write(os.Stdout, layout.Default(), os.Getenv, time.Now()); err != nil {
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
	if len(os.Args) >= 3 && os.Args[1] == "recover" {
		if code, ok := recoverCommand(os.Args[2:]); ok {
			os.Exit(code)
		}
	}
	if len(os.Args) == 3 && os.Args[1] == "status" && os.Args[2] == "--json" {
		if err := status.WriteJSON(os.Stdout, layout.Default(), os.Getenv, time.Now()); err != nil {
			fmt.Fprintln(os.Stderr, "archiver:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if len(os.Args) == 3 && os.Args[1] == "health" && os.Args[2] == "--backups" {
		os.Exit(backupHealth())
	}
	if len(os.Args) == 3 && os.Args[1] == "notify" && os.Args[2] == "test" {
		os.Exit(notifyTest())
	}
	if len(os.Args) >= 2 && os.Args[1] == "doctor" && (len(os.Args) == 2 || (len(os.Args) == 3 && os.Args[2] == "--notify")) {
		os.Exit(doctorCommand(len(os.Args) == 3))
	}
	if len(os.Args) >= 2 && os.Args[1] == "drill" && len(os.Args) <= 4 {
		o := restore.DrillOptions{}
		if len(os.Args) >= 3 {
			o.Service = os.Args[2]
		}
		if len(os.Args) == 4 {
			o.Storage = os.Args[3]
		}
		os.Exit(restoreEnv().Drill(o))
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
	noteDrillSchedule(jobs)
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

// noteDrillSchedule records when drills were first scheduled, so the healthcheck can tell
// drills that never run (each failing before it records anything) from ones not yet due.
func noteDrillSchedule(jobs []daemon.Job) {
	for _, j := range jobs {
		if j.Name != "drill" {
			continue
		}
		l := layout.Default()
		s, err := lockstate.ReadDrillState(l.DrillState())
		if err == nil && s.Scheduled == 0 {
			s.Scheduled = time.Now().Unix()
			lockstate.WriteDrillState(l.DrillState(), s)
		}
	}
}

// doctorCommand runs `archiver doctor` (ADR 30); notify also sends a test notification.
func doctorCommand(notify bool) int {
	re := restoreEnv()
	re.Stdout, re.Stderr = io.Discard, io.Discard
	return doctor.Run(doctor.Env{
		Layout:    re.Layout,
		Source:    re.Source,
		Environ:   re.Environ,
		Hostname:  re.Hostname,
		Duplicacy: re.Duplicacy,
		Out:       os.Stdout,
		CapEff:    re.CapEff,
		Notify:    notify,
		Probe:     re.Snapshots,
	})
}

// recoverCommand runs `archiver recover KIT [--yes] [--hook] [--out DIR]` (ADR 29). The kit
// password comes from the recovery_password secret when one is mounted, else a hidden prompt.
func recoverCommand(args []string) (int, bool) {
	o := restore.RecoverOptions{Out: "/opt/archiver/recovered", Migrate: migrateRestoredWith}
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--yes":
			o.Yes = true
		case a == "--hook":
			o.Hook = true
		case a == "--out" && i+1 < len(args):
			i++
			o.Out = args[i]
		case !strings.HasPrefix(a, "-") && o.Kit == "":
			o.Kit = a
		default:
			return 0, false
		}
	}
	if o.Kit == "" {
		return 0, false
	}
	src := config.FromEnvironment()
	path := os.Getenv("RECOVERY_PASSWORD_FILE")
	if path == "" {
		path = filepath.Join(src.SecretsDir, "recovery_password")
	}
	if b, err := os.ReadFile(path); err == nil {
		o.Password = strings.TrimRight(string(b), "\r\n")
	} else {
		fmt.Print("Recovery kit password: ")
		var restore func()
		if hide := setup.HideTerminal(os.Stdin); hide != nil {
			restore = hide()
		}
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if restore != nil {
			restore()
		}
		fmt.Println()
		o.Password = strings.TrimRight(line, "\r\n")
	}
	return restoreEnv().Recover(o), true
}

// backupHealth runs `archiver health --backups`: backup health and why, exiting 0 OK, 1
// DEGRADED or 2 FAILING for monitors (ADR 32).
func backupHealth() int {
	h := backuphealth.Compute(layout.Default(), os.Getenv, time.Now())
	fmt.Printf("Backup health: %s\n", h.State)
	for _, r := range h.Reasons {
		fmt.Printf("  %s\n", r)
	}
	return h.ExitCode()
}
