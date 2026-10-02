// Package contract defines the interfaces between layers so that the
// server, engine and WhatsApp packages can be built and tested independently.
package contract

import (
	"context"
	"errors"

	"whatsappdoppel/internal/model"
)

// ErrNotImplemented is returned by wave 3 features whose contract exists but
// whose implementation has not landed yet. The server answers 501
// "not_implemented" for it.
var ErrNotImplemented = errors.New("not implemented yet")

// WhatsApp is implemented by wa.Manager (real) and wa.Fake (--fake-wa).
type WhatsApp interface {
	// Start connects with the stored session, or begins QR pairing if there is none.
	Start(ctx context.Context) error
	Status() model.WAStatus
	// Pair (re)starts QR pairing; only valid when no device is linked.
	Pair() error
	Reconnect() error
	Disconnect()
	// Logout unlinks this device from the phone and resets to a fresh, unpaired device.
	Logout(ctx context.Context) error
	// CurrentQR returns the latest QR PNG, or nil when not pairing.
	CurrentQR() []byte

	ListChats(ctx context.Context, q model.ChatQuery) (items []model.ChatItem, total int, err error)
	RefreshChats(ctx context.Context) error
	// ResolveChat canonicalizes any JID (phone, lid or group) into a ChatItem with Key/JID/AltJID/Name.
	ResolveChat(ctx context.Context, jid string) (model.ChatItem, error)
	// Avatar returns profile picture bytes; (nil, "", nil) when the chat has none / is hidden.
	Avatar(ctx context.Context, jid string) (data []byte, contentType string, err error)

	// SendMessage sends a text message, optionally quoting a message and
	// @mentioning people (msg.Text must contain "@<user>" for each mention).
	// The result carries WhatsApp's message id (needed to edit it later).
	SendMessage(ctx context.Context, jid string, msg model.OutMessage) (model.SendResult, error)
	// EditMessage replaces the text of one of our own messages (id from
	// SendResult.ID; WhatsApp allows edits for 20 minutes). msg.Quote is ignored.
	EditMessage(ctx context.Context, jid, id string, msg model.OutMessage) error
	// GroupParticipants returns a group's members (cached); (nil, nil) for
	// DMs and unknown chats.
	GroupParticipants(ctx context.Context, groupJID string) ([]model.Participant, error)
	// React sets an emoji reaction on a message (senderJID = who wrote it).
	React(ctx context.Context, chatJID, senderJID, messageID, emoji string) error
	// MarkRead sends "seen" receipts for ids, which must all be from senderJID
	// (sender may be "" in DMs). chatJID is the address the messages arrived on.
	MarkRead(ctx context.Context, chatJID, senderJID string, ids []string) error
	Typing(ctx context.Context, jid string, on bool)
	// OwnJIDs returns the linked account's JIDs (phone and lid), empty when not linked.
	OwnJIDs() []string

	SetMessageHandler(fn func(model.Incoming))
	Close()
}

// Engine runs all assigned chats concurrently.
type Engine interface {
	HandleIncoming(in model.Incoming)
	// Reload re-reads chats.json/settings and starts/stops/updates per-chat runners.
	Reload()
	Stop()

	// SendApproved sends a pending reply. text "" = the stored text (or, in
	// co-pilot mode, the text of drafts[draft]); draft -1 = no draft chosen.
	SendApproved(ctx context.Context, id, text string, draft int) error
	Regenerate(ctx context.Context, id string) (model.PendingReply, error)
	Discard(id string) error

	// ClearHistory forgets a chat's conversation (in memory and on disk).
	ClearHistory(chatKey string) error

	// SendManual sends text as-is to an assigned chat and records it as the persona's message.
	SendManual(ctx context.Context, chatKey, text string) error
	// Simulate pushes a fake incoming message through the real pipeline for an
	// assigned chat. In groups senderJID picks the member it comes from ("" = random).
	Simulate(chatKey, text string, fromMe bool, senderJID string) error
	// Initiate makes the persona start a conversation now (opener through the
	// normal delivery path, or queued in approval mode). hint is optional.
	Initiate(ctx context.Context, chatKey, hint string) error
	// PromptPreview renders the system prompt the persona would get (chatKey optional).
	PromptPreview(personaID, chatKey string) (string, error)
	// ExpressionPreview writes up to 3 sample replies of p (may be unsaved) at
	// its emoji and length settings; lengthBias "" = the private profile's,
	// roll picks other sample messages.
	ExpressionPreview(ctx context.Context, p model.Persona, lengthBias string, count, roll int) (model.ExpressionPreview, error)

	// ── wave 3 (stubs return ErrNotImplemented until they land) ──

	// ResumeHandoff clears a chat's hand-off pause (activity handoff.resumed).
	ResumeHandoff(chatKey string) error
	// Reveal sends the "it was a persona" message (text "" = the settings
	// template), pauses the chat and returns the text sent. force sends it
	// again after an earlier reveal.
	Reveal(ctx context.Context, chatKey, text string, force bool) (string, error)
	// GenerateRecap writes recaps now: one chat, or every eligible chat when
	// chatKey is "".
	GenerateRecap(ctx context.Context, chatKey string) ([]model.Recap, error)
	// ExtractMemories runs memory extraction for a chat now.
	ExtractMemories(ctx context.Context, chatKey string) (model.MemoryExtractResult, error)
}

// LLM is the provider registry facade used by the server.
type LLM interface {
	ListModels(ctx context.Context, provider string) ([]model.ModelInfo, error)
	Test(ctx context.Context, provider, model string) model.LLMTestResult
	OllamaStatus(ctx context.Context) model.OllamaStatus
	// OllamaPull starts a background pull; progress is published on SSE "ollama.pull".
	OllamaPull(name string) (jobID string, err error)
	// Rebuild recreates provider clients after settings/secrets change.
	Rebuild()
}

type Playground interface {
	Start(personaID string) (sessionID string, err error)
	Send(ctx context.Context, sessionID, text string, group bool) (model.PlaygroundReply, error)
	// SetGoal sets the session's "Goal for this test" ("" = the persona's goal).
	SetGoal(sessionID, goal string) error
	// Initiate asks the persona to speak first (an opener); hint is optional.
	Initiate(ctx context.Context, sessionID string, group bool, hint string) (model.PlaygroundReply, error)
	End(sessionID string)
}

type Builder interface {
	// Draft asks the LLM to write a persona profile; the result is not saved.
	Draft(ctx context.Context, req model.DraftRequest) (p model.Persona, raw string, err error)
	// CloneDraft writes a persona that texts like you from your stored
	// message samples (wave 3); the result is not saved.
	CloneDraft(ctx context.Context, req model.CloneRequest) (model.CloneDraft, error)
}

// Notifier shows desktop notifications (internal/notify, wave 3).
type Notifier interface {
	// Backend names how notifications are shown ("terminal-notifier",
	// "osascript", "notify-send", "powershell", "dry-run" or "none").
	Backend() string
	// Test shows "Notifications are working".
	Test(ctx context.Context) error
}
