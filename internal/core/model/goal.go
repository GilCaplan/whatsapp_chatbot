package model

import "time"

// ─── Goals ───────────────────────────────────────────────────
//
// A persona (or one chat) can have a goal, e.g. "get Josh to say the word
// apple". How it is pursued is set by the persona (goalStyle, goalPlanAhead,
// goalAfterReached) and can be overridden per chat (ChatAssignment.GoalStyle,
// ChatAssignment.GoalPlanAhead). Progress is tracked per chat in GoalStatus.

// Goal styles: how openly the persona pursues its goal.
const (
	GoalStyleSubtle   = "subtle"   // secret, gradual, indirect (default)
	GoalStyleBalanced = "balanced" // may ask about it naturally, never reveals it's a goal
	GoalStyleDirect   = "direct"   // goes for it openly
)

// GoalStyles lists the valid goal styles.
var GoalStyles = []string{GoalStyleSubtle, GoalStyleBalanced, GoalStyleDirect}

// What the persona does once its goal is reached.
const (
	GoalAfterRelax    = "relax"    // stop pursuing and just chat (default)
	GoalAfterContinue = "continue" // keep gently pursuing it
)

// GoalAfterOptions lists the valid goalAfterReached values.
var GoalAfterOptions = []string{GoalAfterRelax, GoalAfterContinue}

// ActGoalReached is the activity type published when a chat's goal is reached.
const ActGoalReached = "goal.reached"

// How a goal was detected as reached (GoalStatus.How).
const (
	GoalHowSaidWord = "said_word" // they said the target word (deterministic check)
	GoalHowAI       = "ai"        // the plan-ahead step saw it happen (with evidence)
)

// GoalStatus is a chat's progress on its goal (persisted in runtime.json).
// It belongs to one goal text: when the chat's effective goal changes, the
// status starts over.
type GoalStatus struct {
	Goal       string     `json:"goal"`  // the goal text this status is about
	Since      time.Time  `json:"since"` // when the engine started working on it
	Reached    bool       `json:"reached"`
	ReachedAt  *time.Time `json:"reachedAt"`
	How        string     `json:"how,omitempty"`      // said_word|ai
	Evidence   string     `json:"evidence,omitempty"` // e.g. "Josh: apple pie, obviously"
	LastPlan   string     `json:"lastPlan,omitempty"` // latest private next move (plan ahead)
	LastPlanAt *time.Time `json:"lastPlanAt,omitempty"`
	Plans      []string   `json:"plans,omitempty"` // recent moves, newest last (fed back to the planner)
}

// Clone returns a deep copy.
func (g GoalStatus) Clone() GoalStatus {
	if g.ReachedAt != nil {
		t := *g.ReachedAt
		g.ReachedAt = &t
	}
	if g.LastPlanAt != nil {
		t := *g.LastPlanAt
		g.LastPlanAt = &t
	}
	g.Plans = append([]string(nil), g.Plans...)
	return g
}

// ChatGoal is a chat's resolved goal settings plus its status (API view).
// Sources are "chat", "persona" or "default".
type ChatGoal struct {
	Text            string `json:"text"`
	Source          string `json:"source"`
	Style           string `json:"style"`
	StyleSource     string `json:"styleSource"`
	PlanAhead       bool   `json:"planAhead"`
	PlanAheadSource string `json:"planAheadSource"`
	AfterReached    string `json:"afterReached"`
	// State is "working" or "reached".
	State      string     `json:"state"`
	ReachedAt  *time.Time `json:"reachedAt"`
	How        string     `json:"how,omitempty"`
	Evidence   string     `json:"evidence,omitempty"`
	LastPlan   string     `json:"lastPlan,omitempty"`
	LastPlanAt *time.Time `json:"lastPlanAt,omitempty"`
}

// PlaygroundGoal is the goal side of a playground reply (never sent to WhatsApp).
type PlaygroundGoal struct {
	Text      string `json:"text"`
	Style     string `json:"style"`
	Plan      string `json:"plan,omitempty"` // the private next move (plan ahead on)
	Reached   bool   `json:"reached"`
	Evidence  string `json:"evidence,omitempty"`
	Rewritten bool   `json:"rewritten,omitempty"` // the first draft gave the goal away and was rewritten
}
