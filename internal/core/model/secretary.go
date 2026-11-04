package model

import "time"

// Personal secretary: a persona that answers as the owner's assistant (it
// says so), takes messages, flags urgent ones, answers FAQs and helps settle
// meetings from the owner's read-only calendars (busy times only).

// SecretaryConfig turns a persona into the owner's personal secretary.
type SecretaryConfig struct {
	OwnerName string `json:"ownerName"` // "Gil"
	Tone      string `json:"tone"`      // friendly|formal ("" = friendly)

	// Duties.
	TakeMessages bool `json:"takeMessages"` // collect who/what/how urgent/how to reach → Messages for you
	FlagUrgent   bool `json:"flagUrgent"`   // urgent messages notify the owner at once
	AnswerFAQs   bool `json:"answerFaqs"`   // answer from FAQs and Shareable
	Meetings     bool `json:"meetings"`     // suggest times from the calendar (never confirms)

	// When people can meet the owner: days (mon..sun) and hours, and how long
	// a meeting usually is; the calendars' busy times are taken out.
	MeetDays       []string `json:"meetDays"`
	MeetFrom       string   `json:"meetFrom"` // "09:00"
	MeetTo         string   `json:"meetTo"`   // "18:00"
	MeetingMinutes int      `json:"meetingMinutes"`
	UseCalendars   bool     `json:"useCalendars"`

	Shareable   string   `json:"shareable"`   // what it may tell people ("travelling until Sunday")
	Never       string   `json:"never"`       // what it must never do or say
	AwayMessage string   `json:"awayMessage"` // an out-of-office line it may use
	FAQs        []FAQ    `json:"faqs"`
	VIPs        []string `json:"vips"` // names or numbers whose messages are always urgent
}

// FAQ is a question the secretary may answer, with the owner's answer.
type FAQ struct {
	Q string `json:"q"`
	A string `json:"a"`
}

// CalendarSource is one of the owner's calendars, read-only.
type CalendarSource struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`             // ics|macalendar
	URL     string `json:"url"`              // the .ics link, or the MACalendar API base (http://127.0.0.1:8080)
	APIKey  string `json:"apiKey,omitempty"` // MACalendar's X-API-Key, if it has one
	Enabled bool   `json:"enabled"`
	// SkipWords: entries whose title contains one of these never count as
	// busy ("reminder", "birthday").
	SkipWords []string `json:"skipWords,omitempty"`
}

// Calendar source kinds.
const (
	CalendarICS        = "ics"
	CalendarMACalendar = "macalendar"
)

// SecretaryNote is a message the secretary took for the owner.
type SecretaryNote struct {
	ID       string    `json:"id"`
	ChatKey  string    `json:"chatKey"`
	ChatName string    `json:"chatName"`
	From     string    `json:"from"`
	FromJID  string    `json:"fromJid,omitempty"`
	Text     string    `json:"text"`               // what they want, in a sentence or two
	Urgent   bool      `json:"urgent"`             // needs the owner soon
	Callback string    `json:"callback,omitempty"` // how/when to get back to them
	Meeting  string    `json:"meeting,omitempty"`  // a meeting they asked for (time, topic)
	At       time.Time `json:"at"`
	Done     bool      `json:"done"`
}

// CalendarCheck is the result of reading one calendar now (Settings test).
type CalendarCheck struct {
	OK       bool           `json:"ok"`
	Error    string         `json:"error,omitempty"`
	Events   int            `json:"events"` // in the next three weeks
	Upcoming []CalendarItem `json:"upcoming"`
}

// CalendarItem is one upcoming event, for the owner's own check.
type CalendarItem struct {
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	AllDay bool      `json:"allDay"`
	Title  string    `json:"title"`
}

// ActSecretaryNote is the activity type of a message taken for the owner
// (meta urgent).
const ActSecretaryNote = "secretary.note"
