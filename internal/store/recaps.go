package store

import (
	"errors"
	"os"
	"slices"
	"sync"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/model"
)

// Daily recaps (recaps.json, pruned to settings recap.keepDays).
// Wave 3, owner: Engineer B.

// recapsMu serialises recaps.json I/O (one data dir per process).
var recapsMu sync.Mutex

func (s *Store) readRecaps() []model.Recap {
	var list []model.Recap
	if err := readJSON(s.paths.RecapsFile(), &list); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return list
}

// Recaps returns recaps newest first, for one chat ("" = all), at most limit
// (<= 0 = all).
func (s *Store) Recaps(chatKey string, limit int) []model.Recap {
	recapsMu.Lock()
	list := s.readRecaps()
	recapsMu.Unlock()
	out := []model.Recap{}
	for i := len(list) - 1; i >= 0; i-- {
		if chatKey != "" && list[i].ChatKey != chatKey {
			continue
		}
		out = append(out, list[i])
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out
}

// AppendRecap stores r (a recap for the same chat and date as an earlier
// one replaces it) and drops recaps older than keepDays.
func (s *Store) AppendRecap(r model.Recap, keepDays int) error {
	if r.ID == "" {
		r.ID = NewID()
	}
	if r.Topics == nil {
		r.Topics = []string{}
	}
	if r.ToKnow == nil {
		r.ToKnow = []string{}
	}
	recapsMu.Lock()
	defer recapsMu.Unlock()
	list := slices.DeleteFunc(s.readRecaps(), func(x model.Recap) bool {
		return x.ChatKey == r.ChatKey && x.Date == r.Date
	})
	list = append(list, r)
	if keepDays > 0 {
		cutoff := time.Now().Add(-time.Duration(keepDays) * 24 * time.Hour)
		list = slices.DeleteFunc(list, func(x model.Recap) bool { return x.GeneratedAt.Before(cutoff) })
	}
	return config.WriteJSONAtomic(s.paths.RecapsFile(), list, 0o600)
}

// DeleteRecaps forgets a chat's recaps (used when the chat is removed).
func (s *Store) DeleteRecaps(chatKey string) error {
	recapsMu.Lock()
	defer recapsMu.Unlock()
	list := s.readRecaps()
	kept := slices.DeleteFunc(slices.Clone(list), func(x model.Recap) bool { return x.ChatKey == chatKey })
	if len(kept) == len(list) {
		return nil
	}
	return config.WriteJSONAtomic(s.paths.RecapsFile(), kept, 0o600)
}
