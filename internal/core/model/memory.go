package model

import "time"

// ─── Memory of people (wave 3) ───────────────────────────────
//
// The persona learns durable facts about the people in a chat (and you can
// add your own). Memories are kept per chat in memories/<chatKey>.json.

// Memory is one thing the persona remembers about someone in a chat.
type Memory struct {
	ID         string     `json:"id"`
	ChatKey    string     `json:"chatKey"`
	PersonJID  string     `json:"personJid,omitempty"`
	Person     string     `json:"person,omitempty"` // display name when learned
	Text       string     `json:"text"`             // one line, ≤ MaxMemoryText characters
	Kind       string     `json:"kind"`             // fact|preference|event|relationship|other
	Source     string     `json:"source"`           // learned|user
	Pinned     bool       `json:"pinned"`
	Confidence int        `json:"confidence"`         // 0–100
	Evidence   string     `json:"evidence,omitempty"` // the words it was learned from
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	ExpiresAt  *time.Time `json:"expiresAt"` // events: forgotten a while after this
	// Sensitive is the sensitive topic it touches ("" = none; see
	// SensitiveCategories), set by the extractor or by you. Sensitive
	// memories never reach other chats unless you unlock them (Scope).
	Sensitive string `json:"sensitive,omitempty"`
	// Scope: "" follows the chat and app settings, "local" never leaves this
	// chat, "shared" is your manual unlock (crosses even when sensitive).
	Scope string `json:"scope,omitempty"`
}

// Memory kinds and sources.
const (
	MemoryFact         = "fact"
	MemoryPreference   = "preference"
	MemoryEvent        = "event"
	MemoryRelationship = "relationship"
	MemoryOther        = "other"

	MemorySourceLearned = "learned"
	MemorySourceUser    = "user"

	MaxMemoryText = 160

	MemoryScopeLocal  = "local"  // never used in other chats
	MemoryScopeShared = "shared" // unlocked by you: may be used in other chats even when sensitive
)

// Sensitive topics: the hand-off categories plus relationships and things
// told in confidence. They never cross into other chats on their own.
const (
	SensitiveRomance = "romance"
	SensitiveSecret  = "secret"
)

// SensitiveCategories lists the valid Memory.Sensitive values in display order.
var SensitiveCategories = []string{HandoffMoney, HandoffHealth, HandoffMeeting, HandoffDistress, HandoffBot, HandoffLegal, SensitiveRomance, SensitiveSecret}

// MemoryKinds lists the valid Memory.Kind values.
var MemoryKinds = []string{MemoryFact, MemoryPreference, MemoryEvent, MemoryRelationship, MemoryOther}

// MemoriesView is GET /api/chats/{key}/memories.
type MemoriesView struct {
	Enabled bool `json:"enabled"` // effective: chat setting, else the app setting
	// CrossEnabled: what is learned here may be used in other chats of the
	// same persona (app setting on, chat shares, memory on).
	CrossEnabled    bool         `json:"crossEnabled"`
	Items           []MemoryItem `json:"items"`
	Pending         int          `json:"pending"` // messages since the last extraction
	LastExtractedAt *time.Time   `json:"lastExtractedAt"`
}

// MemoryItem is a memory as listed in the Memory tab.
type MemoryItem struct {
	Memory
	// Shares: offered to other chats right now (scope, sensitivity, chat
	// and app settings).
	Shares bool `json:"shares"`
	// EffectiveSensitive is Sensitive, else what the keyword classifier
	// finds in the text ("" = not sensitive).
	EffectiveSensitive string `json:"effectiveSensitive"`
}

// MemoryExtractResult is POST /api/chats/{key}/memories/extract.
type MemoryExtractResult struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
}
