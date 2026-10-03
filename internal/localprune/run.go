package localprune

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/SisyphusMD/archiver/internal/inuse"
)

// Options is one local prune. Duplicacy runs in the working directory, a repository that
// knows Storage, with its secrets in the environment.
type Options struct {
	Bin        string   // duplicacy
	Storage    string   // the primary's duplicacy storage name
	Keep       []string // the retention policy, as -keep arguments
	Threads    string
	Exhaustive bool
	InUseDir   string
	Out        io.Writer // duplicacy's output and this prune's own lines, for the maintenance log
}

// Run prunes. It runs alongside any backup (ADR 21): duplicacy's non-exclusive prune never
// deletes an ID's newest revision, and fossils go only once every ID has backed up since.
func Run(ctx context.Context, o Options) error {
	deleted, err := o.prune(ctx)
	if err != nil {
		return err
	}
	// Each prune that deletes also reclaims earlier fossils once duplicacy allows; with
	// nothing deleted, a prune selecting nothing does that, and an exhaustive one also
	// collects unreferenced chunks. Outside the gate: neither deletes a revision.
	args := []string{"prune", "-all", "-storage", o.Storage, "-threads", o.Threads}
	switch {
	case o.Exhaustive:
		args = append(args, "-exhaustive")
	case deleted:
		return nil
	}
	return o.duplicacy(ctx, o.Out, args...)
}

// prune deletes what the policy selects and nothing in use, reporting whether it deleted.
func (o Options) prune(ctx context.Context) (bool, error) {
	release, err := inuse.Gate(ctx, o.InUseDir, o.Storage, true)
	if err != nil {
		return false, err
	}
	defer release()
	busy, err := inuse.Read(o.InUseDir, o.Storage)
	if err != nil {
		return false, fmt.Errorf("reading revisions in use: %w", err)
	}
	var dry strings.Builder
	if err := o.duplicacy(ctx, &dry, append(append([]string{"prune", "-all", "-dry-run", "-storage", o.Storage}, o.Keep...), "-threads", o.Threads)...); err != nil {
		fmt.Fprint(o.Out, dry.String())
		return false, fmt.Errorf("dry run: %w", err)
	}
	now, held := Split(Planned(strings.Split(dry.String(), "\n")), busy.Has)
	for _, id := range sorted(held) {
		fmt.Fprintf(o.Out, "Leaving revisions %s of %s for the next prune: a copy or restore is still reading them.\n", join(held[id]), id)
	}
	if len(now) == 0 {
		fmt.Fprintln(o.Out, "No revisions to delete under the retention policy.")
		return false, nil
	}
	for _, id := range sorted(now) {
		args := []string{"prune", "-id", id, "-storage", o.Storage, "-threads", o.Threads}
		for _, r := range now[id] {
			args = append(args, "-r", strconv.Itoa(r))
		}
		if err := o.duplicacy(ctx, o.Out, args...); err != nil {
			return false, fmt.Errorf("pruning %s: %w", id, err)
		}
	}
	return true, nil
}

func (o Options) duplicacy(ctx context.Context, out io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, o.Bin, args...)
	cmd.Stdout, cmd.Stderr = out, out
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("duplicacy %s: %w", args[0], err)
	}
	return nil
}

func sorted(m map[string][]int) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func join(revs []int) string {
	s := make([]string, len(revs))
	for i, r := range revs {
		s[i] = strconv.Itoa(r)
	}
	return strings.Join(s, ",")
}
