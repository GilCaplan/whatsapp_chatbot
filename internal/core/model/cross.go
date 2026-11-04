package model

import "time"

// ─── Cross-chat context ──────────────────────────────────────
//
// A persona may draw on what it learned in its OTHER chats with the people
// in the current one: in a group, what someone told it one-to-one; in a
// private chat, what happened in groups the contact shares with it. Only
// summaries cross (memories and a short per-chat brief), never messages,
// and sensitive topics never cross on their own.

// Cross-chat modes: how a chat USES the same persona's other chats.
const (
	CrossOff      = "off"      // chats stay separate
	CrossDiscreet = "discreet" // knows it, never brings it up (unless they do)
	CrossOpen     = "open"     // may refer to it lightly with that person only
)

// CrossModes lists the valid modes.
var CrossModes = []string{CrossOff, CrossDiscreet, CrossOpen}

// CrossContext is a chat's cross-chat settings (ChatAssignment.Cross).
type CrossContext struct {
	// Mode: "" = the app default for the chat kind (settings
	// memory.cross.groupMode / dmMode).
	Mode string `json:"mode"`
	// Share: may other chats use what is learned here? nil = yes.
	Share *bool `json:"share"`
}

// Brief is the short rolling summary of a chat that other chats of the same
// persona may draw on (briefs/<chatKey>.json).
type Brief struct {
	ChatKey     string        `json:"chatKey"`
	PersonaID   string        `json:"personaId"`
	Kind        string        `json:"kind"`        // dm|group
	Topics      []string      `json:"topics"`      // ≤ 4, a few words each
	Commitments []string      `json:"commitments"` // ≤ 3 things the persona promised ("bring wine on Sat 4 Oct")
	Tone        string        `json:"tone"`
	People      []BriefPerson `json:"people"`    // groups: ≤ 6 one-line notes per member
	Sensitive   []string      `json:"sensitive"` // sensitive categories the window touched (then nothing of it is carried)
	From        time.Time     `json:"from"`
	To          time.Time     `json:"to"`
	// MessageCount is how many messages the brief was written from.
	MessageCount int       `json:"messageCount"`
	GeneratedAt  time.Time `json:"generatedAt"`
}

// BriefPerson is what one group member said or did, per the brief.
type BriefPerson struct {
	Name string `json:"name"`
	JID  string `json:"jid,omitempty"`
	Note string `json:"note"`
}

// Why a possible source chat is not used (CrossSource.Blocked).
const (
	CrossBlockedHandoff  = "handoff"    // paused: waiting for you
	CrossBlockedRevealed = "revealed"   // you revealed the persona there
	CrossBlockedShareOff = "share_off"  // that chat's sharing is off
	CrossBlockedPerson   = "person_off" // turned off for this person (People tab)
	CrossBlockedMemory   = "memory_off" // learning is off in that chat
)

// CrossView is GET /api/chats/{key}/cross.
type CrossView struct {
	Enabled      bool          `json:"enabled"` // the app setting memory.cross.enabled
	Kind         string        `json:"kind"`
	Mode         string        `json:"mode"`       // effective mode
	ModeSource   string        `json:"modeSource"` // chat|default
	Share        bool          `json:"share"`      // effective: others may use this chat
	ShareSource  string        `json:"shareSource"`
	Sources      []CrossSource `json:"sources"`
	Brief        *Brief        `json:"brief"`        // this chat's own brief (what others may get)
	BriefPending int           `json:"briefPending"` // messages since the last brief
}

// CrossSource is one other chat of the same persona that this chat may draw on.
type CrossSource struct {
	ChatKey   string `json:"chatKey"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Person    string `json:"person"` // who links the chats ("Dana")
	PersonJID string `json:"personJid,omitempty"`
	Items     int    `json:"items"` // memories about that person that would cross
	HasBrief  bool   `json:"hasBrief"`
	Blocked   string `json:"blocked,omitempty"` // see CrossBlocked*
}
