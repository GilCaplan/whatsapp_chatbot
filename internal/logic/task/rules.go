package task

import (
	"fmt"
	"slices"
	"strings"

	"whatsappdoppel/internal/core/model"
)

// Intents the reading step may return.
var Intents = []string{"accept", "reject", "propose", "ask_availability", "ask_reason", "question", "defer", "off_topic", "money", "bot_menu", "other"}

// Conditions (TaskRule.When). "missing:<slot>" is one per slot.
const (
	CondStarted     = "started"       // the task just started and we write first
	CondNoReply     = "no_reply"      // they haven't answered for a while
	CondOutOfNudges = "out_of_nudges" // no answer and every nudge used
	CondLimit       = "limit"         // the message limit is reached
	CondTimeWorks   = "time:works"    // a time they gave works
	CondTimeNo      = "time:no"       // a time they gave doesn't work
	CondTimeDay     = "time:day"      // they gave only a day or part of day
	CondUnclear     = "unclear"       // they agreed but it isn't clear to which time
	CondProbeLimit  = "probe_limit"   // too many "doesn't work" answers
	CondReady       = "ready"         // a time works and every required slot has a value (not confirming yet)
	CondMissing     = "missing:"      // + slot key: a required slot has no value yet
)

// Actions (TaskRule.Do). "ask:<slot>" is one per slot.
const (
	DoStrategy     = "strategy"      // what the privacy strategy says: offer, windows or ask their times
	DoOffer        = "offer"         // offer a few times
	DoWindows      = "offer_windows" // offer parts of days
	DoOfferOnDay   = "offer_on_day"  // offer times on the day they named
	DoAnswerNo     = "answer_time"   // say their time doesn't work, with an alternative
	DoAskTimes     = "ask_their_times"
	DoAskConfirm   = "ask_confirm" // "Tuesday 10:00 then?"
	DoAskWhich     = "ask_which"   // which of the offered times
	DoCommit       = "commit"      // confirm (your approval or auto-confirm), then book
	DoAsk          = "ask:"        // + slot key
	DoCover        = "cover"       // deflect, or one of your lines
	DoChat         = "chat"        // answer in character, then steer back
	DoNudge        = "nudge"
	DoWait         = "wait" // "no rush"
	DoOnProbeLimit = "on_probe_limit"
	DoHandoff      = "handoff"     // pause and hand over to you, with a summary
	DoFinishOK     = "finish:ok"   // end, done
	DoFinishFail   = "finish:fail" // end, not done
	DoNotify       = "notify"      // tell you
)

// Conditions is the fixed list of conditions for spec (slot conditions
// included), in the order the rule editor shows them.
func Conditions(spec model.TaskSpec) []string {
	out := []string{CondStarted, CondTimeWorks, CondTimeNo, CondTimeDay, CondUnclear, CondReady, CondProbeLimit, CondNoReply, CondOutOfNudges, CondLimit}
	for _, in := range Intents {
		out = append(out, "intent:"+in)
	}
	for _, s := range spec.Slots {
		if s.Kind != model.SlotWhen {
			out = append(out, CondMissing+s.Key)
		}
	}
	return out
}

// Actions is the fixed list of actions for spec.
func Actions(spec model.TaskSpec) []string {
	out := []string{DoStrategy, DoOffer, DoWindows, DoOfferOnDay, DoAnswerNo, DoAskTimes, DoAskConfirm, DoAskWhich, DoCommit,
		DoCover, DoChat, DoNudge, DoWait, DoOnProbeLimit, DoHandoff, DoFinishOK, DoFinishFail, DoNotify}
	for _, s := range spec.Slots {
		if s.Kind != model.SlotWhen {
			out = append(out, DoAsk+s.Key)
		}
	}
	return out
}

// sayActions write a message; the others are effects.
var effectActions = []string{DoHandoff, DoFinishOK, DoFinishFail, DoNotify}

// IsEffect reports whether an action only has an effect (writes nothing).
func IsEffect(a string) bool { return slices.Contains(effectActions, a) }

// Steps a rule may be limited to.
var Steps = []string{model.StepOpen, model.StepNegotiate, model.StepConfirming}

// FindTimeRules are the rules of the Find a time template, first match wins.
func FindTimeRules() []model.TaskRule {
	return []model.TaskRule{
		{When: "intent:money", Do: []string{DoCover, DoHandoff}},
		{When: CondLimit, Do: []string{DoHandoff}},
		{When: CondOutOfNudges, Do: []string{DoHandoff}},
		{When: CondNoReply, Do: []string{DoNudge}},
		{When: CondStarted, Do: []string{DoStrategy}},
		{When: "intent:bot_menu", Do: []string{DoHandoff}},
		{When: CondProbeLimit, Do: []string{DoOnProbeLimit}},
		{When: "intent:accept", Step: model.StepConfirming, Do: []string{DoCommit}},
		{When: CondTimeWorks, Do: []string{DoAskConfirm}},
		{When: CondTimeNo, Do: []string{DoAnswerNo}},
		{When: CondTimeDay, Do: []string{DoOfferOnDay}},
		{When: CondUnclear, Do: []string{DoAskWhich}},
		{When: CondReady, Do: []string{DoAskConfirm}},
		{When: "intent:ask_availability", Do: []string{DoStrategy}},
		{When: "intent:ask_reason", Do: []string{DoCover}},
		{When: "intent:reject", Do: []string{DoOffer}},
		{When: "intent:defer", Do: []string{DoWait}},
	}
}

// RuleText is a rule in plain English for logs ("when they agree (while
// confirming): confirm").
func RuleText(r model.TaskRule) string {
	s := "when " + r.When
	if r.Step != "" {
		s += " (" + r.Step + ")"
	}
	return s + ": " + strings.Join(r.Do, ", ")
}

// ValidateRules checks a rule list against the fixed vocabulary.
func ValidateRules(spec model.TaskSpec, rules []model.TaskRule) error {
	if len(rules) > 60 {
		return fmt.Errorf("at most 60 rules")
	}
	conds, acts := Conditions(spec), Actions(spec)
	for i, r := range rules {
		if !slices.Contains(conds, r.When) {
			return fmt.Errorf("rule %d: unknown condition %q", i+1, r.When)
		}
		if r.Step != "" && !slices.Contains(Steps, r.Step) {
			return fmt.Errorf("rule %d: unknown step %q", i+1, r.Step)
		}
		if len(r.Do) == 0 || len(r.Do) > 4 {
			return fmt.Errorf("rule %d: one to four actions", i+1)
		}
		says := 0
		for _, a := range r.Do {
			if !slices.Contains(acts, a) {
				return fmt.Errorf("rule %d: unknown action %q", i+1, a)
			}
			if !IsEffect(a) {
				says++
			}
		}
		if says > 1 {
			return fmt.Errorf("rule %d: only one action that writes a message", i+1)
		}
		if r.MaxTimes < 0 || r.MaxTimes > 50 {
			return fmt.Errorf("rule %d: max times is 0 to 50", i+1)
		}
		if slices.Contains(r.Do, DoCommit) && r.Step != model.StepConfirming {
			return fmt.Errorf("rule %d: confirm only while confirming (after asking them)", i+1)
		}
	}
	return nil
}

// Pick is the first rule that matches facts in step (honouring MaxTimes);
// -1 when none does.
func Pick(rules []model.TaskRule, facts []string, step string, fired map[string]int) int {
	for i, r := range rules {
		if !slices.Contains(facts, r.When) || (r.Step != "" && r.Step != step) {
			continue
		}
		if r.MaxTimes > 0 && fired[fmt.Sprint(i)] >= r.MaxTimes {
			continue
		}
		return i
	}
	return -1
}

// PartnerStyles are how the sandbox's other side behaves (prompt.PartnerStyles words them).
var PartnerStyles = []string{"cooperative", "vague", "difficult", "prober"}
