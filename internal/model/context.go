package model

import "time"

// ChatSummary is the running summary of the older part of a chat: when a
// chat is compacted the messages before ChatAssignment.ContextFrom are
// replaced by this note in what the persona reads (you can edit it).
type ChatSummary struct {
	ChatKey   string    `json:"chatKey"`
	Text      string    `json:"text"`
	Through   time.Time `json:"through"`  // the last message it covers
	Messages  int       `json:"messages"` // how many messages it covers in all
	UpdatedAt time.Time `json:"updatedAt"`
	Edited    bool      `json:"edited"` // you changed the text
}

// MaxSummaryText and MaxInstructions cap the editable texts (runes).
const (
	MaxSummaryText  = 1500
	MaxInstructions = 1000
)

// ContextMessage is one message as the model gets it.
type ContextMessage struct {
	Role    string `json:"role"` // user|assistant
	Content string `json:"content"`
}

// ChatContext is what the persona reads for its next reply in a chat
// (GET /api/chats/{key}/context): the system prompt and the messages,
// exactly as sent (without the private planning step).
type ChatContext struct {
	System       string           `json:"system"`
	Messages     []ContextMessage `json:"messages"`
	Chars        int              `json:"chars"`        // system + messages
	HistoryKept  int              `json:"historyKept"`  // messages it reads
	HistoryTotal int              `json:"historyTotal"` // messages stored for the chat
	Window       int              `json:"window"`       // behaviour historyMessages
	Summary      *ChatSummary     `json:"summary"`
	Instructions string           `json:"instructions"`
	ContextFrom  *time.Time       `json:"contextFrom"`
	AutoCompact  bool             `json:"autoCompact"`
}
