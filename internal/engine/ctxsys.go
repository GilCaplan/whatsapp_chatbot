package engine

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"whatsappdoppel/internal/ctxsys"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
)

// Context systems (ctxsys): each chat resolves one (chat → persona → app)
// and its knobs steer joining in (decide.go), goal planning (goal.go), the
// reply check (replycheck.go), background work (digest.go), the summary
// format (context.go) and the prompt's shape (shapePrompt below).

// systemOf is a chat's context system and where it comes from.
func (e *Engine) systemOf(c model.ChatAssignment) (model.ContextSystem, model.ContextChoice) {
	pid := ""
	if p, ok := e.store.Persona(c.PersonaID); ok {
		pid = p.ContextSystem
	}
	return ctxsys.Resolve(c.ContextSystem, pid, e.cfg.Get().Context.System, e.store.ContextSystem)
}

// paramsOf is a chat's context-system knobs.
func (e *Engine) paramsOf(c model.ChatAssignment) model.ContextParams {
	s, _ := e.systemOf(c)
	return ctxsys.Clamp(s.Params, s.Base)
}

// appParams is the app default system's knobs (the playground).
func (e *Engine) appParams() model.ContextParams {
	s, _ := ctxsys.Resolve("", "", e.cfg.Get().Context.System, e.store.ContextSystem)
	return ctxsys.Clamp(s.Params, s.Base)
}

// shapePrompt applies a system's prompt knobs to the reply options: the
// persona card, the summary format, and the budgets for memories and the
// summary. It returns the history the prompt reads (the last HistoryMax).
func (e *Engine) shapePrompt(o *prompt.Options, cp model.ContextParams, p model.Persona, hist []model.Message) []model.Message {
	if cp.PromptStyle == model.PromptCard {
		if card := strings.TrimSpace(p.Card); card != "" {
			o.Card = card
		} else {
			e.queueCard(p.ID, cp.CardChars) // the full text this time
		}
	}
	if cp.MemoryLines > 0 && len(o.Memories) > cp.MemoryLines {
		o.Memories = o.Memories[:cp.MemoryLines]
	}
	if cp.SummaryChars > 0 {
		o.Summary = clipText(o.Summary, cp.SummaryChars)
	}
	if cp.HistoryMax > 0 && len(hist) > cp.HistoryMax {
		hist = hist[len(hist)-cp.HistoryMax:]
	}
	return hist
}

// clipText cuts s to at most n runes, at a line or sentence end when one is
// close.
func clipText(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)[:n]
	cut := string(r)
	for _, sep := range []string{"\n", ". "} {
		if i := strings.LastIndex(cut, sep); i >= 0 && utf8.RuneCountInString(cut[:i]) > n/2 { // runes, not bytes
			return strings.TrimSpace(cut[:i+len(sep)])
		}
	}
	return strings.TrimSpace(cut) + "…"
}

// ─── Persona cards ───────────────────────────────────────────

const (
	cardTimeout = 90 * time.Second
	cardDelay   = 2 * time.Second
	cardBackoff = 10 * time.Minute
)

func cardJob(personaID string) string { return "card:" + personaID }

// queueCard makes a persona's card in the background (once; the job slot
// keeps it from competing with replies).
func (e *Engine) queueCard(personaID string, chars int) {
	if personaID == "" || e.jobPending(cardJob(personaID)) {
		return
	}
	if t, ok := e.cardTried.Load(personaID); ok && e.clock.Now().Sub(t.(time.Time)) < cardBackoff {
		return // it failed a moment ago: the full text until then
	}
	e.schedule(cardJob(personaID), cardDelay, func(ctx context.Context) {
		if p, ok := e.store.Persona(personaID); !ok || strings.TrimSpace(p.Card) != "" {
			return
		}
		if _, err := e.makeCard(ctx, personaID, chars); err != nil && ctx.Err() == nil {
			e.cardTried.Store(personaID, e.clock.Now())
		}
	})
}

// MakeCard (re)writes a persona's card now (Context page, persona editor);
// chars 0 = the app system's card size.
func (e *Engine) MakeCard(ctx context.Context, personaID string, chars int) (model.Persona, error) {
	if chars <= 0 {
		chars = e.appParams().CardChars
	}
	e.cancelJob(cardJob(personaID))
	var p model.Persona
	var err error
	if !e.withJobSlot(ctx, func() { p, err = e.makeCard(ctx, personaID, chars) }) {
		return p, ctx.Err()
	}
	return p, err
}

func (e *Engine) makeCard(ctx context.Context, personaID string, chars int) (model.Persona, error) {
	p, ok := e.store.Persona(personaID)
	if !ok {
		return p, fmt.Errorf("persona %q: %w", personaID, ErrNotFound)
	}
	prov, mdl, err := e.llm.Resolve(p.LLM)
	if err != nil {
		return p, err
	}
	req := prompt.Card(p, chars)
	req.Model = mdl
	cctx, cancel := context.WithTimeout(ctx, cardTimeout)
	resp, err := prov.Chat(cctx, req)
	cancel()
	if err != nil {
		return p, err
	}
	card := prompt.CleanCard(resp.Text, chars)
	if card == "" {
		return p, fmt.Errorf("the model wrote an empty card")
	}
	p, err = e.store.SetPersonaCard(personaID, card)
	if err == nil && e.hub != nil {
		e.hub.Publish(events.TypePersonasChanged, struct{}{})
	}
	return p, err
}

// ─── Joining in by rules ─────────────────────────────────────

type verdict int

const (
	unsure verdict = iota
	clearYes
	clearNo
)

// Rule reasons (activity meta "reason").
const (
	ReasonAnswersMe     = "answers_me"
	ReasonAsksEveryone  = "asks_everyone"
	ReasonSmallReaction = "small_reaction"
	ReasonOthersTalking = "others_talking"
)

var (
	laughRe   = regexp.MustCompile(`^(?i)(h+a+h*)+$|^(?i)(l+o+l+)+$|^(?i)lmf?ao+$|^ח{2,}$|^x+d+$`)
	reactions = map[string]bool{"ok": true, "okay": true, "k": true, "kk": true, "yes": true, "yep": true, "yeah": true, "no": true, "nope": true,
		"nice": true, "cool": true, "thanks": true, "thank you": true, "ty": true, "thx": true, "+1": true, "same": true, "true": true, "facts": true,
		"rofl": true, "wow": true, "omg": true, "אוקי": true, "סבבה": true, "תודה": true, "כן": true, "לא": true, "אחלה": true, "וואו": true, "נכון": true}
	everyoneWords = []string{"anyone", "anybody", "everyone", "everybody", "guys", "y'all", "you all", "who's in", "who is in", "who wants", "who else",
		"thoughts?", "ideas?", "מישהו", "כולם", "חבר'ה", "חברה?", "מי בא", "מי רוצה", "מי עוד"}
)

// joinRules decides the clear cases of "should it join in?" in code: a
// direct answer to its last message or a question to everyone is a yes; a
// small reaction or two others talking to each other is a no; anything else
// is unsure. recent ends before the message.
func joinRules(text, sender string, recent []model.Message, now time.Time) (verdict, string) {
	t := strings.ToLower(strings.TrimSpace(text))
	bare := strings.TrimFunc(t, func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSpace(r) })
	if bare == "" || reactions[bare] || laughRe.MatchString(bare) || onlySymbols(bare) {
		return clearNo, ReasonSmallReaction
	}
	var last *model.Message
	for i := len(recent) - 1; i >= 0; i-- {
		if recent[i].Kind == model.MsgKindFix || recent[i].Kind == model.MsgKindReveal {
			continue
		}
		last = &recent[i]
		break
	}
	if last != nil && last.Speaker == "me" && last.FromBot && (last.TS.IsZero() || now.Sub(last.TS) < 5*time.Minute) {
		return clearYes, ReasonAnswersMe
	}
	if strings.Contains(t, "?") {
		for _, w := range everyoneWords {
			if strings.Contains(t, w) {
				return clearYes, ReasonAsksEveryone
			}
		}
	}
	// Two others in a back-and-forth: the last 4 messages theirs, only this
	// sender and one other, and it hasn't spoken in the last 6.
	if n := len(recent); n >= 4 && sender != "" {
		people := map[string]bool{sender: true}
		mine := false
		for _, m := range recent[n-min(6, n):] {
			mine = mine || m.Speaker == "me"
		}
		for _, m := range recent[n-4:] {
			people[m.Name] = true
		}
		if !mine && len(people) == 2 {
			return clearNo, ReasonOthersTalking
		}
	}
	return unsure, ""
}

func onlySymbols(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// ─── A chart from your words ─────────────────────────────────

const chartTimeout = 2 * time.Minute

// DraftSystem drafts a context system from a description: the AI draws the
// chart from the building blocks, code lays it out and reads the knobs off
// it (starting from base). Nothing is saved.
func (e *Engine) DraftSystem(ctx context.Context, text, base string) (model.ContextSystem, []string, error) {
	if !ctxsys.IsBase(base) {
		base = model.CtxLean
	}
	prov, mdl, err := e.llm.Resolve(nil)
	if err != nil {
		return model.ContextSystem{}, nil, err
	}
	req := prompt.ChartFromText(text, ctxsys.Blocks())
	req.Model = mdl
	cctx, cancel := context.WithTimeout(ctx, chartTimeout)
	resp, err := prov.Chat(cctx, req)
	cancel()
	if err != nil {
		return model.ContextSystem{}, nil, err
	}
	name, desc, chart, err := prompt.ParseChart(resp.Text)
	if err != nil {
		return model.ContextSystem{}, nil, fmt.Errorf("the model didn't draw a readable chart")
	}
	for i := range chart.Nodes { // ids the model repeats or leaves out
		if chart.Nodes[i].ID == "" {
			chart.Nodes[i].ID = fmt.Sprintf("n%d", i+1)
		}
	}
	seen := map[string]bool{}
	nodes := chart.Nodes[:0]
	for _, n := range chart.Nodes {
		if !seen[n.ID] {
			seen[n.ID] = true
			nodes = append(nodes, n)
		}
	}
	chart.Nodes = nodes
	if err := ctxsys.NormalizeChart(&chart); err != nil {
		return model.ContextSystem{}, nil, err
	}
	ctxsys.Layout(&chart)
	params, changes := ctxsys.Infer(chart, ctxsys.ParamsFor(base))
	if name == "" {
		name = "My system"
	}
	return model.ContextSystem{Name: name, Description: desc, Base: base, Params: params, Chart: chart}, changes, nil
}
