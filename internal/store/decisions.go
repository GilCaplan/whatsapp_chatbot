package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/model"
)

// Decision log: per chat, what the persona did about each message and why
// (decisions/<chatKey>.json, 0600, oldest first). The engine updates it from
// its activity (engine/decisionlog.go); the newest MaxDecisions are kept.

// MaxDecisions is how many log entries a chat keeps.
const MaxDecisions = 500

// decisionsMu serialises decision file I/O (read-modify-write).
var decisionsMu sync.Mutex

func (s *Store) decisionsFile(chatKey string) string {
	return filepath.Join(s.paths.DecisionsDir(), fileKey(chatKey)+".json")
}

func (s *Store) readDecisions(chatKey string) ([]model.LogEntry, error) {
	var out []model.LogEntry
	if err := readJSON(s.decisionsFile(chatKey), &out); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []model.LogEntry{}, nil
		}
		return nil, err
	}
	return nonNil(out), nil
}

// Decisions returns a chat's decision log, newest first (limit <= 0 = all).
func (s *Store) Decisions(chatKey string, limit int) ([]model.LogEntry, error) {
	decisionsMu.Lock()
	all, err := s.readDecisions(chatKey)
	decisionsMu.Unlock()
	if err != nil {
		return []model.LogEntry{}, err
	}
	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}
	out := make([]model.LogEntry, len(all))
	for i, d := range all {
		out[len(all)-1-i] = d
	}
	return out, nil
}

// UpdateDecisions runs fn on a chat's log (oldest first) and saves what it
// returns when changed is true (capped at MaxDecisions).
func (s *Store) UpdateDecisions(chatKey string, fn func([]model.LogEntry) ([]model.LogEntry, bool)) error {
	decisionsMu.Lock()
	defer decisionsMu.Unlock()
	all, err := s.readDecisions(chatKey)
	if err != nil {
		return err
	}
	all, changed := fn(all)
	if !changed {
		return nil
	}
	if len(all) > MaxDecisions {
		all = all[len(all)-MaxDecisions:]
	}
	b, err := json.Marshal(all)
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(s.decisionsFile(chatKey), b, 0o600)
}

// ClearDecisions empties a chat's log (no file is fine).
func (s *Store) ClearDecisions(chatKey string) error {
	decisionsMu.Lock()
	defer decisionsMu.Unlock()
	err := os.Remove(s.decisionsFile(chatKey))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
