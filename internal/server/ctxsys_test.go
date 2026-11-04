package server

import (
	"strings"
	"testing"

	"whatsappdoppel/internal/model"
)

func TestContextSystemsAPI(t *testing.T) {
	e := newEnv(t)
	var list struct {
		Items []struct {
			model.ContextSystem
			Estimate struct {
				PerAnswer float64 `json:"perAnswer"`
			} `json:"estimate"`
			Chats int `json:"chats"`
		} `json:"items"`
		AppDefault string `json:"appDefault"`
	}
	if code := e.do("GET", "/api/context/systems", nil, &list); code != 200 || len(list.Items) != 3 || list.AppDefault != "classic" {
		t.Fatalf("list = %d %+v", code, list)
	}
	if list.Items[0].Estimate.PerAnswer != 4 {
		t.Errorf("classic estimate = %v", list.Items[0].Estimate.PerAnswer)
	}
	var apiErr apiError
	if code := e.do("PUT", "/api/context/systems/lean", map[string]any{"name": "x"}, &apiErr); code != 403 || apiErr.Code != "built_in" {
		t.Errorf("edit built-in = %d %+v", code, apiErr)
	}
	var mine model.ContextSystem
	if code := e.do("POST", "/api/context/systems", map[string]any{"copyFrom": "lean"}, &mine); code != 201 || mine.Name != "Lean (copy)" || mine.Base != "lean" || mine.BuiltIn || len(mine.Chart.Nodes) == 0 {
		t.Fatalf("copy = %d %+v", code, mine)
	}
	mine.Params.PlanEvery = 9
	mine.Chart.Nodes = append(mine.Chart.Nodes, model.ChartNode{ID: "x1", Block: "custom_step", Label: "Ask my mum", X: 10, Y: 10})
	var saved model.ContextSystem
	if code := e.do("PUT", "/api/context/systems/"+mine.ID, mine, &saved); code != 200 || saved.Params.PlanEvery != 9 || len(saved.Chart.Nodes) != len(mine.Chart.Nodes) {
		t.Fatalf("save = %d %+v", code, saved.Params)
	}

	// Chat → system, and the instance view.
	var c model.ChatAssignment
	if code := e.do("POST", "/api/chats", map[string]any{"jid": "15550001@s.whatsapp.net", "personaId": "leo"}, &c); code != 201 {
		t.Fatalf("assign = %d", code)
	}
	key := "/api/chats/" + strings.ReplaceAll(c.Key, ":", "%3A")
	if code := e.do("PATCH", key, map[string]any{"contextSystem": "nope"}, &apiErr); code != 400 || apiErr.Code != "unknown_system" {
		t.Errorf("unknown system = %d %+v", code, apiErr)
	}
	if code := e.do("PUT", "/api/personas/leo/context", map[string]any{"systemId": "fast"}, nil); code != 200 {
		t.Fatalf("persona system = %d", code)
	}
	var inst struct {
		Items []struct {
			ChatKey string              `json:"chatKey"`
			Choice  model.ContextChoice `json:"choice"`
		} `json:"items"`
	}
	e.do("GET", "/api/context/instances", nil, &inst)
	if len(inst.Items) != 1 || inst.Items[0].Choice != (model.ContextChoice{SystemID: "fast", Source: "persona"}) {
		t.Errorf("instances (persona) = %+v", inst)
	}
	if code := e.do("PATCH", key, map[string]any{"contextSystem": mine.ID}, nil); code != 200 {
		t.Fatalf("chat system = %d", code)
	}
	e.do("GET", "/api/context/instances", nil, &inst)
	if inst.Items[0].Choice != (model.ContextChoice{SystemID: mine.ID, Source: "chat"}) {
		t.Errorf("instances (chat) = %+v", inst)
	}
	// Deleting it sends the chat back to the persona's.
	if code := e.do("DELETE", "/api/context/systems/"+mine.ID, nil, nil); code != 204 {
		t.Fatalf("delete = %d", code)
	}
	e.do("GET", "/api/context/instances", nil, &inst)
	if inst.Items[0].Choice.Source != "persona" {
		t.Errorf("after delete = %+v", inst)
	}

	// Chart → knobs.
	var inf struct {
		Params  model.ContextParams `json:"params"`
		Changes []string            `json:"changes"`
	}
	chart := model.ContextChart{Nodes: []model.ChartNode{{ID: "a", Block: "join_ai"}, {ID: "b", Block: "join_rules"}, {ID: "c", Block: "digest"}}}
	if code := e.do("POST", "/api/context/infer", map[string]any{"chart": chart, "base": "classic", "params": model.ContextParams{}}, &inf); code != 200 ||
		inf.Params.JoinIn != model.JoinRulesFirst || inf.Params.Background != model.BackgroundDigest || len(inf.Changes) == 0 {
		t.Errorf("infer = %d %+v", code, inf)
	}
	var catalog struct {
		Blocks []model.ChartBlock `json:"blocks"`
		Params []map[string]any   `json:"params"`
	}
	if code := e.do("GET", "/api/context/catalog", nil, &catalog); code != 200 || len(catalog.Blocks) < 30 || len(catalog.Params) < 15 {
		t.Errorf("catalog = %d %d blocks %d params", code, len(catalog.Blocks), len(catalog.Params))
	}
}
