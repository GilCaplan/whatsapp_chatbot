package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/store"
)

// chatView is a ChatAssignment plus the derived fields of GET /api/chats.
type chatView struct {
	model.ChatAssignment
	PersonaName  string `json:"personaName"`
	PendingCount int    `json:"pendingCount"`
	HistoryCount int    `json:"historyCount"`
	// Goal is the resolved goal settings and progress (handlers_goal.go).
	Goal model.ChatGoal `json:"goal"`
	// NextCheckInAt is when an automatic check-in is scheduled (runtime.json), null = none.
	NextCheckInAt *time.Time `json:"nextCheckInAt"`
}

func (s *Server) chatsChanged() {
	if s.d.Hub != nil {
		s.d.Hub.Publish(events.TypeChatsChanged, struct{}{})
	}
}

func (s *Server) reloadEngine() {
	if s.d.Engine != nil {
		s.d.Engine.Reload()
	}
}

func (s *Server) handleListChats(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	pending := map[string]int{}
	for _, p := range s.d.Store.Approvals() {
		pending[p.ChatKey]++
	}
	names := map[string]string{}
	personas := map[string]model.Persona{}
	for _, p := range s.d.Store.Personas() {
		names[p.ID] = p.Name
		personas[p.ID] = p
	}
	out := []chatView{}
	for _, c := range s.d.Store.Chats() {
		out = append(out, chatView{
			ChatAssignment: c,
			PersonaName:    names[c.PersonaID],
			PendingCount:   pending[c.Key],
			HistoryCount:   s.d.Store.HistoryCount(c.Key),
			Goal:           s.chatGoal(c, personas),
			NextCheckInAt:  s.d.Store.RunnerState(c.Key).ProactiveDueAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateChat(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil || s.d.WA == nil {
		unavailable(w, "WhatsApp")
		return
	}
	var body struct {
		JID          string          `json:"jid"`
		PersonaID    string          `json:"personaId"`
		Mode         *string         `json:"mode"`
		ApprovalMode *bool           `json:"approvalMode"`
		Enabled      *bool           `json:"enabled"`
		Behavior     json.RawMessage `json:"behavior"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	var overrides model.BehaviorOverrides
	if len(body.Behavior) > 0 && string(bytes.TrimSpace(body.Behavior)) != "null" {
		patch, err := parseOverridesPatch(body.Behavior)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_behavior", err.Error())
			return
		}
		overrides = applyOverridesPatch(overrides, patch)
	}
	if body.Mode != nil && !model.ValidChatMode(*body.Mode) {
		writeError(w, http.StatusBadRequest, "invalid_mode", "mode must be auto, approve or copilot")
		return
	}
	body.JID = strings.TrimSpace(body.JID)
	if body.JID == "" {
		writeError(w, http.StatusBadRequest, "missing_jid", "jid is required")
		return
	}
	if _, ok := s.d.Store.Persona(body.PersonaID); !ok {
		writeError(w, http.StatusBadRequest, "unknown_persona", "Choose a persona for this chat")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	item, err := s.d.WA.ResolveChat(ctx, body.JID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "resolve_failed", "Could not find that chat: "+err.Error())
		return
	}
	if item.Key == "" {
		writeError(w, http.StatusBadRequest, "resolve_failed", "Could not find that chat")
		return
	}
	if _, exists := s.d.Store.Chat(item.Key); exists {
		writeError(w, http.StatusConflict, "already_assigned", "This chat already has a persona")
		return
	}
	for _, j := range []string{item.JID, item.AltJID} {
		if j == "" {
			continue
		}
		if _, exists := s.d.Store.ChatByJID(j); exists {
			writeError(w, http.StatusConflict, "already_assigned", "This chat already has a persona")
			return
		}
	}
	kind := item.Kind
	if kind == "" {
		kind = "dm"
		if strings.HasPrefix(item.Key, "group:") {
			kind = "group"
		}
	}
	name := item.Name
	if name == "" {
		name = item.Phone
	}
	c := model.ChatAssignment{
		Key:       item.Key,
		Kind:      kind,
		JID:       item.JID,
		AltJID:    item.AltJID,
		Name:      name,
		PersonaID: body.PersonaID,
		Enabled:   body.Enabled == nil || *body.Enabled,
		Behavior:  overrides,
	}
	if body.ApprovalMode != nil {
		c.ApprovalMode = *body.ApprovalMode
	}
	if body.Mode != nil {
		c.Mode = *body.Mode // wins over approvalMode (the store derives approvalMode)
	}
	saved, err := s.d.Store.UpsertChat(c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.reloadEngine()
	s.chatsChanged()
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) handlePatchChat(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	key := r.PathValue("key")
	var body struct {
		Enabled      *bool           `json:"enabled"`
		PersonaID    *string         `json:"personaId"`
		Mode         *string         `json:"mode"`
		ApprovalMode *bool           `json:"approvalMode"`
		Memory       json.RawMessage `json:"memory"`
		MissionID    *string         `json:"missionId"`
		GoalOverride *string         `json:"goalOverride"`
		GoalStyle    json.RawMessage `json:"goalStyle"`
		GoalPlan     json.RawMessage `json:"goalPlanAhead"`
		Name         *string         `json:"name"`
		Behavior     json.RawMessage `json:"behavior"`
		SnoozedUntil json.RawMessage `json:"snoozedUntil"`
		People       json.RawMessage `json:"people"`
		Cross        json.RawMessage `json:"cross"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.PersonaID != nil {
		if _, ok := s.d.Store.Persona(*body.PersonaID); !ok {
			writeError(w, http.StatusBadRequest, "unknown_persona", "Unknown persona")
			return
		}
	}
	if body.Mode != nil && !model.ValidChatMode(*body.Mode) {
		writeError(w, http.StatusBadRequest, "invalid_mode", "mode must be auto, approve or copilot")
		return
	}
	// memory: absent = keep, null = follow the app setting, bool = this chat.
	var (
		hasMemory bool
		memory    *bool
	)
	if len(body.Memory) > 0 {
		hasMemory = true
		if string(bytes.TrimSpace(body.Memory)) != "null" {
			var v bool
			if err := json.Unmarshal(body.Memory, &v); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_memory", "memory must be true, false or null")
				return
			}
			memory = &v
		}
	}
	// behavior: absent = keep, null = inherit everything, object = partial
	// (a null field inherits that field; availability/proactive replace as blocks).
	var (
		patch      map[string]json.RawMessage
		resetAll   bool
		hasSnooze  = len(body.SnoozedUntil) > 0
		snooze     *time.Time
		hasPatches = len(body.Behavior) > 0
	)
	if hasPatches {
		if string(bytes.TrimSpace(body.Behavior)) == "null" {
			resetAll = true
		} else {
			p, err := parseOverridesPatch(body.Behavior)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_behavior", err.Error())
				return
			}
			patch = p
		}
	}
	goal, err := parseGoalPatch(body.GoalStyle, body.GoalPlan)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_goal", err.Error())
		return
	}
	var people *peoplePatch
	if len(body.People) > 0 && string(bytes.TrimSpace(body.People)) == "null" {
		people = &peoplePatch{reset: true}
	} else if len(body.People) > 0 {
		pp, err := parsePeoplePatch(body.People)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_people", err.Error())
			return
		}
		people = &pp
	}
	var cross *model.CrossContext
	if len(body.Cross) > 0 {
		cur := model.CrossContext{}
		if old, ok := s.d.Store.Chat(key); ok {
			cur = old.Cross
		}
		cc, err := parseCrossPatch(body.Cross, cur)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cross", err.Error())
			return
		}
		cross = &cc
	}
	if hasSnooze {
		t, err := parseSnooze(body.SnoozedUntil, time.Now())
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_snooze", err.Error())
			return
		}
		snooze = t
	}
	c, err := s.d.Store.UpdateChat(key, func(c *model.ChatAssignment) {
		if body.Enabled != nil {
			c.Enabled = *body.Enabled
		}
		if body.PersonaID != nil {
			c.PersonaID = *body.PersonaID
		}
		switch {
		case body.Mode != nil:
			c.Mode = *body.Mode // the store derives approvalMode
		case body.ApprovalMode != nil && *body.ApprovalMode != c.ApprovalMode:
			// Legacy clients: on = approve (co-pilot stays co-pilot), off = auto.
			c.ApprovalMode = *body.ApprovalMode
			c.Mode = model.ModeFromApproval(*body.ApprovalMode)
		}
		if hasMemory {
			c.Memory = memory
		}
		if body.MissionID != nil {
			c.MissionID = strings.TrimSpace(*body.MissionID)
		}
		if body.GoalOverride != nil {
			g := strings.TrimSpace(*body.GoalOverride)
			if g != c.GoalOverride && body.MissionID == nil {
				c.MissionID = "" // a hand-written goal is no longer the mission
			}
			c.GoalOverride = g
		}
		goal.apply(c)
		if body.Name != nil && strings.TrimSpace(*body.Name) != "" {
			c.Name = strings.TrimSpace(*body.Name)
		}
		switch {
		case resetAll:
			c.Behavior = model.BehaviorOverrides{}
		case patch != nil:
			// Rebuilt from JSON, so no pointer is shared with snapshots other
			// goroutines may be reading.
			c.Behavior = applyOverridesPatch(c.Behavior, patch)
		}
		if hasSnooze {
			c.SnoozedUntil = snooze
		}
		if people != nil {
			c.People = people.apply(c.People)
		}
		if cross != nil {
			c.Cross = *cross
		}
	})
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "No such chat assignment")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.reloadEngine()
	s.chatsChanged()
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteChat(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	key := r.PathValue("key")
	if err := s.d.Store.DeleteChat(key); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "No such chat assignment")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	// What the persona learned about the people here, and the chat's recaps,
	// go with the assignment.
	_ = s.d.Store.ClearMemories(key)
	_ = s.d.Store.DeleteRecaps(key)
	_ = s.d.Store.DeleteBrief(key)
	// Pending replies for a chat that is no longer assigned can never be sent.
	for _, p := range s.d.Store.Approvals() {
		if p.ChatKey != key {
			continue
		}
		if s.d.Engine != nil {
			_ = s.d.Engine.Discard(p.ID)
		} else {
			_ = s.d.Store.DeleteApproval(p.ID)
		}
	}
	s.reloadEngine()
	s.chatsChanged()
	writeOK(w)
}

func (s *Server) handleChatHistory(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	key := r.PathValue("key")
	msgs, err := s.d.Store.History(key, clamp(queryInt(r, "limit", 100), 1, 5000))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, nonNilSlice(msgs))
}

func (s *Server) handleClearChatHistory(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	key := r.PathValue("key")
	clear := s.d.Store.ClearHistory
	if s.d.Engine != nil {
		clear = s.d.Engine.ClearHistory
	}
	if err := clear(key); err != nil {
		writeError(w, http.StatusInternalServerError, "clear_failed", err.Error())
		return
	}
	s.chatsChanged()
	writeOK(w)
}

func (s *Server) requireChat(w http.ResponseWriter, key string) (model.ChatAssignment, bool) {
	c, ok := s.d.Store.Chat(key)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "No such chat assignment")
	}
	return c, ok
}

func (s *Server) handleChatSend(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil || s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	key := r.PathValue("key")
	var body struct {
		Text string `json:"text"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Text) == "" {
		writeError(w, http.StatusBadRequest, "missing_text", "Message text is required")
		return
	}
	if _, ok := s.requireChat(w, key); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := s.d.Engine.SendManual(ctx, key, body.Text); err != nil {
		writeError(w, http.StatusBadGateway, "send_failed", err.Error())
		return
	}
	writeOK(w)
}

// handleChatSimulate injects a fake incoming message. Because the generated
// reply would go to a real person, it is only allowed with --fake-wa, for your
// own "message yourself" chat, or when the chat requires approval.
func (s *Server) handleChatSimulate(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil || s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	key := r.PathValue("key")
	var body struct {
		Text      string `json:"text"`
		FromMe    bool   `json:"fromMe"`
		SenderJID string `json:"senderJid"` // groups: who it comes from ("" = a random member)
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Text) == "" {
		writeError(w, http.StatusBadRequest, "missing_text", "Message text is required")
		return
	}
	c, ok := s.requireChat(w, key)
	if !ok {
		return
	}
	own := s.ownJIDs()
	isSelf := own[c.JID] || (c.AltJID != "" && own[c.AltJID])
	if !s.d.FakeWA && !isSelf && !c.ApprovalMode {
		writeError(w, http.StatusForbidden, "simulate_not_allowed",
			"Simulating is only allowed in your own chat, in approval mode, or with --fake-wa (the reply would be sent to a real person)")
		return
	}
	if err := s.d.Engine.Simulate(key, body.Text, body.FromMe, strings.TrimSpace(body.SenderJID)); err != nil {
		writeError(w, http.StatusBadRequest, "simulate_failed", err.Error())
		return
	}
	writeOK(w)
}

// ─── Approvals ───────────────────────────────────────────────

func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	writeJSON(w, http.StatusOK, nonNilSlice(s.d.Store.Approvals()))
}

func (s *Server) requireApproval(w http.ResponseWriter, id string) bool {
	if s.d.Store == nil || s.d.Engine == nil {
		unavailable(w, "Engine")
		return false
	}
	if _, ok := s.d.Store.Approval(id); !ok {
		writeError(w, http.StatusNotFound, "not_found", "This reply is no longer pending")
		return false
	}
	return true
}

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Text  string `json:"text"`
		Draft *int   `json:"draft"` // co-pilot: which draft (0-based); absent = none
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if !s.requireApproval(w, id) {
		return
	}
	draft := -1
	if body.Draft != nil {
		draft = *body.Draft
		if draft < 0 {
			writeError(w, http.StatusBadRequest, "invalid_draft", "draft must be 0 or more")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := s.d.Engine.SendApproved(ctx, id, body.Text, draft); err != nil {
		if strings.Contains(err.Error(), "no such draft") {
			writeError(w, http.StatusBadRequest, "invalid_draft", "This reply has no such draft")
			return
		}
		writeError(w, http.StatusBadGateway, "send_failed", err.Error())
		return
	}
	writeOK(w)
}

func (s *Server) handleRegenerate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.requireApproval(w, id) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
	defer cancel()
	p, err := s.d.Engine.Regenerate(ctx, id)
	if err != nil {
		writeError(w, http.StatusBadGateway, "regenerate_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDiscard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.requireApproval(w, id) {
		return
	}
	if err := s.d.Engine.Discard(id); err != nil {
		writeError(w, http.StatusInternalServerError, "discard_failed", err.Error())
		return
	}
	writeOK(w)
}
