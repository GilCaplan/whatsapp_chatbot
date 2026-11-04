package model

import "time"

// ─── Context systems ─────────────────────────────────────────
//
// A context system is how a chat decides when to think and what the persona
// reads: who decides to join in, how often the goal is planned, what checks
// a reply, which background work runs and how big each prompt part may be.
// Three are built in (internal/logic/ctxsys: Classic, Lean, Fast; locked); you can
// copy one and tune it. The flowchart on each system is a picture of it:
// built-in charts are drawn from the code and locked, your own are free to
// edit (only Params change how chats run). Chosen per chat → persona → app.

// Context system bases (ContextSystem.Base).
const (
	CtxClassic = "classic"
	CtxLean    = "lean"
	CtxFast    = "fast"
)

// ContextSystem is one configurable context system.
type ContextSystem struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Base        string        `json:"base"` // classic|lean|fast: what it was made from
	BuiltIn     bool          `json:"builtIn"`
	Params      ContextParams `json:"params"`
	Chart       ContextChart  `json:"chart"`
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
}

// Join-in modes (ContextParams.JoinIn): who decides whether to answer a
// group message nobody addressed to it.
const (
	JoinAI         = "ai"          // an AI check on every such message
	JoinRulesFirst = "rules_first" // clear cases in code, the AI only when unsure
	JoinRulesOnly  = "rules_only"  // code only; unsure = the chime-in chance
)

// Reply checks (ContextParams.ReplyCheck).
const (
	CheckOff       = "off"
	CheckCode      = "code"       // instant checks in code
	CheckCodeJudge = "code_judge" // code, plus a small AI judge on borderline replies
	CheckJudge     = "judge"      // code, plus the AI judge on every reply
)

// Goal planning (ContextParams.Planning); only where plan ahead is on.
const (
	PlanEveryReply = "every_reply"
	PlanPeriodic   = "periodic" // every PlanEvery replies or when the topic moved on
	PlanOff        = "off"
)

// Background work (ContextParams.Background).
const (
	BackgroundSeparate = "separate" // memories, notes, brief each on their own
	BackgroundDigest   = "digest"   // one call every DigestEvery messages does all
)

// Prompt styles (ContextParams.PromptStyle).
const (
	PromptFull = "full" // the whole persona text
	PromptCard = "card" // a short persona card (Persona.Card) + part budgets
)

// Summary formats (ContextParams.Summary).
const (
	SummaryProse = "prose"
	SummaryFacts = "facts" // one editable fact per line; only changed lines move
)

// ContextParams are the knobs of a context system. Zero budgets = no limit.
type ContextParams struct {
	JoinIn string `json:"joinIn"`

	ReplyCheck   string `json:"replyCheck"`
	CheckRetries int    `json:"checkRetries"` // rewrites with the reason before it stays quiet
	RepeatWindow int    `json:"repeatWindow"` // its own last replies compared for repeats
	EchoWords    int    `json:"echoWords"`    // a run this long copied from its instructions = an echo

	Planning     string `json:"planning"`
	PlanEvery    int    `json:"planEvery"`   // periodic: replies between plans
	TopicShift   int    `json:"topicShift"`  // periodic: messages since the last plan that call for a new one
	MergedCheck  bool   `json:"mergedCheck"` // "has it happened?" inside the plan call (no second call)
	NudgeEvery   int    `json:"nudgeEvery"`  // a goal nudge at most every Nth reply (1 = every reply)
	Background   string `json:"background"`
	DigestEvery  int    `json:"digestEvery"` // digest: messages from them between digests
	PromptStyle  string `json:"promptStyle"`
	CardChars    int    `json:"cardChars"`    // card: the persona card's size
	HistoryMax   int    `json:"historyMax"`   // messages in the reply prompt (0 = everything not yet summarised)
	MemoryLines  int    `json:"memoryLines"`  // memories in the reply prompt (0 = the default)
	SummaryChars int    `json:"summaryChars"` // the older-messages summary in the prompt (0 = all)
	Summary      string `json:"summary"`
}

// ContextChart is a flowchart of a context system: blocks and wires.
type ContextChart struct {
	Nodes []ChartNode `json:"nodes"`
	Edges []ChartEdge `json:"edges"`
}

// ChartNode is one block on a chart (Block is a catalog id, ctxsys.Blocks).
type ChartNode struct {
	ID    string  `json:"id"`
	Block string  `json:"block"`
	Label string  `json:"label"`
	Note  string  `json:"note,omitempty"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
}

// ChartEdge is a wire between two nodes; Dashed = feeds in the background.
type ChartEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Label  string `json:"label,omitempty"`
	Dashed bool   `json:"dashed,omitempty"`
}

// ChartBlock is a building block in the catalog.
type ChartBlock struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Stage       string   `json:"stage"` // arrive|gate|join|goal|prompt|write|check|out|background|custom
	Kind        string   `json:"kind"`  // ai|code|data|io|note
	Description string   `json:"description"`
	Params      []string `json:"params"` // ContextParams json keys this block stands for
}

// ContextChoice is where a chat's system comes from (API view).
type ContextChoice struct {
	SystemID string `json:"systemId"`
	Source   string `json:"source"` // chat|persona|app
}

// ReplayRequest asks the sandbox to replay a chat with some systems.
type ReplayRequest struct {
	ChatKey  string   `json:"chatKey"`
	Systems  []string `json:"systems"`
	Messages int      `json:"messages"` // their last N messages (default 8)
}

// Replay is a sandbox run: a chat's recent messages answered again with
// each system (nothing is sent or saved).
type Replay struct {
	ID        string         `json:"id"`
	ChatKey   string         `json:"chatKey"`
	ChatName  string         `json:"chatName"`
	Status    string         `json:"status"` // running|done|failed|cancelled
	Error     string         `json:"error,omitempty"`
	Done      int            `json:"done"`
	Total     int            `json:"total"`
	Results   []ReplayResult `json:"results"`
	StartedAt time.Time      `json:"startedAt"`
	EndedAt   *time.Time     `json:"endedAt"`
}

// ReplayResult is one system's run.
type ReplayResult struct {
	SystemID    string         `json:"systemId"`
	SystemName  string         `json:"systemName"`
	Calls       int            `json:"calls"`
	CallsByStep map[string]int `json:"callsByStep"`
	PromptChars int            `json:"promptChars"` // average reply prompt
	TotalMs     int64          `json:"totalMs"`
	Replied     int            `json:"replied"`
	Skipped     int            `json:"skipped"`
	Background  int            `json:"background"` // background calls these messages would add
	Steps       []ReplayStep   `json:"steps"`
}

// ReplayStep is one of their messages answered in the sandbox.
type ReplayStep struct {
	From     string   `json:"from"`
	Text     string   `json:"text"`
	Real     string   `json:"real,omitempty"` // what it really answered then ("" = nothing)
	Decision string   `json:"decision"`       // reply|skip
	Why      string   `json:"why"`
	Reply    string   `json:"reply,omitempty"`
	Calls    int      `json:"calls"`
	Ms       int64    `json:"ms"`
	Checks   []string `json:"checks,omitempty"` // reply-check findings
	Plan     string   `json:"plan,omitempty"`
}
