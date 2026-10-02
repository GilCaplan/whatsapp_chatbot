package store

import (
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/model"
)

// Mission history (missions.json, newest first, at most MaxMissions records).
// A record is "open" until its goal is reached (ReachedAt set). Starting or
// abandoning a mission drops the chat's open record; reaching any chat goal
// completes the open record for that goal (or adds one, so goals typed by
// hand count too). The file is small and read on demand.

// MaxMissions caps missions.json.
const MaxMissions = 500

// missionsMu serialises missions.json read-modify-write cycles.
var missionsMu sync.Mutex

func (s *Store) readMissions() []model.MissionRecord {
	var out []model.MissionRecord
	if err := readJSON(s.paths.MissionsFile(), &out); err != nil && !errors.Is(err, os.ErrNotExist) {
		return []model.MissionRecord{}
	}
	if out == nil {
		out = []model.MissionRecord{}
	}
	return out
}

func (s *Store) writeMissions(rs []model.MissionRecord) error {
	if len(rs) > MaxMissions {
		rs = rs[:MaxMissions]
	}
	return config.WriteJSONAtomic(s.paths.MissionsFile(), rs, 0o600)
}

// Missions returns every recorded mission, newest first.
func (s *Store) Missions() []model.MissionRecord {
	missionsMu.Lock()
	defer missionsMu.Unlock()
	return s.readMissions()
}

// StartMission records a started mission (empty ID → new; zero StartedAt →
// now) and returns it. The chat's earlier open record is dropped.
func (s *Store) StartMission(r model.MissionRecord) (model.MissionRecord, error) {
	missionsMu.Lock()
	defer missionsMu.Unlock()
	if r.ID == "" {
		r.ID = NewID()
	}
	if r.StartedAt.IsZero() {
		r.StartedAt = time.Now()
	}
	r.ReachedAt, r.How, r.Evidence = nil, "", ""
	rs := s.readMissions()
	rs = slices.DeleteFunc(rs, func(x model.MissionRecord) bool { return x.ChatKey == r.ChatKey && x.ReachedAt == nil })
	rs = append([]model.MissionRecord{r}, rs...)
	return r, s.writeMissions(rs)
}

// AbandonMission drops the chat's open record (no-op when there is none).
func (s *Store) AbandonMission(chatKey string) error {
	missionsMu.Lock()
	defer missionsMu.Unlock()
	rs := s.readMissions()
	n := len(rs)
	rs = slices.DeleteFunc(rs, func(x model.MissionRecord) bool { return x.ChatKey == chatKey && x.ReachedAt == nil })
	if len(rs) == n {
		return nil
	}
	return s.writeMissions(rs)
}

// CompleteMission marks the chat's open mission for goal as reached (from the
// goal status). With no open record for that goal, a record is added (a goal
// typed by hand or the persona's own goal still counts). ok is false when the
// status is not reached or the same goal was already completed since st.Since.
// Chat/persona names are filled from the store.
func (s *Store) CompleteMission(chatKey, goal string, st model.GoalStatus) (rec model.MissionRecord, ok bool, err error) {
	if !st.Reached || strings.TrimSpace(goal) == "" {
		return model.MissionRecord{}, false, nil
	}
	reached := time.Now()
	if st.ReachedAt != nil {
		reached = *st.ReachedAt
	}
	c, _ := s.Chat(chatKey)
	p, _ := s.Persona(c.PersonaID)

	missionsMu.Lock()
	defer missionsMu.Unlock()
	rs := s.readMissions()
	idx := -1
	for i, x := range rs {
		if x.ChatKey != chatKey || x.Goal != goal {
			continue
		}
		if x.ReachedAt == nil {
			idx = i
			break
		}
		// Already completed for this run of the goal (e.g. reported twice).
		if !st.Since.IsZero() && !x.ReachedAt.Before(st.Since) {
			return x, false, nil
		}
	}
	if idx < 0 {
		since := st.Since
		if since.IsZero() {
			since = reached
		}
		rs = append([]model.MissionRecord{{ID: NewID(), ChatKey: chatKey, Goal: goal, StartedAt: since}}, rs...)
		idx = 0
	}
	r := &rs[idx]
	r.ReachedAt, r.How, r.Evidence = &reached, st.How, st.Evidence
	if r.ChatName == "" {
		r.ChatName = c.Name
	}
	if r.PersonaID == "" {
		r.PersonaID = c.PersonaID
	}
	if r.PersonaName == "" {
		r.PersonaName = p.Name
	}
	if r.TemplateID == "" && c.MissionID != "" && c.GoalOverride == goal {
		r.TemplateID = c.MissionID
	}
	if r.Style == "" {
		r.Style = goalStyleOf(c, p)
	}
	rec = *r
	// Keep newest-first by start time after an insert.
	slices.SortStableFunc(rs, func(a, b model.MissionRecord) int { return b.StartedAt.Compare(a.StartedAt) })
	return rec, true, s.writeMissions(rs)
}

// goalStyleOf is the chat's effective goal style (chat override → persona → subtle).
func goalStyleOf(c model.ChatAssignment, p model.Persona) string {
	if c.GoalStyle != nil && *c.GoalStyle != "" {
		return *c.GoalStyle
	}
	if p.GoalStyle != "" {
		return p.GoalStyle
	}
	return model.GoalStyleSubtle
}
