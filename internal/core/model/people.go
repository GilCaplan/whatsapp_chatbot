package model

import "time"

// ─── Group members, mentions and per-person preferences ──────

// Participant is one group member as WhatsApp reports it.
type Participant struct {
	JID     string `json:"jid"`   // address in the group's addressing mode (lid or phone JID) — the one mentions must use
	Phone   string `json:"phone"` // digits, "" for lid-only members
	LID     string `json:"lid"`   // "<id>@lid" or ""
	Name    string `json:"name"`  // address book > push name > business name; "" when unknown
	IsAdmin bool   `json:"isAdmin"`
	IsSelf  bool   `json:"isSelf"`
}

// OutMessage is one WhatsApp text message to send.
type OutMessage struct {
	Text  string
	Quote *QuoteRef
	// Mentions are full JIDs listed in ContextInfo.MentionedJID; Text must
	// contain "@<user>" for each of them.
	Mentions []string
}

// Nickname is a name you give someone (all chats): the app, the persona's
// prompt and @tags use it instead of their WhatsApp or contact name.
type Nickname struct {
	// IDs are the person's addresses (phone JID, lid, …); matched by their
	// user part, so one nickname covers the phone and lid forms.
	IDs       []string  `json:"ids"`
	Name      string    `json:"name"`
	Original  string    `json:"original,omitempty"` // their WhatsApp name when you set it
	UpdatedAt time.Time `json:"updatedAt"`
}

// MaxNickname is the longest nickname (runes).
const MaxNickname = 40

// People modes (PeopleConfig.Mode).
const (
	PeopleAuto     = "auto"     // by group size (behaviour respondToAllMaxMembers)
	PeopleEveryone = "everyone" // answer everyone
	PeopleSelected = "selected" // only people marked Respond
)

// PersonPrefs is what the owner set for one person in a chat.
type PersonPrefs struct {
	JID      string `json:"jid"`      // participant JID as returned by GroupParticipants (matched by user part of PN or LID)
	Name     string `json:"name"`     // last known display name (for the UI when they left)
	Respond  *bool  `json:"respond"`  // nil = follow the chat's mode
	Priority bool   `json:"priority"` // "always reply": skips the reply-chance / chime-in / AI rolls
	Notes    string `json:"notes"`    // ≤ 300 runes, given to the persona
	// Language of replies to them: "" = the chat's choice, "match" = the
	// language they write in, or a language code.
	Language string `json:"language,omitempty"`
	// Cross (groups): use the same persona's private chat with this person
	// here; nil = on (when the chat uses private chats at all).
	Cross *bool `json:"cross"`
}

// PeopleConfig chooses who the persona answers in a chat (groups) and holds
// per-person notes (groups and the DM contact).
type PeopleConfig struct {
	Mode   string        `json:"mode"` // auto|everyone|selected ("" = auto)
	People []PersonPrefs `json:"people"`
}
