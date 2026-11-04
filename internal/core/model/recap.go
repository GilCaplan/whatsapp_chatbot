package model

import "time"

// ─── Daily recap (wave 3) ────────────────────────────────────

// Recap summarises one chat's day (recaps.json).
type Recap struct {
	ID           string    `json:"id"`
	ChatKey      string    `json:"chatKey"`
	ChatName     string    `json:"chatName"`
	PersonaName  string    `json:"personaName"`
	Date         string    `json:"date"` // "2026-10-01" in this Mac's zone
	From         time.Time `json:"from"`
	To           time.Time `json:"to"`
	MessageCount int       `json:"messageCount"`
	Headline     string    `json:"headline"`
	Topics       []string  `json:"topics"`
	GoalProgress string    `json:"goalProgress"`
	ToKnow       []string  `json:"toKnow"`
	Mood         string    `json:"mood"`
	GeneratedAt  time.Time `json:"generatedAt"`
	OnDemand     bool      `json:"onDemand"`
}

// RecapsView is GET /api/recaps and POST /api/recaps/generate.
type RecapsView struct {
	Items []Recap `json:"items"` // newest first
}
