package model

import "time"

// Tasks (internal/logic/task, engine/task.go): an errand the persona does in a
// chat, with a clear result, e.g. "agree on a meeting time with Dana". Code
// runs the rules and checks every value; the LLM only reads their messages
// (into JSON) and writes ours. A running task is the chat's goal.

// Task templates.
const TaskFindTime = "find-time"

// Strategies: how times are offered (privacy).
const (
	StrategyTheyFirst = "they-first" // ask for their times, offer one match
	StrategyOffers    = "offers"     // offer a few times
	StrategyWindows   = "windows"    // offer parts of days, then narrow down
)

// What happens when the probe limit is reached.
const (
	ProbeOffer   = "offer"   // send a few options instead
	ProbeAsk     = "ask"     // ask for their times
	ProbeHandoff = "handoff" // pause and hand over to you, with a summary
)

// Cover modes: what to say when asked why a time doesn't work.
const (
	CoverDeflect = "deflect" // "I've got something then"
	CoverLines   = "lines"   // one of your own lines
)

// Commit modes and booking.
const (
	CommitApprove  = "approve"      // you approve every confirmation
	CommitAutoFree = "auto-if-free" // confirm by itself when the time is still free
	BookOff        = "off"
	BookICS        = "ics"        // a calendar file you open
	BookMACalendar = "macalendar" // MACalendar's API (POST /events)
)

// Voices.
const (
	VoiceAsYou     = "as-you"    // writes as you
	VoiceAssistant = "assistant" // openly your assistant ("I'm arranging this for Gil")
)

// Slot kinds.
const (
	SlotWhen   = "when" // the meeting time (one per task, checked against your calendar)
	SlotText   = "text"
	SlotPlace  = "place"
	SlotChoice = "choice"
	SlotYesNo  = "yesno"
)

// Task run statuses.
const (
	TaskActive    = "active"
	TaskApproval  = "approval" // a confirmation waits for you
	TaskDone      = "done"
	TaskFailed    = "failed"
	TaskHandedOff = "handed-off"
	TaskStopped   = "stopped"
)

// Task steps.
const (
	StepOpen       = "open"
	StepNegotiate  = "negotiate"
	StepConfirming = "confirming" // we asked "shall we lock it in?"
	StepCommitting = "committing" // waiting for your approval / booking
	StepEnded      = "ended"
)

// TaskPrivacy is how much of your calendar a task may show.
type TaskPrivacy struct {
	Strategy     string   `json:"strategy"`     // they-first|offers|windows
	Offers       int      `json:"offers"`       // 1–3 times per offer
	ProbeLimit   int      `json:"probeLimit"`   // 1–10 "doesn't work" answers per task
	OnProbeLimit string   `json:"onProbeLimit"` // offer|ask|handoff
	Cover        string   `json:"cover"`        // deflect|lines
	CoverLines   []string `json:"coverLines"`   // your own lines (cover = lines)
	BufferMin    int      `json:"bufferMin"`    // 0–60: kept free around busy times when offering
}

// TaskLimits stop a task that goes nowhere.
type TaskLimits struct {
	MaxMessages     int `json:"maxMessages"`     // 4–100 of ours
	MaxDays         int `json:"maxDays"`         // 1–30
	MaxNudges       int `json:"maxNudges"`       // 0–5
	NudgeAfterHours int `json:"nudgeAfterHours"` // 1–168
}

// TaskCommit is how a task confirms.
type TaskCommit struct {
	Mode       string `json:"mode"`       // approve|auto-if-free
	Book       string `json:"book"`       // off|ics|macalendar
	CalendarID string `json:"calendarId"` // the MACalendar source to book in ("" = the first one)
}

// TaskTone is how task messages sound, plus local conventions.
type TaskTone struct {
	Formality  string `json:"formality"`  // casual|neutral|formal
	Directness string `json:"directness"` // soft|balanced|direct
	Warmth     string `json:"warmth"`     // reserved|friendly|warm
	DateStyle  string `json:"dateStyle"`  // dmy ("Tue 10 Nov")|mdy ("Tue Nov 10")
	Clock      string `json:"clock"`      // 24h|12h
}

// TaskOwner is you, as tasks may describe you.
type TaskOwner struct {
	Name  string `json:"name"`  // "Gil"
	About string `json:"about"` // what it may say about you, if it helps
}

// TaskMeeting is when a meeting may happen.
type TaskMeeting struct {
	Minutes     int      `json:"minutes"`     // 15–480
	Days        []string `json:"days"`        // mon..sun ([] = weekdays)
	From        string   `json:"from"`        // "09:00"
	To          string   `json:"to"`          // "18:00"
	HorizonDays int      `json:"horizonDays"` // 1–30 days ahead
	Prefer      string   `json:"prefer"`      // ""|morning|afternoon|evening
	Calendars   bool     `json:"calendars"`   // take your calendars' busy times out
}

// TaskSlot is one fact the task collects or agrees on.
type TaskSlot struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"` // when|text|place|choice|yesno
	Required bool     `json:"required"`
	Choices  []string `json:"choices,omitempty"`
}

// TaskRule: when the condition holds (in that step, "" = any), do the actions.
type TaskRule struct {
	When     string   `json:"when"`
	Step     string   `json:"step,omitempty"`
	Do       []string `json:"do"`
	MaxTimes int      `json:"maxTimes,omitempty"` // 0 = no limit
}

// TaskSpec is a task as you set it up (tasks.json keeps the ones you saved).
type TaskSpec struct {
	ID       string      `json:"id"`
	Title    string      `json:"title"`
	Template string      `json:"template"`
	About    string      `json:"about"` // what the meeting is about ("project kickoff")
	Place    string      `json:"place"` // "Zoom", "the office" ("" = not part of it)
	Voice    string      `json:"voice"` // as-you|assistant
	Meeting  TaskMeeting `json:"meeting"`
	Slots    []TaskSlot  `json:"slots"`
	Rules    []TaskRule  `json:"rules"` // [] = the template's rules
	// nil = settings.tasks.
	Privacy   *TaskPrivacy `json:"privacy"`
	Limits    *TaskLimits  `json:"limits"`
	Commit    *TaskCommit  `json:"commit"`
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

// TaskValue is a collected value, with the words it came from.
type TaskValue struct {
	Value string     `json:"value"`
	Time  *time.Time `json:"time,omitempty"` // kind when
	Quote string     `json:"quote,omitempty"`
	MsgID string     `json:"msgId,omitempty"`
	At    time.Time  `json:"at"`
}

// TaskAnswer is a time they asked about and what we said.
type TaskAnswer struct {
	Time  time.Time `json:"time"`
	Works bool      `json:"works"`
}

// TaskEvent is one line of a run's log.
type TaskEvent struct {
	At    time.Time `json:"at"`
	Kind  string    `json:"kind"` // started|read|rule|sent|commit|booked|ended|note
	Text  string    `json:"text"`
	Rule  string    `json:"rule,omitempty"`
	MsgID string    `json:"msgId,omitempty"`
}

// TaskResult is how a run ended.
type TaskResult struct {
	OK      bool              `json:"ok"`
	When    *time.Time        `json:"when,omitempty"`
	End     *time.Time        `json:"end,omitempty"`
	Values  map[string]string `json:"values,omitempty"`
	Booked  string            `json:"booked,omitempty"`  // ""|ics|macalendar
	EventID string            `json:"eventId,omitempty"` // MACalendar's event id
	Note    string            `json:"note,omitempty"`
}

// TaskRun is a task running (or ended) in one chat.
type TaskRun struct {
	ID string `json:"id"`
	// Rev goes up on every write: a writer that read rev n only saves if it
	// is still n (store.SwapTaskRun), so a slow reply cycle never undoes a
	// Stop or an approval made meanwhile.
	Rev       int                  `json:"rev"`
	ChatKey   string               `json:"chatKey"`
	ChatName  string               `json:"chatName"`
	Spec      TaskSpec             `json:"spec"` // resolved when started
	Owner     TaskOwner            `json:"owner"`
	Tone      TaskTone             `json:"tone"`
	Status    string               `json:"status"`
	Step      string               `json:"step"`
	Values    map[string]TaskValue `json:"values"`
	Offered   []time.Time          `json:"offered"`   // times we offered, oldest first
	LastOffer []time.Time          `json:"lastOffer"` // the times of our latest offer
	Answers   []TaskAnswer         `json:"answers"`   // times they asked about
	Probes    int                  `json:"probes"`    // "doesn't work" answers given
	Fired     map[string]int       `json:"fired"`     // rule index → times fired
	Log       []TaskEvent          `json:"log"`
	Sent      int                  `json:"sent"`   // our messages so far
	Nudges    int                  `json:"nudges"` // nudges sent
	// ReadTo / ReadMsg: their messages up to here (that message) are read.
	ReadTo      time.Time `json:"readTo"`
	ReadMsg     string    `json:"readMsg,omitempty"`
	LastOursAt  time.Time `json:"lastOursAt"`
	LastTheirAt time.Time `json:"lastTheirAt"`
	// Next is a message the engine starts itself (a nudge, the confirmation
	// after your approval): task.Say* and its times ("" = none).
	Next      string      `json:"next,omitempty"`
	NextTimes []time.Time `json:"nextTimes,omitempty"`
	// Proposed is the time waiting for their yes (step confirming) or your approval.
	Proposed  *time.Time  `json:"proposed,omitempty"`
	Result    *TaskResult `json:"result,omitempty"`
	Summary   string      `json:"summary,omitempty"` // for you, when handed over or ended
	StartedAt time.Time   `json:"startedAt"`
	UpdatedAt time.Time   `json:"updatedAt"`
	EndedAt   *time.Time  `json:"endedAt,omitempty"`
}

// Active reports whether the run still works in its chat.
func (r TaskRun) Active() bool { return r.Status == TaskActive || r.Status == TaskApproval }

// TaskTemplate is a ready-made task shown in the picker.
type TaskTemplate struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Blurb string   `json:"blurb"`
	Spec  TaskSpec `json:"spec"`
}

// TaskTry is one sandbox run: an AI plays the other side, nothing is sent
// or saved.
type TaskTry struct {
	Turns  []TaskTryTurn `json:"turns"`
	Result *TaskResult   `json:"result,omitempty"`
	Status string        `json:"status"`
	Log    []TaskEvent   `json:"log"`
}

// TaskTryTurn is one message of a sandbox run.
type TaskTryTurn struct {
	Speaker string `json:"speaker"` // them|me
	Text    string `json:"text"`
	Action  string `json:"action,omitempty"` // what the rules chose (ours)
	Note    string `json:"note,omitempty"`   // what code read or checked
}

// TaskReading is what the reading step (an LLM, checked by code) made of
// their new messages.
type TaskReading struct {
	Intent string         `json:"intent"` // see task.Intents
	Times  []TaskProposal `json:"times"`  // times they proposed or agreed to
	Values []TaskFound    `json:"values"` // other slots they answered
	Quote  string         `json:"quote"`  // their exact words that show the intent
}

// TaskProposal is a time they mentioned: a date, and a time or a part of day.
type TaskProposal struct {
	Date string `json:"date"` // YYYY-MM-DD
	Time string `json:"time"` // HH:MM ("" = only a day or part of day)
	Part string `json:"part"` // morning|afternoon|evening|""
}

// TaskFound is a slot value they gave, with their words.
type TaskFound struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Quote string `json:"quote"`
}
