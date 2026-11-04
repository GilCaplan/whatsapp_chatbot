package engine

import (
	"testing"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/model"
)

func (h *harness) decisions(key string) map[string]model.LogEntry {
	h.t.Helper()
	all, err := h.st.Decisions(key, 0)
	if err != nil {
		h.t.Fatal(err)
	}
	out := map[string]model.LogEntry{}
	for _, d := range all {
		out[d.ID] = d
	}
	return out
}

// Two messages answered by one reply: both are logged as replied, with the
// time from each message to the reply and the steps on the way.
func TestDecisionLogReplied(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	h.msg(dmKey, "m1", "hey")
	h.advance(3 * time.Second)
	h.msg(dmKey, "m2", "you there?")
	if d := h.decisions(dmKey)["m1"]; d.Outcome != model.DecisionPending || d.Sender != "Dana" || d.Text != "hey" {
		t.Fatalf("pending entry = %+v", d)
	}
	h.advance(10 * time.Second)
	h.waitSent(1)
	log := h.decisions(dmKey)
	m1, m2 := log["m1"], log["m2"]
	if m1.Outcome != model.DecisionReplied || m2.Outcome != model.DecisionReplied || m1.Reply == "" {
		t.Fatalf("log = %+v", log)
	}
	if m1.DelaySeconds != 12 || m2.DelaySeconds != 9 {
		t.Errorf("delays = %v, %v", m1.DelaySeconds, m2.DelaySeconds)
	}
	if len(m1.Steps) < 2 || m1.Steps[len(m1.Steps)-1].Type != model.ActSent {
		t.Errorf("steps = %+v", m1.Steps)
	}
}

// Messages the persona stays quiet on are logged with the reason.
func TestDecisionLogSkipped(t *testing.T) {
	h := newHarness(t, both(func(p *model.BehaviorProfile) {
		p.IgnoreLinks = true
		p.ChimeInPercent, p.AIJudgement = 0, false
	}), groupChat())
	h.groupMsg("g1", "look https://example.com")
	h.groupMsg("g2", "anyone around")
	log := h.decisions(groupKey)
	if d := log["g1"]; d.Outcome != model.DecisionSkipped || d.Reason != "link" || d.DecidedAt == nil {
		t.Errorf("link = %+v", d)
	}
	if d := log["g2"]; d.Outcome != model.DecisionSkipped || d.Reason != ReasonChimeOut || d.Why == "" {
		t.Errorf("chime = %+v", d)
	}
}

// An opener the persona writes first gets an entry of its own.
func TestDecisionLogOpener(t *testing.T) {
	h := newHarness(t, both(withSchedule(model.ProactiveSchedule{Repeat: behavior.RepeatFixed, EveryMin: 30})), dmChat())
	h.advance(31 * time.Minute)
	h.waitSent(1)
	all, _ := h.st.Decisions(dmKey, 0)
	if len(all) != 1 || all[0].Kind != "opener" || all[0].Outcome != model.DecisionReplied || all[0].Reply == "" ||
		all[0].Why != "Sending a scheduled message" {
		t.Fatalf("log = %+v", all)
	}
}
