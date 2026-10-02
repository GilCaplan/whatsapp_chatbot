package model

// ─── Reply behaviour ─────────────────────────────────────────
//
// A BehaviorProfile describes HOW a persona replies (timing, chance, shape),
// never WHAT it says. The app keeps two profiles (settings.behavior.private
// and settings.behavior.group); each chat may override any field
// (ChatAssignment.Behavior, nil = inherit). Logic lives in internal/behavior.

// TimeRange is "HH:MM"–"HH:MM" in Availability.Timezone. To <= From wraps
// past midnight ("18:00"–"02:30"); From == To means the whole day.
type TimeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// DayHours lists the active ranges of one weekday (mon..sun); empty = off that day.
type DayHours struct {
	Day    string      `json:"day"`
	Ranges []TimeRange `json:"ranges"`
}

// Availability is the persona's "active hours".
type Availability struct {
	Enabled       bool       `json:"enabled"`
	Timezone      string     `json:"timezone"`      // IANA name, "" = this Mac's zone
	Week          []DayHours `json:"week"`          // normalised to 7 entries mon..sun
	OutsideHours  string     `json:"outsideHours"`  // queue|silent
	CatchUpMaxMin int        `json:"catchUpMaxMin"` // random delay after opening, minutes
}

// Proactive controls check-in messages after a long silence.
type Proactive struct {
	Enabled       bool `json:"enabled"`
	AfterHours    int  `json:"afterHours"`
	MaxPerDay     int  `json:"maxPerDay"`
	SpreadMinutes int  `json:"spreadMinutes"` // random delay once the silence threshold passes
}

// BehaviorProfile is one complete set of reply-behaviour values.
// Fields marked "group" only matter in group chats.
type BehaviorProfile struct {
	Preset string `json:"preset"` // label only: instant|natural|busy|slow|nightowl|custom

	// Responsiveness
	ReplyPercent            int  `json:"replyPercent"`
	ReplyWhenNameMentioned  bool `json:"replyWhenNameMentioned"`  // group
	ReplyWhenAtMentioned    bool `json:"replyWhenAtMentioned"`    // group
	SkipWhenOthersMentioned bool `json:"skipWhenOthersMentioned"` // group
	AIJudgement             bool `json:"aiJudgement"`             // group
	ChimeInPercent          int  `json:"chimeInPercent"`          // group
	IgnoreLinks             bool `json:"ignoreLinks"`
	ReactPercent            int  `json:"reactPercent"`
	PauseWhenYouReply       bool `json:"pauseWhenYouReply"`
	MaxRepliesPerHour       int  `json:"maxRepliesPerHour"` // 0 = unlimited
	MaxRepliesPerDay        int  `json:"maxRepliesPerDay"`  // 0 = unlimited
	CooldownSec             int  `json:"cooldownSec"`
	HistoryMessages         int  `json:"historyMessages"`
	HistoryChars            int  `json:"historyChars"`
	StaleAfterMin           int  `json:"staleAfterMin"`

	// Who it answers
	RespondToAllMaxMembers     int      `json:"respondToAllMaxMembers"`     // group: groups up to this size answer everyone by default (0 = never)
	AnswerAnyoneWhoAddressesIt bool     `json:"answerAnyoneWhoAddressesIt"` // group: people not picked still get an answer when they address it
	MaxStreak                  int      `json:"maxStreak"`                  // replies in a row to one person while nobody else speaks (0 = unlimited)
	TriggerWords               []string `json:"triggerWords"`               // always reply when a message contains one
	MuteWords                  []string `json:"muteWords"`                  // never reply to a message containing one

	// Timing
	NoticeMinSec        int  `json:"noticeMinSec"`
	NoticeMaxSec        int  `json:"noticeMaxSec"`
	MarkRead            bool `json:"markRead"`
	WaitForMoreSec      int  `json:"waitForMoreSec"`
	BurstCapSec         int  `json:"burstCapSec"`
	ThinkMinSec         int  `json:"thinkMinSec"`
	ThinkMaxSec         int  `json:"thinkMaxSec"`
	DistractedPercent   int  `json:"distractedPercent"`
	DistractedMinSec    int  `json:"distractedMinSec"`
	DistractedMaxSec    int  `json:"distractedMaxSec"`
	TypingIndicator     bool `json:"typingIndicator"`
	TypingCharsPerSec   int  `json:"typingCharsPerSec"`
	TypingJitterPercent int  `json:"typingJitterPercent"`
	TypingMinSec        int  `json:"typingMinSec"`
	TypingMaxSec        int  `json:"typingMaxSec"`

	// Message shape
	SplitPercent      int    `json:"splitPercent"`
	SplitMaxParts     int    `json:"splitMaxParts"`
	BubbleGapMinSec   int    `json:"bubbleGapMinSec"`
	BubbleGapMaxSec   int    `json:"bubbleGapMaxSec"`
	QuoteReplyPercent int    `json:"quoteReplyPercent"` // group
	AllowMentions     bool   `json:"allowMentions"`     // group: may @tag people
	MentionMax        int    `json:"mentionMax"`        // group: at most this many tags per reply
	TagReplyPercent   int    `json:"tagReplyPercent"`   // group: start with @Name of the person it answers
	LengthBias        string `json:"lengthBias"`        // shorter|normal|longer|match
	TypoPercent       int    `json:"typoPercent"`       // chance one bubble of a reply has a typo (0–30)
	TypoFixStyle      string `json:"typoFixStyle"`      // correction ("*word" bubble)|edit (edit the message)|none

	// Blocks (overridden as a whole per chat)
	Availability Availability `json:"availability"`
	Proactive    Proactive    `json:"proactive"`

	// Safety
	AutoSendSeconds int    `json:"autoSendSeconds"` // approvals: 0 = never auto-send
	InjectionFilter string `json:"injectionFilter"` // strict|balanced|off
}

// BehaviorOverrides mirrors BehaviorProfile with the same JSON names; a nil
// field (absent in JSON) inherits the app profile for the chat's kind.
type BehaviorOverrides struct {
	Preset string `json:"preset,omitempty"`

	ReplyPercent            *int  `json:"replyPercent,omitempty"`
	ReplyWhenNameMentioned  *bool `json:"replyWhenNameMentioned,omitempty"`
	ReplyWhenAtMentioned    *bool `json:"replyWhenAtMentioned,omitempty"`
	SkipWhenOthersMentioned *bool `json:"skipWhenOthersMentioned,omitempty"`
	AIJudgement             *bool `json:"aiJudgement,omitempty"`
	ChimeInPercent          *int  `json:"chimeInPercent,omitempty"`
	IgnoreLinks             *bool `json:"ignoreLinks,omitempty"`
	ReactPercent            *int  `json:"reactPercent,omitempty"`
	PauseWhenYouReply       *bool `json:"pauseWhenYouReply,omitempty"`
	MaxRepliesPerHour       *int  `json:"maxRepliesPerHour,omitempty"`
	MaxRepliesPerDay        *int  `json:"maxRepliesPerDay,omitempty"`
	CooldownSec             *int  `json:"cooldownSec,omitempty"`
	HistoryMessages         *int  `json:"historyMessages,omitempty"`
	HistoryChars            *int  `json:"historyChars,omitempty"`
	StaleAfterMin           *int  `json:"staleAfterMin,omitempty"`

	RespondToAllMaxMembers     *int      `json:"respondToAllMaxMembers,omitempty"`
	AnswerAnyoneWhoAddressesIt *bool     `json:"answerAnyoneWhoAddressesIt,omitempty"`
	MaxStreak                  *int      `json:"maxStreak,omitempty"`
	TriggerWords               *[]string `json:"triggerWords,omitempty"`
	MuteWords                  *[]string `json:"muteWords,omitempty"`

	NoticeMinSec        *int  `json:"noticeMinSec,omitempty"`
	NoticeMaxSec        *int  `json:"noticeMaxSec,omitempty"`
	MarkRead            *bool `json:"markRead,omitempty"`
	WaitForMoreSec      *int  `json:"waitForMoreSec,omitempty"`
	BurstCapSec         *int  `json:"burstCapSec,omitempty"`
	ThinkMinSec         *int  `json:"thinkMinSec,omitempty"`
	ThinkMaxSec         *int  `json:"thinkMaxSec,omitempty"`
	DistractedPercent   *int  `json:"distractedPercent,omitempty"`
	DistractedMinSec    *int  `json:"distractedMinSec,omitempty"`
	DistractedMaxSec    *int  `json:"distractedMaxSec,omitempty"`
	TypingIndicator     *bool `json:"typingIndicator,omitempty"`
	TypingCharsPerSec   *int  `json:"typingCharsPerSec,omitempty"`
	TypingJitterPercent *int  `json:"typingJitterPercent,omitempty"`
	TypingMinSec        *int  `json:"typingMinSec,omitempty"`
	TypingMaxSec        *int  `json:"typingMaxSec,omitempty"`

	SplitPercent      *int    `json:"splitPercent,omitempty"`
	SplitMaxParts     *int    `json:"splitMaxParts,omitempty"`
	BubbleGapMinSec   *int    `json:"bubbleGapMinSec,omitempty"`
	BubbleGapMaxSec   *int    `json:"bubbleGapMaxSec,omitempty"`
	QuoteReplyPercent *int    `json:"quoteReplyPercent,omitempty"`
	AllowMentions     *bool   `json:"allowMentions,omitempty"`
	MentionMax        *int    `json:"mentionMax,omitempty"`
	TagReplyPercent   *int    `json:"tagReplyPercent,omitempty"`
	LengthBias        *string `json:"lengthBias,omitempty"`
	TypoPercent       *int    `json:"typoPercent,omitempty"`
	TypoFixStyle      *string `json:"typoFixStyle,omitempty"`

	Availability *Availability `json:"availability,omitempty"`
	Proactive    *Proactive    `json:"proactive,omitempty"`

	AutoSendSeconds *int    `json:"autoSendSeconds,omitempty"`
	InjectionFilter *string `json:"injectionFilter,omitempty"`
}

// QuoteRef identifies the message a reply quotes (or a reaction targets).
type QuoteRef struct {
	MessageID string `json:"messageId"`
	SenderJID string `json:"senderJid"`
	Text      string `json:"text"`
}
