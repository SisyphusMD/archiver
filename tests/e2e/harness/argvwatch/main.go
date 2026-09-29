// Command argvwatch runs inside an archiver container's PID namespace and records every
// process argv that contains a secret. The harness builds it statically, so it works in
// any image, and it polls /proc as fast as it can because a leaking command such as an
// openssl invocation can live for only a few milliseconds.
//
// Usage: argvwatch DIR. It reads the secrets, one per line, from DIR/secrets (never argv,
// or it would catch itself), creates DIR/ready after its first full scan, and once
// DIR/stop exists writes DIR/result: the number of processes first seen after the ready
// scan, then one line per leaking argv.
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: argvwatch DIR")
		os.Exit(2)
	}
	dir := os.Args[1]
	raw, err := os.ReadFile(filepath.Join(dir, "secrets"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var secrets [][]byte
	for _, s := range strings.Split(string(raw), "\n") {
		if s != "" {
			secrets = append(secrets, []byte(s))
		}
	}

	self := strconv.Itoa(os.Getpid())
	// A pid is recycled with a new argv, so pid and argv together name a process.
	seen := map[string]bool{}
	hits := map[string]bool{}
	fresh := 0
	scan := func(counting bool) {
		d, err := os.Open("/proc")
		if err != nil {
			return
		}
		names, _ := d.Readdirnames(-1)
		d.Close()
		for _, pid := range names {
			if pid == self || pid[0] < '0' || pid[0] > '9' {
				continue
			}
			argv, err := os.ReadFile("/proc/" + pid + "/cmdline")
			if err != nil || len(argv) == 0 {
				continue
			}
			key := pid + "\x00" + string(argv)
			if seen[key] {
				continue
			}
			seen[key] = true
			if counting {
				fresh++
			}
			for _, s := range secrets {
				if bytes.Contains(argv, s) {
					line := strings.NewReplacer("\x00", " ", "\n", `\n`).Replace(string(bytes.TrimRight(argv, "\x00")))
					hits[line] = true
				}
			}
		}
	}

	scan(false)
	if err := os.WriteFile(filepath.Join(dir, "ready"), nil, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	stop := filepath.Join(dir, "stop")
	for {
		if _, err := os.Stat(stop); err == nil {
			break
		}
		scan(true)
	}

	var out strings.Builder
	fmt.Fprintf(&out, "%d\n", fresh)
	for h := range hits {
		out.WriteString(h + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "result"), []byte(out.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
