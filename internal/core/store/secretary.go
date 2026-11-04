package store

import (
	"errors"
	"os"
	"slices"
	"sync"

	"whatsappdoppel/internal/core/config"
	"whatsappdoppel/internal/core/model"
)

// Personal secretary files: the owner's calendars (calendars.json, 0600 —
// .ics links are secret addresses) and the messages taken for the owner
// (notes.json, oldest first, the newest MaxNotes kept).

// MaxNotes is how many taken messages are kept.
const MaxNotes = 500

var secMu sync.Mutex

func readList[T any](path string) ([]T, error) {
	var out []T
	if err := readJSON(path, &out); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []T{}, nil
		}
		return nil, err
	}
	return nonNil(out), nil
}

// Calendars returns the owner's calendar sources.
func (s *Store) Calendars() []model.CalendarSource {
	secMu.Lock()
	defer secMu.Unlock()
	out, _ := readList[model.CalendarSource](s.paths.CalendarsFile())
	if out == nil {
		return []model.CalendarSource{}
	}
	return out
}

// UpdateCalendars runs fn on the calendar list and saves the result.
func (s *Store) UpdateCalendars(fn func([]model.CalendarSource) ([]model.CalendarSource, error)) ([]model.CalendarSource, error) {
	secMu.Lock()
	defer secMu.Unlock()
	cur, err := readList[model.CalendarSource](s.paths.CalendarsFile())
	if err != nil {
		return nil, err
	}
	next, err := fn(cur)
	if err != nil {
		return nil, err
	}
	next = nonNil(next)
	return next, config.WriteJSONAtomic(s.paths.CalendarsFile(), next, 0o600)
}

// SecretaryNotes returns the messages taken for the owner, newest first.
func (s *Store) SecretaryNotes() []model.SecretaryNote {
	secMu.Lock()
	defer secMu.Unlock()
	all, _ := readList[model.SecretaryNote](s.paths.NotesFile())
	slices.Reverse(all)
	return nonNil(all)
}

// UpdateSecretaryNotes runs fn on the notes (oldest first) and saves them.
func (s *Store) UpdateSecretaryNotes(fn func([]model.SecretaryNote) []model.SecretaryNote) error {
	secMu.Lock()
	defer secMu.Unlock()
	all, err := readList[model.SecretaryNote](s.paths.NotesFile())
	if err != nil {
		return err
	}
	all = fn(all)
	if len(all) > MaxNotes {
		all = all[len(all)-MaxNotes:]
	}
	return config.WriteJSONAtomic(s.paths.NotesFile(), nonNil(all), 0o600)
}
