package store

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/ctxsys"
	"whatsappdoppel/internal/model"
)

// Your context systems (contextsystems.json; the built-in ones live in
// ctxsys and are never stored). Read on demand, own mutex.

var ctxMu sync.Mutex

func (s *Store) readContextSystems() []model.ContextSystem {
	var out []model.ContextSystem
	if err := readJSON(s.paths.ContextSystemsFile(), &out); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return out
}

// ContextSystems returns the built-in systems, then yours (oldest first).
func (s *Store) ContextSystems() []model.ContextSystem {
	ctxMu.Lock()
	mine := s.readContextSystems()
	ctxMu.Unlock()
	return append(ctxsys.Builtins(), mine...)
}

// ContextSystem finds a system by id (built-in or yours).
func (s *Store) ContextSystem(id string) (model.ContextSystem, bool) {
	if ctxsys.IsBase(id) {
		return ctxsys.Builtin(id), true
	}
	if id == "" {
		return model.ContextSystem{}, false
	}
	ctxMu.Lock()
	defer ctxMu.Unlock()
	for _, c := range s.readContextSystems() {
		if c.ID == id {
			return c, true
		}
	}
	return model.ContextSystem{}, false
}

// SaveContextSystem creates (empty ID) or updates one of your systems,
// normalised by ctxsys.Normalize.
func (s *Store) SaveContextSystem(c model.ContextSystem) (model.ContextSystem, error) {
	ctxMu.Lock()
	defer ctxMu.Unlock()
	all := s.readContextSystems()
	now := time.Now()
	i := -1
	if c.ID != "" {
		i = slices.IndexFunc(all, func(x model.ContextSystem) bool { return x.ID == c.ID })
		if i < 0 && !ctxsys.IsBase(c.ID) {
			return c, ErrNotFound
		}
	} else {
		if len(all) >= ctxsys.MaxSystems {
			return c, fmt.Errorf("you can have at most %d systems of your own", ctxsys.MaxSystems)
		}
		c.ID = "cs-" + NewID()
	}
	if err := ctxsys.Normalize(&c); err != nil {
		return c, err
	}
	c.UpdatedAt = now
	if i >= 0 {
		c.CreatedAt = all[i].CreatedAt
		all[i] = c
	} else {
		c.CreatedAt = now
		all = append(all, c)
	}
	return c, config.WriteJSONAtomic(s.paths.ContextSystemsFile(), all, 0o600)
}

// DeleteContextSystem removes one of your systems; chats and personas that
// used it go back to inheriting.
func (s *Store) DeleteContextSystem(id string) error {
	ctxMu.Lock()
	all := s.readContextSystems()
	i := slices.IndexFunc(all, func(x model.ContextSystem) bool { return x.ID == id })
	if i < 0 {
		ctxMu.Unlock()
		return ErrNotFound
	}
	all = slices.Delete(all, i, i+1)
	err := config.WriteJSONAtomic(s.paths.ContextSystemsFile(), nonNil(all), 0o600)
	ctxMu.Unlock()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	chats, personas := false, false
	for k := range s.chats {
		if s.chats[k].ContextSystem == id {
			s.chats[k].ContextSystem, chats = "", true
		}
	}
	for k := range s.personas {
		if s.personas[k].ContextSystem == id {
			s.personas[k].ContextSystem, personas = "", true
		}
	}
	if chats {
		if err := s.saveChats(); err != nil {
			return err
		}
	}
	if personas {
		return s.savePersonas()
	}
	return nil
}

// SetPersonaCard saves a persona's card ("" = make it again when needed).
func (s *Store) SetPersonaCard(id, card string) (model.Persona, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.personaIndex(id)
	if i < 0 {
		return model.Persona{}, ErrNotFound
	}
	now := time.Now()
	p := s.personas[i]
	p.Card = card
	p.CardAt = &now
	if card == "" {
		p.CardAt = nil
	}
	s.personas[i] = p
	return p, s.savePersonas()
}
