package engine

import (
	"context"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"

	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/logic/guard"
	"whatsappdoppel/internal/logic/lang"
	"whatsappdoppel/internal/runtime/llm"
	"whatsappdoppel/internal/runtime/prompt"
)

// The last stage of every reply: the model has written English; from here on
// code decides. The content filter, then the persona check, then translation
// into the chat's language, then the filter again on what will really go out.
// A reply that can't pass is written once more with the reason, then dropped.

// characterFloor: a first-person claim this far from the persona's profile (by
// embeddings) isn't the persona talking. Real lines measure about 0.47+, other
// people's lives 0.30–0.38.
const characterFloor = 0.40

const (
	translateTimeout = 45 * time.Second
	embedTimeout     = 15 * time.Second
)

// tokenLeft is a placeholder that survived translation.
var tokenLeft = regexp.MustCompile(`[XxХхΧχＸｘ]{2,3}\p{Nd}`)

// finishLanguage is the language a reply ends up in ("" = leave it: a task
// writes its exact message in the language directly).
func (e *Engine) finishLanguage(c model.ChatAssignment, hist []model.Message, tk *taskUse) string {
	if tk != nil {
		return ""
	}
	return e.replyLanguage(c, hist)
}

// taskLanguage: a task's message carries checked times and dates, so it is
// written in the chat's language rather than translated.
func (e *Engine) taskLanguage(o *prompt.Options, c model.ChatAssignment, hist []model.Message, tk *taskUse) {
	code, fixed := e.replyRule(c, hist)
	o.Task, o.WriteEnglish, o.ReplyLanguage, o.LanguageFixed = tk.prompt, false, lang.Name(code), fixed
}

// finishReply takes a written reply to what is sent. regen writes it again
// with a note (nil = no second try).
func (e *Engine) finishReply(ctx context.Context, p model.Persona, code string, res genResult, regen func(note string) (genResult, error)) genResult {
	if res.Quiet {
		return res
	}
	if len(res.Drafts) > 0 {
		return e.finishDrafts(ctx, code, res)
	}
	for attempt := 0; ; attempt++ {
		text, fail := e.vet(ctx, p, res.Text)
		if fail == "" {
			res.Text = text
			break
		}
		res.Checks = append(res.Checks, fail)
		if attempt > 0 || regen == nil || ctx.Err() != nil {
			res.Quiet = true
			return res
		}
		progress(ctx, "Code stopped this draft: "+fail+". Writing it again", map[string]any{"stage": "rewrite"})
		again, err := regen(prompt.CheckRetryNote([]string{fail}))
		if err != nil || again.Quiet {
			res.Quiet = true
			return res
		}
		again.Checks, again.CheckRewritten = res.Checks, true
		again.Latency += res.Latency
		res = again
	}
	out, err := e.translated(ctx, p, res.Text, code)
	if err != "" {
		res.Checks = append(res.Checks, err)
		res.Quiet = true
		return res
	}
	if out != res.Text {
		res.Source, res.Text = res.Text, out
	}
	return res
}

// finishDrafts does the same for co-pilot ideas: you pick, so a bad one is just
// left out.
func (e *Engine) finishDrafts(ctx context.Context, code string, res genResult) genResult {
	var kept []model.Draft
	for _, d := range res.Drafts {
		c := e.filtered(d.Text)
		if c.Block != "" {
			continue
		}
		if out, fail := e.translated(ctx, model.Persona{}, c.Text, code); fail == "" {
			d.Text = out
			kept = append(kept, d)
		}
	}
	if len(kept) == 0 {
		res.Checks = append(res.Checks, "none of the ideas could be used")
		res.Quiet = true
		return res
	}
	res.Drafts, res.Text = kept, kept[0].Text
	return res
}

func (e *Engine) filtered(text string) guard.Content {
	f := e.cfg.Get().Safety.Filter
	if !f.Enabled {
		return guard.Content{Text: text}
	}
	return guard.Filter(text, f.Words)
}

func blockText(why string) string {
	switch why {
	case guard.BlockSlur:
		return "it used a slur"
	case guard.BlockSelfHarm:
		return "it talked about self-harm"
	}
	return "nothing was left after taking out the curse words"
}

// vet is what code checks in the English: the filter and the persona.
func (e *Engine) vet(ctx context.Context, p model.Persona, text string) (string, string) {
	c := e.filtered(text)
	if c.Block != "" {
		return "", blockText(c.Block)
	}
	if c.Text != text {
		progress(ctx, "The filter took out curse words", map[string]any{"stage": "filter"})
	}
	if e.cfg.Get().Safety.Character && guard.FirstPersonClaim(c.Text) && !e.soundsLike(ctx, p, c.Text) {
		return "", "it talked about a life that isn't " + p.Name + "'s"
	}
	return c.Text, ""
}

// soundsLike compares a claim with the persona's profile. No embedding model
// means no verdict: it never blocks a reply on its own.
func (e *Engine) soundsLike(ctx context.Context, p model.Persona, text string) bool {
	cctx, cancel := context.WithTimeout(ctx, embedTimeout)
	defer cancel()
	profile, err := e.personaVector(cctx, p)
	if err != nil {
		return true
	}
	v, err := e.llm.Embed(cctx, text)
	if err != nil {
		return true
	}
	score := cosine(profile, v)
	progress(ctx, "Checked it against the persona", map[string]any{"stage": "character", "score": math.Round(score*100) / 100})
	return score >= characterFloor
}

// personaVectors caches each persona's profile vector until it changes.
type personaVectors struct {
	mu sync.Mutex
	m  map[string]cachedVector
}

type cachedVector struct {
	at time.Time
	v  []float64
}

func (e *Engine) personaVector(ctx context.Context, p model.Persona) ([]float64, error) {
	e.vectors.mu.Lock()
	c, ok := e.vectors.m[p.ID]
	e.vectors.mu.Unlock()
	if ok && c.at.Equal(p.UpdatedAt) {
		return c.v, nil
	}
	text := strings.Join([]string{p.Tagline, p.Bio, p.Personality, p.Style, p.Vocabulary}, " ")
	v, err := e.llm.Embed(ctx, text)
	if err != nil {
		return nil, err
	}
	e.vectors.mu.Lock()
	if e.vectors.m == nil {
		e.vectors.m = map[string]cachedVector{}
	}
	e.vectors.m[p.ID] = cachedVector{p.UpdatedAt, v}
	e.vectors.mu.Unlock()
	return v, nil
}

func cosine(a, b []float64) float64 {
	var dot, na, nb float64
	for i := range min(len(a), len(b)) {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / math.Sqrt(na*nb)
}

// translated turns text into code and checks the result ("" = why not).
func (e *Engine) translated(ctx context.Context, p model.Persona, text, code string) (string, string) {
	if code == "" || code == "en" {
		return text, ""
	}
	if c, sure := detectLang(text); sure && c == code {
		return text, "" // already there: the model ignored the English rule
	}
	name := lang.Name(code)
	cctx, cancel := context.WithTimeout(ctx, translateTimeout)
	defer cancel()
	start := time.Now()
	out, how := "", "translator"
	var err error
	if e.translator != nil {
		progress(ctx, "Translating into "+name, map[string]any{"stage": "translate"})
		out, err = e.translator.Translate(cctx, text, code)
	}
	if e.translator == nil || err != nil {
		how = "ai"
		progress(ctx, "The translator isn't ready, so the AI model is translating into "+name, map[string]any{"stage": "translate"})
		out, err = e.modelTranslate(cctx, p, name, text)
		if err == nil && len(languageCheck(replyCheck{lang: name, langFixed: true}, out)) > 0 {
			return "", "it couldn't be translated into " + name
		}
	}
	if err != nil {
		return "", "it couldn't be translated into " + name
	}
	out = strings.TrimSpace(out)
	// Unchanged is fine: a reply of only names has nothing to translate.
	if out == "" || tokenLeft.MatchString(out) || (out != text && !lang.UsesAlphabet(out, code)) {
		progress(ctx, "The translation came out wrong: "+out, map[string]any{"stage": "rewrite", "source": text})
		return "", "the translation into " + name + " came out wrong"
	}
	c := e.filtered(out)
	if c.Block != "" {
		return "", blockText(c.Block) + " in the translation"
	}
	progress(ctx, "Translated into "+name+" in "+fmtDur(time.Since(start)), map[string]any{"stage": "translated", "by": how, "latencyMs": time.Since(start).Milliseconds()})
	return c.Text, ""
}

func (e *Engine) modelTranslate(ctx context.Context, p model.Persona, name, text string) (string, error) {
	prov, mdl, err := e.llm.Resolve(p.LLM)
	if err != nil {
		return "", err
	}
	req := prompt.Translate(name, text)
	req.Model = mdl
	var resp llm.Response
	resp, err = prov.Chat(ctx, req)
	return resp.Text, err
}
