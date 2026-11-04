package server

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/logic/task"
)

type fakeTasks struct {
	started []model.TaskSpec
	open    bool
	err     error
	yes     *bool
}

func (f *fakeTasks) StartTask(_ context.Context, key string, spec model.TaskSpec, open bool) (model.TaskRun, error) {
	if f.err != nil {
		return model.TaskRun{}, f.err
	}
	f.started, f.open = append(f.started, spec), open
	return model.TaskRun{ID: "r1", ChatKey: key, Spec: spec, Status: model.TaskActive}, nil
}
func (f *fakeTasks) StopTask(id string) (model.TaskRun, error) {
	return model.TaskRun{ID: id, Status: model.TaskStopped}, nil
}
func (f *fakeTasks) ApproveTask(_ context.Context, id string, yes bool) (model.TaskRun, error) {
	f.yes = &yes
	return model.TaskRun{ID: id, Status: model.TaskActive}, nil
}
func (f *fakeTasks) TaskICS(id string) ([]byte, error) {
	return []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"), nil
}
func (f *fakeTasks) TaskTry(context.Context, string, model.TaskSpec, string, int) (model.TaskTry, error) {
	return model.TaskTry{Status: model.TaskDone}, nil
}
func (f *fakeTasks) CoverLines(context.Context, string) ([]string, error) {
	return []string{"I've got something on then"}, nil
}

func TestTaskRoutes(t *testing.T) {
	e := newEnv(t)
	ft := &fakeTasks{}
	e.s.d.Tasks = ft
	var c model.ChatAssignment
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "leo", "name": "Dana"}, &c); code != 201 {
		t.Fatalf("assign = %d", code)
	}
	key := strings.ReplaceAll(c.Key, ":", "%3A")

	var view struct {
		Templates  []model.TaskTemplate `json:"templates"`
		Vocabulary taskVocabulary       `json:"vocabulary"`
	}
	e.do("GET", "/api/tasks", nil, &view)
	if len(view.Templates) == 0 || len(view.Vocabulary.Conditions) == 0 || len(view.Vocabulary.PartnerStyles) != 4 {
		t.Fatalf("view = %+v", view)
	}

	// Save, edit, delete a task.
	spec := task.FindTime()
	spec.About = "kickoff"
	var saved model.TaskSpec
	if code := e.do("POST", "/api/tasks", spec, &saved); code != 201 || saved.ID == "" {
		t.Fatalf("save = %d %+v", code, saved)
	}
	saved.Meeting.Minutes = 30
	if code := e.do("PUT", "/api/tasks/"+saved.ID, saved, &saved); code != 200 || saved.Meeting.Minutes != 30 {
		t.Fatalf("edit = %d", code)
	}
	var apiErr apiError
	bad := saved
	bad.Meeting.From, bad.Meeting.To = "18:00", "09:00"
	if code := e.do("PUT", "/api/tasks/"+saved.ID, bad, &apiErr); code != 400 || apiErr.Code != "invalid_task" {
		t.Fatalf("invalid edit = %d %+v", code, apiErr)
	}

	// Start: a saved task, a template, errors.
	var run model.TaskRun
	if code := e.do("POST", "/api/chats/"+key+"/task", map[string]any{"taskId": saved.ID, "open": true}, &run); code != 201 || !ft.open || ft.started[0].About != "kickoff" {
		t.Fatalf("start saved = %d %+v", code, ft)
	}
	if code := e.do("POST", "/api/chats/"+key+"/task", map[string]any{}, &run); code != 201 || ft.started[1].Template != model.TaskFindTime {
		t.Fatalf("start template = %d", code)
	}
	ft.err = task.ErrRunning
	if code := e.do("POST", "/api/chats/"+key+"/task", map[string]any{}, &apiErr); code != 409 || apiErr.Code != "task_running" {
		t.Fatalf("running = %d %+v", code, apiErr)
	}
	ft.err = nil
	if code := e.do("POST", "/api/tasks/runs/r1/approve", map[string]any{}, &apiErr); code != 400 {
		t.Fatalf("approve without yes/no = %d", code)
	}
	if code := e.do("POST", "/api/tasks/runs/r1/approve", map[string]any{"yes": true}, &run); code != 200 || ft.yes == nil || !*ft.yes {
		t.Fatalf("approve = %d", code)
	}
	if code := e.do("DELETE", "/api/tasks/"+saved.ID, nil, nil); code != 200 {
		t.Fatalf("delete = %d", code)
	}

	// The calendar file.
	resp, err := http.Get(e.base + "/api/tasks/runs/r1/event.ics")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/calendar") || !strings.Contains(string(b), "VCALENDAR") {
		t.Fatalf("ics = %s %q", resp.Header.Get("Content-Type"), b)
	}

	// Without the engine: 503.
	e.s.d.Tasks = nil
	if code := e.do("POST", "/api/chats/"+key+"/task", map[string]any{}, &apiErr); code != 503 {
		t.Fatalf("no engine = %d", code)
	}
}
