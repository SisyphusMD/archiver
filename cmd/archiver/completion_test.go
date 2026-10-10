package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The completions name exactly the commands of the usage line.
func TestCompletionsMatchUsage(t *testing.T) {
	line := strings.SplitN(usageText, "\n", 2)[0]
	inner := line[strings.Index(line, "{")+1 : strings.LastIndex(line, "}")]
	seen := map[string]bool{}
	for _, alt := range strings.Split(inner, "|") {
		seen[strings.Fields(alt)[0]] = true
	}
	var usageCmds []string
	for c := range seen {
		usageCmds = append(usageCmds, c)
	}
	sort.Strings(usageCmds)
	if got, want := strings.Join(commandNames(), " "), strings.Join(usageCmds, " "); got != want {
		t.Errorf("completions name %s\nusage names       %s", got, want)
	}
}

// The bash script completes commands and their arguments when bash runs it.
func TestBashCompletionWorks(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	script := bashCompletion() + `
COMP_WORDS=(archiver mai); COMP_CWORD=1; _archiver; echo "${COMPREPLY[*]}"
COMP_WORDS=(archiver stop ""); COMP_CWORD=2; _archiver; echo "${COMPREPLY[*]}"
COMP_WORDS=(archiver logs ""); COMP_CWORD=2; _archiver; echo "[${COMPREPLY[*]}]"
`
	out, err := exec.Command("bash", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	want := "maintenance\nbackup maintenance drill all --immediate\n[]\n"
	if string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestCompletionShells(t *testing.T) {
	for _, sh := range []string{"bash", "zsh", "fish"} {
		if completionCommand([]string{sh}) != 0 {
			t.Errorf("completion %s failed", sh)
		}
	}
	if completionCommand([]string{"tcsh"}) == 0 || completionCommand(nil) == 0 {
		t.Error("completion accepted a bad shell or no argument")
	}
}

// Installed in $fpath, zsh runs the file as _archiver on the first completion: that call
// completes too, not only the ones after it.
func TestZshCompletionAutoload(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("no zsh")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "_archiver"), []byte(zshCompletion()), 0o644); err != nil {
		t.Fatal(err)
	}
	script := `fpath=(` + dir + ` $fpath); autoload -Uz _archiver
compadd() { shift; echo "$*"; }
words=(archiver stop ""); CURRENT=3; _archiver
words=(archiver ""); CURRENT=2; _archiver | grep -q maintenance && echo commands`
	out, err := exec.Command("zsh", "-f", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if want := "backup maintenance drill all --immediate\ncommands\n"; string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
