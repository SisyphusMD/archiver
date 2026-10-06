package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/SisyphusMD/archiver/internal/entrypoint"
)

const usageText = `Usage: archiver {backup|maintenance|stop|pause|resume|logs|status|migrate hooks|mirror|recovery-kit|envelope|restore|auto-restore|auto-restore-all|snapshot-exists|init|healthcheck|help}
Note:
  backup runs the backup pipeline (hooks -> backup -> copies); add --detach to run it in the background.
  maintenance runs per-storage check + prune now (normally scheduled via MAINTENANCE_SCHEDULE); 'maintenance exhaustive' forces the full-listing prune.
  stop takes an optional target (backup|maintenance|all, default all) and --immediate.
  resume may be used in combination with logs.
  mirror --dry-run shows what the copy workers' next mirror pass would delete on each secondary; mirror --allow-large lets that pass exceed the cap.
  migrate hooks [DIR...] converts each service's service-backup-settings.sh into executable pre-backup/post-backup hooks and a filters file.
  recovery-kit uploads the encrypted recovery kit to every storage target; 'recovery-kit force' re-uploads even if unchanged.
  envelope [DIR] writes the printable break-glass envelope (default /opt/archiver/envelope); 'envelope confirm' records it as printed.
  pause|logs|status|restore|auto-restore|auto-restore-all|snapshot-exists|healthcheck|help cannot have further arguments.
  auto-restore and snapshot-exists are non-interactive and driven by environment variables.
`

// usage handles every command line no command took: help, or a mistake, which it names
// before the usage (exit 1). A removed command says what replaced it.
func usage(args []string) int {
	if len(args) == 0 {
		fmt.Println("No arguments provided.")
		fmt.Print(usageText)
		return 1
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "help":
		if len(rest) == 0 {
			fmt.Print(usageText)
			return 0
		}
	case "start", "restart":
		fmt.Fprintf(os.Stderr, "'%s' was removed. Use 'archiver backup --detach' to run a backup in the background\n(and 'archiver stop backup' first if you were restarting). Pruning moved to 'archiver maintenance'.\n", cmd)
		return 1
	case "bundle":
		fmt.Fprint(os.Stderr, entrypoint.BundleHelp)
		return 2
	case "migrate":
		if len(rest) == 0 || rest[0] != "hooks" {
			fmt.Fprint(os.Stderr, entrypoint.BundleHelp)
			return 2
		}
	case "backup":
		for _, a := range rest {
			switch a {
			case "prune", "retain":
				fmt.Fprintf(os.Stderr, "'%s' was removed: backups no longer prune. Pruning runs on MAINTENANCE_SCHEDULE or via 'archiver maintenance'.\n", a)
				return 1
			case "--detach", "-d":
			default:
				fmt.Printf("'%s' is not valid for 'archiver backup' (allowed: --detach).\n", a)
			}
		}
	case "stop":
		targets := 0
		for _, a := range rest {
			switch a {
			case "backup", "maintenance", "all":
				targets++
			case "--immediate":
			default:
				fmt.Printf("'%s' is not valid for 'archiver stop' (allowed: backup, maintenance, all, --immediate).\n", a)
				fmt.Print(usageText)
				return 1
			}
		}
		if targets > 1 {
			fmt.Println("'archiver stop' takes at most one target (backup|maintenance|all).")
		}
	case "resume":
		fmt.Printf("'%s' is not valid for 'archiver resume'.\n", strings.Join(rest, " "))
	case "envelope":
		fmt.Println("'envelope' takes at most one argument: an output directory, or confirm.")
	case "recovery-kit":
		fmt.Println("'recovery-kit' takes at most one argument: force.")
	case "maintenance":
		fmt.Println("'maintenance' takes at most one argument: exhaustive.")
	case "pause", "logs", "status", "restore", "auto-restore", "auto-restore-all", "snapshot-exists", "healthcheck":
		fmt.Printf("'%s' cannot have further arguments.\n", cmd)
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
	}
	fmt.Print(usageText)
	return 1
}
