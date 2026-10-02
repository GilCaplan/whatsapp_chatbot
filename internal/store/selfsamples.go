package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/model"
)

// Samples of your own messages for "Clone yourself"
// (cache/self-samples.jsonl, 0600, FIFO). Wave 3, owner: Engineer A.
// Collected only with your consent (settings clone.collectSamples).

var selfMu sync.Mutex

func (s *Store) readSelfSamples() ([]model.SelfSample, error) {
	f, err := os.Open(s.paths.SelfSamplesFile())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []model.SelfSample
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var x model.SelfSample
		if json.Unmarshal(sc.Bytes(), &x) == nil && strings.TrimSpace(x.Text) != "" {
			out = append(out, x)
		}
	}
	return out, sc.Err()
}

func (s *Store) writeSelfSamples(xs []model.SelfSample) error {
	var sb strings.Builder
	for _, x := range xs {
		b, err := json.Marshal(x)
		if err != nil {
			return err
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return config.WriteFileAtomic(s.paths.SelfSamplesFile(), []byte(sb.String()), 0o600)
}

// AppendSelfSample stores one sample, keeping at most max (FIFO; max <= 0 = 500).
func (s *Store) AppendSelfSample(sample model.SelfSample, max int) error {
	return s.AppendSelfSamples([]model.SelfSample{sample}, max)
}

// AppendSelfSamples stores samples (oldest first), keeping at most max
// (FIFO; max <= 0 = 500). Exact repeats (same time and text) of stored
// samples are skipped: history sync can deliver a message more than once.
func (s *Store) AppendSelfSamples(batch []model.SelfSample, max int) error {
	if max <= 0 {
		max = 500
	}
	selfMu.Lock()
	defer selfMu.Unlock()
	xs, err := s.readSelfSamples()
	if err != nil {
		return err
	}
	type key struct {
		ts   int64
		text string
	}
	seen := make(map[key]bool, len(xs))
	for _, x := range xs {
		seen[key{x.TS.UnixNano(), x.Text}] = true
	}
	var add []model.SelfSample
	for _, x := range batch {
		if strings.TrimSpace(x.Text) == "" {
			continue
		}
		if x.TS.IsZero() {
			x.TS = time.Now()
		}
		k := key{x.TS.UnixNano(), x.Text}
		if seen[k] {
			continue
		}
		seen[k] = true
		add = append(add, x)
	}
	if len(add) == 0 {
		return nil
	}
	if len(xs)+len(add) > max {
		all := append(xs, add...)
		return s.writeSelfSamples(all[len(all)-max:])
	}
	if err := os.MkdirAll(filepath.Dir(s.paths.SelfSamplesFile()), 0o700); err != nil {
		return err
	}
	var sb strings.Builder
	for _, x := range add {
		b, err := json.Marshal(x)
		if err != nil {
			return err
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	f, err := os.OpenFile(s.paths.SelfSamplesFile(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.WriteString(sb.String())
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// SelfSamples returns the newest limit samples (oldest first; limit <= 0 = all).
func (s *Store) SelfSamples(limit int) ([]model.SelfSample, error) {
	selfMu.Lock()
	defer selfMu.Unlock()
	xs, err := s.readSelfSamples()
	if err != nil {
		return []model.SelfSample{}, err
	}
	if limit > 0 && len(xs) > limit {
		xs = xs[len(xs)-limit:]
	}
	return nonNil(xs), nil
}

// SelfSampleStats returns how many samples are stored and the oldest one's time.
func (s *Store) SelfSampleStats() (count int, since *time.Time) {
	selfMu.Lock()
	defer selfMu.Unlock()
	xs, _ := s.readSelfSamples()
	for _, x := range xs {
		if since == nil || x.TS.Before(*since) {
			t := x.TS
			since = &t
		}
	}
	return len(xs), since
}

// ClearSelfSamples deletes every stored sample.
func (s *Store) ClearSelfSamples() error {
	selfMu.Lock()
	defer selfMu.Unlock()
	err := os.Remove(s.paths.SelfSamplesFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
