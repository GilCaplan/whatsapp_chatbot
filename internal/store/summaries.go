package store

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/model"
)

// Chat summaries (compacted older messages), summaries/<chatKey>.json (0600).

// summariesMu serialises summary file I/O.
var summariesMu sync.Mutex

func (s *Store) summaryFile(chatKey string) string {
	return filepath.Join(s.paths.SummariesDir(), fileKey(chatKey)+".json")
}

// Summary returns a chat's summary (ok=false when it has none).
func (s *Store) Summary(chatKey string) (model.ChatSummary, bool) {
	summariesMu.Lock()
	defer summariesMu.Unlock()
	var sm model.ChatSummary
	if err := readJSON(s.summaryFile(chatKey), &sm); err != nil {
		return model.ChatSummary{}, false
	}
	return sm, true
}

// SaveSummary writes a chat's summary (sm.ChatKey must be set).
func (s *Store) SaveSummary(sm model.ChatSummary) error {
	if sm.ChatKey == "" {
		return errors.New("summary needs a chat")
	}
	summariesMu.Lock()
	defer summariesMu.Unlock()
	return config.WriteJSONAtomic(s.summaryFile(sm.ChatKey), sm, 0o600)
}

// DeleteSummary forgets a chat's summary (no file is fine).
func (s *Store) DeleteSummary(chatKey string) error {
	summariesMu.Lock()
	defer summariesMu.Unlock()
	err := os.Remove(s.summaryFile(chatKey))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
