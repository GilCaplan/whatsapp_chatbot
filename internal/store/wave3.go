package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"whatsappdoppel/internal/contract"
	"whatsappdoppel/internal/model"
)

// Shared helpers for the wave 3 files (memories, recaps, missions, self
// samples). Each feature's store functions live in its own file.

// ErrNotImplemented is contract.ErrNotImplemented (wave 3 stubs).
var ErrNotImplemented = contract.ErrNotImplemented

// fileKey makes a chat key safe as a file name (same rule as history files).
func fileKey(chatKey string) string {
	return strings.NewReplacer(":", "_", "/", "_", "@", "_", ".", "_").Replace(chatKey)
}

// UpdateHistory rewrites one history message in place (e.g. after a typo was
// edited on WhatsApp: Text = Corrected, Kind = "edited"). ErrNotFound when
// the chat has no message with that id.
func (s *Store) UpdateHistory(chatKey, id string, fn func(*model.Message)) error {
	s.histMu.Lock()
	defer s.histMu.Unlock()
	path := s.historyFile(chatKey)
	msgs, err := readHistory(path)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	for i := range msgs {
		if msgs[i].ID == id {
			fn(&msgs[i])
			msgs[i].ID = id
			return writeHistory(path, msgs)
		}
	}
	return ErrNotFound
}

// memoriesFile is memories/<chatKey>.json.
func (s *Store) memoriesFile(chatKey string) string {
	return filepath.Join(s.paths.MemoriesDir(), fileKey(chatKey)+".json")
}
