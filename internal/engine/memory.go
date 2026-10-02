package engine

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"whatsappdoppel/internal/crossctx"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/memory"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
	"whatsappdoppel/internal/store"
)

// Memory of people (wave 3, Engineer A; WAVE3_PLAN §3.5). After every few
// messages from the other people (and a quiet spell) a background job asks
// the model what is worth remembering about them; replies then get the most
// relevant memories in the prompt. Memories are kept per chat and can be
// pinned, edited, added and deleted from the Memory tab.

const (
	memoryEvery     = 6                // new messages from them before an extraction
	memoryIdle      = 90 * time.Second // …and this long after the last one
	memoryTimeout   = 45 * time.Second
	memoryMaxTokens = 350
	memoryPromptK   = prompt.MaxMemoryLines
)

func memoryJob(key string) string { return "memory:" + key }

// memoryOn reports whether the persona learns about people in a chat: the
// chat's own switch, else the app setting.
func (e *Engine) memoryOn(c model.ChatAssignment) bool {
	if c.Memory != nil {
		return *c.Memory
	}
	return e.cfg.Get().Memory.Enabled
}

// memoryLines picks what the persona remembers for a reply prompt.
func (e *Engine) memoryLines(c model.ChatAssignment, hist []model.Message, now time.Time) []prompt.MemoryLine {
	if c.Key == "" || !e.memoryOn(c) {
		return nil
	}
	mems, err := e.store.Memories(c.Key)
	if err != nil || len(mems) == 0 {
		return nil
	}
	var recent []string
	for i := len(hist) - 1; i >= 0 && len(recent) < 3; i-- {
		recent = append(recent, hist[i].Text)
	}
	sel := memory.Select(mems, recent, now, memoryPromptK)
	out := make([]prompt.MemoryLine, 0, len(sel))
	for _, m := range sel {
		out = append(out, prompt.MemoryLine{Person: m.Person, Text: m.Text})
	}
	return out
}

// memoryTick counts a new message from them and (re)arms the extraction job
// once enough have arrived.
func (e *Engine) memoryTick(c model.ChatAssignment) {
	e.briefTick(c) // the brief for other chats counts every message (brief.go)
	if !e.memoryOn(c) {
		return
	}
	pending := 0
	e.updateState(c.Key, func(st *store.RunnerState) {
		st.MemoryPending++
		pending = st.MemoryPending
	})
	if pending < memoryEvery {
		return
	}
	key := c.Key
	e.schedule(memoryJob(key), memoryIdle, func(ctx context.Context) {
		cc, ok := e.store.Chat(key)
		if !ok || !cc.Enabled || !e.memoryOn(cc) {
			return
		}
		if _, err := e.extractMemories(ctx, cc); err != nil && ctx.Err() == nil {
			e.act(model.ActError, cc, "", "Couldn't update memories: "+err.Error(), nil)
		}
	})
}

// ExtractMemories runs memory extraction for a chat now (POST
// /api/chats/{key}/memories/extract). It waits for the background slot.
func (e *Engine) ExtractMemories(ctx context.Context, chatKey string) (model.MemoryExtractResult, error) {
	c, ok := e.store.Chat(chatKey)
	if !ok {
		return model.MemoryExtractResult{}, fmt.Errorf("chat %q: %w", chatKey, ErrNotFound)
	}
	e.cancelJob(memoryJob(chatKey))
	var res model.MemoryExtractResult
	var err error
	if !e.withJobSlot(ctx, func() { res, err = e.extractMemories(ctx, c) }) {
		return res, ctx.Err()
	}
	return res, err
}

// extractMemories reads the messages since the last extraction, asks the
// model for new memories, checks them against the chat and merges them.
func (e *Engine) extractMemories(ctx context.Context, c model.ChatAssignment) (model.MemoryExtractResult, error) {
	var res model.MemoryExtractResult
	p, ok := e.store.Persona(c.PersonaID)
	if !ok {
		return res, fmt.Errorf("persona %q not found", c.PersonaID)
	}
	st := e.store.RunnerState(c.Key)
	all, err := e.store.History(c.Key, 0)
	if err != nil {
		return res, err
	}
	var msgs []model.Message
	for _, m := range all {
		if m.TS.After(st.MemoryCursor) && m.Kind != model.MsgKindFix && m.Kind != model.MsgKindReveal {
			msgs = append(msgs, m)
		}
	}
	if len(msgs) > prompt.MaxExtractMessages {
		msgs = msgs[len(msgs)-prompt.MaxExtractMessages:]
	}
	now := e.clock.Now()
	done := func() {
		var cursor time.Time
		if n := len(msgs); n > 0 {
			cursor = msgs[n-1].TS
		}
		e.updateState(c.Key, func(s *store.RunnerState) {
			s.MemoryPending = 0
			if cursor.After(s.MemoryCursor) {
				s.MemoryCursor = cursor
			}
		})
	}
	if !slices.ContainsFunc(msgs, func(m model.Message) bool { return m.Speaker == "them" }) {
		done()
		return res, nil
	}
	existing, err := e.store.Memories(c.Key)
	if err != nil {
		return res, err
	}
	isGroup := c.Kind == "group"
	known := make([]string, 0, len(existing))
	for _, m := range existing {
		line := m.Text
		if m.Person != "" {
			line = m.Person + ": " + line
		}
		known = append(known, line)
	}
	req := prompt.ExtractMemories(p.Name, isGroup, c.Name, known, msgs, now.In(time.Local))
	prov, mdl, err := e.llm.Resolve(p.LLM)
	if err != nil {
		return res, err
	}
	req.Model, req.MaxTokens = mdl, memoryMaxTokens
	cctx, cancel := context.WithTimeout(ctx, memoryTimeout)
	resp, err := prov.Chat(cctx, req)
	cancel()
	if err != nil {
		return res, err
	}
	found, err := prompt.ParseMemories(resp.Text)
	if err != nil {
		return res, err
	}
	cands := e.checkMemories(c, p, msgs, found)
	var added, updated []model.Memory
	if len(cands) > 0 {
		_, err = e.store.UpdateMemories(c.Key, func(cur []model.Memory) ([]model.Memory, error) {
			var out []model.Memory
			out, added, updated = memory.Merge(cur, cands, c.Key, now, memory.MaxPerChat)
			return out, nil
		})
		if err != nil {
			return res, err
		}
	}
	done()
	res.Added, res.Updated = len(added), len(updated)
	if res.Added+res.Updated > 0 {
		e.act(model.ActMemory, c, p.Name, memoryActivityText(added, updated, c.Name),
			map[string]any{"added": res.Added, "updated": res.Updated})
		e.hub.Publish(events.TypeMemoriesChanged, map[string]string{"chatKey": c.Key})
	}
	return res, nil
}

// checkMemories keeps only extracted memories that are about someone else
// in the chat and backed by what they actually wrote (no hallucinations,
// nothing about the persona), and resolves who each one is about.
func (e *Engine) checkMemories(c model.ChatAssignment, p model.Persona, msgs []model.Message, found []prompt.FoundMemory) []memory.Candidate {
	type who struct{ name, jid string }
	var people []who
	var theirText strings.Builder
	for _, m := range msgs {
		if m.Speaker != "them" {
			continue
		}
		theirText.WriteString(" " + m.Text)
		name := m.Name
		if name == "" {
			name = c.Name
		}
		if name != "" && !slices.ContainsFunc(people, func(w who) bool { return strings.EqualFold(w.name, name) }) {
			people = append(people, who{name, m.SenderJID})
		}
	}
	isGroup := c.Kind == "group"
	theirs := map[string]bool{}
	for _, w := range memory.Tokens(theirText.String()) {
		theirs[w] = true
	}
	covered := func(s string) float64 {
		toks := memory.Tokens(s)
		if len(toks) == 0 {
			return 0
		}
		hit := 0
		for _, w := range toks {
			if theirs[w] {
				hit++
			}
		}
		return float64(hit) / float64(len(toks))
	}
	var out []memory.Candidate
	for _, f := range found {
		person := strings.TrimSpace(f.Person)
		if strings.EqualFold(person, p.Name) || strings.EqualFold(firstWord(person), firstWord(p.Name)) {
			continue // about the persona itself
		}
		var w who
		switch {
		case !isGroup:
			w = who{c.Name, c.JID}
			if len(people) > 0 {
				w.name = people[0].name
				if c.Name != "" {
					w.name = c.Name
				}
			}
		default:
			i := slices.IndexFunc(people, func(x who) bool {
				return strings.EqualFold(x.name, person) || (person != "" && strings.EqualFold(firstWord(x.name), firstWord(person)))
			})
			if i < 0 {
				continue // not someone talking here
			}
			w = people[i]
		}
		// Backed by their words: the evidence (or, without one, the memory
		// itself) must mostly be words they wrote.
		if f.Evidence != "" {
			if covered(f.Evidence) < 0.6 {
				continue
			}
		} else if covered(f.Text) < 0.4 {
			continue
		}
		text := humanDates(stripPersonPrefix(f.Text, w.name, person))
		if transient(text) {
			continue // what they did today, not something to remember
		}
		var exp *time.Time
		if f.Expires != "" {
			if t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(f.Expires), time.Local); err == nil {
				end := t.Add(24*time.Hour - time.Second)
				exp = &end
			}
		}
		kind := memory.ValidKind(f.Kind)
		if exp != nil && kind != model.MemoryEvent {
			kind = model.MemoryEvent
		}
		sens := f.Sensitive
		if !crossctx.ValidSensitive(sens) {
			sens = ""
		}
		if sens == "" {
			sens = crossctx.Classify(text + " . " + f.Evidence) // the model misses some; never crosses either way
		}
		if sens == "" {
			sens = sourceSensitive(msgs, f.Evidence) // learned from a sensitive message (e.g. a hand-off)
		}
		out = append(out, memory.Candidate{Person: w.name, PersonJID: w.jid, Text: text, Kind: kind, Evidence: f.Evidence, ExpiresAt: exp, Sensitive: sens})
	}
	return out
}

// sourceSensitive is the sensitive category of the message evidence was
// quoted from ("" when none or not found): a memory learned from a
// sensitive message stays in its chat even when its own words look harmless.
func sourceSensitive(msgs []model.Message, evidence string) string {
	ev := memory.Tokens(evidence)
	if len(ev) == 0 {
		return ""
	}
	for _, m := range msgs {
		if m.Speaker == "them" && memory.Contained(ev, memory.Tokens(m.Text)) >= 0.6 {
			if cat := crossctx.Classify(m.Text); cat != "" {
				return cat
			}
		}
	}
	return ""
}

// transient spots memories about the moment ("was at school all day",
// "just got home"), which small models note despite the prompt.
func transient(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	for _, p := range []string{"was ", "were ", "just ", "is currently ", "is now ", "is tired", "is busy", "is bored", "is hungry", "today ", "spent the day", "had a long day"} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return strings.HasSuffix(t, " today") || strings.HasSuffix(t, " all day") || strings.HasSuffix(t, " right now")
}

var isoDate = regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\b`)

// humanDates writes ISO dates in a memory as "Thu 8 Oct".
func humanDates(s string) string {
	return isoDate.ReplaceAllStringFunc(s, func(d string) string {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			return d
		}
		return t.Format("Mon 2 Jan")
	})
}

// stripPersonPrefix turns "Dana works as a nurse" / "Dana: works…" into
// "works as a nurse".
func stripPersonPrefix(text string, names ...string) string {
	t := strings.TrimSpace(text)
	for _, n := range names {
		for _, cand := range []string{n, firstWord(n)} {
			if cand == "" {
				continue
			}
			if len(t) > len(cand)+1 && strings.EqualFold(t[:len(cand)], cand) {
				rest := strings.TrimLeft(t[len(cand):], ":,'s ")
				if strings.HasPrefix(strings.ToLower(t[len(cand):]), "'s ") {
					rest = "their " + strings.TrimSpace(t[len(cand)+3:])
				}
				if rest != "" {
					return memory.CleanText(rest)
				}
			}
		}
	}
	return memory.CleanText(t)
}

// memoryActivityText: "Remembered 2 new things about Dana", "Updated what it
// knows about Dana and Josh".
func memoryActivityText(added, updated []model.Memory, chatName string) string {
	var names []string
	for _, m := range append(slices.Clone(added), updated...) {
		n := m.Person
		if n == "" {
			n = chatName
		}
		if n != "" && !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	about := ""
	switch len(names) {
	case 0:
		about = "the people here"
	case 1:
		about = names[0]
	case 2:
		about = names[0] + " and " + names[1]
	default:
		about = names[0] + ", " + names[1] + " and others"
	}
	switch {
	case len(added) == 1 && len(updated) == 0:
		return "Remembered something new about " + about
	case len(added) > 1 && len(updated) == 0:
		return fmt.Sprintf("Remembered %d new things about %s", len(added), about)
	case len(added) == 0:
		return "Updated what it knows about " + about
	}
	return fmt.Sprintf("Remembered %d new things about %s (and updated %d)", len(added), about, len(updated))
}
