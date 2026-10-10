package copier

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
)

// Store keeps every worker's state in one file in the logs volume: status reads it, and a
// restarted daemon resumes from it (a target that was down is not alerted again).
type Store struct {
	Path string
	mu   sync.Mutex
}

// Load reads the saved states by target; a missing or unreadable file is an empty one.
func (s *Store) Load() map[string]State {
	out, _ := s.Read()
	return out
}

// Read is Load with the reason the file could not be read; a missing file is no error.
func (s *Store) Read() (map[string]State, error) {
	out := map[string]State{}
	b, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	var list []State
	if err := json.Unmarshal(b, &list); err != nil {
		return out, err
	}
	for _, st := range list {
		out[st.Target] = st
	}
	return out, nil
}

// Save records one worker's state, keeping the others'.
func (s *Store) Save(st State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := s.Load()
	all[st.Target] = st
	return s.write(all)
}

// Prune drops states of targets no longer configured.
func (s *Store) Prune(targets []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := map[string]bool{}
	for _, t := range targets {
		keep[t] = true
	}
	all := s.Load()
	for t := range all {
		if !keep[t] {
			delete(all, t)
		}
	}
	return s.write(all)
}

func (s *Store) write(all map[string]State) error {
	list := make([]State, 0, len(all))
	for _, v := range all {
		list = append(list, v)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Target < list[j].Target })
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil { //nolint:gosec // G306: not a secret: state or notes meant to be readable
		return err
	}
	return os.Rename(tmp, s.Path)
}
