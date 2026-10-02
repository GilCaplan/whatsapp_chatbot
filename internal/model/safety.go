package model

import "time"

// ─── Hand-off (wave 3) ───────────────────────────────────────
//
// A sensitive message (money, health, meeting up, distress, "are you a bot?",
// legal) pauses the persona in that chat until you resume it.

// HandoffState is why a chat is paused waiting for you (ChatAssignment.Handoff).
type HandoffState struct {
	Category  string    `json:"category"` // see Handoff* categories
	Excerpt   string    `json:"excerpt"`  // ≤ 160 characters of the message
	Sender    string    `json:"sender"`   // display name of who wrote it
	MessageID string    `json:"messageId"`
	At        time.Time `json:"at"`
	How       string    `json:"how"` // keyword|ai
}

// Hand-off categories and detection methods.
const (
	HandoffMoney    = "money"
	HandoffHealth   = "health"
	HandoffMeeting  = "meeting"
	HandoffDistress = "distress"
	HandoffBot      = "bot"
	HandoffLegal    = "legal"

	HandoffHowKeyword = "keyword"
	HandoffHowAI      = "ai"
)

// HandoffCategories lists the categories in display order.
var HandoffCategories = []string{HandoffMoney, HandoffHealth, HandoffMeeting, HandoffDistress, HandoffBot, HandoffLegal}

// RevealResult is POST /api/chats/{key}/reveal.
type RevealResult struct {
	OK   bool   `json:"ok"`
	Text string `json:"text"` // what was sent
}
