// Package model holds the plain data types shared by every layer
// (store, engine, wa, llm, server). It has no dependencies on them.
package model

import "time"

// ─── Personas ────────────────────────────────────────────────

// Avatar describes how a persona's picture is rendered.
// Kind "generated" is drawn by the frontend (gradient + custom glyph/initials);
// kind "upload" is served from GET /api/personas/{id}/avatar.
type Avatar struct {
	Kind     string   `json:"kind"` // generated|upload
	Gradient []string `json:"gradient,omitempty"`
	Glyph    string   `json:"glyph,omitempty"` // id of a built-in SVG graphic (see persona.Glyphs); "" = initials
	Initials string   `json:"initials,omitempty"`
	Version  int64    `json:"version,omitempty"` // bumped on upload (cache-busting)
}

type EmojiPrefs struct {
	Usage     string   `json:"usage"` // none|rare|some|lots
	Favorites []string `json:"favorites"`
}

// LLMChoice pins a persona (or a request) to a provider/model.
type LLMChoice struct {
	Provider string `json:"provider"` // ollama|anthropic|openai
	Model    string `json:"model"`
}

type Persona struct {
	ID            string     `json:"id"`
	BuiltIn       bool       `json:"builtIn"`
	Name          string     `json:"name"`
	Tagline       string     `json:"tagline"`
	Avatar        Avatar     `json:"avatar"`
	Bio           string     `json:"bio"`
	Personality   string     `json:"personality"`
	Style         string     `json:"style"`
	Vocabulary    string     `json:"vocabulary"`
	Rules         string     `json:"rules"`
	Language      string     `json:"language"`
	Emoji         EmojiPrefs `json:"emoji"`
	MessageLength string     `json:"messageLength"` // short|medium|long
	Goal          string     `json:"goal"`
	// Goal pursuit (see goal.go): style subtle|balanced|direct, plan ahead
	// (nil = true) and what to do once reached (relax|continue).
	GoalStyle        string     `json:"goalStyle"`
	GoalPlanAhead    *bool      `json:"goalPlanAhead"`
	GoalAfterReached string     `json:"goalAfterReached"`
	DecisionHint     string     `json:"decisionHint"`
	FallbackReply    string     `json:"fallbackReply"`
	AdvancedPrompt   string     `json:"advancedPrompt"`
	LLM              *LLMChoice `json:"llm"`
	// World is where the persona lives: city, country, time zone and daily
	// routine (world.go). The zero value means "same as this Mac, no routine".
	World World `json:"world"`
	// Secretary makes it the owner's personal secretary (secretary.go); nil = an ordinary persona.
	Secretary *SecretaryConfig `json:"secretary,omitempty"`
	// ContextSystem is this persona's context system ("" = the app's; see
	// ctxsys); Card is its short persona card for card-style prompts ("" =
	// not made yet), CardAt when it was made or edited.
	ContextSystem string     `json:"contextSystem"`
	Card          string     `json:"card"`
	CardAt        *time.Time `json:"cardAt"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

// ─── Chats ───────────────────────────────────────────────────

// ChatOverrides is the v1 per-chat fine-tuning shape. It is only read from old
// chats.json files and migrated into ChatAssignment.Behavior by store.Open.
type ChatOverrides struct {
	DebounceSeconds         *int `json:"debounceSeconds"`
	GroupRandomReplyPercent *int `json:"groupRandomReplyPercent"`
	AlwaysReplyInGroup      bool `json:"alwaysReplyInGroup"`
}

// ChatAssignment binds a WhatsApp chat to a persona.
// Key is canonical: "dm:<phone>" | "group:<id>" | "lid:<lid-user>" (DM whose phone is unknown).
type ChatAssignment struct {
	Key       string `json:"key"`
	Kind      string `json:"kind"` // dm|group
	JID       string `json:"jid"`
	AltJID    string `json:"altJid"`
	Name      string `json:"name"`
	PersonaID string `json:"personaId"`
	Enabled   bool   `json:"enabled"`
	// Mode is how replies go out: auto|approve|copilot (see ChatMode*).
	// ApprovalMode is derived from it (Mode != auto) and kept for one more
	// version so older clients and code keep working; store keeps both in sync.
	Mode         string `json:"mode"`
	ApprovalMode bool   `json:"approvalMode"`
	GoalOverride string `json:"goalOverride"`
	// Per-chat goal overrides; nil = the persona's setting.
	GoalStyle     *string           `json:"goalStyle"`
	GoalPlanAhead *bool             `json:"goalPlanAhead"`
	Behavior      BehaviorOverrides `json:"behavior"`
	// People chooses who the persona answers (groups) and holds per-person notes.
	People PeopleConfig `json:"people"`
	// SnoozedUntil pauses the persona in this chat ("Away") until that time.
	SnoozedUntil *time.Time `json:"snoozedUntil"`
	// Memory turns learning about people on or off for this chat (nil = the
	// app setting memory.enabled).
	Memory *bool `json:"memory"`
	// Handoff is set while the persona is paused waiting for you (a sensitive
	// message arrived); nil = not paused.
	Handoff *HandoffState `json:"handoff"`
	// MissionID is the mission template behind goalOverride ("" = free-form goal).
	MissionID string `json:"missionId"`
	// Cross is how this chat uses (and shares) context from the same
	// persona's other chats; zero value = the app defaults.
	Cross CrossContext `json:"cross"`
	// Language: the languages it speaks here and the reply language
	// (nil = settings.behavior.languages for the chat's kind; see internal/logic/lang).
	Language *LanguagePrefs `json:"language,omitempty"`
	// AutoContinue: a message that would pause the chat (hand-off) is only
	// flagged — the persona keeps replying, nothing to resume.
	AutoContinue bool `json:"autoContinue"`
	// What the persona reads (context.go): your instructions for this chat
	// (added to its prompt), where its history starts (nil = everything
	// kept; older messages are left to the summary) and whether old messages
	// are summarised automatically as the history fills up.
	Instructions string     `json:"instructions"`
	ContextFrom  *time.Time `json:"contextFrom"`
	// CompactEvery: long-term memory for this chat — 0 = automatic (settings
	// memory.compact), -1 = off, N = fold into the summary every N messages.
	CompactEvery int `json:"compactEvery,omitempty"`
	// PromptEdits replace parts of the system prompt in this chat only
	// (key → text, keys in EditablePromptSections); the persona is unchanged.
	PromptEdits map[string]string `json:"promptEdits,omitempty"`
	// ContextSystem is this chat's context system ("" = the persona's, else
	// the app's; see ctxsys).
	ContextSystem string `json:"contextSystem"`
	// RevealedAt is when you told this chat it was talking to a persona.
	RevealedAt     *time.Time `json:"revealedAt"`
	LastActivityAt *time.Time `json:"lastActivityAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	// LegacyOverrides is read from v1 chats.json only; nil after migration.
	LegacyOverrides *ChatOverrides `json:"overrides,omitempty"`
}

// Message is one line of per-chat conversation history.
type Message struct {
	ID      string    `json:"id"`
	TS      time.Time `json:"ts"`
	Speaker string    `json:"speaker"` // them|me
	Name    string    `json:"name,omitempty"`
	Text    string    `json:"text"`
	FromBot bool      `json:"fromBot,omitempty"`
	// SenderJID is who wrote a "them" message (groups); Mentions are the
	// display names tagged with @ in Text.
	SenderJID string   `json:"senderJid,omitempty"`
	Mentions  []string `json:"mentions,omitempty"`
	// Kind marks special persona messages ("" = normal; see MsgKind*).
	Kind string `json:"kind,omitempty"`
	// Corrected is the intended text of a bubble that was sent with a typo
	// (the model is always shown Corrected, never the typo).
	Corrected string `json:"corrected,omitempty"`
	// WAID is the WhatsApp message id of a persona bubble (SendResult.ID).
	WAID string `json:"waId,omitempty"`
	// CrossUsed names the people whose context from other chats informed
	// this persona bubble (cross-chat context).
	CrossUsed []string `json:"crossUsed,omitempty"`
}

// Message kinds (Message.Kind).
const (
	MsgKindFix    = "fix"    // the "*word" correction bubble after a typo
	MsgKindEdited = "edited" // a typo bubble that was edited in place
	MsgKindReveal = "reveal" // the "it was a persona" message (sent by you)
)

// Chat modes (ChatAssignment.Mode).
const (
	ChatModeAuto    = "auto"    // replies are sent automatically
	ChatModeApprove = "approve" // replies wait in Approvals
	ChatModeCopilot = "copilot" // three drafts wait in Approvals, you pick one
)

// ChatModes lists the valid chat modes.
var ChatModes = []string{ChatModeAuto, ChatModeApprove, ChatModeCopilot}

// ValidChatMode reports whether m is one of ChatModes.
func ValidChatMode(m string) bool {
	return m == ChatModeAuto || m == ChatModeApprove || m == ChatModeCopilot
}

// ModeFromApproval maps the legacy approvalMode bool to a mode.
func ModeFromApproval(approval bool) string {
	if approval {
		return ChatModeApprove
	}
	return ChatModeAuto
}

// SendResult is what WhatsApp returned for a sent message.
type SendResult struct {
	ID string    `json:"id"` // WhatsApp message id ("" from fakes that don't assign one)
	At time.Time `json:"at"` // server timestamp (zero when unknown)
}

// Draft is one co-pilot alternative for a pending reply.
type Draft struct {
	Tone     string   `json:"tone"` // brief|warm|playful (see DraftTone*)
	Text     string   `json:"text"`
	Mentions []string `json:"mentions,omitempty"` // display names the draft tags with @
}

// Draft tones, in the order they are offered.
const (
	DraftToneBrief   = "brief"
	DraftToneWarm    = "warm"
	DraftTonePlayful = "playful"
)

// PendingReply is a generated reply waiting for the user's approval.
type PendingReply struct {
	ID          string     `json:"id"`
	ChatKey     string     `json:"chatKey"`
	ChatName    string     `json:"chatName"`
	PersonaID   string     `json:"personaId"`
	PersonaName string     `json:"personaName"`
	Text        string     `json:"text"`
	Context     []Message  `json:"context"` // last few messages that led to this reply
	CreatedAt   time.Time  `json:"createdAt"`
	Stale       bool       `json:"stale"`
	AutoSendAt  *time.Time `json:"autoSendAt"`
	Provider    string     `json:"provider,omitempty"`
	Model       string     `json:"model,omitempty"`
	Mentions    []string   `json:"mentions,omitempty"` // display names the reply tags with @
	// Opener: the persona starts the conversation (check-in or "Start the conversation").
	Opener bool `json:"opener,omitempty"`
	// Drafts are the co-pilot alternatives (Text == Drafts[0].Text); empty
	// outside co-pilot mode.
	Drafts []Draft `json:"drafts,omitempty"`
	// CrossUsed names the people whose context from other chats informed it.
	CrossUsed []string `json:"crossUsed,omitempty"`
}

// ─── Activity ────────────────────────────────────────────────

// Activity event types.
const (
	ActIncoming          = "incoming"
	ActDecisionSkip      = "decision.skip"
	ActDecisionReply     = "decision.reply"
	ActBlockedInjection  = "blocked_injection"
	ActNoticing          = "noticing" // hasn't looked at the chat yet
	ActSeen              = "seen"     // opened the chat (read receipts)
	ActWaiting           = "waiting"
	ActThinking          = "thinking" // also "distracted" (meta.distracted=true)
	ActGenerating        = "generating"
	ActTyping            = "typing"
	ActSent              = "sent"
	ActDeferred          = "deferred" // outside active hours / away: reply later
	ActReacted           = "reacted"  // reacted with an emoji instead of replying
	ActProactive         = "proactive"
	ActApprovalQueued    = "approval.queued"
	ActApprovalSent      = "approval.sent"
	ActApprovalDiscarded = "approval.discarded"
	ActError             = "error"
	ActWAStatus          = "wa.status"
	ActSystem            = "system"

	// Wave 3. Typos are not a type of their own: they are "sent" events with
	// meta typo:true / fix:true / edited:true.
	ActHandoff        = "handoff"         // paused: a message needs you (meta category, excerpt, how)
	ActHandoffResumed = "handoff.resumed" // you let the persona continue
	ActReveal         = "reveal"          // you revealed the persona and paused the chat
	ActMemory         = "memory"          // learned things about people (meta added, updated)
	ActRecap          = "recap"           // daily recap ready (meta chats)
)

type ActivityEvent struct {
	ID          int64          `json:"id"`
	TS          time.Time      `json:"ts"`
	Type        string         `json:"type"`
	ChatKey     string         `json:"chatKey,omitempty"`
	ChatName    string         `json:"chatName,omitempty"`
	PersonaName string         `json:"personaName,omitempty"`
	Text        string         `json:"text"`
	Meta        map[string]any `json:"meta,omitempty"`
}

// ─── Decision log ────────────────────────────────────────────

// Decision outcomes: what the persona did about a message.
const (
	DecisionPending  = "pending"  // still on it (noticing, waiting, thinking…)
	DecisionReplied  = "replied"  // answered (alone or together with other messages)
	DecisionSkipped  = "skipped"  // stayed quiet
	DecisionReacted  = "reacted"  // reacted with an emoji instead
	DecisionPaused   = "paused"   // hand-off: left for you
	DecisionBlocked  = "blocked"  // likely prompt injection
	DecisionApproval = "approval" // a reply waits for your approval
	DecisionFailed   = "failed"   // the reply couldn't be written or sent
)

// LogEntry is one entry of a chat's decision log: a message someone sent
// (or an opener the persona wrote first, Kind "opener") and what the persona
// did about it, why, and how long it took.
type LogEntry struct {
	ID        string     `json:"id"` // the WhatsApp message id
	ChatKey   string     `json:"chatKey"`
	Kind      string     `json:"kind,omitempty"` // "" = a message to them, "opener"
	At        time.Time  `json:"at"`             // when it was sent on WhatsApp
	Sender    string     `json:"sender,omitempty"`
	Text      string     `json:"text,omitempty"`
	Outcome   string     `json:"outcome"`
	Reason    string     `json:"reason,omitempty"` // code (activity meta.reason)
	Why       string     `json:"why,omitempty"`    // plain English
	DecidedAt *time.Time `json:"decidedAt,omitempty"`
	// RepliedAt is when the first reply message went out; DelaySeconds is
	// At → RepliedAt.
	RepliedAt    *time.Time `json:"repliedAt,omitempty"`
	DelaySeconds float64    `json:"delaySeconds,omitempty"`
	Reply        string     `json:"reply,omitempty"`
	Steps        []LogStep  `json:"steps,omitempty"`
}

// LogStep is one thing the persona did on the way (seen, waiting,
// thinking, typing…), as written in the activity feed.
type LogStep struct {
	At   time.Time `json:"at"`
	Type string    `json:"type"` // activity type
	Text string    `json:"text"`
}

// ─── WhatsApp ────────────────────────────────────────────────

// WhatsApp connection states.
const (
	WADisconnected = "disconnected"
	WAConnecting   = "connecting"
	WAAwaitingQR   = "awaiting_qr"
	WAPairing      = "pairing"
	WAConnected    = "connected"
	WALoggedOut    = "logged_out"
	WAError        = "error"
	WAReplaced     = "replaced"
	WABanned       = "banned"
	WAOutdated     = "outdated"
)

type WAMe struct {
	JID       string `json:"jid"`
	LID       string `json:"lid"`
	PushName  string `json:"pushName"`
	Phone     string `json:"phone"`
	AvatarURL string `json:"avatarUrl"` // e.g. /api/wa/avatar?jid=...
}

type WAStatus struct {
	State     string    `json:"state"`
	Me        *WAMe     `json:"me"`
	LastError string    `json:"lastError"`
	Since     time.Time `json:"since"`
}

// QRFrame is published on SSE "wa.qr".
type QRFrame struct {
	PNG          string `json:"png"` // data:image/png;base64,...
	ExpiresInSec int    `json:"expiresInSec"`
}

type ChatItem struct {
	Key              string     `json:"key"`
	Kind             string     `json:"kind"` // dm|group
	JID              string     `json:"jid"`
	AltJID           string     `json:"altJid"`
	Name             string     `json:"name"`
	Phone            string     `json:"phone"`
	ParticipantCount int        `json:"participantCount"`
	LastMessageAt    *time.Time `json:"lastMessageAt"`
	IsSelf           bool       `json:"isSelf"`
	Assigned         bool       `json:"assigned"`
}

// GroupChange reports that you joined or left (were removed from) a group.
type GroupChange struct {
	Key    string `json:"key"`
	JID    string `json:"jid"`
	Name   string `json:"name"`
	Joined bool   `json:"joined"`
}

type ChatQuery struct {
	Q      string // case-insensitive substring of name/phone
	Kind   string // all|dm|group
	Tab    string // recent|contacts|groups  ("assigned" is handled by the server)
	Limit  int
	Offset int
}

// Incoming is a normalized WhatsApp message handed from the wa layer to the engine.
type Incoming struct {
	ChatKey   string // canonical key (see ChatAssignment.Key)
	ChatJID   string // the address the message actually arrived on (send replies here first)
	AltJID    string // the other address for the same chat (phone↔lid), may be ""
	IsGroup   bool
	IsFromMe  bool
	SenderJID string
	PushName  string
	// WAName is their own PushName when a nickname replaced it (engine.
	// HandleIncoming, Nicknamed set; may be ""); history keeps this one,
	// nicknames apply on read.
	WAName        string
	Nicknamed     bool
	Text          string
	MentionedJIDs []string
	Timestamp     time.Time
	MessageID     string
	// Media is "" for text, else image|video|audio|sticker (MediaImage...);
	// Text is then the caption, or a placeholder like "[photo]" without one.
	Media string
}

// Incoming media kinds (Incoming.Media).
const (
	MediaImage   = "image"
	MediaVideo   = "video"
	MediaAudio   = "audio"
	MediaSticker = "sticker"
)

// ─── LLM ─────────────────────────────────────────────────────

type ModelInfo struct {
	ID    string         `json:"id"`
	Label string         `json:"label"`
	Meta  map[string]any `json:"meta,omitempty"`
}

type OllamaStatus struct {
	Reachable bool        `json:"reachable"`
	Version   string      `json:"version"`
	Models    []ModelInfo `json:"models"`
	Error     string      `json:"error,omitempty"`
}

// PullProgress is published on SSE "ollama.pull".
type PullProgress struct {
	JobID     string  `json:"jobId"`
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	Completed int64   `json:"completed"`
	Total     int64   `json:"total"`
	Percent   float64 `json:"percent"`
	Done      bool    `json:"done"`
	Error     string  `json:"error,omitempty"`
}

type LLMTestResult struct {
	OK        bool   `json:"ok"`
	LatencyMs int64  `json:"latencyMs"`
	Sample    string `json:"sample"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Error     string `json:"error,omitempty"`
}

// ─── Playground / builder ────────────────────────────────────

type Decision struct {
	WouldReply bool   `json:"wouldReply"`
	Reason     string `json:"reason"`
}

type PlaygroundReply struct {
	Reply       string    `json:"reply"`
	Decision    *Decision `json:"decision,omitempty"` // only when group simulation is on
	LatencyMs   int64     `json:"latencyMs"`
	Blocked     bool      `json:"blocked"`
	BlockReason string    `json:"blockReason,omitempty"`
	Provider    string    `json:"provider"`
	Model       string    `json:"model"`
	Speaker     string    `json:"speaker,omitempty"`  // group mode: who the user's message was attributed to
	Mentions    []string  `json:"mentions,omitempty"` // display names the reply tags with @
	// Goal: plan, reached state and rewrite flag (see goal.go).
	Goal *PlaygroundGoal `json:"goal,omitempty"`
	// Drafts: co-pilot alternatives when the playground asked for them.
	Drafts []Draft `json:"drafts,omitempty"`
	// Cross: what the persona drew on from a real private chat ("Pretend
	// this group includes…"); nil when none.
	Cross *PlaygroundCross `json:"cross,omitempty"`
}

// PlaygroundCross reports the cross-chat context of a playground reply
// (never the notes themselves).
type PlaygroundCross struct {
	Mode      string   `json:"mode"`
	People    []string `json:"people"`
	Items     int      `json:"items"`
	Rewritten bool     `json:"rewritten"`
	Dropped   bool     `json:"dropped"`
}

// ExpressionPreview is POST /api/personas/{id}/expression-preview: sample
// replies at the persona's emoji and length settings.
type ExpressionPreview struct {
	Samples   []ExpressionSample `json:"samples"`
	Provider  string             `json:"provider"`
	Model     string             `json:"model"`
	LatencyMs int64              `json:"latencyMs"`
}

type ExpressionSample struct {
	Incoming string `json:"incoming"`
	Reply    string `json:"reply"`
	Words    int    `json:"words"`
	Emoji    int    `json:"emoji"`
	Budget   int    `json:"budget"`   // word budget for this message
	Tone     string `json:"tone"`     // how the incoming message was read
	Adjusted bool   `json:"adjusted"` // the expression rules changed the model's draft
}

type DraftRequest struct {
	Description string `json:"description"`
	Samples     string `json:"samples"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
}
