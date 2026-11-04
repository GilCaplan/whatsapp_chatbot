package ctxsys

import (
	"fmt"
	"slices"
	"strconv"

	"whatsappdoppel/internal/core/model"
)

// Block kinds (model.ChartBlock.Kind).
const (
	KindAI   = "ai"   // an AI call
	KindCode = "code" // decided in code, instant
	KindData = "data" // something the prompt reads
	KindIO   = "io"   // a message in or out
	KindNote = "note" // your own step or a note
)

// Stages in flow order (model.ChartBlock.Stage).
var Stages = []string{"arrive", "gate", "join", "goal", "prompt", "write", "check", "out", "background", "custom"}

var blocks = []model.ChartBlock{
	{ID: "message_in", Label: "Message arrives", Stage: "arrive", Kind: KindIO, Description: "A new message in the chat."},

	{ID: "rules", Label: "Rules in code", Stage: "gate", Kind: KindCode, Description: "Languages, links, stickers, who it answers, mute words, rate limits and active hours."},
	{ID: "handoff", Label: "Hand-off check", Stage: "gate", Kind: KindCode, Description: "Money, health, meetings or distress pause the chat for you."},
	{ID: "injection", Label: "Injection filter", Stage: "gate", Kind: KindCode, Description: "Ignores messages that try to take over the persona."},

	{ID: "join_ai", Label: "AI: join in?", Stage: "join", Kind: KindAI, Description: "A small AI call: would the persona jump in here?", Params: []string{"joinIn"}},
	{ID: "join_rules", Label: "Rules: is it for me?", Stage: "join", Kind: KindCode, Description: "Its name, an @tag, a direct answer to it or a question to everyone decide at once; small reactions are a clear no.", Params: []string{"joinIn"}},
	{ID: "chance", Label: "Chime-in chance", Stage: "join", Kind: KindCode, Description: "A random roll with the behaviour's chime-in percent."},

	{ID: "say_word", Label: "Say-the-word check", Stage: "goal", Kind: KindCode, Description: "Exact word goals are checked on every message in code."},
	{ID: "planner", Label: "AI: goal planner", Stage: "goal", Kind: KindAI, Description: "Plans the next move toward the goal, privately.", Params: []string{"planning", "planEvery", "topicShift"}},
	{ID: "goal_check", Label: "AI: has the goal happened?", Stage: "goal", Kind: KindAI, Description: "A narrow yes/no check next to the planner.", Params: []string{"mergedCheck"}},
	{ID: "plan_merged", Label: "AI: plan + check", Stage: "goal", Kind: KindAI, Description: "Plan and \"has it happened?\" in one call, every few replies or on a new topic.", Params: []string{"planning", "planEvery", "topicShift", "mergedCheck"}},
	{ID: "nudge_cap", Label: "Goal nudge cap", Stage: "goal", Kind: KindCode, Description: "Nudges toward the goal only every few replies; in between it just chats.", Params: []string{"nudgeEvery"}},

	{ID: "persona_full", Label: "Full persona text", Stage: "prompt", Kind: KindData, Description: "Everything you wrote about the persona.", Params: []string{"promptStyle"}},
	{ID: "persona_card", Label: "Persona card", Stage: "prompt", Kind: KindData, Description: "A short version of the persona, made once; you can edit it.", Params: []string{"promptStyle", "cardChars"}},
	{ID: "recent", Label: "Recent messages", Stage: "prompt", Kind: KindData, Description: "The conversation word for word.", Params: []string{"historyMax"}},
	{ID: "summary_prose", Label: "Summary of older messages", Stage: "prompt", Kind: KindData, Description: "A paragraph about what came before.", Params: []string{"summary", "summaryChars"}},
	{ID: "summary_facts", Label: "Facts from older messages", Stage: "prompt", Kind: KindData, Description: "People, plans and dates, promises, open questions, jokes; one line each.", Params: []string{"summary", "summaryChars"}},
	{ID: "memories", Label: "Memories about people", Stage: "prompt", Kind: KindData, Description: "What it learned about the people here.", Params: []string{"memoryLines"}},
	{ID: "cross", Label: "From its other chats", Stage: "prompt", Kind: KindData, Description: "What it knows about these people from its other chats."},
	{ID: "world", Label: "Right now", Stage: "prompt", Kind: KindData, Description: "Time, place and its daily routine."},
	{ID: "instructions", Label: "Your instructions", Stage: "prompt", Kind: KindData, Description: "What you told it for this chat."},
	{ID: "budgets", Label: "Size budgets", Stage: "prompt", Kind: KindCode, Description: "A size limit per prompt part.", Params: []string{"historyMax", "memoryLines", "summaryChars", "cardChars"}},

	{ID: "reply_ai", Label: "AI: write the reply", Stage: "write", Kind: KindAI, Description: "The reply itself."},
	{ID: "drafts_ai", Label: "AI: three drafts", Stage: "write", Kind: KindAI, Description: "Co-pilot mode: three tones for you to pick from."},

	{ID: "leak_guard", Label: "Leak guards", Stage: "check", Kind: KindCode, Description: "The goal, other chats and speaking as someone else; a leaky reply is rewritten."},
	{ID: "expression", Label: "Length and emoji", Stage: "check", Kind: KindCode, Description: "Trims long replies and keeps the emoji level."},
	{ID: "check_code", Label: "Reply check", Stage: "check", Kind: KindCode, Description: "Copied instructions, its plan sent as the message, repeats of its last replies, the wrong language.", Params: []string{"replyCheck", "checkRetries", "repeatWindow", "echoWords"}},
	{ID: "judge_ai", Label: "AI judge", Stage: "check", Kind: KindAI, Description: "A small AI check: does this reply make sense here?", Params: []string{"replyCheck"}},
	{ID: "rewrite_ai", Label: "AI: rewrite", Stage: "check", Kind: KindAI, Description: "Writes the reply again with the reason it failed.", Params: []string{"checkRetries"}},

	{ID: "send", Label: "Send", Stage: "out", Kind: KindIO, Description: "Typing, then the message."},
	{ID: "quiet", Label: "Stay quiet", Stage: "out", Kind: KindIO, Description: "Nothing is sent."},
	{ID: "approval", Label: "Wait for your approval", Stage: "out", Kind: KindIO, Description: "Approve mode: you send it."},

	{ID: "memory_job", Label: "AI: learn memories", Stage: "background", Kind: KindAI, Description: "Every few messages: what it learned about people.", Params: []string{"background"}},
	{ID: "brief_job", Label: "AI: brief for other chats", Stage: "background", Kind: KindAI, Description: "A short summary its other chats may use.", Params: []string{"background"}},
	{ID: "notes_job", Label: "AI: take messages", Stage: "background", Kind: KindAI, Description: "A secretary writes down what people want from you.", Params: []string{"background"}},
	{ID: "digest", Label: "AI: one digest", Stage: "background", Kind: KindAI, Description: "One call does memories, the brief and a secretary's notes.", Params: []string{"background", "digestEvery"}},
	{ID: "compaction", Label: "AI: fold into the summary", Stage: "background", Kind: KindAI, Description: "Older messages go into the summary when the window is full.", Params: []string{"summary"}},
	{ID: "card_job", Label: "AI: make the persona card", Stage: "background", Kind: KindAI, Description: "Once per persona, when a card is needed.", Params: []string{"promptStyle"}},

	{ID: "custom_step", Label: "Your step", Stage: "custom", Kind: KindNote, Description: "Anything you want to show; a design note."},
	{ID: "note", Label: "Note", Stage: "custom", Kind: KindNote, Description: "A comment on the chart."},
}

// Blocks is the building-block catalog.
func Blocks() []model.ChartBlock { return slices.Clone(blocks) }

// BlockByID finds a building block.
func BlockByID(id string) (model.ChartBlock, bool) {
	for _, b := range blocks {
		if b.ID == id {
			return b, true
		}
	}
	return model.ChartBlock{}, false
}

// ─── Built-in charts ─────────────────────────────────────────

type cb struct {
	nodes []model.ChartNode
	edges []model.ChartEdge
}

func (c *cb) n(id, block string, label ...string) {
	b, _ := BlockByID(block)
	l := b.Label
	if len(label) > 0 {
		l = label[0]
	}
	c.nodes = append(c.nodes, model.ChartNode{ID: id, Block: block, Label: l})
}

func (c *cb) e(from, to string, label ...string) {
	e := model.ChartEdge{From: from, To: to}
	if len(label) > 0 {
		e.Label = label[0]
	}
	c.edges = append(c.edges, e)
}

func (c *cb) feed(from, to string, label ...string) {
	c.e(from, to, label...)
	c.edges[len(c.edges)-1].Dashed = true
}

// Chart draws a built-in system from its knobs.
func Chart(base string) model.ContextChart {
	p := ParamsFor(base)
	var c cb
	c.n("in", "message_in")
	c.n("rules", "rules")
	c.n("quiet", "quiet")
	c.e("in", "rules")
	c.e("rules", "quiet", "skip")
	switch p.JoinIn {
	case model.JoinAI:
		c.n("join", "join_ai")
		c.e("rules", "join", "group")
		c.e("join", "quiet", "no")
	case model.JoinRulesFirst:
		c.n("jrules", "join_rules")
		c.n("join", "join_ai", "AI: join in? (tiny prompt)")
		c.e("rules", "jrules", "group")
		c.e("jrules", "quiet", "clear no")
		c.e("jrules", "join", "unsure")
		c.e("join", "quiet", "no")
	case model.JoinRulesOnly:
		c.n("jrules", "join_rules")
		c.n("join", "chance")
		c.e("rules", "jrules", "group")
		c.e("jrules", "quiet", "clear no")
		c.e("jrules", "join", "unsure")
		c.e("join", "quiet", "no")
	}
	c.n("reply", "reply_ai")
	c.e("rules", "reply", "private chat")
	if p.JoinIn != model.JoinAI {
		c.e("jrules", "reply", "clear yes")
	}
	c.e("join", "reply", "yes")
	switch p.Planning {
	case model.PlanEveryReply:
		c.n("plan", "planner", "AI: goal planner (every reply)")
		c.n("gcheck", "goal_check")
		c.feed("plan", "reply", "next move")
		c.feed("gcheck", "reply", "reached?")
	case model.PlanPeriodic:
		c.n("plan", "plan_merged", "AI: plan + check (every "+strconv.Itoa(p.PlanEvery)+" replies)")
		c.feed("plan", "reply", "next move")
	default:
		c.n("say", "say_word")
		c.feed("say", "reply", "reached?")
	}
	if p.NudgeEvery > 1 {
		c.n("nudge", "nudge_cap", "Goal nudge every "+strconv.Itoa(p.NudgeEvery)+" replies")
		c.feed("nudge", "reply")
	}
	if p.PromptStyle == model.PromptCard {
		c.n("persona", "persona_card", fmt.Sprintf("Persona card (%d chars)", p.CardChars))
		c.n("budget", "budgets")
		c.feed("budget", "reply", "sizes")
	} else {
		c.n("persona", "persona_full")
	}
	recent := "Recent messages (not yet summarised)"
	if p.HistoryMax > 0 {
		recent = fmt.Sprintf("Recent messages (last %d)", p.HistoryMax)
	}
	c.n("recent", "recent", recent)
	if p.Summary == model.SummaryFacts {
		c.n("summary", "summary_facts")
	} else {
		c.n("summary", "summary_prose")
	}
	c.n("mem", "memories")
	for _, id := range []string{"persona", "recent", "summary", "mem"} {
		c.feed(id, "reply")
	}
	c.n("guard", "leak_guard")
	c.n("send", "send")
	c.e("reply", "guard")
	c.n("expr", "expression")
	if p.ReplyCheck == model.CheckOff {
		c.e("guard", "expr")
		c.e("expr", "send")
		c.n("rewrite", "rewrite_ai")
		c.e("guard", "rewrite", "leak")
		c.e("rewrite", "expr")
	} else {
		c.n("check", "check_code")
		c.e("guard", "expr")
		c.e("expr", "check")
		c.n("rewrite", "rewrite_ai")
		c.e("check", "rewrite", "fails")
		c.e("rewrite", "check")
		c.e("check", "quiet", "fails again")
		if p.ReplyCheck == model.CheckJudge || p.ReplyCheck == model.CheckCodeJudge {
			c.n("judge", "judge_ai")
			l := "unsure"
			if p.ReplyCheck == model.CheckJudge {
				l = "every reply"
			}
			c.e("check", "judge", l)
			c.e("judge", "send", "fine")
			c.e("judge", "rewrite", "odd")
		}
		c.e("check", "send", "pass")
	}
	if p.Background == model.BackgroundDigest {
		c.n("digest", "digest", "AI: digest (every "+strconv.Itoa(p.DigestEvery)+" messages)")
		c.feed("digest", "mem")
	} else {
		c.n("memjob", "memory_job")
		c.n("brief", "brief_job")
		c.feed("memjob", "mem")
		c.n("cross", "cross")
		c.feed("brief", "cross", "other chats")
	}
	c.n("compact", "compaction")
	c.feed("compact", "summary")
	if p.PromptStyle == model.PromptCard {
		c.n("cardjob", "card_job")
		c.feed("cardjob", "persona", "once")
	}
	ch := model.ContextChart{Nodes: c.nodes, Edges: c.edges}
	Layout(&ch)
	return ch
}

// ─── Layout ──────────────────────────────────────────────────

// Node size and spacing used by Layout (the UI draws with the same sizes).
const (
	NodeW = 200
	NodeH = 56
	gapX  = 40
	gapY  = 40
)

// Layout places the nodes: the main flow (solid wires) top to bottom by
// depth, prompt inputs and background jobs (nodes with only dashed wires
// out) in columns to the right, next to what they feed. Retry loops don't
// push nodes down.
func Layout(c *model.ContextChart) {
	idx := map[string]int{}
	for i, n := range c.Nodes {
		idx[n.ID] = i
	}
	feeds := map[string]bool{}
	solidOut := map[string]bool{}
	for _, e := range c.Edges {
		if !e.Dashed {
			solidOut[e.From] = true
		}
	}
	for _, e := range c.Edges {
		if e.Dashed && !solidOut[e.From] {
			feeds[e.From] = true
		}
	}
	main := func(e model.ChartEdge) bool {
		_, okf := idx[e.From]
		_, okt := idx[e.To]
		return okf && okt && !e.Dashed && !feeds[e.From]
	}
	back := backEdges(c, main)
	depth := make([]int, len(c.Nodes))
	for range len(c.Nodes) {
		changed := false
		for k, e := range c.Edges {
			if !main(e) || back[k] {
				continue
			}
			f, t := idx[e.From], idx[e.To]
			if depth[t] < depth[f]+1 {
				depth[t] = depth[f] + 1
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	rows := map[int][]int{}
	maxDepth := 0
	for i, n := range c.Nodes {
		if feeds[n.ID] {
			continue
		}
		rows[depth[i]] = append(rows[depth[i]], i)
		maxDepth = max(maxDepth, depth[i])
	}
	widest := NodeW
	for _, r := range rows {
		widest = max(widest, len(r)*NodeW+(len(r)-1)*gapX)
	}
	for d := 0; d <= maxDepth; d++ {
		r := rows[d]
		w := len(r)*NodeW + (len(r)-1)*gapX
		for k, i := range r {
			c.Nodes[i].X = float64((widest-w)/2 + k*(NodeW+gapX))
			c.Nodes[i].Y = float64(d * (NodeH + gapY))
		}
	}
	sideX := widest + 2*gapX // prompt inputs; background jobs one column further
	// Feeds next to the node they feed: first those feeding the main flow,
	// then those feeding another feed (a job filling a prompt input).
	used := map[[2]int]bool{}
	placed := map[string]bool{}
	for pass := 0; pass < 2; pass++ {
		for i, n := range c.Nodes {
			if !feeds[n.ID] || placed[n.ID] {
				continue
			}
			target := -1
			for _, e := range c.Edges {
				if t, ok := idx[e.To]; ok && e.From == n.ID {
					target = t
					break
				}
			}
			if pass == 0 && target >= 0 && feeds[c.Nodes[target].ID] {
				continue
			}
			col := sideX
			if b, _ := BlockByID(n.Block); b.Stage == "background" {
				col = sideX + NodeW + gapX
			}
			row := 0
			if target >= 0 {
				row = int(c.Nodes[target].Y) / (NodeH + gapY)
			}
			for used[[2]int{col, row}] {
				row++
			}
			used[[2]int{col, row}] = true
			placed[n.ID] = true
			c.Nodes[i].X, c.Nodes[i].Y = float64(col), float64(row*(NodeH+gapY))
		}
	}
}

// backEdges marks the main-flow wires that close a loop (depth-first from
// the nodes nothing leads to, in chart order).
func backEdges(c *model.ContextChart, main func(model.ChartEdge) bool) map[int]bool {
	in := map[string]int{}
	for _, e := range c.Edges {
		if main(e) {
			in[e.To]++
		}
	}
	back := map[int]bool{}
	state := map[string]int{} // 1 on the stack, 2 done
	var visit func(id string)
	visit = func(id string) {
		state[id] = 1
		for k, e := range c.Edges {
			if e.From != id || !main(e) {
				continue
			}
			switch state[e.To] {
			case 1:
				back[k] = true
			case 0:
				visit(e.To)
			}
		}
		state[id] = 2
	}
	for _, n := range c.Nodes {
		if in[n.ID] == 0 && state[n.ID] == 0 {
			visit(n.ID)
		}
	}
	for _, n := range c.Nodes {
		if state[n.ID] == 0 {
			visit(n.ID)
		}
	}
	return back
}

// ─── Chart → knobs ───────────────────────────────────────────

// Infer reads knobs off a chart's blocks (starting from p): it returns the
// knobs a chart like this stands for and what changed, in plain English.
// Blocks that stand for nothing (notes, your steps) change nothing.
func Infer(c model.ContextChart, p model.ContextParams) (model.ContextParams, []string) {
	has := map[string]bool{}
	for _, n := range c.Nodes {
		has[n.Block] = true
	}
	var changes []string
	set := func(cur *string, v, what string) {
		if *cur != v {
			*cur = v
			changes = append(changes, what)
		}
	}
	switch {
	case has["join_rules"] && has["join_ai"]:
		set(&p.JoinIn, model.JoinRulesFirst, "Joining in: rules first, AI only when unsure")
	case has["join_ai"]:
		set(&p.JoinIn, model.JoinAI, "Joining in: AI check on every message")
	case has["join_rules"] || has["chance"]:
		set(&p.JoinIn, model.JoinRulesOnly, "Joining in: rules only")
	}
	switch {
	case has["plan_merged"]:
		set(&p.Planning, model.PlanPeriodic, "Goal planning: every few replies")
		if !p.MergedCheck {
			p.MergedCheck = true
			changes = append(changes, "Plan and check in one call")
		}
	case has["planner"]:
		set(&p.Planning, model.PlanEveryReply, "Goal planning: before every reply")
		if p.MergedCheck == has["goal_check"] {
			p.MergedCheck = !has["goal_check"]
			changes = append(changes, "Plan and check: "+map[bool]string{true: "one call", false: "two calls"}[p.MergedCheck])
		}
	default:
		set(&p.Planning, model.PlanOff, "Goal planning: off")
	}
	if has["nudge_cap"] && p.NudgeEvery <= 1 {
		p.NudgeEvery = 3
		changes = append(changes, "A goal nudge at most every 3 replies")
	} else if !has["nudge_cap"] && p.NudgeEvery > 1 {
		p.NudgeEvery = 1
		changes = append(changes, "A goal nudge may come in every reply")
	}
	switch {
	case has["judge_ai"] && has["check_code"]:
		if p.ReplyCheck != model.CheckJudge {
			set(&p.ReplyCheck, model.CheckCodeJudge, "Checking replies: code, plus an AI judge when unsure")
		}
	case has["judge_ai"]:
		set(&p.ReplyCheck, model.CheckJudge, "Checking replies: AI judge every reply")
	case has["check_code"]:
		set(&p.ReplyCheck, model.CheckCode, "Checking replies: instant checks in code")
	default:
		set(&p.ReplyCheck, model.CheckOff, "Checking replies: off")
	}
	switch {
	case has["digest"]:
		set(&p.Background, model.BackgroundDigest, "Background work: one digest call")
	case has["memory_job"] || has["brief_job"] || has["notes_job"]:
		set(&p.Background, model.BackgroundSeparate, "Background work: separate jobs")
	}
	switch {
	case has["persona_card"]:
		set(&p.PromptStyle, model.PromptCard, "Persona: short card")
	case has["persona_full"]:
		set(&p.PromptStyle, model.PromptFull, "Persona: full text")
	}
	switch {
	case has["summary_facts"]:
		set(&p.Summary, model.SummaryFacts, "Summary: structured facts")
	case has["summary_prose"]:
		set(&p.Summary, model.SummaryProse, "Summary: a paragraph")
	}
	return p, changes
}
