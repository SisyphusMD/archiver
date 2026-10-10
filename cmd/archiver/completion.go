package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// completions maps each command a person types to the words that may follow it (ADR 44).
// A test holds it to the usage line, so a new command cannot miss its completion.
var completions = map[string][]string{
	"backup":           {"--detach"},
	"maintenance":      {"exhaustive"},
	"drill":            nil,
	"doctor":           {"--notify"},
	"notify":           {"test"},
	"recover":          {"--yes", "--hook", "--out"},
	"stop":             {"backup", "maintenance", "drill", "all", "--immediate"},
	"pause":            nil,
	"resume":           nil,
	"logs":             nil,
	"status":           {"--json"},
	"health":           {"--backups"},
	"migrate":          {"hooks"},
	"mirror":           {"--dry-run", "--allow-large"},
	"recovery-kit":     {"force"},
	"envelope":         {"confirm"},
	"restore":          nil,
	"auto-restore":     nil,
	"auto-restore-all": nil,
	"snapshot-exists":  nil,
	"init":             nil,
	"healthcheck":      nil,
	"completion":       {"bash", "zsh", "fish"},
	"help":             nil,
}

func completionCommand(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: archiver completion bash|zsh|fish")
		return 1
	}
	var script string
	switch args[0] {
	case "bash":
		script = bashCompletion()
	case "zsh":
		script = zshCompletion()
	case "fish":
		script = fishCompletion()
	default:
		fmt.Fprintf(os.Stderr, "archiver completion: '%s' is not bash, zsh or fish\n", args[0])
		return 1
	}
	fmt.Print(script)
	return 0
}

func commandNames() []string {
	names := make([]string, 0, len(completions))
	for c := range completions {
		names = append(names, c)
	}
	sort.Strings(names)
	return names
}

func bashCompletion() string {
	var b strings.Builder
	b.WriteString("# archiver completion for bash (eval \"$(archiver completion bash)\")\n")
	b.WriteString("_archiver() {\n\tlocal cur=${COMP_WORDS[COMP_CWORD]} words\n")
	b.WriteString("\tif [ \"$COMP_CWORD\" -eq 1 ]; then\n")
	fmt.Fprintf(&b, "\t\twords=%q\n", strings.Join(commandNames(), " "))
	b.WriteString("\telse\n\t\tcase ${COMP_WORDS[1]} in\n")
	for _, c := range commandNames() {
		if len(completions[c]) > 0 {
			fmt.Fprintf(&b, "\t\t%s) words=%q ;;\n", c, strings.Join(completions[c], " "))
		}
	}
	b.WriteString("\t\t*) words= ;;\n\t\tesac\n\tfi\n")
	b.WriteString("\tCOMPREPLY=($(compgen -W \"$words\" -- \"$cur\"))\n}\ncomplete -F _archiver archiver\n")
	return b.String()
}

func fishCompletion() string {
	var b strings.Builder
	b.WriteString("# archiver completion for fish (archiver completion fish | source)\n")
	b.WriteString("complete -c archiver -f\n")
	fmt.Fprintf(&b, "complete -c archiver -n __fish_use_subcommand -a %q\n", strings.Join(commandNames(), " "))
	for _, c := range commandNames() {
		if len(completions[c]) > 0 {
			fmt.Fprintf(&b, "complete -c archiver -n '__fish_seen_subcommand_from %s' -a %q\n", c, strings.Join(completions[c], " "))
		}
	}
	return b.String()
}

// zshCompletion works both sourced (with compinit loaded) and installed in $fpath as
// _archiver, where zsh runs the file itself on the first completion: it then completes too.
func zshCompletion() string {
	var b strings.Builder
	b.WriteString("#compdef archiver\n# archiver completion for zsh (archiver completion zsh > \"${fpath[1]}/_archiver\")\n")
	b.WriteString("_archiver() {\n\tif (( CURRENT == 2 )); then\n")
	fmt.Fprintf(&b, "\t\tcompadd -- %s\n", strings.Join(commandNames(), " "))
	b.WriteString("\telse\n\t\tcase ${words[2]} in\n")
	for _, c := range commandNames() {
		if len(completions[c]) > 0 {
			fmt.Fprintf(&b, "\t\t%s) compadd -- %s ;;\n", c, strings.Join(completions[c], " "))
		}
	}
	b.WriteString("\t\tesac\n\tfi\n}\n")
	b.WriteString("if [ \"${funcstack[1]}\" = _archiver ]; then\n\t_archiver \"$@\"\nelse\n\tcompdef _archiver archiver\nfi\n")
	return b.String()
}
