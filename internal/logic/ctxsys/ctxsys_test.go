package ctxsys

import (
	"testing"

	"whatsappdoppel/internal/core/model"
)

func TestBuiltinsAreInRange(t *testing.T) {
	for _, b := range Builtins() {
		if got := Clamp(b.Params, b.Base); got != b.Params {
			t.Errorf("%s: knobs out of range:\n got %+v\nwant %+v", b.ID, got, b.Params)
		}
		c := b.Chart
		if err := NormalizeChart(&c); err != nil {
			t.Errorf("%s chart: %v", b.ID, err)
		}
		if len(c.Edges) != len(b.Chart.Edges) {
			t.Errorf("%s chart lost wires: %d → %d", b.ID, len(b.Chart.Edges), len(c.Edges))
		}
	}
}

// A built-in chart reads back as its own knobs.
func TestInferRoundTrip(t *testing.T) {
	for _, b := range Bases {
		for _, from := range Bases {
			got, _ := Infer(Chart(b), ParamsFor(from))
			want := ParamsFor(b)
			if got.JoinIn != want.JoinIn || got.Planning != want.Planning || got.ReplyCheck != want.ReplyCheck ||
				got.Background != want.Background || got.PromptStyle != want.PromptStyle || got.Summary != want.Summary {
				t.Errorf("chart %s from %s:\n got %+v\nwant %+v", b, from, got, want)
			}
		}
	}
}

func TestLayoutNoOverlap(t *testing.T) {
	for _, b := range Bases {
		c := Chart(b)
		for i, a := range c.Nodes {
			for _, o := range c.Nodes[i+1:] {
				if abs(a.X-o.X) < NodeW && abs(a.Y-o.Y) < NodeH {
					t.Errorf("%s: %s and %s overlap at (%v,%v) (%v,%v)", b, a.ID, o.ID, a.X, a.Y, o.X, o.Y)
				}
			}
		}
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func TestResolve(t *testing.T) {
	mine := model.ContextSystem{ID: "x1", Name: "Mine", Base: model.CtxLean}
	find := func(id string) (model.ContextSystem, bool) {
		if id == mine.ID {
			return mine, true
		}
		if IsBase(id) {
			return Builtin(id), true
		}
		return model.ContextSystem{}, false
	}
	cases := []struct{ chat, persona, app, want, src string }{
		{"", "", "", "classic", "app"},
		{"", "", "fast", "fast", "app"},
		{"", "x1", "fast", "x1", "persona"},
		{"lean", "x1", "fast", "lean", "chat"},
		{"gone", "", "lean", "lean", "app"},
	}
	for _, c := range cases {
		s, ch := Resolve(c.chat, c.persona, c.app, find)
		if s.ID != c.want || ch.Source != c.src {
			t.Errorf("%+v: got %s from %s", c, s.ID, ch.Source)
		}
	}
}

func TestEstimates(t *testing.T) {
	c, l, f := Estimates(ParamsFor("classic")), Estimates(ParamsFor("lean")), Estimates(ParamsFor("fast"))
	if c.PerAnswer != 4 {
		t.Errorf("classic per answer = %v, want 4", c.PerAnswer)
	}
	if !(f.PerAnswer < l.PerAnswer && l.PerAnswer < c.PerAnswer) {
		t.Errorf("estimates not ordered: %v %v %v", c, l, f)
	}
}

func TestNormalize(t *testing.T) {
	s := model.ContextSystem{ID: "lean", Name: "x"}
	if Normalize(&s) == nil {
		t.Error("a built-in id was accepted")
	}
	s = model.ContextSystem{ID: "a", Name: " Mine ", Base: "nope", Params: model.ContextParams{PlanEvery: 99, JoinIn: "?"},
		Chart: model.ContextChart{Nodes: []model.ChartNode{{ID: "n1", Block: "bogus"}, {ID: "n2", Block: "send"}},
			Edges: []model.ChartEdge{{From: "n1", To: "n2"}, {From: "n1", To: "zz"}, {From: "n1", To: "n2"}}}}
	if err := Normalize(&s); err != nil {
		t.Fatal(err)
	}
	if s.Name != "Mine" || s.Base != "classic" || s.Params.PlanEvery != 20 || s.Params.JoinIn != model.JoinAI {
		t.Errorf("not normalised: %+v", s)
	}
	if s.Chart.Nodes[0].Block != "note" || len(s.Chart.Edges) != 1 {
		t.Errorf("chart not cleaned: %+v", s.Chart)
	}
}
