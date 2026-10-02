package model

import "time"

// ─── Clone yourself (wave 3) ─────────────────────────────────
//
// With your consent (settings clone.collectSamples) Doppel keeps a private
// sample of your own WhatsApp messages (cache/self-samples.jsonl) so the AI
// builder can write a persona that texts like you.

// SelfSample is one of your own messages, redacted (numbers, links).
type SelfSample struct {
	TS   time.Time `json:"ts"`
	Kind string    `json:"kind"` // dm|group
	Text string    `json:"text"`
}

// CloneSamplesView is GET /api/clone/samples.
type CloneSamplesView struct {
	Enabled bool       `json:"enabled"` // settings clone.collectSamples
	Count   int        `json:"count"`
	Since   *time.Time `json:"since"`
	Preview []string   `json:"preview"` // the last few texts
}

// CloneRequest is POST /api/clone/draft.
type CloneRequest struct {
	Name     string `json:"name"`
	Extra    string `json:"extra"` // anything else to know about you
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// CloneDraft is the result of POST /api/clone/draft (not saved).
type CloneDraft struct {
	Persona     Persona `json:"persona"`
	Raw         string  `json:"raw"`
	SampleCount int     `json:"sampleCount"`
}

// MinCloneSamples is how many samples the clone builder needs.
const MinCloneSamples = 20
