// Package forget removes the snapshots of services a host no longer backs up (`archiver
// forget`). Nothing else ever does: Duplicacy's prune passes over an ID with no revision in
// seven days, and mirroring leaves alone an ID the primary has no revision of.
package forget

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/SisyphusMD/archiver/internal/config"
	"github.com/SisyphusMD/archiver/internal/inuse"
)

// Env is what forgetting needs. List and Remove reach a storage (kit.ListDir and
// kit.RemoveDir), paths relative to its root.
type Env struct {
	Ctx      context.Context
	Cfg      *config.Config
	Hostname string
	InUseDir string
	Out      io.Writer
	In       io.Reader
	Terminal bool // In is a person who can answer
	List     func(t config.Target, path string) ([]string, error)
	Remove   func(t config.Target, path string) error
}

// Configured are the service names SERVICE_DIRECTORIES gives, a literal path counting even
// while it is not mounted.
func Configured(cfg *config.Config) map[string]bool {
	dirs, unmatched := config.ExpandServiceDirectories(cfg.ServiceDirectories)
	for _, u := range unmatched {
		if !config.HasMeta(u) {
			dirs = append(dirs, config.Unescape(u))
		}
	}
	names := map[string]bool{}
	for _, d := range dirs {
		names[config.ServiceName(d)] = true
	}
	return names
}

// Orphans are the snapshot IDs among ids of this host that no configured service backs up.
// An ID of another host whose name starts with this one's and a hyphen counts too: only a
// person can tell them apart.
func Orphans(ids []string, host string, configured map[string]bool) []string {
	var out []string
	for _, id := range ids {
		if name, ok := strings.CutPrefix(id, host+"-"); ok && !configured[name] {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// Run forgets each of names, a service name or a whole snapshot ID, on every storage.
func Run(e Env, names []string, yes bool) int {
	configured := Configured(e.Cfg)
	var ids []string
	for _, n := range names {
		id := n
		if !strings.HasPrefix(n, e.Hostname+"-") {
			id = e.Hostname + "-" + n
		}
		name := strings.TrimPrefix(id, e.Hostname+"-")
		switch {
		case !config.ValidSnapshotID(id):
			fmt.Fprintf(e.Out, "'%s' is not a snapshot ID.\n", n)
			return 2
		case configured[name]:
			fmt.Fprintf(e.Out, "%s is still a configured service (SERVICE_DIRECTORIES); remove it there first, or its next backup starts it again.\n", id)
			return 1
		}
		ids = append(ids, id)
	}

	// Every storage is listed before anything goes, so the plan shown is the whole of it.
	type found struct {
		t    config.Target
		id   string
		revs []int
	}
	var plan []found
	failed := false
	for _, t := range e.Cfg.Targets {
		for _, id := range ids {
			entries, err := e.List(t, "snapshots/"+id)
			if err != nil {
				fmt.Fprintf(e.Out, "  '%s': cannot list %s: %v\n", t.Name, id, err)
				failed = true
				continue
			}
			var revs []int
			for _, n := range entries {
				if r, err := strconv.Atoi(n); err == nil {
					revs = append(revs, r)
				}
			}
			if len(revs) == 0 {
				continue
			}
			sort.Ints(revs)
			plan = append(plan, found{t, id, revs})
			fmt.Fprintf(e.Out, "  '%s': %s, %d revision(s) (%d to %d)\n", t.Name, id, len(revs), revs[0], revs[len(revs)-1])
		}
	}
	// A storage left out could carry an ID back: the primary to every secondary through its
	// copies, so nothing is deleted unless every storage was listed.
	if failed {
		fmt.Fprintln(e.Out, "Nothing deleted: every storage must be reachable.")
		return 1
	}
	if len(plan) == 0 {
		fmt.Fprintln(e.Out, "Nothing to forget: no storage holds those snapshots.")
		return 0
	}
	if !yes {
		if !e.Terminal {
			fmt.Fprintln(e.Out, "This deletes those revisions for good. Run again with --yes to go ahead.")
			return 1
		}
		fmt.Fprint(e.Out, "Delete these revisions for good? (y/N): ")
		line, _ := bufio.NewReader(e.In).ReadString('\n')
		if a := strings.TrimSpace(line); a != "y" && a != "Y" {
			fmt.Fprintln(e.Out, "Nothing deleted.")
			return 1
		}
	}

	// The primary first: a copy started meanwhile then has nothing of these IDs to carry. A
	// failure ends the run, since a copy still carrying an ID from the primary would write it
	// to a secondary again.
	for _, f := range plan {
		if err := e.forget(f.t, f.id); err != nil {
			fmt.Fprintf(e.Out, "  '%s': %s NOT forgotten, stopping: %v\n", f.t.Name, f.id, err)
			return 1
		}
		fmt.Fprintf(e.Out, "  '%s': forgot %s\n", f.t.Name, f.id)
	}
	fmt.Fprintln(e.Out, "Their chunks are freed by the next exhaustive prune: run 'archiver maintenance exhaustive' (copy workers run theirs once caught up).")
	return 0
}

// forget deletes the ID's snapshot files on t, which is all a prune deletes at once: the
// chunks no longer referenced go in an exhaustive prune's two steps, so a backup running
// meanwhile never loses one. Under the storage's gate, taken as a prune takes it (ADR 19),
// it lists the revisions again and leaves the ID alone if a copy or restore reads any of
// them, a revision published since the plan included.
func (e Env) forget(t config.Target, id string) error {
	release, err := inuse.Gate(e.Ctx, e.InUseDir, t.StorageName(), true)
	if err != nil {
		return err
	}
	defer release()
	busy, err := inuse.Read(e.InUseDir, t.StorageName())
	if err != nil {
		return fmt.Errorf("reading revisions in use: %w", err)
	}
	entries, err := e.List(t, "snapshots/"+id)
	if err != nil {
		return err
	}
	for _, n := range entries {
		if r, err := strconv.Atoi(n); err == nil && busy.Has(id, r) {
			return fmt.Errorf("revision %d is being read by a copy or restore; run again once it ends", r)
		}
	}
	return e.Remove(t, "snapshots/"+id)
}
