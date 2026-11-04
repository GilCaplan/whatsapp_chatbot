// Package ctxsys holds the context systems (leaf: model + stdlib): the three
// built-in ones (Classic = how replies worked before, Lean, Fast), the knobs
// and their ranges, choosing a chat's system (chat → persona → app), the
// flowchart building blocks and the charts of the built-in systems.
package ctxsys

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"whatsappdoppel/internal/core/model"
)

// Default is the app default when settings name none: Classic, so nothing
// changes until you pick another.
const Default = model.CtxClassic

// Limits on your own systems and charts.
const (
	MaxSystems     = 40
	MaxName        = 60
	MaxDescription = 400
	MaxNodes       = 80
	MaxEdges       = 160
	MaxLabel       = 60
	MaxNote        = 300
)

// ParamsFor is the knob set of a base.
func ParamsFor(base string) model.ContextParams {
	switch base {
	case model.CtxLean:
		return model.ContextParams{
			JoinIn:     model.JoinRulesFirst,
			ReplyCheck: model.CheckCode, CheckRetries: 1, RepeatWindow: 5, EchoWords: 5,
			Planning: model.PlanPeriodic, PlanEvery: 5, TopicShift: 12, MergedCheck: true, NudgeEvery: 3,
			Background: model.BackgroundDigest, DigestEvery: 25,
			PromptStyle: model.PromptCard, CardChars: 600, HistoryMax: 40, MemoryLines: 8, SummaryChars: 1500,
			Summary: model.SummaryFacts,
		}
	case model.CtxFast:
		return model.ContextParams{
			JoinIn:     model.JoinRulesOnly,
			ReplyCheck: model.CheckCode, CheckRetries: 1, RepeatWindow: 5, EchoWords: 5,
			Planning: model.PlanOff, PlanEvery: 5, TopicShift: 12, MergedCheck: true, NudgeEvery: 3,
			Background: model.BackgroundDigest, DigestEvery: 40,
			PromptStyle: model.PromptCard, CardChars: 400, HistoryMax: 25, MemoryLines: 5, SummaryChars: 800,
			Summary: model.SummaryFacts,
		}
	}
	return model.ContextParams{ // Classic: exactly how replies worked before context systems
		JoinIn:     model.JoinAI,
		ReplyCheck: model.CheckOff, CheckRetries: 1, RepeatWindow: 5, EchoWords: 5,
		Planning: model.PlanEveryReply, PlanEvery: 5, TopicShift: 12, MergedCheck: false, NudgeEvery: 1,
		Background: model.BackgroundSeparate, DigestEvery: 25,
		PromptStyle: model.PromptFull, CardChars: 600,
		Summary: model.SummaryProse,
	}
}

var builtinText = map[string][2]string{
	model.CtxClassic: {"Classic", "How replies worked until now: an AI check on every group message, the goal planned before every reply, the full persona text and separate background jobs."},
	model.CtxLean:    {"Lean", "Code decides the clear cases and checks every reply; the AI joins in only when unsure, plans the goal every few replies and does background work in one digest. A short persona card and a size budget per part."},
	model.CtxFast:    {"Fast", "One AI call per reply: rules decide when to join in (a chance roll when unsure), no goal planner, a short persona card, small budgets and one digest now and then."},
}

// Bases lists the built-in ids in order.
var Bases = []string{model.CtxClassic, model.CtxLean, model.CtxFast}

// IsBase reports whether id is a built-in system.
func IsBase(id string) bool { return slices.Contains(Bases, id) }

// Builtins returns the built-in systems (charts drawn from the code).
func Builtins() []model.ContextSystem {
	out := make([]model.ContextSystem, 0, len(Bases))
	for _, b := range Bases {
		out = append(out, Builtin(b))
	}
	return out
}

// builtins are drawn once: chats look their system up on every message, and
// the charts (with their layout) never change at run time.
var builtins = sync.OnceValue(func() map[string]model.ContextSystem {
	out := make(map[string]model.ContextSystem, len(Bases))
	for _, base := range Bases {
		t := builtinText[base]
		out[base] = model.ContextSystem{ID: base, Name: t[0], Description: t[1], Base: base, BuiltIn: true,
			Params: ParamsFor(base), Chart: Chart(base)}
	}
	return out
})

// Builtin returns one built-in system (Classic for an unknown id); the chart
// is a copy the caller may change.
func Builtin(base string) model.ContextSystem {
	if !IsBase(base) {
		base = model.CtxClassic
	}
	s := builtins()[base]
	s.Chart.Nodes, s.Chart.Edges = slices.Clone(s.Chart.Nodes), slices.Clone(s.Chart.Edges)
	return s
}

// ─── Choosing a chat's system ────────────────────────────────

// Lookup finds a system by id (built-in or yours).
type Lookup func(id string) (model.ContextSystem, bool)

// Resolve picks a chat's system: the chat's, else the persona's, else the
// app's, else Classic. An id that no longer exists falls through.
func Resolve(chatID, personaID, appID string, find Lookup) (model.ContextSystem, model.ContextChoice) {
	for _, l := range []struct{ id, src string }{{chatID, "chat"}, {personaID, "persona"}, {appID, "app"}} {
		if l.id == "" {
			continue
		}
		if s, ok := find(l.id); ok {
			return s, model.ContextChoice{SystemID: s.ID, Source: l.src}
		}
	}
	s := Builtin(Default)
	return s, model.ContextChoice{SystemID: s.ID, Source: "app"}
}

// ─── Knobs ───────────────────────────────────────────────────

// Option is one choice of an enum knob.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ParamSpec describes a knob for the UI (Settings tab of a system).
type ParamSpec struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Help    string   `json:"help"`
	Group   string   `json:"group"` // join|check|goal|background|prompt
	Type    string   `json:"type"`  // enum|int|bool
	Options []Option `json:"options,omitempty"`
	Min     int      `json:"min,omitempty"`
	Max     int      `json:"max,omitempty"`
	Zero    string   `json:"zero,omitempty"` // what 0 means ("no limit"), when allowed
	Unit    string   `json:"unit,omitempty"`
	// ShowIf: shown only while another knob has one of these values.
	ShowIf map[string][]string `json:"showIf,omitempty"`
}

// Specs lists every knob in display order.
func Specs() []ParamSpec {
	return []ParamSpec{
		{Key: "joinIn", Label: "Joining in (groups)", Group: "join", Type: "enum",
			Help:    "Who decides whether to answer a group message that isn't addressed to it.",
			Options: []Option{{model.JoinAI, "AI check on every message"}, {model.JoinRulesFirst, "Rules first, AI only when unsure"}, {model.JoinRulesOnly, "Rules only (chance when unsure)"}}},
		{Key: "replyCheck", Label: "Checking replies", Group: "check", Type: "enum",
			Help:    "Catches nonsense before it is sent: copied instructions, repeats of its own replies, a plan sent as the message, the wrong language.",
			Options: []Option{{model.CheckOff, "Off"}, {model.CheckCode, "Instant checks in code"}, {model.CheckCodeJudge, "Code, plus an AI judge when unsure"}, {model.CheckJudge, "Code, plus an AI judge every reply"}}},
		{Key: "checkRetries", Label: "Rewrites before staying quiet", Group: "check", Type: "int", Min: 0, Max: 2,
			Help:   "A reply that fails a check is written again with the reason this many times; then it stays quiet.",
			ShowIf: map[string][]string{"replyCheck": {model.CheckCode, model.CheckCodeJudge, model.CheckJudge}}},
		{Key: "repeatWindow", Label: "Compare with its last replies", Group: "check", Type: "int", Min: 0, Max: 20, Unit: "replies", Zero: "don't compare",
			Help:   "A reply that starts like, or mostly repeats, one of these is written again.",
			ShowIf: map[string][]string{"replyCheck": {model.CheckCode, model.CheckCodeJudge, model.CheckJudge}}},
		{Key: "echoWords", Label: "Copied words that count as an echo", Group: "check", Type: "int", Min: 3, Max: 12, Unit: "words",
			Help:   "This many words in a row taken from its own instructions or plan means it repeated them instead of chatting.",
			ShowIf: map[string][]string{"replyCheck": {model.CheckCode, model.CheckCodeJudge, model.CheckJudge}}},
		{Key: "planning", Label: "Goal planning", Group: "goal", Type: "enum",
			Help:    "Only in chats with a goal and plan ahead on. Say-the-word goals are always checked instantly in code.",
			Options: []Option{{model.PlanEveryReply, "Before every reply"}, {model.PlanPeriodic, "Every few replies or on a new topic"}, {model.PlanOff, "Off"}}},
		{Key: "planEvery", Label: "Replies between plans", Group: "goal", Type: "int", Min: 1, Max: 20, Unit: "replies",
			ShowIf: map[string][]string{"planning": {model.PlanPeriodic}}},
		{Key: "topicShift", Label: "New plan after", Group: "goal", Type: "int", Min: 4, Max: 60, Unit: "messages",
			Help:   "This many messages since the last plan count as a new topic.",
			ShowIf: map[string][]string{"planning": {model.PlanPeriodic}}},
		{Key: "mergedCheck", Label: "Plan and \"has it happened?\" in one call", Group: "goal", Type: "bool",
			Help:   "Off asks a second, narrow question alongside each plan (more reliable on small models, one more call).",
			ShowIf: map[string][]string{"planning": {model.PlanEveryReply, model.PlanPeriodic}}},
		{Key: "nudgeEvery", Label: "A goal nudge at most every", Group: "goal", Type: "int", Min: 1, Max: 10, Unit: "replies",
			Help: "In between, it just chats. 1 = it may nudge in every reply."},
		{Key: "background", Label: "Background work", Group: "background", Type: "enum",
			Help:    "Learning memories, the brief for its other chats and a secretary's notes.",
			Options: []Option{{model.BackgroundSeparate, "Separate jobs"}, {model.BackgroundDigest, "One digest call"}}},
		{Key: "digestEvery", Label: "Digest every", Group: "background", Type: "int", Min: 5, Max: 60, Unit: "messages",
			ShowIf: map[string][]string{"background": {model.BackgroundDigest}}},
		{Key: "summary", Label: "Summary of older messages", Group: "background", Type: "enum",
			Help:    "Facts are one editable line each (people, plans, promises, open questions, jokes); only changed lines are rewritten, so it doesn't drift.",
			Options: []Option{{model.SummaryProse, "A paragraph"}, {model.SummaryFacts, "Structured facts"}}},
		{Key: "promptStyle", Label: "Persona in the prompt", Group: "prompt", Type: "enum",
			Help:    "A card is a short version of the persona, made once by the AI; you can edit it.",
			Options: []Option{{model.PromptFull, "Full persona text"}, {model.PromptCard, "Short persona card"}}},
		{Key: "cardChars", Label: "Card size", Group: "prompt", Type: "int", Min: 200, Max: 2000, Unit: "characters",
			ShowIf: map[string][]string{"promptStyle": {model.PromptCard}}},
		{Key: "historyMax", Label: "Messages it reads", Group: "prompt", Type: "int", Min: 10, Max: 200, Zero: "everything not yet summarised", Unit: "messages"},
		{Key: "memoryLines", Label: "Memories it reads", Group: "prompt", Type: "int", Min: 1, Max: 20, Zero: "the default", Unit: "memories"},
		{Key: "summaryChars", Label: "Summary size it reads", Group: "prompt", Type: "int", Min: 200, Max: 4000, Zero: "all of it", Unit: "characters"},
	}
}

func spec(key string) (ParamSpec, bool) {
	for _, s := range Specs() {
		if s.Key == key {
			return s, true
		}
	}
	return ParamSpec{}, false
}

func enumOK(key, v string) bool {
	s, _ := spec(key)
	return slices.ContainsFunc(s.Options, func(o Option) bool { return o.Value == v })
}

func clampInt(key string, v, def int) int {
	s, _ := spec(key)
	if v == 0 && s.Zero != "" {
		return 0
	}
	if v == 0 {
		return def
	}
	return max(s.Min, min(s.Max, v))
}

// Clamp fixes every knob into its range; bad enums take the base's value.
func Clamp(p model.ContextParams, base string) model.ContextParams {
	d := ParamsFor(base)
	enum := func(key string, v *string, def string) {
		if !enumOK(key, *v) {
			*v = def
		}
	}
	enum("joinIn", &p.JoinIn, d.JoinIn)
	enum("replyCheck", &p.ReplyCheck, d.ReplyCheck)
	enum("planning", &p.Planning, d.Planning)
	enum("background", &p.Background, d.Background)
	enum("promptStyle", &p.PromptStyle, d.PromptStyle)
	enum("summary", &p.Summary, d.Summary)
	p.CheckRetries = max(0, min(2, p.CheckRetries))
	p.RepeatWindow = max(0, min(20, p.RepeatWindow))
	p.EchoWords = clampInt("echoWords", p.EchoWords, d.EchoWords)
	p.PlanEvery = clampInt("planEvery", p.PlanEvery, d.PlanEvery)
	p.TopicShift = clampInt("topicShift", p.TopicShift, d.TopicShift)
	p.NudgeEvery = clampInt("nudgeEvery", p.NudgeEvery, d.NudgeEvery)
	p.DigestEvery = clampInt("digestEvery", p.DigestEvery, d.DigestEvery)
	p.CardChars = clampInt("cardChars", p.CardChars, d.CardChars)
	p.HistoryMax = clampInt("historyMax", p.HistoryMax, 0)
	p.MemoryLines = clampInt("memoryLines", p.MemoryLines, 0)
	p.SummaryChars = clampInt("summaryChars", p.SummaryChars, 0)
	return p
}

// Normalize checks one of your systems before it is saved: name, base,
// knobs in range, a well-formed chart. Built-in ids can't be used.
func Normalize(s *model.ContextSystem) error {
	s.Name = strings.TrimSpace(s.Name)
	s.Description = strings.TrimSpace(s.Description)
	switch {
	case s.Name == "":
		return errors.New("give the system a name")
	case utf8.RuneCountInString(s.Name) > MaxName:
		return fmt.Errorf("the name can be at most %d characters", MaxName)
	case utf8.RuneCountInString(s.Description) > MaxDescription:
		return fmt.Errorf("the description can be at most %d characters", MaxDescription)
	case IsBase(s.ID):
		return errors.New("the built-in systems can't be changed; copy one first")
	}
	if !IsBase(s.Base) {
		s.Base = model.CtxClassic
	}
	s.BuiltIn = false
	s.Params = Clamp(s.Params, s.Base)
	return NormalizeChart(&s.Chart)
}

// NormalizeChart checks a chart: known blocks, unique node ids, wires
// between existing nodes, sizes capped. Unknown blocks become notes.
func NormalizeChart(c *model.ContextChart) error {
	if len(c.Nodes) > MaxNodes {
		return fmt.Errorf("a chart can have at most %d blocks", MaxNodes)
	}
	if len(c.Edges) > MaxEdges {
		return fmt.Errorf("a chart can have at most %d wires", MaxEdges)
	}
	ids := map[string]bool{}
	for i := range c.Nodes {
		n := &c.Nodes[i]
		n.ID = strings.TrimSpace(n.ID)
		if n.ID == "" || ids[n.ID] || len(n.ID) > 40 {
			return errors.New("every block needs its own id")
		}
		ids[n.ID] = true
		b, ok := BlockByID(n.Block)
		if !ok {
			n.Block = "note"
			b, _ = BlockByID("note")
		}
		n.Label = clip(n.Label, MaxLabel)
		if n.Label == "" {
			n.Label = b.Label
		}
		n.Note = clip(n.Note, MaxNote)
		n.X, n.Y = max(-5000, min(5000, n.X)), max(-5000, min(5000, n.Y))
	}
	seen := map[string]bool{}
	edges := c.Edges[:0]
	for _, e := range c.Edges {
		k := e.From + ">" + e.To
		if !ids[e.From] || !ids[e.To] || e.From == e.To || seen[k] {
			continue
		}
		seen[k] = true
		e.Label = clip(e.Label, 30)
		edges = append(edges, e)
	}
	c.Edges = edges
	if c.Nodes == nil {
		c.Nodes = []model.ChartNode{}
	}
	if c.Edges == nil {
		c.Edges = []model.ChartEdge{}
	}
	return nil
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:n]))
}

// ─── Estimates ───────────────────────────────────────────────

// Estimate is a rough count of AI calls, from the knobs alone (the sandbox
// measures the real thing).
type Estimate struct {
	// PerAnswer: calls one after another before a group reply with a goal
	// (join-in check, planning, reply, judge, rewrites).
	PerAnswer float64 `json:"perAnswer"`
	// Background: background calls per 100 messages.
	Background float64 `json:"background"`
}

// replyShare is how often a group message is answered (measured: 41 of 117).
const replyShare = 0.35

// Estimates the knobs' AI calls.
func Estimates(p model.ContextParams) Estimate {
	var e Estimate
	switch p.JoinIn {
	case model.JoinAI:
		e.PerAnswer += 1
	case model.JoinRulesFirst:
		e.PerAnswer += 0.35 // the share the rules leave unsure
	}
	plan := 2.0
	if p.MergedCheck {
		plan = 1
	}
	switch p.Planning {
	case model.PlanEveryReply:
		e.PerAnswer += plan
	case model.PlanPeriodic:
		e.PerAnswer += plan / float64(max(1, p.PlanEvery))
	}
	e.PerAnswer++ // the reply
	switch p.ReplyCheck {
	case model.CheckJudge:
		e.PerAnswer += 1
	case model.CheckCodeJudge:
		e.PerAnswer += 0.2
	}
	if p.ReplyCheck != model.CheckOff {
		e.PerAnswer += 0.1 * float64(p.CheckRetries)
	}
	switch p.Background {
	case model.BackgroundDigest:
		e.Background = 100 / float64(max(1, p.DigestEvery))
	default:
		e.Background = 100.0/6 + 100.0/8 // memories every 6 of theirs, the brief every 8
	}
	e.Background += 100.0 / 70 // folding into the summary
	e.PerAnswer = float64(int(e.PerAnswer*10+0.5)) / 10
	e.Background = float64(int(e.Background*10+0.5)) / 10
	return e
}
