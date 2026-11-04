package model

import "time"

// Connection ratings: once a day (20+ messages in the group that day) the
// persona scores, in character, how connected it felt to each person who
// wrote. Private to the owner: nothing is sent to WhatsApp.

// RatingDay is one day's ratings in a chat.
type RatingDay struct {
	ChatKey  string        `json:"chatKey"`
	Date     string        `json:"date"` // YYYY-MM-DD (this Mac's zone)
	At       time.Time     `json:"at"`
	Messages int           `json:"messages"` // messages it read
	OnDemand bool          `json:"onDemand,omitempty"`
	Scores   []PersonScore `json:"scores"` // best first
}

// PersonScore is one person's rating on a day.
type PersonScore struct {
	Name   string `json:"name"`
	JID    string `json:"jid,omitempty"`
	Score  int    `json:"score"` // 0–100
	Why    string `json:"why"`
	Change *int   `json:"change,omitempty"` // since their previous rating (nil = first)
}

// ActRating is the activity type of a new day of ratings.
const ActRating = "rating"
