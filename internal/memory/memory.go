// Package memory merges what the persona learned about people into a chat's
// memories (dedupe, update, cap, expiry) and picks the ones worth showing
// in a reply prompt. It is a leaf package (model + stdlib).
package memory

import (
	"crypto/rand"
	"encoding/hex"
	"math"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"whatsappdoppel/internal/model"
)

// Limits.
const (
	MaxPerChat     = 60                  // memories kept per chat (pinned and your own are never evicted)
	EventGrace     = 7 * 24 * time.Hour  // an event is forgotten this long after its date
	SameThreshold  = 0.6                 // Jaccard similarity that counts as "the same memory" (updated)
	DupeThreshold  = 0.85                // so similar it is just a repeat (skipped)
	LearnedConf    = 70                  // confidence of a learned memory
	recencyHalfLif = 14 * 24 * time.Hour // recency score halves every two weeks
)

// Candidate is a memory found by the extractor, with its person resolved.
type Candidate struct {
	Person    string // display name
	PersonJID string // "" when unknown
	Text      string
	Kind      string
	Evidence  string
	ExpiresAt *time.Time
	Sensitive string // "" or a model.SensitiveCategories value
}

// NewID returns a random memory id.
func NewID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CleanText makes a memory text one trimmed line of at most MaxMemoryText runes.
func CleanText(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimRight(s, ".")
	if utf8.RuneCountInString(s) > model.MaxMemoryText {
		s = strings.TrimSpace(string([]rune(s)[:model.MaxMemoryText]))
	}
	return s
}

// ValidKind returns k when it is a memory kind, else "other".
func ValidKind(k string) string {
	k = strings.ToLower(strings.TrimSpace(k))
	if slices.Contains(model.MemoryKinds, k) {
		return k
	}
	return model.MemoryOther
}

// Expired reports whether an event memory is past its grace period.
func Expired(m model.Memory, now time.Time) bool {
	return !m.Pinned && m.ExpiresAt != nil && now.After(m.ExpiresAt.Add(EventGrace))
}

// samePerson reports whether a memory and a candidate are about the same person.
func samePerson(m model.Memory, c Candidate) bool {
	if m.PersonJID != "" && c.PersonJID != "" {
		return m.PersonJID == c.PersonJID
	}
	a, b := strings.ToLower(strings.TrimSpace(m.Person)), strings.ToLower(strings.TrimSpace(c.Person))
	if a == b {
		return true
	}
	return a != "" && b != "" && firstWord(a) == firstWord(b)
}

func firstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return s
}

// Merge folds found memories into existing ones: a candidate close to an
// existing memory about the same person updates it (text, kind, evidence,
// expiry; id and pin kept — your own memories are never rewritten), a
// near-repeat is skipped, anything else is added as learned. Expired events
// are dropped and the list is capped at max (0 = MaxPerChat) by evicting
// the oldest unpinned learned memories. It returns the new list and what
// was added and updated.
func Merge(existing []model.Memory, found []Candidate, chatKey string, now time.Time, max int) (out, added, updated []model.Memory) {
	if max <= 0 {
		max = MaxPerChat
	}
	out = make([]model.Memory, 0, len(existing)+len(found))
	for _, m := range existing {
		if !Expired(m, now) {
			out = append(out, m)
		}
	}
	touched := map[string]bool{}
	for _, c := range found {
		c.Text = CleanText(c.Text)
		if c.Text == "" {
			continue
		}
		toks := Tokens(c.Text)
		best, bestSim := -1, 0.0
		for i, m := range out {
			if !samePerson(m, c) {
				continue
			}
			if sim := Jaccard(toks, Tokens(m.Text)); sim > bestSim {
				best, bestSim = i, sim
			}
		}
		switch {
		case best >= 0 && (bestSim >= DupeThreshold || Contained(toks, Tokens(out[best].Text)) >= 0.9):
			continue // already known (or a less detailed version of it)
		case best >= 0 && bestSim >= SameThreshold:
			m := &out[best]
			if m.Source == model.MemorySourceUser || touched[m.ID] {
				continue
			}
			m.Text, m.Kind, m.UpdatedAt = c.Text, ValidKind(c.Kind), now
			if c.Evidence != "" {
				m.Evidence = c.Evidence
			}
			if c.ExpiresAt != nil {
				t := *c.ExpiresAt
				m.ExpiresAt = &t
			}
			if m.PersonJID == "" {
				m.PersonJID = c.PersonJID
			}
			if c.Sensitive != "" && m.Sensitive == "" {
				m.Sensitive = c.Sensitive // never quietly made less sensitive
			}
			touched[m.ID] = true
			updated = append(updated, *m)
		default:
			m := model.Memory{
				ID: NewID(), ChatKey: chatKey, PersonJID: c.PersonJID, Person: c.Person,
				Text: c.Text, Kind: ValidKind(c.Kind), Source: model.MemorySourceLearned,
				Confidence: LearnedConf, Evidence: c.Evidence, CreatedAt: now, UpdatedAt: now, Sensitive: c.Sensitive,
			}
			if c.ExpiresAt != nil {
				t := *c.ExpiresAt
				m.ExpiresAt = &t
			}
			out = append(out, m)
			touched[m.ID] = true
			added = append(added, m)
		}
	}
	out = Cap(out, max)
	// Report only what survived the cap.
	keep := map[string]bool{}
	for _, m := range out {
		keep[m.ID] = true
	}
	added = slices.DeleteFunc(added, func(m model.Memory) bool { return !keep[m.ID] })
	return out, added, updated
}

// Cap evicts the oldest unpinned learned memories until at most max remain.
func Cap(mems []model.Memory, max int) []model.Memory {
	for len(mems) > max {
		victim := -1
		for i, m := range mems {
			if m.Pinned || m.Source == model.MemorySourceUser {
				continue
			}
			if victim < 0 || m.UpdatedAt.Before(mems[victim].UpdatedAt) {
				victim = i
			}
		}
		if victim < 0 {
			break // only pinned/your own left: keep them all
		}
		mems = slices.Delete(mems, victim, victim+1)
	}
	return mems
}

// Sort orders memories for display: pinned first, then by person, newest first.
func Sort(mems []model.Memory) {
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

// Select picks up to k memories for a reply prompt: pinned first, then by a
// score of recency, overlap with what's being talked about (recent texts)
// and events coming up within a week. Expired events are skipped.
func Select(mems []model.Memory, recent []string, now time.Time, k int) []model.Memory {
	if k <= 0 || len(mems) == 0 {
		return nil
	}
	topic := map[string]bool{}
	for _, t := range recent {
		for _, w := range Tokens(t) {
			topic[w] = true
		}
	}
	type scored struct {
		m     model.Memory
		score float64
	}
	var list []scored
	for _, m := range mems {
		if Expired(m, now) {
			continue
		}
		s := 0.0
		if m.Pinned {
			s += 100
		}
		age := now.Sub(m.UpdatedAt)
		if age < 0 {
			age = 0
		}
		s += math.Pow(0.5, float64(age)/float64(recencyHalfLif))
		toks := Tokens(m.Text + " " + m.Person)
		hits := 0
		for _, w := range toks {
			if topic[w] {
				hits++
			}
		}
		s += 1.2 * float64(min(hits, 2)) // talking about it right now matters most
		if m.ExpiresAt != nil {
			if d := m.ExpiresAt.Sub(now); d > -48*time.Hour && d < 7*24*time.Hour {
				s += 1
			}
		}
		if m.Source == model.MemorySourceUser {
			s += 0.3
		}
		list = append(list, scored{m, s})
	}
	slices.SortStableFunc(list, func(a, b scored) int {
		switch {
		case a.score > b.score:
			return -1
		case a.score < b.score:
			return 1
		}
		return 0
	})
	if len(list) > k {
		list = list[:k]
	}
	out := make([]model.Memory, len(list))
	for i, s := range list {
		out[i] = s.m
	}
	return out
}

// ─── similarity ──────────────────────────────────────────────

var stop = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "has": true, "have": true, "her": true, "his": true,
	"their": true, "they": true, "she": true, "him": true, "was": true, "are": true, "is": true, "on": true,
	"at": true, "in": true, "of": true, "to": true, "a": true, "an": true, "about": true, "from": true, "will": true,
}

// Tokens are a text's lower-case words (letters/digits, 2+ runes) minus
// common stop words.
func Tokens(s string) []string {
	f := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	out := f[:0]
	seen := map[string]bool{}
	for _, w := range f {
		if utf8.RuneCountInString(w) < 2 || stop[w] || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	return out
}

// Contained is the share of a's tokens that are also in b (0 when a is empty).
func Contained(a, b []string) float64 {
	if len(a) == 0 {
		return 0
	}
	set := map[string]bool{}
	for _, w := range b {
		set[w] = true
	}
	n := 0
	for _, w := range a {
		if set[w] {
			n++
		}
	}
	return float64(n) / float64(len(a))
}

// Jaccard is |a∩b| / |a∪b| of two token sets (0 when both are empty).
func Jaccard(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	set := map[string]bool{}
	for _, w := range a {
		set[w] = true
	}
	inter := 0
	union := len(set)
	for _, w := range b {
		if set[w] {
			inter++
		} else {
			union++
		}
	}
	return float64(inter) / float64(union)
}
