package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

// Wave 3, Engineer B: hand-off resume, reveal, recaps, notify-test.

type fakeNotifier struct {
	backend string
	err     error
	tests   int
}

func (n *fakeNotifier) Backend() string     { return n.backend }
func (n *fakeNotifier) EventsEnabled() bool { return false }
func (n *fakeNotifier) Test(context.Context) error {
	n.tests++
	return n.err
}

func assignDana(t *testing.T, e *testEnv) (model.ChatAssignment, string) {
	t.Helper()
	var c model.ChatAssignment
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "leo"}, &c); code != 201 {
		t.Fatalf("assign = %d", code)
	}
	return c, strings.ReplaceAll(c.Key, ":", "%3A")
}

func TestHandoffResumeEndpoint(t *testing.T) {
	e := newEnv(t)
	c, key := assignDana(t, e)
	if _, err := e.st.UpdateChat(c.Key, func(x *model.ChatAssignment) {
		x.Handoff = &model.HandoffState{Category: model.HandoffBot, Excerpt: "are you a bot?", At: time.Now()}
	}); err != nil {
		t.Fatal(err)
	}
	var list []chatView
	e.do("GET", "/api/chats", nil, &list)
	if len(list) != 1 || list[0].Handoff == nil || list[0].Handoff.Category != model.HandoffBot {
		t.Fatalf("chat list shows the hand-off: %+v", list)
	}
	var got model.ChatAssignment
	if code := e.do("POST", "/api/chats/"+key+"/handoff/resume", nil, &got); code != 200 || got.Key != c.Key {
		t.Fatalf("resume = %d %+v", code, got)
	}
	if strings.Join(e.eng.wave3Calls, ",") != "resume|"+c.Key {
		t.Errorf("calls = %v", e.eng.wave3Calls)
	}
	if code := e.do("POST", "/api/chats/dm%3Anope/handoff/resume", nil, nil); code != 404 {
		t.Errorf("unknown = %d", code)
	}
	e.eng.wave3Err = errors.New("boom")
	var apiErr apiError
	if code := e.do("POST", "/api/chats/"+key+"/handoff/resume", nil, &apiErr); code != 500 || apiErr.Code != "resume_failed" {
		t.Errorf("failure = %d %+v", code, apiErr)
	}
}

func TestRevealEndpoint(t *testing.T) {
	e := newEnv(t)
	c, key := assignDana(t, e)
	var res model.RevealResult
	if code := e.do("POST", "/api/chats/"+key+"/reveal", map[string]any{"text": "it was me all along"}, &res); code != 200 || !res.OK || res.Text != "it was me all along" {
		t.Fatalf("reveal = %d %+v", code, res)
	}
	// Already revealed (and still paused): 409 unless force.
	now := time.Now()
	if _, err := e.st.UpdateChat(c.Key, func(x *model.ChatAssignment) { x.RevealedAt, x.Enabled = &now, false }); err != nil {
		t.Fatal(err)
	}
	var apiErr apiError
	if code := e.do("POST", "/api/chats/"+key+"/reveal", map[string]any{}, &apiErr); code != 409 || apiErr.Code != "already_revealed" {
		t.Errorf("again = %d %+v", code, apiErr)
	}
	if code := e.do("POST", "/api/chats/"+key+"/reveal", map[string]any{"force": true}, &res); code != 200 || res.Text != "it was a persona" {
		t.Errorf("force = %d %+v", code, res)
	}
	if code := e.do("POST", "/api/chats/"+key+"/reveal", map[string]any{"text": strings.Repeat("x", 2001), "force": true}, &apiErr); code != 400 || apiErr.Code != "text_too_long" {
		t.Errorf("too long = %d %+v", code, apiErr)
	}
	want := "reveal|" + c.Key + "|it was me all along|false,reveal|" + c.Key + "||true"
	if got := strings.Join(e.eng.wave3Calls, ","); got != want {
		t.Errorf("calls = %s", got)
	}
	e.eng.wave3Err = errors.New("WhatsApp is offline")
	if code := e.do("POST", "/api/chats/"+key+"/reveal", map[string]any{"force": true}, &apiErr); code != 502 || apiErr.Code != "reveal_failed" {
		t.Errorf("send failure = %d %+v", code, apiErr)
	}
}

func TestRecapsEndpoints(t *testing.T) {
	e := newEnv(t)
	c, key := assignDana(t, e)
	var rv model.RecapsView
	if code := e.do("POST", "/api/recaps/generate", map[string]any{"chatKey": c.Key}, &rv); code != 200 || len(rv.Items) != 1 || rv.Items[0].Headline != "A quiet day" {
		t.Fatalf("generate = %d %+v", code, rv)
	}
	if code := e.do("POST", "/api/recaps/generate", nil, &rv); code != 200 {
		t.Errorf("generate all = %d", code)
	}
	if code := e.do("POST", "/api/recaps/generate", map[string]any{"chatKey": "dm:nope"}, nil); code != 404 {
		t.Errorf("unknown chat = %d", code)
	}
	if err := e.st.AppendRecap(model.Recap{ChatKey: c.Key, Date: "2026-10-01", Headline: "stored", GeneratedAt: time.Now()}, 30); err != nil {
		t.Fatal(err)
	}
	if code := e.do("GET", "/api/recaps?chat="+key+"&limit=5", nil, &rv); code != 200 || len(rv.Items) != 1 || rv.Items[0].Headline != "stored" {
		t.Errorf("list = %d %+v", code, rv)
	}
	if got := strings.Join(e.eng.wave3Calls, ","); got != "recap|"+c.Key+",recap|" {
		t.Errorf("calls = %s", got)
	}
}

func TestNotifyTestEndpoint(t *testing.T) {
	e := newEnv(t)
	n := &fakeNotifier{backend: "osascript"}
	e.s.d.Notifier = n
	var out map[string]any
	if code := e.do("POST", "/api/system/notify-test?dry=1", nil, &out); code != 200 || out["backend"] != "osascript" || out["events"] != false || n.tests != 0 {
		t.Fatalf("dry = %d %+v (tests %d)", code, out, n.tests)
	}
	if code := e.do("POST", "/api/system/notify-test", nil, &out); code != 200 || out["ok"] != true || n.tests != 1 {
		t.Fatalf("test = %d %+v", code, out)
	}
	n.err = errors.New("osascript failed")
	var apiErr apiError
	if code := e.do("POST", "/api/system/notify-test", nil, &apiErr); code != 502 || apiErr.Code != "notify_failed" {
		t.Errorf("failure = %d %+v", code, apiErr)
	}
}
