# Tasks: design (draft)

Status: MVP built (Find a time in private chats, privacy layer, secretary hand-over, booking, sandbox, rule editor). Phases 2–3 below are not built yet.

## 1. What it is

A **Task** is an errand the persona does in a chat, with a clear result: "agree on a meeting time with
Dana next week, 1 hour, mornings preferred". It is a goal with structure:

- **Slots**: facts to collect or agree on (date, time, length, place, text, choice, yes/no), each
  with limits.
- **Rules**: *when [condition] → do [action]*, evaluated by code, top to bottom, first match wins.
- **The LLM in narrow jobs**: *Understand* (their message → JSON), *Write* (an action → a message
  in the persona's voice), *Fall back* (pick one allowed action when no rule matches), *Build*
  (a description → a draft spec; cover lines from a prompt).
- **Commit points**: binding moments (confirming a time) pass code checks plus your approval or
  your auto-confirm rule.

Code decides and checks, the LLM reads and writes. No value from the LLM is used before code has
checked it.

## 2. Decisions

| Topic | Decision |
|---|---|
| Task vs goal | A running task **is** the chat's goal: it replaces the goal section; the goal comes back when the task ends |
| Persona | You choose. A generic **Assistant** persona ships as the default: neutral, with tone settings (formality, directness, warmth, length) and local conventions (date format, 24h clock, week start, greeting style). Plus an "About you" field and the calendar connection |
| Priorities | Configurable; MVP is the one-to-one Find a time task |
| Rules | You build them (templates + blanks, "describe it", the when/then list under Advanced) |
| Rule language | A fixed list of conditions and actions, no free-form expressions |
| Auto-confirm | Off by default; per task "auto-confirm if inside my free times" |
| Money | Never discussed. Money intent → deflect + hand off |
| Prompt injection | See section 5 |
| Limits | Configurable (default 20 messages, 7 days, 2 nudges). The task keeps its own compact state, so it survives chat compaction (section 9) |
| Booking | MACalendar's existing API (`POST /events`), unchanged; or an .ics file; or off |
| Secretary | Its Meetings duty becomes a Find a time task with "you confirm"; both use the privacy layer |
| Groups | Polls and chat collection both available, to experiment with (later phase) |
| Languages | Configurable like chats: `settings.behavior.languages` → chat → person |
| Calendar privacy | Section 4. Strategy, cover lines and probe limit are settings with app defaults |
| Blur | No |
| Reminders | Don't block: entries with no end time are zero length |

## 3. Model

```go
type TaskSpec struct {            // tasks.json: templates you saved
    ID, Title, Template string
    Slots   []Slot
    Rules   []Rule                // empty = the template's rules
    Privacy *PrivacyConfig        // nil = settings.tasks.privacy
    Limits  *TaskLimits           // nil = settings.tasks.limits
    Commit  CommitConfig          // approve | auto-if-free; book: off | ics | macalendar
    Voice   string                // as-you | assistant ("I'm arranging this for Gil")
}
type Slot struct { Key, Label, Kind string; Required bool; Limits SlotLimits; Value *SlotValue }
// Kind: datetime | date | time | duration | place | text | choice | yesno
// SlotLimits: days, from/to hours, length, within free times, choices, max length

type Rule struct { When Condition; Do []Action; Step string; Party string; MaxTimes int }

type TaskRun struct {             // taskruns/<id>.json
    ID, SpecID, ChatKey string
    Step     string               // open | negotiate | confirming | committing | done | failed | handed_off
    Slots    map[string]SlotValue // with evidence: message id + exact quote
    Revealed []Reveal             // per person: time → works | doesn't, and what was offered
    Probes   map[string]int       // "doesn't work" answers per person
    Fired    []FiredRule          // the decision log for the task
    Counts   TaskCounts           // messages, nudges
    Result   *TaskResult          // agreed values, booking id
    StartedAt, DeadlineAt time.Time
}
```

**Conditions** (fixed list): `started`, `intent:<i>` with i in accept · reject · propose · counter ·
ask_availability · ask_reason · question · defer · off_topic · money · bot_menu; `slot_filled:<k>`,
`slot_invalid:<k>`, `all_filled`, `probe_limit`, `no_reply:<dur>`, `deadline`, `approved`,
`declined`, `calendar_conflict`. Optional guards: `party`, `step`, `maxTimes`.

**Actions** (fixed list): `offer:<n>`, `ask:<slot>`, `answer_time` (works / doesn't + one
alternative), `ask_their_times`, `ask_confirm`, `commit`, `cover` (deflect or one of your lines),
`nudge`, `wait:<dur>`, `chat` (answer an off-topic message in character, then return),
`handoff` (pause + summary to you), `finish:<ok|fail>`, `notify`.

Each template ships a full rule list. A user rule list is checked on save: every step reachable,
every required slot askable, a `finish` exists, `commit` only after `ask_confirm`.

## 4. Calendar privacy layer

The calendar never reaches an LLM: no titles, places, attendees or free/busy maps. This also
replaces the secretary's `OWNER'S FREE TIMES` list, which today shows the whole week's free
slots (and so the shape of the busy times).

```go
// internal/logic/task/avail.go: pure, given busy intervals from engine (calendar.Service.Busy)
type Availability struct { /* busy, meeting days/hours, length, buffer, loc */ }
func (a Availability) Offer(n int, strategy string, revealed []Reveal, rng Sampler) []Slot  // round times, buffers, random pick
func (a Availability) Check(t time.Time) (ok bool, alt *time.Time)
func (a Availability) Windows(n int) []Window                                              // "Tuesday morning"
func Leak(reply string, allowed []time.Time, titles []string, lang string) []string        // code-only check
```

- **Strategies** (setting, app default + per task): *they go first* (ask for their times,
  intersect, offer one match) · *a few offers* (2–3) · *rough windows* (part of day, then narrow).
- **Picking:** round times only, a buffer around busy blocks, random among good slots.
- **The LLM sees only results:** `OFFER: Tue 10:00, Thu 15:00`, `THEIR TIME Wed 14:00: doesn't
  work, ALTERNATIVE Wed 16:00`. Never a reason.
- **Probe limit** (default 3 per person per task) → configured fallback: send options, ask their
  times, or **hand off** (pause, notify with a summary: offered, said, stuck on).
- **Consistency:** the same time asked twice gets the same answer (`Revealed`).
- **Cover lines:** setting `cover: deflect | lines`. "Help me write these" drafts lines from your
  prompt (Build job). Only saved lines can be sent.
- **Leak check on every draft:** times/dates must be in the allowed set; no words from event
  titles/places; no reason words (EN+HE). One rewrite, then a safe template message.
- **Re-check before commit:** the calendar is read again right before booking.
- Planner, fallback and builder never get calendar input. Activity text never has titles.

Calendar fix (both sources): a timed entry with no end is zero length (doesn't block), instead
of 1 hour (`ics.go` no-DTEND branch, MACalendar rows without `end_time`). Optional
"ignore entries whose title contains…" list.

## 5. Prompt-injection defences

- Their text only reaches *Understand*, inside a fenced data block, with the existing `guard`
  scoring. *Understand* returns a fixed JSON schema (intent + slot candidates + quote); unknown
  fields are dropped.
- Every slot value must quote their message (checked like `goals.EvidenceFound`) and pass the
  slot limits. Nothing from a message can change rules, slots, limits, step or commit settings.
- *Write* gets the chosen action + slot values, not their raw instructions as orders.
- `accept` before `commit` needs an explicit confirmation turn (`ask_confirm`) with a quote.
- Money intent or money words → `cover` + `handoff`, never negotiated.
- Hard caps (messages, days) stop bot-to-bot loops.

## 6. Secretary

`SecretaryConfig.Meetings` starts a Find a time task when someone asks to meet. The secretary's
meeting days, hours and length are its defaults, and commit is "you confirm" (as today). Turning
confirm on in the task books it. The secretary prompt's free-times list goes away in favour of the
task's TASK section.

## 7. Booking (MACalendar, unchanged)

- `POST {base}/events` `{title, date: "YYYY-MM-DD", start_time: "HH:MM", end_time: "HH:MM",
  client_token: <runId>}` with `X-API-Key` (same as the read client). `client_token` makes
  retries safe (MACalendar won't book twice).
- Reschedule/cancel later: `PATCH` / `DELETE /events/<id>`.
- Booking fails → no confirmation sent; you're notified; the task waits in `committing`.

## 8. Prompt placement

The **TASK** section takes the goal's place (after the chat context, before chat instructions):
what the task is, the current action, slot values, and the calendar results it may say. It is open
when `Voice = assistant`, plain "your plan" when `as-you`. Prompt text in `internal/runtime/prompt/task.go`
(goldens).

## 9. Limits and context

Limits: messages, days, nudges, probe limit (settings → spec). The run keeps slots, offers,
`Revealed` and evidence, so the prompt gets a short TASK section instead of the whole
negotiation, and compaction can't lose it. *Understand* reads only messages since the task
started (capped).

## 10. Storage, API, UI

- `tasks.json` (specs), `taskruns/<id>.json`, `ChatAssignment.taskRun` (active run id),
  `settings.tasks` (privacy, cover, limits, commit, booking; settings version bump).
- API (`docs/API.md`): `GET/POST/PUT/DELETE /api/tasks`, `GET /api/tasks/templates`,
  `POST /api/tasks/draft` (describe → spec), `POST /api/tasks/cover-lines`,
  `POST|GET|DELETE /api/chats/{key}/task` (start with blanks / view / stop),
  `POST /api/tasks/{id}/try` (sandbox). SSE `task.changed`; activity `task.*` with `meta.msgId`.
- UI: a Tasks page (templates, your tasks, history) and a Task tab in the chat panel (progress,
  slots, what was offered, approve/stop). Simple first; rules under Advanced.
- Sandbox: an LLM plays the other side (cooperative, vague, difficult, prober, bot).

## 11. Code layout

| Path | Role |
|---|---|
| `internal/core/model/task.go` | types |
| `internal/logic/task` (new leaf: `model` + `calendar` + `lang`) | templates, spec checks, rule matching `(spec, run, event) → []Action`, slot parsing/limits, availability + `Leak`, cover-line pick |
| `internal/runtime/prompt/task.go` | Understand, Write, fallback, builder, cover-line prompts |
| `internal/runtime/engine/task.go` | runs: hooks into the reply cycle (replaces `chatGoalTurn` while a run is active), timers via `Engine.clock`, calls via the job slot, approvals, booking, decision log |
| `internal/core/store/tasks.go` | specs + runs |
| `internal/host/server` | routes |
| `web/pages/tasks.js`, `web/components/drawer/task.js` | UI |

## 12. Phases

1. **MVP:** privacy layer (+ secretary moved onto it, calendar fix), Find a time in one DM, the
   Assistant persona, Understand/Write, approve at commit, MACalendar/.ics booking, sandbox.
2. Your own rules + Build, collect info, reschedule/cancel, business bots (literal mode).
3. Groups (polls / chat collection, decision rule), go-between across DMs.

## 13. Open

- Go-between across DMs (phase 3): one task, several chats; only slot values cross.
- Bot detection signals and button/list message parsing in `wa`.
