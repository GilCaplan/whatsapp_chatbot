package engine

// Expression eval: emoji usage, message length, reactions and @tags of the
// built-in personas against a real local Ollama, through the engine's reply
// path (prompt options → prompt.Compose → generateReply → expressDraft →
// applyMentions). Not part of the normal test run:
//
//	DOPPEL_EXPR_EVAL=1 EXPR_EVAL_OUT=/path/to/dir EXPR_EVAL_LABEL=after EXPR_EVAL_MODE=after \
//	  go test ./internal/engine -run TestExpressionEval -v -timeout 2h
//
// EXPR_EVAL_MODE: after (default: new prompt + enforcement), raw (new prompt,
// no enforcement) or before (the previous prompt text, no enforcement,
// random-favourite reactions). Records are appended to <label>.jsonl as they
// finish and an interrupted run resumes; <label>.md is the summary.
// Optional: EXPR_EVAL_PARTS (emoji,length,tags,reactions — default all),
// EXPR_EVAL_REPS (default 2; emoji sweep repetitions), EXPR_EVAL_PAR
// (default 3), EXPR_EVAL_MODEL (default llama3.1:8b), EXPR_EVAL_NO_MATCH=1.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/mention"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/persona"
	"whatsappdoppel/internal/prompt"
	"whatsappdoppel/internal/store"
)

type evalMsg struct {
	ID, Cat, Text string
}

// Categories: joke, good, sad (serious/bad news), thanks, surprise, plan,
// question, rant (venting), short (one word).
var exprMsgs = []evalMsg{
	{"joke1", "joke", "why did the scarecrow win an award? because he was outstanding in his field"},
	{"joke2", "joke", "my cat just knocked my coffee onto my laptop and then looked me dead in the eye lmao"},
	{"good1", "good", "I GOT THE JOB!!! starting next month"},
	{"good2", "good", "we're engaged!! he proposed last night on the beach"},
	{"sad1", "sad", "my grandmother passed away this morning"},
	{"sad2", "sad", "I got laid off today. not really sure what to do now"},
	{"sad3", "sad", "honestly I've been feeling really depressed lately and can't get out of bed"},
	{"thanks1", "thanks", "thank you so much for yesterday, you're the best"},
	{"surprise1", "surprise", "wait, Noa and Tom broke up??"},
	{"plan1", "plan", "drinks on friday at 8?"},
	{"plan2", "plan", "I booked the cabin for the weekend, you in?"},
	{"q1", "question", "what time should we meet tomorrow?"},
	{"q2", "question", "do you know any good restaurants in Tel Aviv?"},
	{"rant1", "rant", "ok I need to vent. my landlord AGAIN didn't fix the heater, it's been three weeks, he keeps saying tomorrow tomorrow and then disappears, meanwhile I'm sleeping in two hoodies and the kids are cold and I'm paying full rent on time every month. I'm so done with this guy, seriously thinking of withholding rent or just moving out"},
	{"rant2", "rant", "today's meeting was a nightmare. my manager presented my analysis as his own idea in front of the whole team, then asked me why the numbers were late when he's the one who changed the scope twice. I just sat there smiling like an idiot. I don't even know if I should say something or just let it go"},
	{"short1", "short", "ok"},
	{"short2", "joke", "lol"},
	{"short3", "short", "hey"},
	{"he_joke", "joke", "חחחח אתה לא תאמין מה קרה לי היום בסופר, נתקעתי עם העגלה בדלת"},
	{"he_sad", "sad", "סבא שלי נפטר אתמול, אני הרוס"},
	{"he_good", "good", "קיבלתי את הדירה!!! סוף סוף"},
	{"he_q", "question", "מה אתה עושה בסופ״ש?"},
}

// lengthMsgs is the subset used by the length sweep.
var lengthMsgIDs = []string{"joke1", "good1", "sad2", "he_sad", "q1", "q2", "plan1", "rant1", "rant2", "short1", "short3", "he_q"}

var exprPersonas = []string{"leo", "kyle", "luna", "brad", "chad"}

// Acceptable reactions per category; laughing on sad/rant is "severe".
var reactOK = map[string]string{
	"joke":     "😂 🤣 😆 💀 😅 😹 🙈 😄 😁",
	"good":     "🎉 🥳 🔥 ❤️ 👏 🙌 💪 🍾 🥂 🍻 🍸 🤩 😍 ✨ 💯 🚀 ⭐ 🏆 👑 🙏 😊 🥰",
	"sad":      "❤️ 🙏 😢 😔 🫂 💔 🤍 😞 🥺 🫶 😥",
	"thanks":   "❤️ 🥰 😘 🤗 🙏 🫶 💕 😊 🤍 ✨ 🥹 😍",
	"surprise": "😮 😱 👀 🤯 😲 😳 🙀 🫢 🍵 😯",
	"plan":     "👍 👌 🤝 🙌 ✅ 🔥 💯 🍻 🥂 👏 😎 🎉 🍸 💪",
	"question": "🤔 👀 👍",
	"short":    "👍 👋 🙂 😊 ❤️ 👌 🤙",
	"rant":     "😤 😩 🙄 😢 ❤️ 🙏 😬 🫂 💔 😮‍💨 😡 🤦 😔",
}

var laughing = "😂 🤣 😆 💀 😅 😹 😄 😁"

// tag scenarios (group, members from pgRoster). {P} is the persona's name.
type tagScenario struct {
	ID      string
	Hist    [][2]string // name, text
	Allowed []string    // members it may tag; empty = none
}

var tagScenarios = []tagScenario{
	{"direct", [][2]string{{"Noam", "anyone watching the game tonight?"}, {"Dana", "nah busy"}, {"Maya", "{P} what do you think of my new haircut? be honest"}}, []string{"Maya"}},
	{"two_asks", [][2]string{{"Dana", "{P} are you coming on saturday?"}, {"Noam", "{P} also did you see my message about the money?"}}, []string{"Dana", "Noam"}},
	{"chatter", [][2]string{{"Dana", "this weather is insane"}, {"Eitan", "right?? 35 degrees in october"}, {"Maya", "i'm literally melting"}}, nil},
	{"pull_in", [][2]string{{"Dana", "has anyone heard from Eitan? he's been quiet all week"}, {"Maya", "no idea, maybe he's travelling"}}, []string{"Eitan"}},
	{"general", [][2]string{{"Noam", "who's in for drinks on friday?"}}, []string{"Noam"}},
}

type tagCfg struct {
	Name  string
	Allow bool
	Max   int
}

var tagCfgs = []tagCfg{{"off", false, 1}, {"on_max1", true, 1}, {"on_max3", true, 3}}

type exprRec struct {
	Key          string   `json:"key"`
	Part         string   `json:"part"`
	Persona      string   `json:"persona"`
	Usage        string   `json:"usage,omitempty"`
	Length       string   `json:"length,omitempty"`
	Bias         string   `json:"bias,omitempty"`
	Msg          string   `json:"msg,omitempty"`
	Cat          string   `json:"cat,omitempty"`
	InWords      int      `json:"inWords,omitempty"`
	Scenario     string   `json:"scenario,omitempty"`
	Cfg          string   `json:"cfg,omitempty"`
	Raw          string   `json:"raw"`
	Text         string   `json:"text"`
	Emoji        int      `json:"emoji"`
	Words        int      `json:"words"`
	Chars        int      `json:"chars"`
	RawAt        int      `json:"rawAt,omitempty"`
	Tagged       []string `json:"tagged,omitempty"`
	Invalid      int      `json:"invalid,omitempty"`
	Approp       bool     `json:"approp"`
	Retried      bool     `json:"lengthRetried,omitempty"`
	Trimmed      bool     `json:"trimmed,omitempty"`
	EmojiRemoved int      `json:"emojiRemoved,omitempty"`
	EmojiAdded   bool     `json:"emojiAdded,omitempty"`
	RawEmoji     int      `json:"rawEmoji"`
	RawWords     int      `json:"rawWords"`
	Latency      int64    `json:"latencyMs"`
	Err          string   `json:"err,omitempty"`
}

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v > 0 {
		return v
	}
	return def
}

func newEvalEngine(t *testing.T, mdl string) *Engine {
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Update(func(s *config.Settings) {
		s.LLM.DefaultProvider, s.LLM.OllamaURL, s.LLM.OllamaModel = "ollama", "http://127.0.0.1:11434", mdl
	}); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(paths, persona.Seeds())
	if err != nil {
		t.Fatal(err)
	}
	hub := events.NewHub()
	reg := llm.NewRegistry(cfg, hub)
	wa := &fakeWA{own: []string{ownJID, "99999999999@lid"}, fail: map[string]bool{}}
	e := New(Deps{Config: cfg, Store: st, Hub: hub, WA: wa, LLM: reg, Sampler: behavior.NewSampler(nil)})
	t.Cleanup(e.Stop)
	return e
}

var atTok = regexp.MustCompile(`@[\p{L}\p{N}_]+`)

func wordCount(s string) int {
	n := 0
	for _, f := range strings.Fields(prompt.StripEmoji(s)) {
		if strings.IndexFunc(f, func(r rune) bool { return r > ' ' && !strings.ContainsRune(".,!?;:-–—…\"'()", r) }) >= 0 {
			n++
		}
	}
	return n
}

// evalReply runs one reply through the engine path (as pipeline.generate)
// and returns the result, the model's raw draft and the people tagged.
func evalReply(ctx context.Context, e *Engine, mode string, p model.Persona, c model.ChatAssignment, bp model.BehaviorProfile, hist []model.Message, dir *mention.Directory, answered pendingMsg) (genResult, string, []mention.Entry, error) {
	opts := e.promptOptions(c, bp, dir, hist)
	isGroup := c.Kind == "group"
	last := hist[len(hist)-1].Text
	build := func(t prompt.GoalTurn) llm.Request {
		opts.Goal = t
		r := prompt.Compose(p, c, hist, isGroup, opts)
		if mode == "before" {
			r.System = legacySystem(r.System, p, last, bp.LengthBias)
		}
		return r
	}
	res, err := e.generateReply(ctx, p, build(prompt.GoalTurn{}))
	if err != nil {
		return res, "", nil, err
	}
	rawText := res.Text
	if mode == "after" {
		res = e.expressDraft(ctx, p, bp, hist, false, res, func(note string) (genResult, error) {
			r := build(prompt.GoalTurn{})
			r.System += note
			return e.generateReply(ctx, p, r)
		})
	}
	var tagged []mention.Entry
	res.Text, tagged = e.applyMentions(res.Text, dir, bp, answered, hist, false)
	return res, rawText, tagged, nil
}

// ─── the previous prompt text (BEFORE baseline), verbatim ────

func legacyEmojiRule(e model.EmojiPrefs) string {
	favs := ""
	if len(e.Favorites) > 0 {
		favs = ", ideally from: " + strings.Join(e.Favorites, " ")
	}
	switch e.Usage {
	case "none":
		return "Never use emoji."
	case "some":
		return "Use an emoji now and then, at most two per message" + favs + "."
	case "lots":
		return "Use emoji freely" + favs + "."
	default:
		return "At most one emoji per message, and only sometimes" + favs + "."
	}
}

func legacyLengthRule(l string) string {
	switch l {
	case "medium":
		return "2-3 sentences."
	case "long":
		return "Up to a short paragraph."
	default:
		return "1-2 short sentences, no paragraphs."
	}
}

func legacyGuidance(messageLength, lastMsg, bias string) string {
	if bias == "shorter" || bias == "longer" {
		ladder := []string{"short", "medium", "long"}
		i := map[string]int{"medium": 1, "long": 2}[messageLength]
		if bias == "shorter" {
			i = max(i-1, 0)
		} else {
			i = min(i+1, 2)
		}
		messageLength = ladder[i]
	}
	long := len(strings.Fields(lastMsg)) > 10
	switch messageLength {
	case "medium":
		if long {
			return "Moderate length. 2-3 sentences max."
		}
		return "Keep it brief. 1-2 sentences."
	case "long":
		if long {
			return "You can go a bit longer: up to a short paragraph."
		}
		return "Keep it fairly brief. 1-3 sentences."
	default:
		if long {
			return "Moderate length. 2-3 sentences max."
		}
		return "Keep it ultra brief. One short sentence."
	}
}

// legacyTagRule is the previous "Tagging:" sentence of PeopleSection
// ({first} = first listed name, {per} = "one person" / "N people").
const legacyTagRule = "Tagging: you may address someone by writing @ followed by their name exactly as listed (e.g. @{first}) — only when it helps: speaking to one person directly, answering one person while several are talking, or pulling someone into the conversation. At most {per} per reply, never tag everyone, and most replies should tag nobody."

// legacySystem rewrites a current system prompt into the previous one: the
// emoji and length lines, the tagging rule and the GUIDANCE tail.
func legacySystem(sys string, p model.Persona, lastMsg, bias string) string {
	lines := strings.Split(sys, "\n")
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "- Emoji: "):
			lines[i] = "- Emoji: " + legacyEmojiRule(p.Emoji)
		case strings.HasPrefix(l, "- Length: "):
			lines[i] = "- Length: " + legacyLengthRule(p.MessageLength)
		case strings.HasPrefix(l, "Tagging: "):
			first := ""
			if j := strings.Index(l, "(e.g. @"); j >= 0 {
				first = strings.SplitN(l[j+len("(e.g. @"):], ")", 2)[0]
			}
			per := "one person"
			for n := 2; n <= 5; n++ {
				if strings.Contains(l, fmt.Sprintf("%d people", n)) {
					per = fmt.Sprintf("%d people", n)
				}
			}
			lines[i] = strings.NewReplacer("{first}", first, "{per}", per).Replace(legacyTagRule)
		}
	}
	sys = strings.Join(lines, "\n")
	if i := strings.LastIndex(sys, "\n\nGUIDANCE: "); i >= 0 {
		sys = sys[:i] + "\n\nGUIDANCE: " + legacyGuidance(p.MessageLength, lastMsg, bias)
	}
	return sys
}

// evalPickReaction is the reaction the engine would send to text.
func evalPickReaction(e *Engine, mode string, p model.Persona, text string) string {
	if mode == "before" {
		return behavior.PickReaction(p, e.rng)
	}
	emoji, _, _ := e.reactionFor(p, "Dana", text)
	return emoji
}

func TestExpressionEval(t *testing.T) {
	if os.Getenv("DOPPEL_EXPR_EVAL") == "" {
		t.Skip("set DOPPEL_EXPR_EVAL=1 to run against a local Ollama")
	}
	outDir := os.Getenv("EXPR_EVAL_OUT")
	if outDir == "" {
		outDir = t.TempDir()
	}
	label := os.Getenv("EXPR_EVAL_LABEL")
	if label == "" {
		label = "run"
	}
	mdl := os.Getenv("EXPR_EVAL_MODEL")
	if mdl == "" {
		mdl = "llama3.1:8b"
	}
	parts := os.Getenv("EXPR_EVAL_PARTS")
	if parts == "" {
		parts = "emoji,length,tags,reactions"
	}
	reps, par := envInt("EXPR_EVAL_REPS", 2), envInt("EXPR_EVAL_PAR", 3)
	mode := os.Getenv("EXPR_EVAL_MODE")
	if mode == "" {
		mode = "after"
	}
	e := newEvalEngine(t, mdl)
	base := e.cfg.Get().Behavior

	msgByID := map[string]evalMsg{}
	for _, m := range exprMsgs {
		msgByID[m.ID] = m
	}
	type job struct {
		key string
		run func(ctx context.Context) exprRec
	}
	var jobs []job
	dmJob := func(part, pid, usage, length, bias string, m evalMsg, rep int) job {
		return job{fmt.Sprintf("%s|%s|%s|%s|%s|%s|%d", part, pid, usage, length, bias, m.ID, rep), func(ctx context.Context) exprRec {
			p, _ := persona.Seed(pid)
			p.Emoji.Usage, p.MessageLength = usage, length
			bp := base.Private
			bp.LengthBias = bias
			c := model.ChatAssignment{Key: "dm:eval", Kind: "dm", Name: "Dana", PersonaID: pid, Enabled: true}
			hist := []model.Message{{ID: "m1", TS: time.Now(), Speaker: "them", Name: "Dana", Text: m.Text}}
			rec := exprRec{Part: part, Persona: pid, Usage: usage, Length: length, Bias: bias, Msg: m.ID, Cat: m.Cat, InWords: wordCount(m.Text)}
			start := time.Now()
			res, rawText, _, err := evalReply(ctx, e, mode, p, c, bp, hist, nil, pendingMsg{ID: "m1"})
			rec.Latency = time.Since(start).Milliseconds()
			if err != nil {
				rec.Err = err.Error()
				return rec
			}
			rec.Raw, rec.Text = rawText, res.Text
			rec.Emoji, rec.Words, rec.Chars = prompt.CountEmoji(res.Text), wordCount(res.Text), utf8.RuneCountInString(res.Text)
			rec.Retried = res.LengthRetried
			rec.Trimmed, rec.EmojiRemoved, rec.EmojiAdded = res.Trimmed, res.EmojiRemoved, res.EmojiAdded != ""
			rec.RawEmoji, rec.RawWords = prompt.CountEmoji(rawText), wordCount(rawText)
			return rec
		}}
	}
	if strings.Contains(parts, "emoji") {
		for li, usage := range []string{"none", "rare", "some", "lots"} {
			for r := 0; r < reps; r++ {
				for mi, m := range exprMsgs {
					pid := exprPersonas[(mi+li+r*2)%len(exprPersonas)]
					jobs = append(jobs, dmJob("emoji", pid, usage, "short", "normal", m, r))
				}
			}
		}
	}
	if strings.Contains(parts, "length") {
		for li, length := range []string{"short", "medium", "long"} {
			for _, bias := range []string{"shorter", "normal", "longer", "match"} {
				if bias == "match" && os.Getenv("EXPR_EVAL_NO_MATCH") != "" {
					continue
				}
				for i, id := range lengthMsgIDs {
					// Same persona per message for every bias (and every run).
					jobs = append(jobs, dmJob("length", exprPersonas[(i+li)%len(exprPersonas)], "rare", length, bias, msgByID[id], 0))
				}
			}
		}
	}
	if strings.Contains(parts, "tags") {
		for _, pid := range []string{"leo", "luna"} {
			for _, sc := range tagScenarios {
				for _, tc := range tagCfgs {
					for r := 0; r < 2; r++ {
						jobs = append(jobs, job{fmt.Sprintf("tags|%s|%s|%s|%d", pid, sc.ID, tc.Name, r), func(ctx context.Context) exprRec {
							p, _ := persona.Seed(pid)
							bp := base.Group
							bp.AllowMentions, bp.MentionMax, bp.TagReplyPercent = tc.Allow, tc.Max, 0
							c := model.ChatAssignment{Key: "group:eval", Kind: "group", Name: "Friends", PersonaID: pid, Enabled: true}
							var hist []model.Message
							for i, h := range sc.Hist {
								who := pgRoster[slices.IndexFunc(pgRoster, func(x model.Participant) bool { return x.Name == h[0] })]
								hist = append(hist, model.Message{ID: fmt.Sprintf("g%d", i), TS: time.Now(), Speaker: "them", Name: who.Name, SenderJID: who.JID, Text: strings.ReplaceAll(h[1], "{P}", p.Name)})
							}
							last := hist[len(hist)-1]
							dir := e.directoryFrom(pgRoster, hist)
							rec := exprRec{Part: "tags", Persona: pid, Scenario: sc.ID, Cfg: tc.Name}
							start := time.Now()
							res, rawText, tagged, err := evalReply(ctx, e, mode, p, c, bp, hist, dir, pendingMsg{ID: last.ID, SenderJID: last.SenderJID, Name: last.Name})
							rec.Latency = time.Since(start).Milliseconds()
							if err != nil {
								rec.Err = err.Error()
								return rec
							}
							rec.Raw, rec.Text = rawText, res.Text
							rec.Emoji, rec.Words, rec.Chars = prompt.CountEmoji(res.Text), wordCount(res.Text), utf8.RuneCountInString(res.Text)
							rec.RawAt = len(atTok.FindAllString(rawText, -1))
							_, valid := mention.Normalize(rawText, dir, mention.Policy{Allow: true, Max: 99})
							rec.Invalid = max(0, rec.RawAt-len(valid))
							rec.Tagged = mention.Names(tagged)
							rec.Approp = true
							for _, n := range rec.Tagged {
								if !slices.Contains(sc.Allowed, n) {
									rec.Approp = false
								}
							}
							return rec
						}})
					}
				}
			}
		}
	}

	// Resume: records already in <label>.jsonl are kept and their jobs skipped.
	_ = os.MkdirAll(outDir, 0o755)
	path := filepath.Join(outDir, label+".jsonl")
	done := map[string]bool{}
	var recs []exprRec
	if data, err := os.ReadFile(path); err == nil {
		for _, l := range strings.Split(string(data), "\n") {
			var r exprRec
			if json.Unmarshal([]byte(l), &r) == nil && r.Key != "" && r.Err == "" {
				done[r.Key] = true
				recs = append(recs, r)
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, r := range recs {
		_ = enc.Encode(r)
	}
	var todo []job
	for _, j := range jobs {
		if !done[j.key] {
			todo = append(todo, j)
		}
	}
	t.Logf("%s (mode %s): %d LLM jobs (%d done before), parallelism %d", label, mode, len(jobs), len(jobs)-len(todo), par)
	var wg sync.WaitGroup
	sem := make(chan struct{}, par)
	var mu sync.Mutex
	n := 0
	for _, j := range todo {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			r := j.run(ctx)
			r.Key = j.key
			mu.Lock()
			defer mu.Unlock()
			recs = append(recs, r)
			if r.Err == "" {
				_ = enc.Encode(r)
			}
			n++
			if n%25 == 0 {
				t.Logf("%d/%d", n, len(todo))
			}
		}()
	}
	wg.Wait()

	all := recs
	if strings.Contains(parts, "reactions") {
		all = append(slices.Clone(recs), reactionRecs(e, mode)...)
	}
	sum := summarize(label+" ("+mode+")", all)
	_ = os.WriteFile(filepath.Join(outDir, label+".md"), []byte(sum), 0o644)
	t.Log("\n" + sum)
}

// reactionRecs picks a reaction for every message × persona (20 draws each).
func reactionRecs(e *Engine, mode string) []exprRec {
	var out []exprRec
	for _, pid := range exprPersonas {
		p, _ := persona.Seed(pid)
		for _, m := range exprMsgs {
			for i := 0; i < 20; i++ {
				emo := evalPickReaction(e, mode, p, m.Text)
				ok := strings.Contains(" "+reactOK[m.Cat]+" ", " "+emo+" ")
				out = append(out, exprRec{Part: "reactions", Persona: pid, Msg: m.ID, Cat: m.Cat, Text: emo, Approp: ok,
					Invalid: b2i((m.Cat == "sad" || m.Cat == "rant") && strings.Contains(" "+laughing+" ", " "+emo+" "))})
			}
		}
	}
	return out
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func pct(a, b int) string {
	if b == 0 {
		return "–"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(a)/float64(b))
}

func quantiles(xs []int) (p50, p90, mx int, mean float64) {
	if len(xs) == 0 {
		return
	}
	s := slices.Clone(xs)
	sort.Ints(s)
	sum := 0
	for _, x := range s {
		sum += x
	}
	return s[len(s)/2], s[int(math.Ceil(0.9*float64(len(s))))-1], s[len(s)-1], float64(sum) / float64(len(s))
}

func summarize(label string, recs []exprRec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Expression eval — %s\n\n", label)
	errs := 0
	for _, r := range recs {
		if r.Err != "" {
			errs++
		}
	}
	fmt.Fprintf(&b, "errors: %d\n\n", errs)

	// Emoji by level.
	b.WriteString("## Emoji by usage level (persona length short, bias normal)\n\n| usage | n | replies with emoji | emoji/reply | max in one | emoji on sad | emoji on sad+rant |\n|---|---|---|---|---|---|---|\n")
	for _, u := range []string{"none", "rare", "some", "lots"} {
		var n, with, tot, mx, sadN, sadWith, srN, srWith int
		for _, r := range recs {
			if r.Part != "emoji" || r.Usage != u || r.Err != "" {
				continue
			}
			n++
			tot += r.Emoji
			mx = max(mx, r.Emoji)
			if r.Emoji > 0 {
				with++
			}
			if r.Cat == "sad" {
				sadN++
				if r.Emoji > 0 {
					sadWith++
				}
			}
			if r.Cat == "sad" || r.Cat == "rant" {
				srN++
				if r.Emoji > 0 {
					srWith++
				}
			}
		}
		if n > 0 {
			fmt.Fprintf(&b, "| %s | %d | %s | %.2f | %d | %s (%d/%d) | %s |\n", u, n, pct(with, n), float64(tot)/float64(n), mx, pct(sadWith, sadN), sadWith, sadN, pct(srWith, srN))
		}
	}
	// Emoji by category for some/lots.
	b.WriteString("\n### Replies with emoji by message category (all levels except none)\n\n| category | n | with emoji |\n|---|---|---|\n")
	cats := []string{"joke", "good", "thanks", "surprise", "plan", "question", "short", "rant", "sad"}
	for _, c := range cats {
		var n, with int
		for _, r := range recs {
			if r.Part == "emoji" && r.Usage != "none" && r.Cat == c && r.Err == "" {
				n++
				if r.Emoji > 0 {
					with++
				}
			}
		}
		fmt.Fprintf(&b, "| %s | %d | %s |\n", c, n, pct(with, n))
	}

	// Length grid.
	b.WriteString("\n## Words per reply by messageLength × lengthBias (emoji rare)\n\n| length | bias | n | mean | p50 | p90 | max | over hard limit | retried | trimmed |\n|---|---|---|---|---|---|---|---|---|---|\n")
	for _, l := range []string{"short", "medium", "long"} {
		for _, bi := range []string{"shorter", "normal", "longer", "match"} {
			var ws []int
			ret, trim, over := 0, 0, 0
			for _, r := range recs {
				if r.Part == "length" && r.Length == l && r.Bias == bi && r.Err == "" {
					ws = append(ws, r.Words)
					ret += b2i(r.Retried)
					trim += b2i(r.Trimmed)
					in := ""
					for _, m := range exprMsgs {
						if m.ID == r.Msg {
							in = m.Text
						}
					}
					over += b2i(r.Words > prompt.HardLimit(prompt.WordBudget(l, bi, in)))
				}
			}
			if len(ws) > 0 {
				p50, p90, mx, mean := quantiles(ws)
				fmt.Fprintf(&b, "| %s | %s | %d | %.1f | %d | %d | %d | %s | %d | %d |\n", l, bi, len(ws), mean, p50, p90, mx, pct(over, len(ws)), ret, trim)
			}
		}
	}
	b.WriteString("\n### Words per reply by incoming message (length part, all settings)\n\n| msg | in words | mean out | short/normal out |\n|---|---|---|---|\n")
	for _, id := range lengthMsgIDs {
		var ws []int
		sn := -1
		in := 0
		for _, r := range recs {
			if r.Part == "length" && r.Msg == id && r.Err == "" {
				ws = append(ws, r.Words)
				in = r.InWords
				if r.Length == "short" && r.Bias == "normal" {
					sn = r.Words
				}
			}
		}
		if len(ws) > 0 {
			_, _, _, mean := quantiles(ws)
			fmt.Fprintf(&b, "| %s | %d | %.1f | %d |\n", id, in, mean, sn)
		}
	}

	// Reactions.
	b.WriteString("\n## Reactions (20 draws per persona × message)\n\n| persona | appropriate | laughing at sad/rant |\n|---|---|---|\n")
	for _, pid := range append(slices.Clone(exprPersonas), "all") {
		var n, ok, bad, badN int
		for _, r := range recs {
			if r.Part != "reactions" || (pid != "all" && r.Persona != pid) {
				continue
			}
			n++
			ok += b2i(r.Approp)
			if r.Cat == "sad" || r.Cat == "rant" {
				badN++
				bad += r.Invalid
			}
		}
		if n > 0 {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", pid, pct(ok, n), pct(bad, badN))
		}
	}
	b.WriteString("\n| category | appropriate | most common |\n|---|---|---|\n")
	for _, c := range cats {
		var n, ok int
		count := map[string]int{}
		for _, r := range recs {
			if r.Part == "reactions" && r.Cat == c {
				n++
				ok += b2i(r.Approp)
				count[r.Text]++
			}
		}
		type kv struct {
			k string
			v int
		}
		var kvs []kv
		for k, v := range count {
			kvs = append(kvs, kv{k, v})
		}
		sort.Slice(kvs, func(i, j int) bool { return kvs[i].v > kvs[j].v || kvs[i].v == kvs[j].v && kvs[i].k < kvs[j].k })
		var top []string
		for i := 0; i < min(4, len(kvs)); i++ {
			top = append(top, fmt.Sprintf("%s×%d", kvs[i].k, kvs[i].v))
		}
		if n > 0 {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", c, pct(ok, n), strings.Join(top, " "))
		}
	}

	// Tags.
	b.WriteString("\n## @tags in groups (Leo, Luna; tagReplyPercent 0)\n\n| config | scenario | n | tag rate | tags/msg | raw @ in LLM text | invalid raw @ | appropriate |\n|---|---|---|---|---|---|---|---|\n")
	for _, tc := range tagCfgs {
		for _, sc := range append(slices.Clone(tagScenarios), tagScenario{ID: "ALL"}) {
			var n, with, tags, rawAt, inv, ok int
			for _, r := range recs {
				if r.Part != "tags" || r.Cfg != tc.Name || (sc.ID != "ALL" && r.Scenario != sc.ID) || r.Err != "" {
					continue
				}
				n++
				if len(r.Tagged) > 0 {
					with++
				}
				tags += len(r.Tagged)
				rawAt += r.RawAt
				inv += r.Invalid
				ok += b2i(r.Approp)
			}
			if n > 0 {
				fmt.Fprintf(&b, "| %s | %s | %d | %s | %.2f | %d | %d | %s |\n", tc.Name, sc.ID, n, pct(with, n), float64(tags)/float64(n), rawAt, inv, pct(ok, n))
			}
		}
	}
	return b.String()
}
