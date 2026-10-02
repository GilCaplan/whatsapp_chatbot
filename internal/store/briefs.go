package store

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/model"
)

// Chat briefs (cross-chat context): one short summary per chat that other
// chats of the same persona may draw on, briefs/<chatKey>.json (0600).

// briefsMu serialises brief file I/O.
var briefsMu sync.Mutex

func (s *Store) briefFile(chatKey string) string {
	return filepath.Join(s.paths.BriefsDir(), fileKey(chatKey)+".json")
}

// Brief returns a chat's brief (ok=false when it has none).
func (s *Store) Brief(chatKey string) (model.Brief, bool, error) {
	briefsMu.Lock()
	defer briefsMu.Unlock()
	var b model.Brief
	if err := readJSON(s.briefFile(chatKey), &b); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.Brief{}, false, nil
		}
		return model.Brief{}, false, err
	}
	return b, true, nil
}

// SaveBrief writes a chat's brief (b.ChatKey must be set).
func (s *Store) SaveBrief(b model.Brief) error {
	if b.ChatKey == "" {
		return errors.New("brief needs a chat")
	}
	briefsMu.Lock()
	defer briefsMu.Unlock()
	return config.WriteJSONAtomic(s.briefFile(b.ChatKey), b, 0o600)
}

// DeleteBrief forgets a chat's brief (no file is fine).
func (s *Store) DeleteBrief(chatKey string) error {
	briefsMu.Lock()
	defer briefsMu.Unlock()
	err := os.Remove(s.briefFile(chatKey))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
