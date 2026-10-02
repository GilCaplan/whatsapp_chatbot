package model

import "time"

// ─── Missions and achievements (wave 3) ──────────────────────
//
// A mission is a goal picked from a template ("Get {name} to say {word}").
// Starting one sets the chat's goalOverride + missionId; reaching the goal
// completes it (missions.json). Templates and achievements live in
// internal/mission.

// MissionTemplate is one ready-made goal with blanks to fill.
type MissionTemplate struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Blurb      string  `json:"blurb"`
	Goal       string  `json:"goal"` // with {key} placeholders for Blanks
	Category   string  `json:"category"`
	Blanks     []Blank `json:"blanks"`
	Difficulty int     `json:"difficulty"` // 1–3
	Badge      string  `json:"badge"`      // SVG badge id (web/components/badges.js)
	Detect     string  `json:"detect"`     // "" | "say_word" | "media:image"
}

// Blank is one placeholder of a MissionTemplate.
type Blank struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder"`
}

// MissionRecord is one started (and maybe completed) mission.
type MissionRecord struct {
	ID          string     `json:"id"`
	ChatKey     string     `json:"chatKey"`
	ChatName    string     `json:"chatName"`
	PersonaID   string     `json:"personaId"`
	PersonaName string     `json:"personaName"`
	Goal        string     `json:"goal"`
	TemplateID  string     `json:"templateId"`
	Style       string     `json:"style"`
	StartedAt   time.Time  `json:"startedAt"`
	ReachedAt   *time.Time `json:"reachedAt"`
	How         string     `json:"how"`
	Evidence    string     `json:"evidence"`
}

// Achievement is a badge earned by completing missions.
type Achievement struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Blurb      string     `json:"blurb"`
	Badge      string     `json:"badge"`
	Unlocked   bool       `json:"unlocked"`
	UnlockedAt *time.Time `json:"unlockedAt"`
	Progress   int        `json:"progress"`
	Target     int        `json:"target"`
}

// ActiveMission is a chat currently working on a goal (GET /api/missions).
type ActiveMission struct {
	ChatKey     string   `json:"chatKey"`
	ChatName    string   `json:"chatName"`
	Kind        string   `json:"kind"`
	JID         string   `json:"jid"`
	PersonaID   string   `json:"personaId"`
	PersonaName string   `json:"personaName"`
	MissionID   string   `json:"missionId"`
	Goal        ChatGoal `json:"goal"`
}

// MissionsView is GET /api/missions.
type MissionsView struct {
	Templates    []MissionTemplate `json:"templates"`
	Active       []ActiveMission   `json:"active"`
	History      []MissionRecord   `json:"history"`
	Achievements []Achievement     `json:"achievements"`
}

// MissionStartRequest is POST /api/missions/start.
type MissionStartRequest struct {
	ChatKey    string            `json:"chatKey"`
	TemplateID string            `json:"templateId"`
	Blanks     map[string]string `json:"blanks"`
	Goal       string            `json:"goal"` // free-form goal (no template)
}
