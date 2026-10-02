package store

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/model"
)

// Memories of people, one JSON file per chat (memories/<chatKey>.json, 0600).
// Wave 3, owner: Engineer A.

// memMu serialises memory file I/O (read-modify-write).
var memMu sync.Mutex

func (s *Store) readMemories(chatKey string) ([]model.Memory, error) {
	var out []model.Memory
	if err := readJSON(s.memoriesFile(chatKey), &out); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []model.Memory{}, nil
		}
		return nil, err
	}
	return nonNil(out), nil
}

func (s *Store) writeMemories(chatKey string, mems []model.Memory) error {
	if len(mems) == 0 {
		err := os.Remove(s.memoriesFile(chatKey))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	b, err := json.MarshalIndent(mems, "", "  ")
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(s.memoriesFile(chatKey), b, 0o600)
}

// sortMemories: pinned first, then newest first.
func sortMemories(mems []model.Memory) {
	slices.SortStableFunc(mems, func(a, b model.Memory) int {
		if a.Pinned != b.Pinned {
			if a.Pinned {
				return -1
			}
			return 1
		}
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})
}

// Memories returns a chat's memories (pinned first, then newest).
func (s *Store) Memories(chatKey string) ([]model.Memory, error) {
	memMu.Lock()
	defer memMu.Unlock()
	mems, err := s.readMemories(chatKey)
	if err != nil {
		return []model.Memory{}, err
	}
	sortMemories(mems)
	return mems, nil
}

// MemoryCount is how many memories a chat has (0 on error).
func (s *Store) MemoryCount(chatKey string) int {
	mems, _ := s.Memories(chatKey)
	return len(mems)
}

// UpsertMemory adds m (empty ID → new) or replaces the memory with its ID
// (ErrNotFound when m.ID is set but unknown). ChatKey must be set.
func (s *Store) UpsertMemory(m model.Memory) (model.Memory, error) {
	if strings.TrimSpace(m.ChatKey) == "" {
		return m, errors.New("memory needs a chat")
	}
	var saved model.Memory
	_, err := s.UpdateMemories(m.ChatKey, func(mems []model.Memory) ([]model.Memory, error) {
		now := time.Now()
		if m.ID == "" {
			m.ID = NewID()
			if m.CreatedAt.IsZero() {
				m.CreatedAt = now
			}
			if m.UpdatedAt.IsZero() {
				m.UpdatedAt = now
			}
			saved = m
			return append(mems, m), nil
		}
		for i := range mems {
			if mems[i].ID == m.ID {
				if m.CreatedAt.IsZero() {
					m.CreatedAt = mems[i].CreatedAt
				}
				if m.UpdatedAt.IsZero() {
					m.UpdatedAt = now
				}
				mems[i] = m
				saved = m
				return mems, nil
			}
		}
		return nil, ErrNotFound
	})
	return saved, err
}

// UpdateMemories changes a chat's memories atomically: fn gets the current
// list and returns the new one (an error leaves the file untouched).
func (s *Store) UpdateMemories(chatKey string, fn func([]model.Memory) ([]model.Memory, error)) ([]model.Memory, error) {
	memMu.Lock()
	defer memMu.Unlock()
	mems, err := s.readMemories(chatKey)
	if err != nil {
		return nil, err
	}
	out, err := fn(mems)
	if err != nil {
		return nil, err
	}
	out = nonNil(out)
	sortMemories(out)
	return out, s.writeMemories(chatKey, out)
}

// DeleteMemory removes one memory (ErrNotFound when absent).
func (s *Store) DeleteMemory(chatKey, id string) error {
	_, err := s.UpdateMemories(chatKey, func(mems []model.Memory) ([]model.Memory, error) {
		i := slices.IndexFunc(mems, func(m model.Memory) bool { return m.ID == id })
		if i < 0 {
			return nil, ErrNotFound
		}
		return slices.Delete(mems, i, i+1), nil
	})
	return err
}

// ClearMemories forgets everything learned in a chat (no file is fine).
func (s *Store) ClearMemories(chatKey string) error {
	memMu.Lock()
	defer memMu.Unlock()
	return s.writeMemories(chatKey, nil)
}

// ClearAllMemories forgets the memories of every chat.
func (s *Store) ClearAllMemories() error {
	memMu.Lock()
	defer memMu.Unlock()
	err := os.RemoveAll(s.paths.MemoriesDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
