package server

import (
	"testing"

	"whatsappdoppel/internal/model"
)

func TestExpressionPreview(t *testing.T) {
	e := newEnv(t)
	var out model.ExpressionPreview
	if code := e.do("POST", "/api/personas/leo/expression-preview", map[string]any{}, &out); code != 200 || len(out.Samples) != 3 {
		t.Fatalf("saved persona = %d %+v", code, out)
	}
	if got := out.Samples[0].Reply; got != "Leo   " { // the test seed has no emoji/length fields
		t.Errorf("saved persona not used: %q", got)
	}

	// Unsaved editor fields win over the stored persona; count and roll pass through.
	body := map[string]any{
		"persona":    map[string]any{"name": "Leo", "emoji": map[string]any{"usage": "lots", "favorites": []string{}}, "messageLength": "long"},
		"lengthBias": "match", "count": 2, "roll": 5,
	}
	if code := e.do("POST", "/api/personas/leo/expression-preview", body, &out); code != 200 || len(out.Samples) != 2 {
		t.Fatalf("unsaved fields = %d %+v", code, out)
	}
	if s := out.Samples[1]; s.Reply != "Leo lots long match" || s.Incoming != "msg 2/5" {
		t.Errorf("sample = %+v", s)
	}

	// A brand-new persona needs its fields.
	var apiErr map[string]string
	if code := e.do("POST", "/api/personas/new/expression-preview", map[string]any{}, &apiErr); code != 400 || apiErr["code"] != "missing_persona" {
		t.Errorf("new without fields = %d %v", code, apiErr)
	}
	body = map[string]any{"persona": map[string]any{"name": "", "emoji": map[string]any{"usage": "none"}, "messageLength": "medium"}}
	if code := e.do("POST", "/api/personas/new/expression-preview", body, &out); code != 200 || len(out.Samples) != 3 || out.Samples[0].Reply != "Them none medium " {
		t.Errorf("new with fields = %d %+v", code, out)
	}

	if code := e.do("POST", "/api/personas/nope/expression-preview", map[string]any{}, &apiErr); code != 404 {
		t.Errorf("unknown persona = %d", code)
	}
	if code := e.do("POST", "/api/personas/leo/expression-preview", map[string]any{"lengthBias": "huge"}, &apiErr); code != 400 || apiErr["code"] != "invalid_length_bias" {
		t.Errorf("bad bias = %d %v", code, apiErr)
	}
}
