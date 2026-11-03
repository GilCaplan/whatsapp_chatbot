package store

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/model"
)

// Nicknames you give people (nicknames.json, all chats). They are applied
// where names are read — history (History), incoming senders and group
// rosters (engine) and the API views — so changing or removing one takes
// effect everywhere, also for messages already stored.

type nickState struct {
	mu     sync.Mutex
	loaded bool
	list   []model.Nickname
	byUser map[string]string // user part → nickname
}

func (s *Store) nickLoadLocked() {
	n := &s.nicks
	if n.loaded {
		return
	}
	n.loaded = true
	if err := readJSON(s.paths.NicknamesFile(), &n.list); err != nil && !errors.Is(err, os.ErrNotExist) {
		n.list = nil
	}
	n.indexLocked()
}

func (n *nickState) indexLocked() {
	n.byUser = map[string]string{}
	for _, nk := range n.list {
		for _, id := range nk.IDs {
			if u := behavior.UserOf(id); u != "" {
				n.byUser[u] = nk.Name
			}
		}
	}
}

// Nicknames returns every nickname (newest first).
func (s *Store) Nicknames() []model.Nickname {
	s.nicks.mu.Lock()
	defer s.nicks.mu.Unlock()
	s.nickLoadLocked()
	out := slices.Clone(s.nicks.list)
	slices.SortStableFunc(out, func(a, b model.Nickname) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return nonNil(out)
}

// NicknameFor returns the nickname of the person with any of ids ("" = none).
func (s *Store) NicknameFor(ids ...string) string {
	s.nicks.mu.Lock()
	defer s.nicks.mu.Unlock()
	s.nickLoadLocked()
	for _, id := range ids {
		if nk := s.nicks.byUser[behavior.UserOf(id)]; nk != "" && behavior.UserOf(id) != "" {
			return nk
		}
	}
	return ""
}

// ApplyNicknames renames the senders of msgs that have a nickname (in place).
func (s *Store) ApplyNicknames(msgs []model.Message) {
	s.nicks.mu.Lock()
	defer s.nicks.mu.Unlock()
	s.nickLoadLocked()
	if len(s.nicks.byUser) == 0 {
		return
	}
	for i := range msgs {
		if m := &msgs[i]; m.Speaker == "them" && m.SenderJID != "" {
			if nk := s.nicks.byUser[behavior.UserOf(m.SenderJID)]; nk != "" {
				m.Name = nk
			}
		}
	}
}

// CleanNickname trims and validates a nickname ("" = remove).
func CleanNickname(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if utf8.RuneCountInString(name) > model.MaxNickname {
		return "", fmt.Errorf("a nickname has at most %d characters", model.MaxNickname)
	}
	if strings.ContainsAny(name, "@:") {
		return "", errors.New("a nickname can't contain @ or :")
	}
	return name, nil
}

// SetNickname gives the person with ids a nickname, replacing the one any of
// ids had; an empty name removes it. original is their WhatsApp name.
func (s *Store) SetNickname(ids []string, name, original string) (model.Nickname, error) {
	name, err := CleanNickname(name)
	if err != nil {
		return model.Nickname{}, err
	}
	users := map[string]bool{}
	var keep []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if u := behavior.UserOf(id); u != "" && !users[u] {
			users[u] = true
			keep = append(keep, id)
		}
	}
	if len(keep) == 0 {
		return model.Nickname{}, errors.New("say who the nickname is for")
	}
	s.nicks.mu.Lock()
	defer s.nicks.mu.Unlock()
	s.nickLoadLocked()
	var list []model.Nickname
	for _, nk := range s.nicks.list {
		mine := false
		for _, id := range nk.IDs {
			if users[behavior.UserOf(id)] {
				mine = true
				break
			}
		}
		if !mine {
			list = append(list, nk)
			continue
		}
		if original == "" {
			original = nk.Original
		}
		for _, id := range nk.IDs { // keep addresses we knew them by
			if u := behavior.UserOf(id); u != "" && !users[u] {
				users[u] = true
				keep = append(keep, id)
			}
		}
	}
	nk := model.Nickname{IDs: keep, Name: name, Original: strings.TrimSpace(original), UpdatedAt: time.Now()}
	if name != "" {
		list = append(list, nk)
	}
	if err := config.WriteJSONAtomic(s.paths.NicknamesFile(), nonNil(list), 0o600); err != nil {
		return model.Nickname{}, err
	}
	s.nicks.list = list
	s.nicks.indexLocked()
	return nk, nil
}
