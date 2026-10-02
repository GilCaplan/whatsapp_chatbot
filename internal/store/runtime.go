package store

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/model"
)

// RunnerState is the per-chat engine state that must survive restarts
// (runtime.json): rate-limit counters, silence tracking for proactive
// check-ins and a queued wake-up from outside active hours. In-flight reply
// phases are deliberately not persisted.
type RunnerState struct {
	LastIncomingAt   time.Time   `json:"lastIncomingAt"`
	LastReplyAt      time.Time   `json:"lastReplyAt"`
	LastYouRepliedAt time.Time   `json:"lastYouRepliedAt"`
	Replies          []time.Time `json:"replies"`   // persona replies, pruned to 24 h
	Proactive        []time.Time `json:"proactive"` // proactive check-ins, pruned to 24 h
	ProactiveDueAt   *time.Time  `json:"proactiveDueAt"`
	QueuedWakeAt     *time.Time  `json:"queuedWakeAt"`
	// Goal is the chat's progress on its goal (nil = not started).
	Goal *model.GoalStatus `json:"goal,omitempty"`

	// Wave 3: memory extraction cursor (history after this is not yet
	// mined), "them" messages since the last extraction, and the last recap.
	MemoryCursor  time.Time `json:"memoryCursor"`
	MemoryPending int       `json:"memoryPending"`
	LastRecapAt   time.Time `json:"lastRecapAt"`
}

// Prune drops reply/check-in timestamps older than 24 h before now.
func (st *RunnerState) Prune(now time.Time) {
	old := func(t time.Time) bool { return now.Sub(t) > 24*time.Hour }
	st.Replies = slices.DeleteFunc(st.Replies, old)
	st.Proactive = slices.DeleteFunc(st.Proactive, old)
}

// RepliesSince counts persona replies at or after t.
func (st RunnerState) RepliesSince(t time.Time) int {
	n := 0
	for _, r := range st.Replies {
		if !r.Before(t) {
			n++
		}
	}
	return n
}

// ProactiveSince counts proactive check-ins at or after t.
func (st RunnerState) ProactiveSince(t time.Time) int {
	n := 0
	for _, r := range st.Proactive {
		if !r.Before(t) {
			n++
		}
	}
	return n
}

func (st RunnerState) clone() RunnerState {
	st.Replies = slices.Clone(st.Replies)
	st.Proactive = slices.Clone(st.Proactive)
	if st.ProactiveDueAt != nil {
		t := *st.ProactiveDueAt
		st.ProactiveDueAt = &t
	}
	if st.QueuedWakeAt != nil {
		t := *st.QueuedWakeAt
		st.QueuedWakeAt = &t
	}
	if st.Goal != nil {
		g := st.Goal.Clone()
		st.Goal = &g
	}
	return st
}

type runtimeState struct {
	mu    sync.Mutex
	chats map[string]RunnerState
}

func (s *Store) loadRuntime() error {
	s.rt.chats = map[string]RunnerState{}
	if err := readJSON(s.paths.RuntimeFile(), &s.rt.chats); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Runtime state is best-effort: a corrupt file is replaced, not fatal.
		s.rt.chats = map[string]RunnerState{}
		return nil
	}
	if s.rt.chats == nil {
		s.rt.chats = map[string]RunnerState{}
	}
	return nil
}

// RunnerState returns a copy of a chat's persisted runtime state (zero if none).
func (s *Store) RunnerState(key string) RunnerState {
	s.rt.mu.Lock()
	defer s.rt.mu.Unlock()
	return s.rt.chats[key].clone()
}

// SaveRunnerState stores a chat's runtime state and rewrites runtime.json.
func (s *Store) SaveRunnerState(key string, st RunnerState) error {
	s.rt.mu.Lock()
	defer s.rt.mu.Unlock()
	s.rt.chats[key] = st.clone()
	return s.saveRuntimeLocked()
}

// UpdateRunnerState applies fn to a chat's runtime state under the lock and persists it.
func (s *Store) UpdateRunnerState(key string, fn func(*RunnerState)) (RunnerState, error) {
	s.rt.mu.Lock()
	defer s.rt.mu.Unlock()
	st := s.rt.chats[key].clone()
	fn(&st)
	s.rt.chats[key] = st
	return st.clone(), s.saveRuntimeLocked()
}

// DeleteRunnerState forgets a chat's runtime state.
func (s *Store) DeleteRunnerState(key string) {
	s.rt.mu.Lock()
	defer s.rt.mu.Unlock()
	if _, ok := s.rt.chats[key]; ok {
		delete(s.rt.chats, key)
		_ = s.saveRuntimeLocked()
	}
}

func (s *Store) saveRuntimeLocked() error {
	if err := config.WriteJSONAtomic(s.paths.RuntimeFile(), s.rt.chats, 0o600); err != nil {
		return fmt.Errorf("runtime.json: %w", err)
	}
	return nil
}
