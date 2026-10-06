package main

import (
	"os"
	"time"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/daemon"
	"github.com/SisyphusMD/archiver/internal/layout"
	"github.com/SisyphusMD/archiver/internal/logging"
	"github.com/SisyphusMD/archiver/internal/notify"
	"github.com/SisyphusMD/archiver/internal/pipeline"
	"github.com/SisyphusMD/archiver/internal/runctl"
)

// runControl runs `archiver stop`, `pause` and `resume`; ok is false for any other command.
// A malformed command line gets the usage.
func runControl(cmd string, args []string) (code int, ok bool) {
	switch cmd {
	case "stop":
		target, immediate, targets := "all", false, 0
		for _, a := range args {
			switch a {
			case "--immediate":
				immediate = true
			case "backup", "maintenance", "all":
				target = a
				targets++
			default:
				return 0, false
			}
		}
		if targets > 1 {
			return 0, false // ambiguous
		}
		return runctl.Stop(controlEnv(), target, immediate), true
	case "pause":
		if len(args) > 0 {
			return 0, false
		}
		return runctl.Pause(controlEnv()), true
	case "resume":
		if len(args) > 1 || (len(args) == 1 && args[0] != "logs") {
			return 0, false
		}
		code := runctl.Resume(controlEnv())
		if len(args) == 1 && code == 0 {
			return followLogs(), true
		}
		return code, true
	}
	return 0, false
}

func controlEnv() runctl.Env {
	l := layout.Default()
	log := &logging.Log{Dir: l.LogDir(), Basename: "archiver", Stdout: os.Stdout}
	e := runctl.Env{
		Layout: l, Out: os.Stdout, Log: log, Now: time.Now,
		Notify: func(string, string) {},
		Workers: func(cmd string) bool {
			reply, err := daemon.Send(l.DaemonSocket(), cmd)
			return err == nil && reply == daemon.ReplyOK
		},
	}
	// Notifying needs the configuration; acting on the run does not, so a configuration
	// error only costs the notification.
	if cfg, _, err := config.Load(config.FromEnvironment(), nil); err == nil && cfg.Pushover() && cfg.PushoverAPIToken != "" && cfg.PushoverUserKey != "" {
		n := &notify.Notifier{
			Pushover: true, Token: cfg.PushoverAPIToken, User: cfg.PushoverUserKey,
			Hostname: pipeline.Hostname(os.Getenv),
			Logf: func(failed bool, msg string) {
				level := logging.Info
				if failed {
					level = logging.Error
				}
				log.Message(level, "", msg)
			},
		}
		e.Notify = func(title, msg string) { n.Send(title, msg) }
	}
	return e
}
