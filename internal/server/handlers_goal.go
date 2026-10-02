package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"whatsappdoppel/internal/goals"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/store"
)

// chatGoal resolves a chat's goal settings and progress (persona + chat
// overrides + runtime status).
func (s *Server) chatGoal(c model.ChatAssignment, personas map[string]model.Persona) model.ChatGoal {
	p, ok := personas[c.PersonaID]
	if !ok {
		p, _ = s.d.Store.Persona(c.PersonaID)
	}
	st := s.d.Store.RunnerState(c.Key)
	return goals.View(p, c, st.Goal)
}

// goalPatch is the goal part of PATCH /api/chats/{key}: absent = keep,
// null = use the persona's setting, value = override.
type goalPatch struct {
	setStyle, setPlan bool
	style             *string
	plan              *bool
}

func parseGoalPatch(styleRaw, planRaw json.RawMessage) (goalPatch, error) {
	var gp goalPatch
	if len(styleRaw) > 0 {
		gp.setStyle = true
		if string(bytes.TrimSpace(styleRaw)) != "null" {
			var v string
			if err := json.Unmarshal(styleRaw, &v); err != nil {
				return gp, errors.New("goalStyle must be a string or null")
			}
			v = strings.ToLower(strings.TrimSpace(v))
			if !slices.Contains(model.GoalStyles, v) {
				return gp, fmt.Errorf("goalStyle must be one of %s, or null", strings.Join(model.GoalStyles, ", "))
			}
			gp.style = &v
		}
	}
	if len(planRaw) > 0 {
		gp.setPlan = true
		if string(bytes.TrimSpace(planRaw)) != "null" {
			var v bool
			if err := json.Unmarshal(planRaw, &v); err != nil {
				return gp, errors.New("goalPlanAhead must be true, false or null")
			}
			gp.plan = &v
		}
	}
	return gp, nil
}

func (gp goalPatch) apply(c *model.ChatAssignment) {
	if gp.setStyle {
		c.GoalStyle = gp.style
	}
	if gp.setPlan {
		c.GoalPlanAhead = gp.plan
	}
}

// handleGoalReset forgets a chat's goal progress: the persona starts
// pursuing the goal again.
func (s *Server) handleGoalReset(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	if _, err := s.d.Store.UpdateRunnerState(c.Key, func(st *store.RunnerState) { st.Goal = nil }); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.chatsChanged()
	writeJSON(w, http.StatusOK, s.chatGoal(c, nil))
}

// handleInitiate makes the persona start a conversation now ("Start the
// conversation"). The opener goes through the normal delivery path, or waits
// in Approvals when the chat is in approval mode.
func (s *Server) handleInitiate(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil || s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	var body struct {
		Hint string `json:"hint"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if len([]rune(body.Hint)) > maxInitiateHint {
		writeError(w, http.StatusBadRequest, "invalid_hint", fmt.Sprintf("The topic can be at most %d characters", maxInitiateHint))
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	if !c.Enabled {
		writeError(w, http.StatusConflict, "chat_disabled", "Replies are off for this chat — turn them on to let the persona start a conversation")
		return
	}
	if err := s.d.Engine.Initiate(r.Context(), c.Key, body.Hint); err != nil {
		switch {
		case strings.Contains(err.Error(), "disabled"):
			writeError(w, http.StatusConflict, "chat_disabled", "Replies are off for this chat — turn them on to let the persona start a conversation")
		case strings.Contains(err.Error(), "already writing"):
			writeError(w, http.StatusConflict, "busy", err.Error())
		case strings.Contains(err.Error(), "not found"):
			writeError(w, http.StatusNotFound, "not_found", err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "initiate_failed", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "approval": c.ApprovalMode})
}

// maxInitiateHint mirrors prompt.MaxOpenerHint (the server does not import prompt).
const maxInitiateHint = 300
