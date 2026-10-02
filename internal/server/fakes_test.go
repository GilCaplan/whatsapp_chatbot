package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"whatsappdoppel/internal/contract"
	"whatsappdoppel/internal/model"
)

// ─── fake WhatsApp ───────────────────────────────────────────

type fakeWA struct {
	mu      sync.Mutex
	chats   []model.ChatItem
	sent    []string
	rosters map[string][]model.Participant
}

func newFakeWA() *fakeWA {
	return &fakeWA{chats: []model.ChatItem{
		{Key: "dm:15550001", Kind: "dm", JID: "15550001@s.whatsapp.net", AltJID: "999001@lid", Name: "Dana", Phone: "15550001"},
		{Key: "dm:15550002", Kind: "dm", JID: "15550002@s.whatsapp.net", Name: "Eli", Phone: "15550002"},
		{Key: "group:1203630001", Kind: "group", JID: "1203630001@g.us", Name: "Friends", ParticipantCount: 5},
		{Key: "dm:15550000", Kind: "dm", JID: "15550000@s.whatsapp.net", Name: "You (message yourself)", IsSelf: true},
	}}
}

func (f *fakeWA) Start(ctx context.Context) error { return nil }
func (f *fakeWA) Status() model.WAStatus {
	return model.WAStatus{State: model.WAConnected, Me: &model.WAMe{JID: "15550000@s.whatsapp.net", PushName: "Me"}}
}
func (f *fakeWA) Pair() error                      { return nil }
func (f *fakeWA) Reconnect() error                 { return nil }
func (f *fakeWA) Disconnect()                      {}
func (f *fakeWA) Logout(ctx context.Context) error { return nil }
func (f *fakeWA) CurrentQR() []byte                { return nil }
func (f *fakeWA) ListChats(ctx context.Context, q model.ChatQuery) ([]model.ChatItem, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []model.ChatItem
	for _, c := range f.chats {
		if q.Q != "" && !strings.Contains(strings.ToLower(c.Name), strings.ToLower(q.Q)) {
			continue
		}
		out = append(out, c)
	}
	return out, len(out), nil
}
func (f *fakeWA) RefreshChats(ctx context.Context) error { return nil }
func (f *fakeWA) ResolveChat(ctx context.Context, jid string) (model.ChatItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.chats {
		if c.JID == jid || c.AltJID == jid {
			return c, nil
		}
	}
	return model.ChatItem{}, errors.New("unknown jid")
}
func (f *fakeWA) Avatar(ctx context.Context, jid string) ([]byte, string, error) {
	return nil, "", nil
}
func (f *fakeWA) Send(ctx context.Context, jid, text string) error {
	f.mu.Lock()
	f.sent = append(f.sent, text)
	f.mu.Unlock()
	return nil
}
func (f *fakeWA) SendMessage(ctx context.Context, jid string, msg model.OutMessage) (model.SendResult, error) {
	return model.SendResult{ID: "S1"}, f.Send(ctx, jid, msg.Text)
}
func (f *fakeWA) EditMessage(ctx context.Context, jid, id string, msg model.OutMessage) error {
	return nil
}
func (f *fakeWA) GroupParticipants(ctx context.Context, jid string) ([]model.Participant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rosters[jid], nil
}
func (f *fakeWA) React(ctx context.Context, chatJID, senderJID, messageID, emoji string) error {
	return nil
}
func (f *fakeWA) MarkRead(ctx context.Context, chatJID, senderJID string, ids []string) error {
	return nil
}
func (f *fakeWA) Typing(ctx context.Context, jid string, on bool) {}
func (f *fakeWA) OwnJIDs() []string                               { return []string{"15550000@s.whatsapp.net"} }
func (f *fakeWA) SetMessageHandler(fn func(model.Incoming))       {}
func (f *fakeWA) Close()                                          {}

// ─── fake Engine ─────────────────────────────────────────────

type fakeEngine struct {
	reloads   atomic.Int32
	mu        sync.Mutex
	simulated []string
	discarded []string
	cleared   []string
	// goal_test.go: manual "start a conversation"
	initiated   []string
	initiateErr error
	// approvals: "id|text|draft"
	approved []string
	// wave 3: nil = ErrNotImplemented (the Phase 0 stubs)
	wave3Err   error
	wave3Calls []string // hand-off resume, reveal, recap (see record)
}

func (e *fakeEngine) HandleIncoming(in model.Incoming) {}
func (e *fakeEngine) Reload()                          { e.reloads.Add(1) }
func (e *fakeEngine) Stop()                            {}
func (e *fakeEngine) SendApproved(ctx context.Context, id, text string, draft int) error {
	e.mu.Lock()
	e.approved = append(e.approved, fmt.Sprintf("%s|%s|%d", id, text, draft))
	e.mu.Unlock()
	return nil
}
func (e *fakeEngine) w3() error {
	if e.wave3Err != nil {
		return e.wave3Err
	}
	return contract.ErrNotImplemented
}

// Hand-off, reveal and recaps (wave 3, landed): recorded as "op|key|…".
func (e *fakeEngine) record(op string) {
	e.mu.Lock()
	e.wave3Calls = append(e.wave3Calls, op)
	e.mu.Unlock()
}
func (e *fakeEngine) ResumeHandoff(chatKey string) error {
	e.record("resume|" + chatKey)
	return e.wave3Err
}
func (e *fakeEngine) Reveal(ctx context.Context, chatKey, text string, force bool) (string, error) {
	e.record(fmt.Sprintf("reveal|%s|%s|%v", chatKey, text, force))
	if text == "" {
		text = "it was a persona"
	}
	return text, e.wave3Err
}
func (e *fakeEngine) GenerateRecap(ctx context.Context, chatKey string) ([]model.Recap, error) {
	e.record("recap|" + chatKey)
	if e.wave3Err != nil {
		return nil, e.wave3Err
	}
	return []model.Recap{{ID: "r1", ChatKey: chatKey, Headline: "A quiet day"}}, nil
}
func (e *fakeEngine) ExtractMemories(ctx context.Context, chatKey string) (model.MemoryExtractResult, error) {
	e.record("extract|" + chatKey) // memory (wave 3, landed)
	if e.wave3Err != nil {
		return model.MemoryExtractResult{}, e.wave3Err
	}
	return model.MemoryExtractResult{Added: 1}, nil
}
func (e *fakeEngine) Regenerate(ctx context.Context, id string) (model.PendingReply, error) {
	return model.PendingReply{ID: id, Text: "again"}, nil
}
func (e *fakeEngine) Discard(id string) error {
	e.mu.Lock()
	e.discarded = append(e.discarded, id)
	e.mu.Unlock()
	return nil
}
func (e *fakeEngine) ClearHistory(chatKey string) error {
	e.mu.Lock()
	e.cleared = append(e.cleared, chatKey)
	e.mu.Unlock()
	return nil
}
func (e *fakeEngine) SendManual(ctx context.Context, chatKey, text string) error { return nil }

// Initiate records "key|hint"; initiateErr (if set) is returned instead.
func (e *fakeEngine) Initiate(ctx context.Context, chatKey, hint string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.initiateErr != nil {
		return e.initiateErr
	}
	e.initiated = append(e.initiated, chatKey+"|"+hint)
	return nil
}
func (e *fakeEngine) Simulate(chatKey, text string, fromMe bool, senderJID string) error {
	e.mu.Lock()
	rec := chatKey + "|" + text
	if senderJID != "" {
		rec += "|" + senderJID
	}
	e.simulated = append(e.simulated, rec)
	e.mu.Unlock()
	return nil
}
func (e *fakeEngine) PromptPreview(personaID, chatKey string) (string, error) {
	return "SYSTEM for " + personaID, nil
}
func (e *fakeEngine) ExpressionPreview(ctx context.Context, p model.Persona, lengthBias string, count, roll int) (model.ExpressionPreview, error) {
	out := model.ExpressionPreview{Provider: "fake", Model: "fake-1"}
	for i := range count {
		out.Samples = append(out.Samples, model.ExpressionSample{
			Incoming: fmt.Sprintf("msg %d/%d", i+1, roll), Reply: p.Name + " " + p.Emoji.Usage + " " + p.MessageLength + " " + lengthBias, Words: 4,
		})
	}
	return out, nil
}

// ─── fake LLM ────────────────────────────────────────────────

type fakeLLM struct{ rebuilds atomic.Int32 }

func (l *fakeLLM) ListModels(ctx context.Context, provider string) ([]model.ModelInfo, error) {
	return []model.ModelInfo{{ID: "fake-1", Label: "Fake 1"}}, nil
}
func (l *fakeLLM) Test(ctx context.Context, provider, mdl string) model.LLMTestResult {
	return model.LLMTestResult{OK: true, Sample: "hi there friend", Provider: provider, Model: mdl}
}
func (l *fakeLLM) OllamaStatus(ctx context.Context) model.OllamaStatus {
	return model.OllamaStatus{Reachable: true, Version: "0.0.0"}
}
func (l *fakeLLM) OllamaPull(name string) (string, error) { return "job1", nil }
func (l *fakeLLM) Rebuild()                               { l.rebuilds.Add(1) }
