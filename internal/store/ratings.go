package store

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/model"
)

// Connection ratings, ratings/<chatKey>.json (0600, oldest first, one entry
// per day: a new rating on the same day replaces it).

// MaxRatingDays is how many days of ratings a chat keeps.
const MaxRatingDays = 90

var ratingsMu sync.Mutex

func (s *Store) ratingsFile(chatKey string) string {
	return filepath.Join(s.paths.RatingsDir(), fileKey(chatKey)+".json")
}

func (s *Store) readRatings(chatKey string) ([]model.RatingDay, error) {
	var out []model.RatingDay
	if err := readJSON(s.ratingsFile(chatKey), &out); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []model.RatingDay{}, nil
		}
		return nil, err
	}
	return nonNil(out), nil
}

// Ratings returns a chat's ratings, newest day first.
func (s *Store) Ratings(chatKey string) ([]model.RatingDay, error) {
	ratingsMu.Lock()
	all, err := s.readRatings(chatKey)
	ratingsMu.Unlock()
	if err != nil {
		return []model.RatingDay{}, err
	}
	out := make([]model.RatingDay, len(all))
	for i, d := range all {
		out[len(all)-1-i] = d
	}
	return out, nil
}

// SaveRating stores a day's ratings (replacing that day's earlier ones).
func (s *Store) SaveRating(d model.RatingDay) error {
	if d.ChatKey == "" {
		return errors.New("rating needs a chat")
	}
	ratingsMu.Lock()
	defer ratingsMu.Unlock()
	all, err := s.readRatings(d.ChatKey)
	if err != nil {
		return err
	}
	if n := len(all); n > 0 && all[n-1].Date == d.Date {
		all = all[:n-1]
	}
	all = append(all, d)
	if len(all) > MaxRatingDays {
		all = all[len(all)-MaxRatingDays:]
	}
	return config.WriteJSONAtomic(s.ratingsFile(d.ChatKey), all, 0o600)
}

// DeleteRatings forgets a chat's ratings (no file is fine).
func (s *Store) DeleteRatings(chatKey string) error {
	ratingsMu.Lock()
	defer ratingsMu.Unlock()
	err := os.Remove(s.ratingsFile(chatKey))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
