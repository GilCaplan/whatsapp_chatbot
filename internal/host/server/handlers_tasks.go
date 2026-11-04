package server

import (
	"cmp"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"whatsappdoppel/internal/core/events"
	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/core/store"
	"whatsappdoppel/internal/logic/task"
)

// Tasks (internal/logic/task, engine/task.go): saved tasks, templates and the
// fixed rule vocabulary, runs per chat, approving a confirmation, the
// calendar file of a booked meeting, the sandbox and cover-line drafts.

// writeTaskError maps the engine's task errors.
func writeTaskError(w http.ResponseWriter, err error) {
	msg := err.Error()
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Not found")
	case errors.Is(err, task.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_task", strings.TrimPrefix(msg, task.ErrInvalid.Error()+": "))
	case errors.Is(err, task.ErrRunning):
		writeError(w, http.StatusConflict, "task_running", msg)
	case errors.Is(err, task.ErrState):
		writeError(w, http.StatusConflict, "task_state", msg)
	case strings.Contains(msg, "disabled"):
		writeError(w, http.StatusConflict, "chat_disabled", "Replies are off for this chat: turn them on to run a task")
	default:
		writeError(w, http.StatusBadGateway, "task_failed", msg)
	}
}

// taskVocabulary is what the rule editor offers.
type taskVocabulary struct {
	Conditions    []string `json:"conditions"`
	Actions       []string `json:"actions"`
	Steps         []string `json:"steps"`
	Intents       []string `json:"intents"`
	PartnerStyles []string `json:"partnerStyles"`
}

// GET /api/tasks → {items (saved), templates, vocabulary}
func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	spec := task.FindTime()
	// The neutral Assistant persona, offered to installs that predate it
	// (or deleted it): null when it is there.
	var assistant *model.Persona
	have := slices.ContainsFunc(s.d.Store.Personas(), func(p model.Persona) bool {
		return p.ID == "assistant" || strings.EqualFold(strings.TrimSpace(p.Name), "assistant")
	})
	if !have && s.d.Seed != nil {
		if p, ok := s.d.Seed("assistant"); ok {
			p.ID, p.BuiltIn = "", false
			assistant = &p
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"assistant": assistant,
		"items":     nonNilSlice(s.d.Store.TaskSpecs()),
		"templates": task.Templates(),
		"vocabulary": taskVocabulary{Conditions: task.Conditions(spec), Actions: task.Actions(spec), Steps: task.Steps,
			Intents: task.Intents, PartnerStyles: task.PartnerStyles},
		"defaultRules": task.FindTimeRules(),
	})
}

// POST /api/tasks (save a new task) · PUT /api/tasks/{id}
func (s *Server) handleSaveTask(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	var spec model.TaskSpec
	if !decodeJSON(w, r, &spec) {
		return
	}
	if err := task.Normalize(&spec); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_task", err.Error())
		return
	}
	id := r.PathValue("id")
	now := time.Now()
	var saved model.TaskSpec
	_, err := s.d.Store.UpdateTaskSpecs(func(all []model.TaskSpec) ([]model.TaskSpec, error) {
		if id == "" {
			if len(all) >= 100 {
				return nil, errors.New("at most 100 saved tasks")
			}
			spec.ID, spec.CreatedAt, spec.UpdatedAt = store.NewID(), now, now
			saved = spec
			return append(all, spec), nil
		}
		i := slices.IndexFunc(all, func(t model.TaskSpec) bool { return t.ID == id })
		if i < 0 {
			return nil, store.ErrNotFound
		}
		spec.ID, spec.CreatedAt, spec.UpdatedAt = id, all[i].CreatedAt, now
		all[i], saved = spec, spec
		return all, nil
	})
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "No such task")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, "invalid_task", err.Error())
		return
	}
	s.d.Hub.Publish(events.TypeTasksChanged, map[string]string{})
	status := http.StatusOK
	if id == "" {
		status = http.StatusCreated
	}
	writeJSON(w, status, saved)
}

// DELETE /api/tasks/{id}
func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	id := r.PathValue("id")
	_, err := s.d.Store.UpdateTaskSpecs(func(all []model.TaskSpec) ([]model.TaskSpec, error) {
		i := slices.IndexFunc(all, func(t model.TaskSpec) bool { return t.ID == id })
		if i < 0 {
			return nil, store.ErrNotFound
		}
		return slices.Delete(all, i, i+1), nil
	})
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "No such task")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_failed", err.Error())
		return
	}
	s.d.Hub.Publish(events.TypeTasksChanged, map[string]string{})
	writeOK(w)
}

// GET /api/tasks/runs?chatKey=&active=1&limit= (newest first)
func (s *Server) handleTaskRuns(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 300 {
		limit = 100
	}
	out := []model.TaskRun{}
	for _, run := range s.d.Store.TaskRuns() {
		if k := q.Get("chatKey"); k != "" && run.ChatKey != k {
			continue
		}
		if q.Get("active") == "1" && !run.Active() {
			continue
		}
		out = append(out, run)
		if len(out) >= limit {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// GET /api/tasks/runs/{id}
func (s *Server) handleTaskRun(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	run, ok := s.d.Store.TaskRun(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "No such task run")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// startTaskBody: a spec, or a saved task's id, or a template id.
type startTaskBody struct {
	Spec     *model.TaskSpec `json:"spec"`
	TaskID   string          `json:"taskId"`
	Template string          `json:"template"`
	Open     bool            `json:"open"` // write first now
}

// POST /api/chats/{key}/task
func (s *Server) handleStartTask(w http.ResponseWriter, r *http.Request) {
	if s.d.Tasks == nil {
		unavailable(w, "Tasks")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	var b startTaskBody
	if !decodeJSON(w, r, &b) {
		return
	}
	var spec model.TaskSpec
	switch {
	case b.Spec != nil:
		spec = *b.Spec
	case b.TaskID != "":
		i := slices.IndexFunc(s.d.Store.TaskSpecs(), func(t model.TaskSpec) bool { return t.ID == b.TaskID })
		if i < 0 {
			writeError(w, http.StatusNotFound, "not_found", "No such saved task")
			return
		}
		spec = s.d.Store.TaskSpecs()[i]
	default:
		t, ok := task.Template(cmp.Or(b.Template, model.TaskFindTime))
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_task", "No such template")
			return
		}
		spec = t
	}
	run, err := s.d.Tasks.StartTask(r.Context(), c.Key, spec, b.Open)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

// POST /api/tasks/runs/{id}/stop
func (s *Server) handleStopTask(w http.ResponseWriter, r *http.Request) {
	if s.d.Tasks == nil {
		unavailable(w, "Tasks")
		return
	}
	run, err := s.d.Tasks.StopTask(r.PathValue("id"))
	if err != nil {
		writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// POST /api/tasks/runs/{id}/approve {yes}
func (s *Server) handleApproveTask(w http.ResponseWriter, r *http.Request) {
	if s.d.Tasks == nil {
		unavailable(w, "Tasks")
		return
	}
	var b struct {
		Yes *bool `json:"yes"`
	}
	if !decodeJSON(w, r, &b) {
		return
	}
	if b.Yes == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Say yes or no")
		return
	}
	run, err := s.d.Tasks.ApproveTask(r.Context(), r.PathValue("id"), *b.Yes)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// GET /api/tasks/runs/{id}/event.ics
func (s *Server) handleTaskICS(w http.ResponseWriter, r *http.Request) {
	if s.d.Tasks == nil {
		unavailable(w, "Tasks")
		return
	}
	b, err := s.d.Tasks.TaskICS(r.PathValue("id"))
	if err != nil {
		writeTaskError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="meeting.ics"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

// POST /api/tasks/try {personaId, spec, style, turns}
func (s *Server) handleTryTask(w http.ResponseWriter, r *http.Request) {
	if s.d.Tasks == nil {
		unavailable(w, "Tasks")
		return
	}
	var b struct {
		PersonaID string         `json:"personaId"`
		Spec      model.TaskSpec `json:"spec"`
		Style     string         `json:"style"`
		Turns     int            `json:"turns"`
	}
	if !decodeJSON(w, r, &b) {
		return
	}
	if b.Style != "" && !slices.Contains(task.PartnerStyles, b.Style) {
		writeError(w, http.StatusBadRequest, "bad_request", "Unknown style")
		return
	}
	out, err := s.d.Tasks.TaskTry(r.Context(), b.PersonaID, b.Spec, cmp.Or(b.Style, "cooperative"), cmp.Or(b.Turns, 6))
	if err != nil {
		writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/tasks/cover-lines {description}
func (s *Server) handleCoverLines(w http.ResponseWriter, r *http.Request) {
	if s.d.Tasks == nil {
		unavailable(w, "Tasks")
		return
	}
	var b struct {
		Description string `json:"description"`
	}
	if !decodeJSON(w, r, &b) {
		return
	}
	if len([]rune(b.Description)) > 400 {
		writeError(w, http.StatusBadRequest, "bad_request", "A description has at most 400 characters")
		return
	}
	lines, err := s.d.Tasks.CoverLines(r.Context(), b.Description)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines})
}
