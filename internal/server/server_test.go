package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/store"
)

type testEnv struct {
	t      *testing.T
	s      *Server
	base   string
	wa     *fakeWA
	eng    *fakeEngine
	llm    *fakeLLM
	hub    *events.Hub
	cfg    *config.Manager
	st     *store.Store
	quitMu sync.Mutex
	quit   bool
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	seeds := []model.Persona{
		{ID: "leo", BuiltIn: true, Name: "Leo", Tagline: "seed tagline", Avatar: model.Avatar{Kind: "generated"}},
		{ID: "kyle", BuiltIn: true, Name: "Kyle", Avatar: model.Avatar{Kind: "generated"}},
	}
	st, err := store.Open(paths, seeds)
	if err != nil {
		t.Fatal(err)
	}
	e := &testEnv{t: t, wa: newFakeWA(), eng: &fakeEngine{}, llm: &fakeLLM{}, hub: events.NewHub(), cfg: cfg, st: st}
	e.s = New(Deps{
		Config: cfg, Store: st, Hub: e.hub, WA: e.wa, Engine: e.eng, LLM: e.llm,
		Seed: func(id string) (model.Persona, bool) {
			for _, p := range seeds {
				if p.ID == id {
					return p, true
				}
			}
			return model.Persona{}, false
		},
		Web: fstest.MapFS{
			"index.html":    {Data: []byte(`<meta name="doppel-token" content="__DOPPEL_TOKEN__">`)},
			"app.js":        {Data: []byte(`console.log(1)`)},
			"styles/a.css":  {Data: []byte(`body{}`)},
			"embed.go":      {Data: []byte(`package web`)},
			"assets/i.svg":  {Data: []byte(`<svg/>`)},
			"pages/chat.js": {Data: []byte(`export {}`)},
		},
		Version: "test",
		FakeWA:  true,
		OnQuit: func() {
			e.quitMu.Lock()
			e.quit = true
			e.quitMu.Unlock()
		},
		Logf: t.Logf,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- e.s.Serve(ln) }()
	e.base = fmt.Sprintf("http://127.0.0.1:%d", portOf(ln.Addr()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := e.s.Shutdown(ctx); err != nil {
			t.Errorf("shutdown: %v", err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Serve returned %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Serve did not return after Shutdown")
		}
	})
	return e
}

// do sends a request (with the token unless noToken) and decodes JSON into out.
func (e *testEnv) do(method, path string, body any, out any, opts ...func(*http.Request)) int {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.base+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Doppel-Token", e.s.Token())
	for _, o := range opts {
		o(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			e.t.Fatalf("%s %s: decode %q: %v", method, path, data, err)
		}
	}
	return resp.StatusCode
}

func noToken(r *http.Request) { r.Header.Del("X-Doppel-Token") }

func TestHealthAndIndexToken(t *testing.T) {
	e := newEnv(t)
	var h map[string]any
	if code := e.do("GET", "/api/health", nil, &h, noToken); code != 200 {
		t.Fatalf("health = %d", code)
	}
	if h["token"] != e.s.Token() || h["version"] != "test" || h["fakeWA"] != true {
		t.Fatalf("health = %v", h)
	}
	resp, err := http.Get(e.base + "/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), e.s.Token()) || strings.Contains(string(b), "__DOPPEL_TOKEN__") {
		t.Fatalf("index not templated: %s", b)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("index Cache-Control = %q", resp.Header.Get("Cache-Control"))
	}
	for path, want := range map[string]string{"/app.js": "text/javascript", "/styles/a.css": "text/css", "/assets/i.svg": "image/svg+xml"} {
		resp, err := http.Get(e.base + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), want) {
			t.Errorf("%s: %d %q", path, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	}
	resp, _ = http.Get(e.base + "/embed.go")
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("embed.go served: %d", resp.StatusCode)
	}
	var apiErr apiError
	if code := e.do("GET", "/api/nope", nil, &apiErr); code != 404 || apiErr.Code != "not_found" {
		t.Errorf("unknown api = %d %+v", code, apiErr)
	}
}

func TestTokenRequired(t *testing.T) {
	e := newEnv(t)
	var apiErr apiError
	if code := e.do("PUT", "/api/settings", map[string]any{"theme": "dark"}, &apiErr, noToken); code != 401 || apiErr.Code != "bad_token" {
		t.Fatalf("no token: %d %+v", code, apiErr)
	}
	if code := e.do("PUT", "/api/settings", map[string]any{"theme": "dark"}, &apiErr,
		func(r *http.Request) { r.Header.Set("X-Doppel-Token", "wrong") }); code != 401 {
		t.Fatalf("wrong token: %d", code)
	}
	if code := e.do("PUT", "/api/settings", map[string]any{"theme": "dark"}, nil); code != 200 {
		t.Fatalf("with token: %d", code)
	}
	// JSON bodies must be declared as JSON.
	if code := e.do("PUT", "/api/settings", map[string]any{"theme": "dark"}, &apiErr,
		func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }); code != 415 {
		t.Fatalf("text/plain body: %d", code)
	}
	// GETs need no token.
	if code := e.do("GET", "/api/settings", nil, nil, noToken); code != 200 {
		t.Fatalf("GET settings without token: %d", code)
	}
}

func TestHostAndOriginGuard(t *testing.T) {
	e := newEnv(t)
	port := strings.TrimPrefix(e.base, "http://127.0.0.1:")
	cases := []struct {
		name string
		mod  func(*http.Request)
		want int
	}{
		{"evil host", func(r *http.Request) { r.Host = "evil.example:" + port }, 403},
		{"wrong port", func(r *http.Request) { r.Host = "127.0.0.1:1" }, 403},
		{"localhost ok", func(r *http.Request) { r.Host = "localhost:" + port }, 200},
		{"bad origin", func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") }, 403},
		{"good origin", func(r *http.Request) { r.Header.Set("Origin", "http://127.0.0.1:"+port) }, 200},
		{"cross-site", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }, 403},
		{"same-origin", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-origin") }, 200},
	}
	for _, c := range cases {
		if code := e.do("GET", "/api/settings", nil, nil, c.mod); code != c.want {
			t.Errorf("%s: got %d want %d", c.name, code, c.want)
		}
	}
	// A cross-site top-level navigation to the UI is fine (e.g. a link in another app).
	req, _ := http.NewRequest("GET", e.base+"/", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("cross-site navigation: %d", resp.StatusCode)
	}
}

func TestSettingsDeepMerge(t *testing.T) {
	e := newEnv(t)
	before := e.cfg.Get()
	ch, _, cancel := e.hub.Subscribe(0)
	defer cancel()

	var got settingsView
	code := e.do("PUT", "/api/settings", map[string]any{
		"behavior": map[string]any{"private": map[string]any{"waitForMoreSec": 3}},
		"llm":      map[string]any{"temperature": 0.3},
		"port":     1234, // ignored: ports change via /api/system/port
		"secrets":  map[string]any{"anthropic": map[string]any{"set": true}},
	}, &got)
	if code != 200 {
		t.Fatalf("PUT settings = %d", code)
	}
	if got.Behavior.Private.WaitForMoreSec != 3 || got.LLM.Temperature != 0.3 || got.Behavior.Private.Preset != "custom" {
		t.Fatalf("patch not applied: %+v", got.Settings)
	}
	if got.Behavior.Private.BurstCapSec != before.Behavior.Private.BurstCapSec ||
		got.Behavior.TriggerPrefix != before.Behavior.TriggerPrefix ||
		got.Behavior.Group.WaitForMoreSec != before.Behavior.Group.WaitForMoreSec ||
		got.LLM.OllamaURL != before.LLM.OllamaURL || got.Theme != before.Theme {
		t.Fatalf("siblings clobbered: %+v", got.Settings)
	}
	if got.Port != before.Port {
		t.Fatalf("port changed via PUT: %d", got.Port)
	}
	if got.Secrets == nil || got.Secrets["anthropic"].Set {
		t.Fatalf("secrets view = %+v", got.Secrets)
	}
	if e.cfg.Get().Behavior.Private.WaitForMoreSec != 3 {
		t.Fatal("not persisted in manager")
	}
	if e.eng.reloads.Load() == 0 || e.llm.rebuilds.Load() == 0 {
		t.Fatal("engine/llm not reloaded")
	}
	select {
	case ev := <-ch:
		if ev.Type != events.TypeSettingsChanged {
			t.Fatalf("event = %s", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("no settings.changed event")
	}

	var apiErr apiError
	if code := e.do("PUT", "/api/settings", map[string]any{"theme": "neon"}, &apiErr); code != 400 || apiErr.Code != "invalid_settings" {
		t.Fatalf("invalid theme: %d %+v", code, apiErr)
	}
	if e.cfg.Get().Theme != "system" {
		t.Fatal("invalid theme persisted")
	}
	// defaultModel alone updates the default provider's model.
	if code := e.do("PUT", "/api/settings", map[string]any{"llm": map[string]any{"defaultModel": "qwen3:8b"}}, &got); code != 200 {
		t.Fatalf("defaultModel: %d", code)
	}
	if got.LLM.OllamaModel != "qwen3:8b" || got.LLM.DefaultModel != "qwen3:8b" {
		t.Fatalf("defaultModel not mapped: %+v", got.LLM)
	}
}

func TestMergeSettingsUnit(t *testing.T) {
	cur := config.Defaults()
	next, err := MergeSettings(cur, []byte(`{"behavior":{"group":{"autoSendSeconds":30,"ignoreLinks":false}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if next.Behavior.Group.AutoSendSeconds != 30 || next.Behavior.Group.IgnoreLinks || !next.Behavior.Group.TypingIndicator ||
		next.Behavior.Private.AutoSendSeconds != 0 {
		t.Fatalf("merge = %+v", next.Behavior)
	}
	if _, err := MergeSettings(cur, []byte(`{"behavior":{"group":{"chimeInPercent":150}}}`)); err == nil ||
		!strings.Contains(err.Error(), "behavior.group.chimeInPercent") {
		t.Fatalf("expected range error, got %v", err)
	}
}

func TestSecretsOmittedVsEmpty(t *testing.T) {
	e := newEnv(t)
	var view map[string]config.SecretView
	e.do("PUT", "/api/settings/secrets", map[string]any{"anthropicKey": "sk-ant-123456", "openaiKey": "sk-oa-abcdef"}, &view)
	if !view["anthropic"].Set || view["anthropic"].Hint != "…3456" || !view["openai"].Set {
		t.Fatalf("set: %+v", view)
	}
	e.do("PUT", "/api/settings/secrets", map[string]any{"openaiKey": ""}, &view)
	if !view["anthropic"].Set || view["openai"].Set {
		t.Fatalf("clear openai only: %+v", view)
	}
	if e.cfg.Secrets().AnthropicKey != "sk-ant-123456" {
		t.Fatal("anthropic key lost")
	}
}

func TestPortScan(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	busy := portOf(ln.Addr())
	if PortFree(busy) {
		t.Fatalf("PortFree(%d) on a bound port", busy)
	}
	free := FreePorts(busy, busy+50, 5, 0)
	for _, p := range free {
		if p == busy {
			t.Fatalf("busy port %d reported free", busy)
		}
	}
	if len(free) == 0 {
		t.Fatal("no free ports found")
	}
	if got := FreePorts(1024, 65535, 1000, 0); len(got) > MaxPortProbes {
		t.Fatalf("probe cap exceeded: %d", len(got))
	}

	e := newEnv(t)
	var res struct {
		Current int   `json:"current"`
		Free    []int `json:"free"`
	}
	if code := e.do("GET", "/api/system/ports?from=20000&to=20100&limit=3", nil, &res); code != 200 || len(res.Free) == 0 || len(res.Free) > 3 {
		t.Fatalf("ports = %d %+v", code, res)
	}
	if res.Current != e.s.Port() {
		t.Fatalf("current = %d want %d", res.Current, e.s.Port())
	}
}

func readSSEEvent(t *testing.T, rd *bufio.Reader) (id, typ, data string) {
	t.Helper()
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case line == "":
			if typ != "" {
				return
			}
		case strings.HasPrefix(line, "id: "):
			id = line[4:]
		case strings.HasPrefix(line, "event: "):
			typ = line[7:]
		case strings.HasPrefix(line, "data: "):
			data += line[6:]
		}
	}
}

func TestSSEInitialStatusReplayAndShutdown(t *testing.T) {
	e := newEnv(t)
	e.hub.Publish(events.TypeChatsChanged, struct{}{}) // id 1
	e.hub.Publish(events.TypePersonasChanged, struct{}{})

	req, _ := http.NewRequest("GET", e.base+"/api/events", nil)
	req.Header.Set("Last-Event-ID", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type %q", ct)
	}
	rd := bufio.NewReader(resp.Body)
	id, typ, data := readSSEEvent(t, rd)
	if typ != events.TypeWAStatus || id != "" {
		t.Fatalf("first event = %q id=%q", typ, id)
	}
	var st model.WAStatus
	if err := json.Unmarshal([]byte(data), &st); err != nil || st.State != model.WAConnected {
		t.Fatalf("status data %q: %v", data, err)
	}
	if id, typ, _ = readSSEEvent(t, rd); typ != events.TypePersonasChanged || id != "2" {
		t.Fatalf("replay = %q id=%q", typ, id)
	}
	e.hub.Activity(model.ActivityEvent{Type: model.ActSent, Text: "yo"})
	if _, typ, data = readSSEEvent(t, rd); typ != events.TypeActivity || !strings.Contains(data, `"yo"`) {
		t.Fatalf("live = %q %s", typ, data)
	}

	// Shutdown must not hang on the open stream.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	if err := e.s.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown with open SSE: %v", err)
	}
	if time.Since(start) > 1500*time.Millisecond {
		t.Fatalf("shutdown took %s", time.Since(start))
	}
}

func TestChatsAssignFlow(t *testing.T) {
	e := newEnv(t)
	var apiErr apiError
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "nobody"}, &apiErr); code != 400 || apiErr.Code != "unknown_persona" {
		t.Fatalf("unknown persona: %d %+v", code, apiErr)
	}
	var c model.ChatAssignment
	// Assign via the lid address; the server must canonicalize through ResolveChat.
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "999001@lid", "personaId": "leo", "approvalMode": true}, &c); code != 201 {
		t.Fatalf("assign = %d", code)
	}
	if c.Key != "dm:15550001" || c.JID != "15550001@s.whatsapp.net" || c.AltJID != "999001@lid" || !c.Enabled || !c.ApprovalMode || c.Name != "Dana" {
		t.Fatalf("assignment = %+v", c)
	}
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "kyle"}, &apiErr); code != 409 || apiErr.Code != "already_assigned" {
		t.Fatalf("duplicate = %d %+v", code, apiErr)
	}
	if e.eng.reloads.Load() == 0 {
		t.Fatal("engine not reloaded")
	}

	var list []chatView
	e.do("GET", "/api/chats", nil, &list)
	if len(list) != 1 || list[0].PersonaName != "Leo" || list[0].HistoryCount != 0 {
		t.Fatalf("list = %+v", list)
	}

	var wl struct {
		Items []model.ChatItem `json:"items"`
		Total int              `json:"total"`
	}
	e.do("GET", "/api/wa/chats?tab=recent", nil, &wl)
	assigned := 0
	for _, it := range wl.Items {
		if it.Assigned {
			assigned++
			if it.Key != "dm:15550001" {
				t.Errorf("wrong item flagged: %+v", it)
			}
		}
	}
	if assigned != 1 || wl.Total != 4 {
		t.Fatalf("recent: assigned=%d total=%d", assigned, wl.Total)
	}
	e.do("GET", "/api/wa/chats?tab=assigned&q=dan", nil, &wl)
	if wl.Total != 1 || !wl.Items[0].Assigned || wl.Items[0].Phone != "15550001" {
		t.Fatalf("assigned tab = %+v", wl)
	}
	e.do("GET", "/api/wa/chats?tab=assigned&q=zzz", nil, &wl)
	if wl.Total != 0 || len(wl.Items) != 0 {
		t.Fatalf("assigned filter = %+v", wl)
	}

	key := url.PathEscape("dm:15550001")
	key = strings.ReplaceAll(key, ":", "%3A")
	if code := e.do("PATCH", "/api/chats/"+key, map[string]any{
		"personaId": "kyle", "goalOverride": " plan dinner ",
		"behavior": map[string]any{"waitForMoreSec": 4},
	}, &c); code != 200 {
		t.Fatalf("patch = %d", code)
	}
	if c.PersonaID != "kyle" || c.GoalOverride != "plan dinner" || c.Behavior.WaitForMoreSec == nil || *c.Behavior.WaitForMoreSec != 4 || !c.ApprovalMode {
		t.Fatalf("patched = %+v", c)
	}

	if code := e.do("POST", "/api/chats/"+key+"/simulate", map[string]any{"text": "hi"}, nil); code != 200 {
		t.Fatalf("simulate = %d", code)
	}
	_ = e.st.UpsertApproval(model.PendingReply{ID: "p1", ChatKey: "dm:15550001", Text: "x"})
	e.do("GET", "/api/chats", nil, &list)
	if list[0].PendingCount != 1 {
		t.Fatalf("pendingCount = %d", list[0].PendingCount)
	}

	// Deleting a persona disables its chats.
	if code := e.do("DELETE", "/api/personas/kyle", nil, nil); code != 200 {
		t.Fatalf("delete persona = %d", code)
	}
	got, _ := e.st.Chat("dm:15550001")
	if got.Enabled {
		t.Fatal("chat still enabled after its persona was deleted")
	}

	if code := e.do("DELETE", "/api/chats/"+key, nil, nil); code != 200 {
		t.Fatalf("delete = %d", code)
	}
	if code := e.do("DELETE", "/api/chats/"+key, nil, &apiErr); code != 404 {
		t.Fatalf("delete again = %d", code)
	}
	e.eng.mu.Lock()
	discarded := len(e.eng.discarded)
	e.eng.mu.Unlock()
	if discarded != 1 {
		t.Fatalf("pending approvals not discarded: %d", discarded)
	}
}

func TestSimulateGuard(t *testing.T) {
	e := newEnv(t)
	e.s.d.FakeWA = false
	var c model.ChatAssignment
	e.do("POST", "/api/chats", map[string]any{"jid": "15550002@s.whatsapp.net", "personaId": "leo"}, &c)
	var apiErr apiError
	if code := e.do("POST", "/api/chats/dm%3A15550002/simulate", map[string]any{"text": "hi"}, &apiErr); code != 403 {
		t.Fatalf("simulate on a real contact = %d", code)
	}
	e.do("POST", "/api/chats", map[string]any{"jid": "15550000@s.whatsapp.net", "personaId": "leo"}, &c)
	if code := e.do("POST", "/api/chats/dm%3A15550000/simulate", map[string]any{"text": "hi"}, nil); code != 200 {
		t.Fatalf("simulate on self chat = %d", code)
	}
}

func TestPersonasCRUDAndAvatar(t *testing.T) {
	e := newEnv(t)
	var p model.Persona
	if code := e.do("POST", "/api/personas", map[string]any{"id": "hack", "builtIn": true, "name": "Zed Quinn"}, &p); code != 201 {
		t.Fatalf("create = %d", code)
	}
	if p.ID == "hack" || p.BuiltIn || p.Avatar.Kind != "generated" {
		t.Fatalf("created = %+v", p)
	}
	var apiErr apiError
	if code := e.do("POST", "/api/personas", map[string]any{"name": "  "}, &apiErr); code != 400 {
		t.Fatalf("nameless = %d", code)
	}

	// Avatar upload: a 300×200 PNG becomes a 512×512 PNG.
	img := image.NewRGBA(image.Rect(0, 0, 300, 200))
	for y := range 200 {
		for x := range 300 {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var pngBuf bytes.Buffer
	png.Encode(&pngBuf, img)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "a.png")
	fw.Write(pngBuf.Bytes())
	mw.Close()
	req, _ := http.NewRequest("PUT", e.base+"/api/personas/"+p.ID+"/avatar", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Doppel-Token", e.s.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	json.NewDecoder(resp.Body).Decode(&p)
	resp.Body.Close()
	if resp.StatusCode != 200 || p.Avatar.Kind != "upload" || p.Avatar.Version == 0 {
		t.Fatalf("upload = %d %+v", resp.StatusCode, p.Avatar)
	}
	resp, err = http.Get(fmt.Sprintf("%s/api/personas/%s/avatar?v=%d", e.base, p.ID, p.Avatar.Version))
	if err != nil {
		t.Fatal(err)
	}
	out, err := png.Decode(resp.Body)
	resp.Body.Close()
	if err != nil || out.Bounds().Dx() != 512 || out.Bounds().Dy() != 512 {
		t.Fatalf("avatar: %v %v", err, out)
	}

	// Duplicate copies the picture.
	var dup model.Persona
	if code := e.do("POST", "/api/personas/"+p.ID+"/duplicate", nil, &dup); code != 201 {
		t.Fatalf("duplicate = %d", code)
	}
	if dup.ID == p.ID || dup.Name != "Zed Quinn copy" || dup.Avatar.Kind != "upload" {
		t.Fatalf("dup = %+v", dup)
	}

	// PUT keeps builtIn/id authoritative.
	p.Tagline = "new"
	p.BuiltIn = true
	if code := e.do("PUT", "/api/personas/"+p.ID, p, &p); code != 200 || p.BuiltIn || p.Tagline != "new" {
		t.Fatalf("put = %d %+v", code, p)
	}

	if code := e.do("DELETE", "/api/personas/"+p.ID+"/avatar", nil, &p); code != 200 || p.Avatar.Kind != "generated" {
		t.Fatalf("delete avatar = %d %+v", code, p.Avatar)
	}

	// Reset: built-ins only.
	if code := e.do("POST", "/api/personas/"+p.ID+"/reset", nil, &apiErr); code != 400 {
		t.Fatalf("reset custom = %d", code)
	}
	var leo model.Persona
	e.do("PUT", "/api/personas/leo", map[string]any{"name": "Leo", "tagline": "edited"}, &leo)
	if code := e.do("POST", "/api/personas/leo/reset", nil, &leo); code != 200 || leo.Tagline != "seed tagline" || !leo.BuiltIn {
		t.Fatalf("reset = %d %+v", code, leo)
	}
	var prev map[string]string
	if code := e.do("GET", "/api/personas/leo/prompt-preview", nil, &prev); code != 200 || prev["system"] == "" {
		t.Fatalf("preview = %d %v", code, prev)
	}
}

func TestProcessAvatarRejectsGarbage(t *testing.T) {
	if _, err := ProcessAvatar([]byte("not an image"), 512); err == nil {
		t.Fatal("expected error")
	}
	img := image.NewRGBA(image.Rect(0, 0, 2000, 1000))
	var buf bytes.Buffer
	png.Encode(&buf, img)
	out, err := ProcessAvatar(buf.Bytes(), 512)
	if err != nil {
		t.Fatal(err)
	}
	dec, _ := png.Decode(bytes.NewReader(out))
	if dec.Bounds().Dx() != 512 {
		t.Fatalf("size %v", dec.Bounds())
	}
}

func TestRebindAndQuit(t *testing.T) {
	old := rebindGrace
	rebindGrace = 200 * time.Millisecond
	defer func() { rebindGrace = old }()

	e := newEnv(t)
	var changed int
	e.s.d.OnPortChange = func(p int) { changed = p }
	newPort := FreePorts(20000, 30000, 1, 0)[0]

	var apiErr apiError
	if code := e.do("POST", "/api/system/port", map[string]any{"port": e.s.Port()}, &apiErr); code != 400 || apiErr.Code != "invalid_port" {
		t.Fatalf("same port = %d %+v", code, apiErr)
	}
	var res map[string]string
	if code := e.do("POST", "/api/system/port", map[string]any{"port": newPort}, &res); code != 200 {
		t.Fatalf("rebind = %d", code)
	}
	want := fmt.Sprintf("http://127.0.0.1:%d/", newPort)
	if res["url"] != want || changed != newPort || e.s.Port() != newPort || e.cfg.Get().Port != newPort {
		t.Fatalf("rebind res=%v changed=%d port=%d cfg=%d", res, changed, e.s.Port(), e.cfg.Get().Port)
	}
	resp, err := http.Get(want + "api/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("new port health = %d", resp.StatusCode)
	}
	time.Sleep(600 * time.Millisecond)
	if _, err := http.Get(e.base + "/api/health"); err == nil {
		t.Fatal("old port still serving after grace period")
	}

	e.base = strings.TrimSuffix(want, "/")
	if code := e.do("POST", "/api/system/quit", nil, nil); code != 200 {
		t.Fatalf("quit = %d", code)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		e.quitMu.Lock()
		q := e.quit
		e.quitMu.Unlock()
		if q {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("OnQuit not called")
}

func TestPlaygroundAndApprovalsUnavailable(t *testing.T) {
	e := newEnv(t)
	var apiErr apiError
	if code := e.do("POST", "/api/playground", map[string]any{"personaId": "leo"}, &apiErr); code != 503 || apiErr.Code != "unavailable" {
		t.Fatalf("playground without dep = %d %+v", code, apiErr)
	}
	if code := e.do("POST", "/api/approvals/nope/approve", nil, &apiErr); code != 404 {
		t.Fatalf("approve unknown = %d", code)
	}
	var act struct {
		Items  []model.ActivityEvent `json:"items"`
		LastID int64                 `json:"lastId"`
	}
	e.hub.Activity(model.ActivityEvent{Type: model.ActSent, ChatKey: "dm:1", Text: "a"})
	e.hub.Activity(model.ActivityEvent{Type: model.ActError, ChatKey: "dm:2", Text: "b"})
	e.do("GET", "/api/activity?types=error", nil, &act)
	if len(act.Items) != 1 || act.LastID != 2 {
		t.Fatalf("activity = %+v", act)
	}
	e.do("DELETE", "/api/activity", nil, nil)
	e.do("GET", "/api/activity", nil, &act)
	if len(act.Items) != 0 {
		t.Fatalf("activity after clear = %+v", act)
	}
}
