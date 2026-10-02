package server

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/goals"
	"whatsappdoppel/internal/mission"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/store"
)

// Missions: ready-made goals (internal/mission) started on a chat. Starting
// one sets the chat's goalOverride + missionId and starts the goal over; the
// engine completes the record when the goal is reached (engine/goal.go).

// GET /api/missions
func (s *Server) handleMissions(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	personas := map[string]model.Persona{}
	for _, p := range s.d.Store.Personas() {
		personas[p.ID] = p
	}
	active := []model.ActiveMission{}
	for _, c := range s.d.Store.Chats() {
		g := s.chatGoal(c, personas)
		if strings.TrimSpace(g.Text) == "" || g.State == "reached" {
			continue
		}
		missionID := ""
		if c.GoalOverride != "" && c.GoalOverride == g.Text {
			missionID = c.MissionID
		}
		active = append(active, model.ActiveMission{
			ChatKey: c.Key, ChatName: c.Name, Kind: c.Kind, JID: c.JID,
			PersonaID: c.PersonaID, PersonaName: personas[c.PersonaID].Name,
			MissionID: missionID, Goal: g,
		})
	}
	history := s.d.Store.Missions()
	writeJSON(w, http.StatusOK, model.MissionsView{
		Templates:    mission.Templates(),
		Active:       active,
		History:      nonNilSlice(history),
		Achievements: mission.Achievements(history, time.Local),
	})
}

// POST /api/missions/start {chatKey, templateId?, blanks?, goal?}
func (s *Server) handleStartMission(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	var body model.MissionStartRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	c, ok := s.requireChat(w, body.ChatKey)
	if !ok {
		return
	}
	var goal string
	tplID := strings.TrimSpace(body.TemplateID)
	if tplID != "" {
		tpl, ok := mission.Template(tplID)
		if !ok {
			writeError(w, http.StatusBadRequest, "unknown_mission", "No such mission")
			return
		}
		g, err := mission.Fill(tpl, body.Blanks)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_blanks", "Fill in every blank: "+strings.TrimPrefix(err.Error(), mission.ErrBlank.Error()+": "))
			return
		}
		goal = g
	} else {
		goal = strings.Join(strings.Fields(body.Goal), " ")
		if goal == "" {
			writeError(w, http.StatusBadRequest, "invalid_goal", "Pick a mission or write a goal")
			return
		}
	}
	if utf8.RuneCountInString(goal) > mission.MaxGoal {
		writeError(w, http.StatusBadRequest, "invalid_goal", "The goal can be at most 500 characters")
		return
	}
	c, err := s.d.Store.UpdateChat(c.Key, func(c *model.ChatAssignment) {
		c.GoalOverride, c.MissionID = goal, tplID
	})
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "No such chat assignment")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	// Start over, even when the same goal was reached before.
	if _, err := s.d.Store.UpdateRunnerState(c.Key, func(st *store.RunnerState) { st.Goal = nil }); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	p, _ := s.d.Store.Persona(c.PersonaID)
	if _, err := s.d.Store.StartMission(model.MissionRecord{
		ChatKey: c.Key, ChatName: c.Name, PersonaID: c.PersonaID, PersonaName: p.Name,
		Goal: goal, TemplateID: tplID, Style: goals.Resolve(p, c).Style, StartedAt: time.Now(),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.reloadEngine()
	s.chatsChanged()
	s.missionsChanged()
	writeJSON(w, http.StatusOK, c)
}

// POST /api/missions/{chatKey}/abandon — the chat goes back to the persona's goal (if any).
func (s *Server) handleAbandonMission(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("chatKey"))
	if !ok {
		return
	}
	c, err := s.d.Store.UpdateChat(c.Key, func(c *model.ChatAssignment) { c.GoalOverride, c.MissionID = "", "" })
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	if _, err := s.d.Store.UpdateRunnerState(c.Key, func(st *store.RunnerState) { st.Goal = nil }); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	if err := s.d.Store.AbandonMission(c.Key); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.reloadEngine()
	s.chatsChanged()
	s.missionsChanged()
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) missionsChanged() {
	if s.d.Hub != nil {
		s.d.Hub.Publish(events.TypeMissionsChanged, struct{}{})
	}
}
