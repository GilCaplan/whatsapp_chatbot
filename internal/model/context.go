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

// PromptSection is one labelled part of the system prompt.
type PromptSection struct {
	Key   string `json:"key"`   // character|safety|context-open|people|summary|memories|cross|world|late|context-close|history|goal|instructions|stay|language|guidance
	Label string `json:"label"` // plain English
	Group string `json:"group,omitempty"`
	Text  string `json:"text"`
	// Editable parts can be replaced for one chat (ChatAssignment.PromptEdits);
	// Original is the part as written without the edit (only when Edited).
	Editable bool   `json:"editable,omitempty"`
	Edited   bool   `json:"edited,omitempty"`
	Original string `json:"original,omitempty"`
}

// EditablePromptSections are the parts of the system prompt a chat can
// replace with its own text: the fixed ones. Parts rebuilt for every reply
// (people, memories, goal, language, guidance…) are changed where they come from.
var EditablePromptSections = []string{"character", "safety", "history", "stay"}

// MaxPromptEdit caps one edited part (runes).
const MaxPromptEdit = 8000

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
	Sections     []PromptSection  `json:"sections"` // System in its parts (joined = System)
	Messages     []ContextMessage `json:"messages"`
	Chars        int              `json:"chars"`        // system + messages
	HistoryKept  int              `json:"historyKept"`  // messages it reads
	HistoryTotal int              `json:"historyTotal"` // messages stored for the chat
	Window       int              `json:"window"`       // behaviour historyMessages
	Summary      *ChatSummary     `json:"summary"`
	Instructions string           `json:"instructions"`
	ContextFrom  *time.Time       `json:"contextFrom"`
	Compact      CompactStatus    `json:"compact"`
}

// CompactStatus is how a chat's long-term memory works right now.
type CompactStatus struct {
	Setting int  `json:"setting"` // ChatAssignment.CompactEvery (0 automatic, -1 off, N)
	On      bool `json:"on"`
	Keep    int  `json:"keep"`   // always read word for word
	Every   int  `json:"every"`  // new messages between summaries (now)
	Fast    bool `json:"fast"`   // a fast chat right now (Every = fastEvery)
	NextIn  int  `json:"nextIn"` // messages until the next summary
}
