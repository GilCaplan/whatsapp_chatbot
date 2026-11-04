# Feature rules

<!-- Moved word for word from CLAUDE.md; CLAUDE.md points here. -->

How each feature is wired (read the one you are changing). Shared rules stay in CLAUDE.md.

- Vibe dials (Speed, Chattiness, Boldness; 5 levels each) are **derived, never stored**: `behavior.DialTable()` is the
  single source of truth (served in `GET /api/behavior/presets` → `dials`); moving a dial writes its fields through the
  normal Settings/PATCH paths, `behavior.ReadDials` (mirrored in `web/components/behaviour/vibe-dials.js`) reads a profile back
  (exact level or nearest + "tuned"). Every field belongs to at most one dial and must stay monotone; every preset must
  land exactly on a level of every dial (`TestPresetsLandOnDials`) — re-check both when changing a preset or a range.
  Group-only fields are absent from the private table. In the UI the dials sit between the presets and the timeline;
  the full form is behind an **Advanced** switch, and form rows a dial owns carry its coloured label.
- Missions are goals: `POST /api/missions/start` sets `goalOverride` + `missionId` (template id) and starts the goal
  status over; `engine.publishGoalReached` completes the chat's open `MissionRecord` (or adds one, so any reached goal
  counts) and publishes `missions.changed`; media missions are reached in `engine.goalMedia` (from `goalIncoming`,
  `Incoming.Media`). Achievements are computed from `missions.json`, never stored. Hand-editing the goal text drops
  `missionId`.
- Looks ("skins", `settings.skin`): colours, blurs, radii and shadows only via tokens (`styles/tokens.css` = Liquid Glass
  defaults, `styles/skins.css` = overrides per `<html data-skin>`); never hard-code a colour in a component. `data-theme` on
  `<html>` is always the *resolved* mode (boot.js before paint, then `skins.js applyAppearance`); Midnight/Neon are dark-only
  and Daylight light-only without rewriting `settings.theme`. `web/skins.js` mirrors `config.Skins` and the ONLY map in
  `boot.js`. Illustrations read `var(--art-*, <glass colour>)`; data colours (avatars, medals, dials) are tinted per look
  with `--avatar-filter`/`--art-filter`. Settings › Appearance previews use the real tokens (`.skin-preview[data-skin]`).
- Reply behaviour = 3 layers: `settings.behavior.private|group` → `ChatAssignment.behavior` (nil field =
  inherit; `availability`/`proactive` are whole blocks) → `behavior.Resolve`. Never read timing from anywhere
  else. `PATCH /api/chats/{key}` `behavior`: absent = keep, `null` = inherit (field or everything).
- Goals are not behaviour: `Persona.goal/goalStyle/goalPlanAhead/goalAfterReached` → `ChatAssignment.goalOverride/
  goalStyle/goalPlanAhead` (nil = persona's) → `goals.Resolve`. The reply prompt frames the goal as a secret
  "PRIVATE AGENDA" (`subtle` default: never state/hint it, never write the target word, rapport first, one nudge per
  message, set-ups where it happens by itself). With plan ahead, `engine.chatGoalTurn` first runs a private JSON-mode
  call (`prompt.Plan`) whose `next` move goes into the reply prompt as "YOUR NEXT MOVE" (never into WhatsApp/history)
  and, alongside it, a narrow yes/no `prompt.GoalCheck` ("has it already happened?") whose quote must be found in
  their messages since the goal was set. "Say the word" goals are checked
  deterministically on every incoming message (`engine.goalIncoming`). `engine.goalReply` rewrites leaky drafts
  (`goals.Leak`) and replies written as another group member ("Josh: …", `prompt.SpeaksAs` → `engine.ownVoice`). Progress lives in `RunnerState.Goal` (runtime.json), keyed by goal text; activity `goal.reached`.
  "Start a conversation" (`POST /api/chats/{key}/initiate`, `engine.Initiate` in `opener.go`) starts a proactive
  cycle straight at *thinking* with `prompt.Opener{Manual, Hint, Silence}` (goal-aware, group-aware opener; planner
  in opening mode); automatic check-ins use the same opener prompt with the silence length.
  Engine tests turn plan ahead off in the harness (`planAheadOff`) and enable it per chat. Tune prompts with the
  live eval: `DOPPEL_GOAL_EVAL=1 GOAL_EVAL_OUT=<dir> GOAL_EVAL_LABEL=x GOAL_EVAL_PLAN=1 GOAL_EVAL_GUARD=1 go test
  ./internal/runtime/prompt -run 'TestGoalEval$' -v -timeout 3h` (needs Ollama with llama3.1:8b; ~30 min).
- Scheduled messages live in the behaviour block `proactive.schedule` (`onStart`, `times`, `repeat`, `everyMin`,
  `spreadMin`, `maxPerDay`; zero values = defaults, so presets stay exact). `engine/schedule.go` keeps one timer per
  runner; a due moment is sent only if the chat can take it right then (`scheduleBlockedLocked`: WA connected, still in
  the group, no hand-off, not Away, idle, active hours, routine, cap), otherwise skipped — **never sent late**, also for
  moments missed while the app was closed. `onStart` fires on Reload for newly enabled chats (not on app start:
  `Engine.loaded`) and on `HandleGroupChange` (joined). State in runtime.json (`scheduleAt`, `scheduleNextAt`,
  `scheduleRepeat`, `scheduleDoneTo`, `scheduled`); scheduled sends are not check-ins (`recordReply(proactive, checkIn)`).
  Randomness via `Sampler.Float`.
- WhatsApp sync: the web app calls `POST /api/wa/chats/refresh` on every page load (`store.syncWhatsApp`). Contacts and
  groups are stale-while-revalidate (one background rebuild; only the very first load blocks), `GetJoinedGroups` is
  bounded (20 s), keepalive timeouts flip the state to `connecting` and sends check `Manager.ready` first (45 s cap).
- Cross-chat context is not behaviour (like memory and goals): `settings.memory.cross` (v5) → `ChatAssignment.cross`
  (mode/share; nil = default) → `PersonPrefs.cross` (groups, per person) → `Memory.scope` (`local` lock / `shared`
  unlock) → `crossctx.ModeFor`/`Carries`. Same persona only; sources are memories + the per-chat brief, never messages.
  Groups default to Discreet (the prompt section is background only), DMs to Open; sensitive topics never cross on
  their own. `engine.promptOptions` returns the `*crossUse` that `guardedReply` checks with `crossctx.Leak` (retry with
  `prompt.CrossRetryNote`, then without the section) — like `goals.Leak`. Build closures take
  `(prompt.GoalTurn, prompt.CrossTurn)`. The planner/goal check never get cross input. Activity texts never contain
  the notes. Tune with the live eval: `DOPPEL_CROSS_EVAL=1 CROSS_EVAL_OUT=<dir> CROSS_EVAL_LABEL=x CROSS_EVAL_MODE=discreet
  CROSS_EVAL_GUARD=1 go test ./internal/runtime/engine -run 'TestCrossEval$' -v -timeout 3h` (Ollama llama3.1:8b; ~10 min per config).
- Decision log: every activity about a particular message carries its id (`withMsg(meta, ids...)` → `meta.msgId`/`msgIds`;
  reply-cycle events use `cycle.msgIDs()`), so `logActivity` can file it under the right entry; events without ids
  go to the newest pending entry (decisions) or every pending one (steps). Add ids when you add a new decision point.
- Names: nicknames (`store/nicknames.go`, all chats) are applied where names are read — `store.History`,
  `Engine.HandleIncoming` (sender), `Engine.roster` (members), `Runner.historyCopyLocked` — never written into history.
- @tags must earn the ping (`engine/mentions.go`): a cooldown (15 min / 4 replies after a tag), the person answered
  only in a busy conversation (3+ speakers or someone wrote after them), anyone else only to pull them in (not if they
  wrote in the last 8 messages). The model's tags that break a rule go through `mention.Keep`.
- Languages are not behaviour (presets/dials never touch them): `settings.behavior.languages.{private,group}` →
  `ChatAssignment.language` (whole block) → `PersonPrefs.language` (reply language only); `engine/language.go` gates
  foreign messages and sets `prompt.Options.ReplyLanguage` (`REPLY LANGUAGE`, after STAY YOURSELF so it wins over the
  persona's own Language line).
- The system prompt is built from labelled parts (`prompt.SystemSections`; `SystemPrompt` joins them). A chat's
  `promptEdits` replace the editable parts (`model.EditablePromptSections`: character, safety, history, stay) in that
  chat only; delays are drawn with `behavior.Delay` along the profile's `timingCurve`.
- Connection ratings (`engine/ratings.go`, `prompt/rating.go`): own daily timer at `recap.time` (independent of
  `recap.enabled`), groups with 20+ messages in 24 h, one JSON call through the job slot; `ratings/<chatKey>.json`.
- Context systems (`internal/logic/ctxsys`, Context page): a chat runs `ChatAssignment.contextSystem` → `Persona.contextSystem` →
  `settings.context.system` → Classic (`engine.systemOf`/`paramsOf`). Classic's knobs must keep the old behaviour exactly.
  Knobs steer: `decide` (`joinIn`; `joinRules` in `engine/ctxsys.go`), `planAhead` (`planning`/`planDue`, `mergedCheck`,
  `nudgeEvery` → `GoalTurn.Rest`), `checkedReply` (`replycheck.go`: code checks, judge, rewrite, `genResult.Quiet` →
  `decision.skip reply_check`), `memoryTick` → `digestTick` (`digest.go`: one call, each part saved by the job's own
  `saveMemories`/`saveBrief`/`saveNotes`), `compactChat` (facts: `prompt.CompactFacts`/`ApplyFacts`, `ChatSummary.Format`),
  `shapePrompt` (card via `Persona.Card`/`queueCard`, budgets). Charts are pictures: only `params` run. The sandbox
  (`replay.go`) counts calls with `llm.Meter` (context-carried; `Registry.Resolve` wraps providers) and never writes state.
- Personal secretary (`model.SecretaryConfig` on the persona; `engine/secretary.go`, `prompt/secretary.go`): its job is a
  prompt part right after the character; meeting help gets `OWNER'S FREE TIMES` (meeting hours minus calendar busy times,
  never titles) inside the chat context; `secretaryTick` → job `secnote:<key>` (45 s, 5 s for urgent words/VIPs) writes
  notes (`NoteCursor`), activity `secretary.note` notifies. Built with `web/components/persona/secretary-builder.js` from the
  Personas page's single "Build a persona" chooser (`build-chooser.js`).
- Long-term memory (`engine/context.go`): `compactPlan` (settings `memory.compact` → chat `compactEvery`; fast chats use
  `fastEvery`); the persona reads everything since `contextFrom`, `historyCap` sizes the window (and the store's keep) to
  hold it; at keep+every messages all but keep fold into the summary.
- What it reads (`engine/context.go`): history from `ChatAssignment.contextFrom` on (`fromContext`), plus the summary
  (`EARLIER IN THIS CHAT`, inside the chat-context block) and your `instructions` (after the goal). Compaction (manual
  or `autoCompact` when the window is full) writes the summary and moves `contextFrom`. `autoContinue` turns hand-off
  pauses into flags (activity `handoff` with `meta.autoContinued`, no notification).
- Typos (`engine/typo.go`): `behavior.VariedTypo` over seven kinds; `RunnerState.Typos` (last 10) keeps a kind to at
  most 2 and never twice in a row, and rotates the correction shape (`behavior.FixForms`).
