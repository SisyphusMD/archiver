// Command archiver is the image's CLI. While the bash implementation is being replaced
// (ADR 8), it only routes: each command line runs the bash program that implements it,
// which replaces this process, so signals, the terminal, and the exit status pass straight
// through as if the bash program had been called directly.
package main

import (
	"fmt"
	"os"
	"syscall"
)

const (
	bashCLI    = "/opt/archiver/archiver.sh"
	initScript = "/opt/archiver/lib/scripts/init.sh"
)

func main() {
	target, args := route(os.Args[1:])
	argv := append([]string{target}, args...)
	err := syscall.Exec(target, argv, os.Environ())
	// Exec only returns on failure.
	fmt.Fprintf(os.Stderr, "archiver: cannot run %s: %v\n", target, err)
	os.Exit(127)
}

// route picks the program that implements a command line and the arguments it gets.
// init has no archiver.sh verb: the entrypoint used to run its script directly, and now
// comes through here like every other caller.
func route(args []string) (string, []string) {
	if len(args) > 0 && args[0] == "init" {
		return initScript, args[1:]
	}
	return bashCLI, args
}
