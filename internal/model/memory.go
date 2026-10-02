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
)

// MemoryKinds lists the valid Memory.Kind values.
var MemoryKinds = []string{MemoryFact, MemoryPreference, MemoryEvent, MemoryRelationship, MemoryOther}

// MemoriesView is GET /api/chats/{key}/memories.
type MemoriesView struct {
	Enabled         bool       `json:"enabled"` // effective: chat setting, else the app setting
	Items           []Memory   `json:"items"`
	Pending         int        `json:"pending"` // messages since the last extraction
	LastExtractedAt *time.Time `json:"lastExtractedAt"`
}

// MemoryExtractResult is POST /api/chats/{key}/memories/extract.
type MemoryExtractResult struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
}
