package persona

import (
	"strings"
	"testing"

	"whatsappdoppel/internal/model"
)

func TestNormalizeGoalSettings(t *testing.T) {
	p := model.Persona{Name: "X", GoalStyle: " Direct ", GoalAfterReached: "CONTINUE"}
	if err := Normalize(&p); err != nil {
		t.Fatal(err)
	}
	if p.GoalStyle != model.GoalStyleDirect || p.GoalAfterReached != model.GoalAfterContinue || p.GoalPlanAhead == nil || !*p.GoalPlanAhead {
		t.Errorf("normalized: style=%q after=%q plan=%v", p.GoalStyle, p.GoalAfterReached, p.GoalPlanAhead)
	}
	off := false
	p = model.Persona{Name: "X", GoalStyle: "sneaky", GoalAfterReached: "?", GoalPlanAhead: &off}
	_ = Normalize(&p)
	if p.GoalStyle != model.GoalStyleSubtle || p.GoalAfterReached != model.GoalAfterRelax || *p.GoalPlanAhead {
		t.Errorf("fallbacks: style=%q after=%q plan=%v", p.GoalStyle, p.GoalAfterReached, *p.GoalPlanAhead)
	}
}

func TestSeedsPursueGoalsWithTact(t *testing.T) {
	for _, p := range Seeds() {
		if p.GoalStyle != model.GoalStyleSubtle || p.GoalPlanAhead == nil || !*p.GoalPlanAhead || p.GoalAfterReached != model.GoalAfterRelax {
			t.Errorf("%s goal settings: %q %v %q", p.ID, p.GoalStyle, p.GoalPlanAhead, p.GoalAfterReached)
		}
		if strings.Contains(p.Rules, "GOAL") || strings.Contains(strings.ToLower(p.Rules), "prioritize the") {
			t.Errorf("%s rules still push the goal bluntly: %q", p.ID, p.Rules)
		}
	}
	a, b := Seeds(), Seeds()
	*a[0].GoalPlanAhead = false
	if !*b[0].GoalPlanAhead {
		t.Error("seed copies must not share GoalPlanAhead")
	}
}
