package model

// ─── Where the persona lives (wave 3) ────────────────────────
//
// A persona can live somewhere else than you: its own city, country, time
// zone and a daily routine. The zero value is valid and means "same time zone
// as this Mac, no routine". Logic lives in internal/world.

// World is the persona's home and daily routine.
type World struct {
	City     string         `json:"city"`
	Country  string         `json:"country"`  // free text; world.Weekend matches a small table
	Timezone string         `json:"timezone"` // IANA name; "" = this Mac ("same as me")
	Routine  []RoutineBlock `json:"routine"`  // in Timezone; at most MaxRoutineBlocks
}

// RoutineBlock is one recurring part of the persona's day ("gym 07:00–08:00").
type RoutineBlock struct {
	Label string   `json:"label"` // "gym", "work", "dinner", "sleep" (free text, ≤ 24 characters)
	Days  []string `json:"days"`  // mon..sun; empty = every day
	From  string   `json:"from"`  // "HH:MM"; To <= From wraps past midnight (like TimeRange)
	To    string   `json:"to"`
	Reach string   `json:"reach"` // normal|slow|unreachable (see Reach*)
}

// How reachable the persona is during a routine block (RoutineBlock.Reach).
const (
	ReachNormal      = "normal"      // answers as usual
	ReachSlow        = "slow"        // notices messages later
	ReachUnreachable = "unreachable" // answers after the block ends
)

// Reaches lists the valid RoutineBlock.Reach values.
var Reaches = []string{ReachNormal, ReachSlow, ReachUnreachable}

// Routine limits.
const (
	MaxRoutineBlocks  = 12
	MaxRoutineLabel   = 24
	PersonaZoneMarker = "persona" // Availability.Timezone: follow the persona's World.Timezone
)
