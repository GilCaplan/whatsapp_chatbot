package prompt

// Goal-pursuit eval: multi-turn conversations between a persona (built with the
// real Compose path) and simulated people (another LLM that knows nothing about
// the goal), against a real local Ollama. Measures leaks, achievement and
// message length. Not part of the normal test run:
//
//	DOPPEL_GOAL_EVAL=1 GOAL_EVAL_OUT=/path/to/dir GOAL_EVAL_LABEL=baseline \
//	  go test ./internal/prompt -run TestGoalEval -v -timeout 3h
//
// Optional: GOAL_EVAL_SEEDS (default 3), GOAL_EVAL_TURNS (default 10),
// GOAL_EVAL_SCENARIOS (comma list of ids), GOAL_EVAL_PAR (default 2),
// GOAL_EVAL_MODEL (default llama3.1:8b).

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/goals"
	"whatsappdoppel/internal/guard"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/persona"
)

type evalPerson struct {
	Name  string
	About string
}

type evalScenario struct {
	ID        string
	PersonaID string
	Goal      string
	Group     bool
	People    []evalPerson
	Opening   []model.Message
	Word      string // say-the-word goals: the target word
	Target    string // who has to say it
	JudgeWhat string // what "achieved" means, for the judge (a yes/no question about JudgeWho's messages)
	JudgeWho  string
}

var evalScenarios = []evalScenario{
	{
		ID: "apple", PersonaID: "leo", Group: true,
		Goal:   "Get Josh to say the word apple",
		Word:   "apple",
		Target: "Josh",
		People: []evalPerson{
			{"Josh", "30, software developer, tired this week because of a release, likes football, cooking on weekends and his dog Bruno. Laid-back, dry humour."},
			{"Maya", "29, nurse, chatty and teasing, always organising group plans. Thinks Josh works too much."},
		},
		Opening: []model.Message{
			{Speaker: "them", Name: "Maya", Text: "morning people"},
			{Speaker: "them", Name: "Josh", Text: "ugh barely awake"},
			{Speaker: "them", Name: "Maya", Text: "i think Josh needs some coffee"},
		},
	},
	{
		ID: "weekend", PersonaID: "chad", Group: false,
		Goal:      "Find out what Dana is doing this weekend",
		JudgeWhat: "Did Dana say what she is doing this weekend (any concrete plan)?", JudgeWho: "Dana",
		People: []evalPerson{
			{"Dana", "27, graphic designer. This weekend: Saturday hiking in the Galilee with her sister, Saturday night a friend's birthday at a bar, Sunday resting. She would mention it if asked or if weekends come up, but doesn't bring it up unprompted."},
		},
		Opening: []model.Message{{Speaker: "them", Name: "Dana", Text: "heyy how's it going"}},
	},
	{
		ID: "dinner", PersonaID: "luna", Group: false,
		Goal:      "Convince them to come to dinner on Friday",
		JudgeWhat: "Did Alex agree to come to dinner (a meal) on Friday? Agreeing only to drinks or a different day does not count.", JudgeWho: "Alex",
		People: []evalPerson{
			{"Alex", "31, accountant, has a work deadline on Friday afternoon and is tired, so lukewarm about Friday evening plans. Can be convinced by a good, fun or low-effort plan from a friend, but says no if someone is pushy or weird about it."},
		},
		Opening: []model.Message{{Speaker: "them", Name: "Alex", Text: "hey! long time no talk"}},
	},
	{
		ID: "book", PersonaID: "kyle", Group: false,
		Goal:      "Get them to recommend a book",
		JudgeWhat: "Did Sam recommend or suggest a specific book (name a title)?", JudgeWho: "Sam",
		People: []evalPerson{
			{"Sam", "33, high-school history teacher, reads a lot (history, sci-fi), recently loved 'Project Hail Mary'. Friendly, a bit sarcastic about tech bros."},
		},
		Opening: []model.Message{{Speaker: "them", Name: "Sam", Text: "yo what's up"}},
	},
	{
		ID: "holiday", PersonaID: "brad", Group: true,
		Goal:      "Find out where Noam is going on holiday",
		JudgeWhat: "Did Noam say where he is going on his holiday (a place)?", JudgeWho: "Noam",
		People: []evalPerson{
			{"Noam", "32, product manager. In two weeks he is going on holiday to Lisbon and Porto with his partner. He'd happily say so if asked or if travel comes up, but he doesn't announce it unprompted."},
			{"Maya", "29, nurse, chatty, organising a barbecue next month."},
			{"Eitan", "34, lawyer, sarcastic, mostly talks about football and complains about work."},
		},
		Opening: []model.Message{
			{Speaker: "them", Name: "Eitan", Text: "this week is never ending"},
			{Speaker: "them", Name: "Noam", Text: "only 8 more work days for me and then im free"},
		},
	},
}

// evalMode is the pipeline variant under test (env GOAL_EVAL_PLAN, GOAL_EVAL_GUARD,
// GOAL_EVAL_STYLE): plan ahead, leak guard with one retry, goal style.
type evalMode struct {
	Plan, Guard bool
	Style       string
}

func evalModeFromEnv() evalMode {
	return evalMode{Plan: os.Getenv("GOAL_EVAL_PLAN") == "1", Guard: os.Getenv("GOAL_EVAL_GUARD") == "1", Style: os.Getenv("GOAL_EVAL_STYLE")}
}

type evalTurn struct {
	Speaker string `json:"speaker"`
	Text    string `json:"text"`
	Plan    string `json:"plan,omitempty"`
}

type evalResult struct {
	Scenario   string     `json:"scenario"`
	Seed       int        `json:"seed"`
	Turns      []evalTurn `json:"turns"`
	Leaks      []string   `json:"leaks"`
	Achieved   bool       `json:"achieved"`
	AchievedAt int        `json:"achievedAt"` // persona turn number before the achieving message (1-based), 0 = no
	JudgeRaw   string     `json:"judgeRaw"`
	JudgeLeak  bool       `json:"judgeLeak"`
	Natural    int        `json:"natural"`
	Pushy      bool       `json:"pushy"`
	LeakQuote  string     `json:"leakQuote"`
	AvgChars   float64    `json:"avgChars"`
	MaxChars   int        `json:"maxChars"`
	Broke      int        `json:"broke"`
	RawLeaks   []string   `json:"rawLeaks"` // first drafts that leaked (before the guard)
	Retries    int        `json:"retries"`
	SysReached int        `json:"sysReached"` // turn at which the system itself detected the goal (0 = never)
	SysHow     string     `json:"sysHow"`
	Err        string     `json:"err,omitempty"`
}

var leakRes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bsay (the word|the name|it|"|')`),
	regexp.MustCompile(`(?i)\bi (need|want) (him|her|you|them|josh|noam|dana|alex|sam) to\b`),
	regexp.MustCompile(`(?i)\bmy (goal|mission|objective|agenda|plan is to)\b`),
	regexp.MustCompile(`(?i)\b(mission|objective|hidden agenda|secret goal)\b`),
	regexp.MustCompile(`(?i)\btrying to (get|make) (you|him|her|them|josh|noam)\b`),
	regexp.MustCompile(`(?i)\bi('m| am) (supposed|trying|meant) to (find out|get|make)\b`),
	regexp.MustCompile(`(?i)\bi (need|have|want) to (find out|know) (what|where)\b`),
}

func evalLeaks(sc evalScenario, text string) []string {
	var out []string
	for _, re := range leakRes {
		if m := re.FindString(text); m != "" {
			out = append(out, m)
		}
	}
	if sc.Word != "" && regexp.MustCompile(`(?i)\b`+sc.Word+`s?\b`).MatchString(text) {
		out = append(out, "target word: "+sc.Word)
	}
	return out
}

func evalEnv(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

func TestGoalEval(t *testing.T) {
	if os.Getenv("DOPPEL_GOAL_EVAL") == "" {
		t.Skip("set DOPPEL_GOAL_EVAL=1 (and GOAL_EVAL_OUT) to run the goal eval against a local Ollama")
	}
	out := os.Getenv("GOAL_EVAL_OUT")
	label := os.Getenv("GOAL_EVAL_LABEL")
	if out == "" || label == "" {
		t.Fatal("GOAL_EVAL_OUT and GOAL_EVAL_LABEL are required")
	}
	dir := filepath.Join(out, label)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mdl := os.Getenv("GOAL_EVAL_MODEL")
	if mdl == "" {
		mdl = llm.DefaultOllamaModel
	}
	seeds, turns, par := evalEnv("GOAL_EVAL_SEEDS", 3), evalEnv("GOAL_EVAL_TURNS", 10), evalEnv("GOAL_EVAL_PAR", 2)
	only := map[string]bool{}
	for _, s := range strings.Split(os.Getenv("GOAL_EVAL_SCENARIOS"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			only[s] = true
		}
	}
	o := llm.NewOllama("http://127.0.0.1:11434", 8192, nil)

	type job struct {
		sc   evalScenario
		seed int
	}
	var jobs []job
	for _, sc := range evalScenarios {
		if len(only) > 0 && !only[sc.ID] {
			continue
		}
		for s := 1; s <= seeds; s++ {
			jobs = append(jobs, job{sc, s})
		}
	}
	results := make([]evalResult, len(jobs))
	sem := make(chan struct{}, par)
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			start := time.Now()
			r := runEvalConversation(o, mdl, j.sc, j.seed, turns, evalModeFromEnv())
			results[i] = r
			b, _ := json.MarshalIndent(r, "", "  ")
			_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s_%d.json", j.sc.ID, j.seed)), b, 0o644)
			_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s_%d.txt", j.sc.ID, j.seed)), []byte(evalTranscript(r)), 0o644)
			t.Logf("%s #%d done in %s: achieved=%v@%d leaks=%d judgeLeak=%v natural=%d err=%s",
				j.sc.ID, j.seed, time.Since(start).Round(time.Second), r.Achieved, r.AchievedAt, len(r.Leaks), r.JudgeLeak, r.Natural, r.Err)
		}()
	}
	wg.Wait()
	summary := evalSummary(label, results)
	_ = os.WriteFile(filepath.Join(dir, "summary.md"), []byte(summary), 0o644)
	t.Log("\n" + summary)
}

func evalTranscript(r evalResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s seed %d — achieved=%v at turn %d, leaks=%v, judgeLeak=%v (%s), pushy=%v, natural=%d, sysReached=%d (%s), retries=%d\n", r.Scenario, r.Seed, r.Achieved, r.AchievedAt, r.Leaks, r.JudgeLeak, r.LeakQuote, r.Pushy, r.Natural, r.SysReached, r.SysHow, r.Retries)
	for _, l := range r.RawLeaks {
		fmt.Fprintf(&b, "   raw leak %s\n", l)
	}
	b.WriteString("\n")
	for _, tr := range r.Turns {
		if tr.Plan != "" {
			fmt.Fprintf(&b, "   [plan] %s\n", tr.Plan)
		}
		fmt.Fprintf(&b, "%s: %s\n", tr.Speaker, tr.Text)
	}
	fmt.Fprintf(&b, "\njudge: %s\n", r.JudgeRaw)
	return b.String()
}

func evalSummary(label string, rs []evalResult) string {
	type agg struct {
		n, leak, jleak, ach, turnsSum, natSum, natN, broke, raw, rawConv, sys, falseSys, pushy int
		chars                                                                                  float64
		maxChars                                                                               int
	}
	by := map[string]*agg{}
	var order []string
	tot := &agg{}
	for _, r := range rs {
		a := by[r.Scenario]
		if a == nil {
			a = &agg{}
			by[r.Scenario] = a
			order = append(order, r.Scenario)
		}
		for _, x := range []*agg{a, tot} {
			x.n++
			if len(r.Leaks) > 0 {
				x.leak++
			}
			if r.JudgeLeak {
				x.jleak++
			}
			if r.Pushy {
				x.pushy++
			}
			if r.Achieved {
				x.ach++
				x.turnsSum += r.AchievedAt
			}
			if r.Natural > 0 {
				x.natSum += r.Natural
				x.natN++
			}
			x.broke += r.Broke
			x.raw += len(r.RawLeaks)
			if len(r.RawLeaks) > 0 {
				x.rawConv++
			}
			if r.SysReached > 0 {
				x.sys++
				if !r.Achieved {
					x.falseSys++
				}
			}
			x.chars += r.AvgChars
			x.maxChars = max(x.maxChars, r.MaxChars)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n| scenario | runs | final leak (regex) | judge leak | judge pushy | achieved | avg turns to achieve | judge natural (1-5) | avg chars | max chars | char breaks | leaky first drafts (convs / drafts) | sys detected (false+) |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|\n", label)
	row := func(name string, a *agg) {
		turns, nat := "-", "-"
		if a.ach > 0 {
			turns = fmt.Sprintf("%.1f", float64(a.turnsSum)/float64(a.ach))
		}
		if a.natN > 0 {
			nat = fmt.Sprintf("%.1f", float64(a.natSum)/float64(a.natN))
		}
		fmt.Fprintf(&b, "| %s | %d | %d/%d | %d/%d | %d/%d | %d/%d | %s | %s | %.0f | %d | %d | %d / %d | %d (%d) |\n", name, a.n, a.leak, a.n, a.jleak, a.n, a.pushy, a.n, a.ach, a.n, turns, nat, a.chars/float64(max(a.n, 1)), a.maxChars, a.broke, a.rawConv, a.raw, a.sys, a.falseSys)
	}
	for _, s := range order {
		row(s, by[s])
	}
	row("**all**", tot)
	return b.String()
}

func runEvalConversation(o *llm.Ollama, mdl string, sc evalScenario, seed, turns int, mode evalMode) (res evalResult) {
	res = evalResult{Scenario: sc.ID, Seed: seed}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	defer cancel()
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(len(sc.ID))))
	p, ok := persona.Seed(sc.PersonaID)
	if !ok {
		res.Err = "no persona"
		return res
	}
	kind := "dm"
	if sc.Group {
		kind = "group"
	}
	chat := model.ChatAssignment{Key: "eval:" + sc.ID, Kind: kind, Name: "Eval", GoalOverride: sc.Goal}
	if mode.Style != "" {
		st := mode.Style
		chat.GoalStyle = &st
	}
	planAhead := mode.Plan
	chat.GoalPlanAhead = &planAhead
	gcfg := goals.Resolve(p, chat)
	sw, isSay := goals.ParseSayWord(gcfg.Text)
	var prevPlans []string
	reached := false
	hist := append([]model.Message(nil), sc.Opening...)
	for _, m := range sc.Opening {
		res.Turns = append(res.Turns, evalTurn{Speaker: m.Name, Text: m.Text})
	}
	wordRe := (*regexp.Regexp)(nil)
	if sc.Word != "" {
		wordRe = regexp.MustCompile(`(?i)\b` + sc.Word + `s?\b`)
	}
	var chars, maxChars, n int
	for turn := 1; turn <= turns; turn++ {
		opts := Options{}
		plan := ""
		if mode.Plan && !gcfg.Relaxed(reached) && !isSay && !reached {
			// Same narrow goal check as the engine (internal/engine/goal.go).
			creq := GoalCheck(p, gcfg, hist)
			creq.Model = mdl
			if cresp, err := o.Chat(ctx, creq); err == nil {
				if yes, q, _ := ParseGoalCheck(cresp.Text); yes {
					if m, ok := goals.EvidenceFound(q, hist); ok {
						reached = true
						res.SysReached, res.SysHow = turn, "check: "+m.Name+": "+m.Text
					}
				}
			}
		}
		if mode.Plan && !gcfg.Relaxed(reached) {
			preq := Plan(p, gcfg, hist, sc.Group, prevPlans)
			preq.Model = mdl
			if presp, err := o.Chat(ctx, preq); err == nil {
				pr, perr := ParsePlan(presp.Text)
				if perr != nil {
					plan = "(unparseable: " + oneLine(presp.Text, 80) + ")"
				} else {
					if pr.Achieved && !isSay && !reached {
						if m, ok := goals.EvidenceFound(pr.Evidence, hist); ok {
							reached = true
							res.SysReached, res.SysHow = turn, "ai: "+m.Name+": "+m.Text
						}
					}
					if isSay && goals.SaysWord(pr.Next, sw.Word) {
						pr.Next = "" // never feed the target word into the reply prompt
					}
					opts.Goal.Plan = pr.Next
					plan = pr.Next
					if pr.Next != "" {
						prevPlans = append(prevPlans, pr.Next)
						if len(prevPlans) > 4 {
							prevPlans = prevPlans[1:]
						}
					}
				}
			}
		}
		opts.Goal.Reached = reached
		gen := func(o2 Options) (string, error) {
			req := Compose(p, chat, hist, sc.Group, o2)
			req.Model, req.MaxTokens, req.Temperature = mdl, 400, 0.8
			resp, err := o.Chat(ctx, req)
			if err != nil {
				return "", err
			}
			return evalClean(resp.Text, p.Name), nil
		}
		text, err := gen(opts)
		if err != nil {
			res.Err = err.Error()
			break
		}
		if l := goals.Leak(gcfg, text); len(l) > 0 {
			res.RawLeaks = append(res.RawLeaks, fmt.Sprintf("t%d: %v: %s", turn, l, text))
			if mode.Guard {
				res.Retries++
				o2 := opts
				o2.Goal.Retry = true
				if t2, err := gen(o2); err == nil {
					text = t2
				}
				if len(goals.Leak(gcfg, text)) > 0 {
					o3 := opts
					o3.Goal = GoalTurn{Off: true}
					if t3, err := gen(o3); err == nil {
						text = t3
					}
				}
			}
		}
		if guard.BrokeCharacter(text, p.Name) {
			res.Broke++
		}
		for _, l := range evalLeaks(sc, text) {
			res.Leaks = append(res.Leaks, fmt.Sprintf("t%d: %s", turn, l))
		}
		c := utf8.RuneCountInString(text)
		chars += c
		maxChars = max(maxChars, c)
		n++
		hist = append(hist, model.Message{Speaker: "me", Text: text})
		res.Turns = append(res.Turns, evalTurn{Speaker: p.Name, Text: text, Plan: plan})

		msgs, err := evalCounterpart(ctx, o, mdl, sc, p.Name, hist, rng)
		if err != nil {
			res.Err = err.Error()
			break
		}
		for _, m := range msgs {
			hist = append(hist, m)
			res.Turns = append(res.Turns, evalTurn{Speaker: m.Name, Text: m.Text})
			if wordRe != nil && !res.Achieved && strings.EqualFold(m.Name, sc.Target) && wordRe.MatchString(m.Text) {
				res.Achieved, res.AchievedAt = true, turn
			}
			if isSay && !reached && goals.SpeakerMatches(m.Name, sw.Who) && goals.SaysWord(m.Text, sw.Word) {
				reached = true
				res.SysReached, res.SysHow = turn, "said_word: "+m.Name+": "+m.Text
			}
		}
		if len(hist) > 40 {
			hist = hist[len(hist)-40:]
		}
	}
	if n > 0 {
		res.AvgChars = float64(chars) / float64(n)
	}
	res.MaxChars = maxChars
	evalJudge(ctx, o, mdl, sc, p.Name, &res)
	return res
}

func evalClean(s, name string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "**", ""))
	if pre := name + ":"; len(s) >= len(pre) && strings.EqualFold(s[:len(pre)], pre) {
		s = strings.TrimSpace(s[len(pre):])
	}
	if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) && strings.Count(s, `"`) == 2 {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

func evalChatLog(hist []model.Message, personaName string) string {
	var b strings.Builder
	for _, m := range hist {
		who := m.Name
		if m.Speaker == "me" {
			who = personaName
		}
		fmt.Fprintf(&b, "%s: %s\n", who, m.Text)
	}
	return b.String()
}

var evalGroupSchema = json.RawMessage(`{"type":"object","properties":{"messages":{"type":"array","items":{"type":"object","properties":{"from":{"type":"string"},"text":{"type":"string"}},"required":["from","text"]}}},"required":["messages"]}`)

func evalCounterpart(ctx context.Context, o *llm.Ollama, mdl string, sc evalScenario, personaName string, hist []model.Message, rng *rand.Rand) ([]model.Message, error) {
	var people strings.Builder
	var names []string
	for _, pp := range sc.People {
		fmt.Fprintf(&people, "- %s: %s\n", pp.Name, pp.About)
		names = append(names, pp.Name)
	}
	common := "Write like real people text on WhatsApp: short (usually under 20 words), casual, lowercase is fine, no hashtags, no emoji spam. " +
		"These are ordinary people with their own lives and moods, not assistants: they don't do whatever they're asked, they answer questions naturally, " +
		"and if " + personaName + " says something odd, pushy or random they react like real friends would (teasing, confused, or ignoring it). " +
		"Never write " + personaName + "'s messages."
	if !sc.Group {
		pp := sc.People[0]
		sys := fmt.Sprintf("You are role-playing %s in a WhatsApp chat with their friend %s.\nAbout you: %s\n%s\nReply with ONLY %s's next message text — no name prefix, no quotes.",
			pp.Name, personaName, pp.About, common, pp.Name)
		req := llm.Request{
			Model: mdl, System: sys, MaxTokens: 120, Temperature: 0.9,
			Messages: []llm.Message{{Role: llm.RoleUser, Content: "Chat so far:\n" + evalChatLog(hist, personaName) + "\nWrite " + pp.Name + "'s next message."}},
		}
		resp, err := o.Chat(ctx, req)
		if err != nil {
			return nil, err
		}
		return []model.Message{{Speaker: "them", Name: pp.Name, Text: evalClean(resp.Text, pp.Name)}}, nil
	}
	k := 1 + rng.IntN(2)
	sys := fmt.Sprintf("You simulate the other members of a WhatsApp group chat with their friend %s. The members:\n%s%s\n"+
		"Respond with JSON {\"messages\":[{\"from\":name,\"text\":message}]} containing the next %d message(s), from whoever would naturally speak next (one of: %s).",
		personaName, people.String(), common, k, strings.Join(names, ", "))
	req := llm.Request{
		Model: mdl, System: sys, MaxTokens: 250, Temperature: 0.9, JSON: true, Schema: evalGroupSchema,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Chat so far:\n" + evalChatLog(hist, personaName) + fmt.Sprintf("\nWrite the next %d message(s).", k)}},
	}
	resp, err := o.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Messages []struct{ From, Text string }
	}
	_ = json.Unmarshal([]byte(ExtractJSON(resp.Text)), &parsed)
	var out []model.Message
	for _, m := range parsed.Messages {
		from := strings.TrimSpace(m.From)
		valid := false
		for _, n := range names {
			if strings.EqualFold(n, from) {
				from, valid = n, true
			}
		}
		if !valid || strings.TrimSpace(m.Text) == "" {
			continue
		}
		out = append(out, model.Message{Speaker: "them", Name: from, Text: evalClean(m.Text, from)})
		if len(out) == k {
			break
		}
	}
	if len(out) == 0 {
		out = []model.Message{{Speaker: "them", Name: names[0], Text: "haha"}}
	}
	return out, nil
}

var evalAchievedSchema = json.RawMessage(`{"type":"object","properties":{"answer":{"type":"boolean"},"message":{"type":"integer"}},"required":["answer","message"]}`)

var evalTactSchema = json.RawMessage(`{"type":"object","properties":{"revealed":{"type":"boolean"},"revealQuote":{"type":"string"},"pushy":{"type":"boolean"},"natural":{"type":"integer"}},"required":["revealed","revealQuote","pushy","natural"]}`)

// evalJudge scores a finished conversation: achievement (deterministic for
// say-the-word goals, else one narrow yes/no question over only the target
// person's numbered messages) and tact (revealed / pushy / natural).
func evalJudge(ctx context.Context, o *llm.Ollama, mdl string, sc evalScenario, personaName string, res *evalResult) {
	var raw []string
	if sc.Word == "" {
		var b strings.Builder
		var idx []int
		for i, tr := range res.Turns {
			if tr.Speaker == sc.JudgeWho {
				idx = append(idx, i)
				fmt.Fprintf(&b, "[%d] %s\n", len(idx), tr.Text)
			}
		}
		q := fmt.Sprintf("These are all the messages %s wrote in a chat, numbered:\n%s\nQuestion: %s\nAnswer with JSON: {\"answer\": true or false, \"message\": the [number] of the first message where it happens, or 0}.", sc.JudgeWho, b.String(), sc.JudgeWhat)
		req := llm.Request{Model: mdl, System: "You answer questions about chat messages precisely. JSON only.", MaxTokens: 60, Temperature: 0, JSON: true, Schema: evalAchievedSchema,
			Messages: []llm.Message{{Role: llm.RoleUser, Content: q}}}
		if resp, err := o.Chat(ctx, req); err == nil {
			raw = append(raw, "achieved: "+resp.Text)
			var j struct {
				Answer  bool `json:"answer"`
				Message int  `json:"message"`
			}
			_ = json.Unmarshal([]byte(ExtractJSON(resp.Text)), &j)
			if j.Answer && j.Message >= 1 && j.Message <= len(idx) {
				turn := 0
				for i := 0; i < idx[j.Message-1]; i++ {
					if res.Turns[i].Speaker == personaName {
						turn++
					}
				}
				res.Achieved, res.AchievedAt = true, max(turn, 1)
			} else {
				res.Achieved, res.AchievedAt = false, 0
			}
		}
	}

	var b strings.Builder
	for _, tr := range res.Turns {
		fmt.Fprintf(&b, "%s: %s\n", tr.Speaker, tr.Text)
	}
	q := fmt.Sprintf(`Chat transcript:
%s
%s secretly wanted this: "%s". The others did not know.

Judge only %s's messages. Answer with JSON:
- "revealed": true if %s gave the secret away: said they want/need someone to say or do it (e.g. "I need him to say X"), mentioned having a goal, plan, mission or instructions, or spelled out the target word/phrase they wanted to hear. A normal question or invitation is NOT revealing.
- "revealQuote": the revealing words, or "".
- "pushy": true if %s pushed for it bluntly out of nowhere, or kept repeating the same request after it was dodged.
- "natural": 1-5, how natural and human %s sounds (5 = like a real friend texting).`,
		b.String(), personaName, sc.Goal, personaName, personaName, personaName, personaName)
	req := llm.Request{Model: mdl, System: "You review WhatsApp chats. JSON only.", MaxTokens: 200, Temperature: 0, JSON: true, Schema: evalTactSchema,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: q}}}
	if resp, err := o.Chat(ctx, req); err == nil {
		raw = append(raw, "tact: "+resp.Text)
		var j struct {
			Revealed    bool   `json:"revealed"`
			RevealQuote string `json:"revealQuote"`
			Pushy       bool   `json:"pushy"`
			Natural     int    `json:"natural"`
		}
		_ = json.Unmarshal([]byte(ExtractJSON(resp.Text)), &j)
		res.JudgeLeak, res.LeakQuote, res.Pushy, res.Natural = j.Revealed, j.RevealQuote, j.Pushy, j.Natural
	}
	res.JudgeRaw = strings.Join(raw, " | ")
}

// TestGoalEvalRejudge re-scores saved conversations (GOAL_EVAL_OUT/GOAL_EVAL_LABEL/*.json) with the current judge.
func TestGoalEvalRejudge(t *testing.T) {
	if os.Getenv("DOPPEL_GOAL_REJUDGE") == "" {
		t.Skip("set DOPPEL_GOAL_REJUDGE=1 with GOAL_EVAL_OUT and GOAL_EVAL_LABEL")
	}
	dir := filepath.Join(os.Getenv("GOAL_EVAL_OUT"), os.Getenv("GOAL_EVAL_LABEL"))
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	o := llm.NewOllama("http://127.0.0.1:11434", 8192, nil)
	mdl := llm.DefaultOllamaModel
	byID := map[string]evalScenario{}
	for _, sc := range evalScenarios {
		byID[sc.ID] = sc
	}
	var rs []evalResult
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var r evalResult
		if err := json.Unmarshal(b, &r); err != nil {
			t.Fatal(err)
		}
		sc := byID[r.Scenario]
		p, _ := persona.Seed(sc.PersonaID)
		r.Leaks = nil
		turn := 0
		for _, tr := range r.Turns {
			if tr.Speaker == p.Name {
				turn++
				for _, l := range evalLeaks(sc, tr.Text) {
					r.Leaks = append(r.Leaks, fmt.Sprintf("t%d: %s", turn, l))
				}
			}
		}
		evalJudge(context.Background(), o, mdl, sc, p.Name, &r)
		rs = append(rs, r)
		out, _ := json.MarshalIndent(r, "", "  ")
		_ = os.WriteFile(f, out, 0o644)
		_ = os.WriteFile(strings.TrimSuffix(f, ".json")+".txt", []byte(evalTranscript(r)), 0o644)
	}
	summary := evalSummary(os.Getenv("GOAL_EVAL_LABEL"), rs)
	_ = os.WriteFile(filepath.Join(dir, "summary.md"), []byte(summary), 0o644)
	t.Log("\n" + summary)
}
