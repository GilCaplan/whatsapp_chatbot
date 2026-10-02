package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"whatsappdoppel/internal/crossctx"
	"whatsappdoppel/internal/guard"
	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/mention"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
	"whatsappdoppel/internal/store"
)

const playgroundTTL = time.Hour

// playground keeps in-memory practice chats that use the same guard → prompt →
// provider path as real chats, without WhatsApp.
type playground struct {
	e        *Engine
	mu       sync.Mutex
	sessions map[string]*pgSession
}

type pgSession struct {
	mu        sync.Mutex
	personaID string
	history   []model.Message
	lastUsed  time.Time
	turns     int // group mode: rotates the speaker through pgRoster
	// goal is the optional "Goal for this test" (chat goal override); see goal.go.
	goal       string
	goalStatus model.GoalStatus
	// crossSource / crossMode: "Pretend this group includes…" (cross.go).
	crossSource string
	crossMode   string
}

func newPlayground(e *Engine) *playground {
	return &playground{e: e, sessions: map[string]*pgSession{}}
}

func (pg *playground) Start(personaID string) (string, error) {
	if _, ok := pg.e.store.Persona(personaID); !ok {
		return "", fmt.Errorf("persona %q: %w", personaID, ErrNotFound)
	}
	id := store.NewID()
	pg.mu.Lock()
	defer pg.mu.Unlock()
	pg.purgeLocked()
	pg.sessions[id] = &pgSession{personaID: personaID, lastUsed: time.Now()}
	return id, nil
}

func (pg *playground) End(id string) {
	pg.mu.Lock()
	delete(pg.sessions, id)
	pg.mu.Unlock()
}

func (pg *playground) purgeLocked() {
	for id, s := range pg.sessions {
		s.mu.Lock()
		expired := time.Since(s.lastUsed) > playgroundTTL
		s.mu.Unlock()
		if expired {
			delete(pg.sessions, id)
		}
	}
}

func (pg *playground) session(id string) (*pgSession, error) {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	pg.purgeLocked()
	s, ok := pg.sessions[id]
	if !ok {
		return nil, fmt.Errorf("playground session %q (expired?): %w", id, ErrNotFound)
	}
	return s, nil
}

// Send runs one playground turn. With group=true the group decision is simulated
// first and reported in Decision (no reply when it says skip).
func (pg *playground) Send(ctx context.Context, id, text string, group bool) (model.PlaygroundReply, error) {
	e := pg.e
	s, err := pg.session(id)
	if err != nil {
		return model.PlaygroundReply{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUsed = time.Now()

	text = strings.TrimSpace(text)
	if text == "" {
		return model.PlaygroundReply{}, errors.New("text is required")
	}
	p, ok := e.store.Persona(s.personaID)
	if !ok {
		return model.PlaygroundReply{}, fmt.Errorf("persona %q: %w", s.personaID, ErrNotFound)
	}
	rp := e.cfg.Get().Behavior.Private
	if group {
		rp = e.cfg.Get().Behavior.Group
	}
	out := model.PlaygroundReply{}
	if prov, mdl, err := e.llm.Resolve(p.LLM); err == nil {
		out.Provider, out.Model = prov.Name(), mdl
	}

	g := guard.Inspect(text, rp.InjectionFilter)
	if g.Blocked {
		out.Blocked = true
		out.BlockReason = "Ignored as a likely prompt-injection attempt (" + strings.Join(g.Matches, ", ") + ")"
		return out, nil
	}
	kind := "dm"
	if group {
		kind = "group"
	}
	chat := model.ChatAssignment{Key: "playground:" + id, Kind: kind, Name: "Playground", PersonaID: p.ID, Enabled: true, GoalOverride: s.goal}
	// In group mode the messages come from a small rotating cast, so tagging
	// ("@Dana") and "several people are talking" can be tried out.
	who := model.Participant{Name: "Friend"}
	var dir *mention.Directory
	roster := pgRoster
	if group {
		if src, ok := e.playgroundSource(s, p.ID); ok {
			roster = slices.Clone(pgRoster)
			roster[0] = model.Participant{JID: src.JID, Phone: crossUserPhone(src.JID), Name: playgroundName(src.Name)}
		}
		who = roster[s.turns%len(roster)]
		s.turns++
		out.Speaker = who.Name
		dir = mention.NewDirectory(roster, nil)
	}
	s.history = trimHistory(append(s.history, model.Message{
		ID: store.NewID(), TS: time.Now(), Speaker: "them", Name: who.Name, SenderJID: who.JID, Text: g.Text,
	}), rp.HistoryMessages, rp.HistoryChars)

	start := time.Now()
	if group {
		d := e.decide(ctx, p, rp, g.Text, nil)
		out.Decision = &model.Decision{WouldReply: d.Reply, Reason: d.Reason}
		if !d.Reply {
			out.LatencyMs = time.Since(start).Milliseconds()
			return out, nil
		}
	}
	hist := slices.Clone(s.history)
	opts, _ := e.promptOptions(chat, rp, dir, hist)
	var cross *crossUse
	if group {
		cross = e.playgroundCross(s, chat, dir, hist)
		if cross.active() {
			opts.Cross = cross.Input
		}
	}
	gcfg, gturn := e.playgroundGoalTurn(ctx, p, chat, &s.goalStatus, hist, group, g.Text, false)
	res, err := e.expressiveReply(ctx, p, rp, hist, false, gcfg, gturn, cross, func(t prompt.GoalTurn, x prompt.CrossTurn) llm.Request {
		opts.Goal, opts.CrossTurn = t, x
		return prompt.Compose(p, chat, hist, group, opts)
	})
	out.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		return out, err
	}
	var tagged []mention.Entry
	res.Text, tagged = e.applyMentions(res.Text, dir, rp, pendingMsg{SenderJID: who.JID, Name: who.Name}, hist, false)
	out.Mentions = mention.Names(tagged)
	out.Goal = playgroundGoalView(gcfg, gturn, s.goalStatus, res)
	out.Reply, out.Provider, out.Model = res.Text, res.Provider, res.Model
	if cross.active() {
		out.Cross = &model.PlaygroundCross{Mode: cross.Mode, People: cross.People, Items: len(cross.Items), Rewritten: res.CrossRewritten, Dropped: res.CrossDropped}
	}
	s.history = trimHistory(append(s.history, model.Message{
		ID: store.NewID(), TS: time.Now(), Speaker: "me", Text: res.Text, FromBot: true, Mentions: out.Mentions,
	}), rp.HistoryMessages, rp.HistoryChars)
	return out, nil
}

// SetCross binds the cast member "Dana" to a real private chat of the
// session's persona (source "" = off). mode "" = the app default for groups.
func (pg *playground) SetCross(id, source, mode string) error {
	s, err := pg.session(id)
	if err != nil {
		return err
	}
	if mode != "" && !crossctx.ValidMode(mode) {
		return fmt.Errorf("cross mode %q: %w", mode, ErrInvalidCross)
	}
	if source != "" {
		c, ok := pg.e.store.Chat(source)
		if !ok || isGroupChat(c) || c.PersonaID != s.personaID {
			return fmt.Errorf("%q is not a private chat of this persona: %w", source, ErrInvalidCross)
		}
	}
	s.mu.Lock()
	s.crossSource, s.crossMode = source, mode
	s.mu.Unlock()
	return nil
}

// ErrInvalidCross: SetCross got an unknown mode or a chat that can't be a source.
var ErrInvalidCross = errors.New("invalid cross-chat source")

// playgroundSource is the session's bound private chat (s.mu held).
func (e *Engine) playgroundSource(s *pgSession, personaID string) (model.ChatAssignment, bool) {
	if s.crossSource == "" {
		return model.ChatAssignment{}, false
	}
	c, ok := e.store.Chat(s.crossSource)
	if !ok || isGroupChat(c) || c.PersonaID != personaID || c.JID == "" {
		return model.ChatAssignment{}, false
	}
	return c, true
}

// playgroundCross is the cross-chat context of a playground group turn: the
// real rules and sources, as if the cast were a real group (s.mu held).
func (e *Engine) playgroundCross(s *pgSession, chat model.ChatAssignment, d *mention.Directory, hist []model.Message) *crossUse {
	if s.crossSource == "" {
		return nil
	}
	r := e.crossRules()
	c := chat
	c.Kind, c.Cross.Mode = "group", s.crossMode
	mode, _ := crossctx.ModeFor(r, c)
	if mode == model.CrossOff {
		return nil
	}
	return e.crossFor(e.ctx, c, d, hist, e.clock.Now(), r, mode)
}

func crossUserPhone(jid string) string {
	if strings.HasSuffix(jid, "@s.whatsapp.net") {
		return crossctx.UserOf(jid)
	}
	return ""
}

// playgroundName is the first name of a contact (the cast is first names).
func playgroundName(name string) string {
	if f := strings.Fields(mention.CleanName(name)); len(f) > 0 {
		return f[0]
	}
	return "Dana"
}

// pgRoster is the playground's group cast (synthetic numbers).
var pgRoster = []model.Participant{
	{JID: "15550300001@s.whatsapp.net", Phone: "15550300001", Name: "Dana"},
	{JID: "15550300002@s.whatsapp.net", Phone: "15550300002", Name: "Noam"},
	{JID: "15550300003@s.whatsapp.net", Phone: "15550300003", Name: "Maya"},
	{JID: "15550300004@s.whatsapp.net", Phone: "15550300004", Name: "Eitan"},
}

// builder drafts personas with the default (or requested) provider in JSON mode.
type builder struct{ e *Engine }

func (b builder) Draft(ctx context.Context, req model.DraftRequest) (model.Persona, string, error) {
	desc, samples := strings.TrimSpace(req.Description), strings.TrimSpace(req.Samples)
	if desc == "" && samples == "" {
		return model.Persona{}, "", errors.New("describe the persona (or paste sample messages)")
	}
	var choice *model.LLMChoice
	if req.Provider != "" || req.Model != "" {
		choice = &model.LLMChoice{Provider: req.Provider, Model: req.Model}
	}
	prov, mdl, err := b.e.llm.Resolve(choice)
	if err != nil {
		return model.Persona{}, "", err
	}
	r := prompt.Builder(desc, samples)
	r.Model = mdl
	ctx, cancel := context.WithTimeout(ctx, llm.TimeoutBuilder)
	defer cancel()
	resp, err := prov.Chat(ctx, r)
	if err != nil {
		return model.Persona{}, "", err
	}
	p, err := prompt.ParseDraft(resp.Text)
	return p, resp.Text, err
}
