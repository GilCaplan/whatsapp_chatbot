# CLAUDE.md

Guidance for Claude Code sessions working in this repository.

## What this is

**WhatsApp Doppel** — a local web app for macOS, Windows and Linux (single Go binary, embedded web UI) that links
ONE WhatsApp account (via `go.mau.fi/whatsmeow`) and lets LLM personas reply in the chats
the user assigns. LLM providers: Ollama (local, default), Anthropic (official Go SDK),
OpenAI (plain HTTP). The old single-file CLI bot (`bot.go`/`persona.go`) was replaced; its
logic lives on in `internal/engine`, `internal/prompt`, `internal/guard` and the persona seeds.

Module: `whatsappdoppel`, Go 1.26, pure Go: SQLite is `modernc.org/sqlite` (driver `"sqlite"`), so every
target cross-compiles with `CGO_ENABLED=0` (only `go test -race` needs cgo).

Installing the app for a user (any OS, including as an agent): follow **[INSTALL_FOR_AGENTS.md](INSTALL_FOR_AGENTS.md)**
(safety rules, exact commands, verification). Keep it in sync when installers or paths change.

## Commands

```bash
make build            # build/whatsapp-doppel
make dev-fake         # serve with fake WhatsApp + fake LLM, data in ./data/fake, opens browser
make run              # serve for real with ./data
make test             # go test -race ./...
make vet              # go vet + gofmt check
make smoke            # scripts/smoke.sh: end-to-end against a throwaway fake server
make app              # (Mac) build/WhatsappDoppel.app (icon, Info.plist, ad-hoc codesign; ARCHS="arm64 amd64" = universal)
make install          # (Mac) app → /Applications + Desktop alias + ./WhatsappDoppel.app symlink (ask the user first)
make dist             # dist/: macOS universal zip (Mac only), Linux tar.gz + Windows zip (amd64, arm64), SHA256SUMS
scripts/install-from-source.sh   # one-command install (Mac → install_app.sh, Linux → scripts/linux/install.sh)
scripts\install-from-source.ps1  # Windows: GUI exe with icon → scripts/windows/install.ps1; scripts\dev.ps1 = Makefile for Windows
go run . serve --fake-wa --fake-llm --data-dir ./data/fake --port 7788
go run . status|quit [--data-dir D]
```

Do not run `go get` / `go mod tidy` casually — dependencies (notably the whatsmeow pin) are
deliberate; upgrading whatsmeow runs one-way DB migrations on `whatsapp.db` (back it up).

## Layout

| Path | Role |
|---|---|
| `main.go` | subcommands: `launch` (default, what the .app runs), `serve`, `status`, `quit`, `version`; `version`/`devProjectDir` set via `-ldflags -X` |
| `internal/app` | `Run(Options)`: wires config → store → events hub → WA (real/fake) → llm registry → engine → server; lock, port choice, `instance.json`, graceful shutdown |
| `internal/launcher` | `instance.json` read/write, `server.lock` (via `platform.TryLockFile`), `/api/health` probe (token must match), detached spawn of `serve --from-launcher`, `Quit` |
| `internal/platform` | every OS difference (leaf: stdlib + `x/sys`), one file per OS: `DefaultDataDir` (`os.UserConfigDir`), `IsTerminal`, `OpenURL`/`OpenFolder` (open, xdg-open, ShellExecute), `TryLockFile`/`UnlockFile` (flock; LockFileEx on one byte far past EOF), `DetachAttrs`/`HideWindow`, `Alert` (osascript, zenity/kdialog/notify-send, MessageBox), `AttachParentConsole` (Windows GUI build) |
| `internal/server` | HTTP API per `docs/API.md` (Go 1.22 mux patterns), SSE `/api/events`, middleware (Host/Origin/Sec-Fetch-Site guard, `X-Doppel-Token`, JSON errors, logging, recovery), static UI with token templating, port scan + live `Rebind`, avatar processing |
| `internal/engine` | per-chat `Runner`s: reply-cycle state machine (`pipeline.go`: idle → noticing → waiting → thinking → typing, or queued outside active hours), group decisions, guard, prompt, generation, split/quoted delivery, reactions, rate limits, proactive check-ins (`proactive.go`), approvals; "who it answers" gate + streak guard (`people.go`), @tags (`mentions.go`: member directory, prompt options, tag validation/encoding); playground + AI builder; wave 3: chat modes auto/approve/co-pilot (`drafts.go`: three tone drafts), typos (`typo.go`), routine gating + late-reply notes (`routine.go`), serialised background jobs (`jobs.go`: memory extraction `memory.go`, daily recaps `recap.go`, chat briefs `brief.go`), cross-chat context (`cross.go`: sources, selection, prompt section, `guardedReply` leak guard in `goal.go`), hand-off pause/resume (`handoff.go`), reveal (`reveal.go`), clone builder (`clone.go`), missions (`goal.go`: `publishGoalReached` → `store.CompleteMission`, `goalMedia`) |
| `internal/behavior` | reply-behaviour logic (leaf, imports only `model`): presets, ranges/enums + `Validate`/`Clamp`/`Normalize`/`ValidateOverrides`, `Resolve` (app profile → chat overrides, with sources), `NextOpen` (active hours), `Sampler`/`Fixed`, `PlanDelivery`/`PlanBubbles`/`SplitText`/`TypingDuration`/`PickReaction`, typos (`typo.go`), v1 migration; vibe dials (`dials.go`: `Dials`/`DialTable`, `ApplyDial`, `ReadDials`) and the one-shot v4 relabel of old Busy/Slow presets (`relabel.go`) |
| `internal/mission` | missions (leaf, `model` + stdlib): ~15 goal templates with blanks (`Templates`, `Fill`, `Detector`/`MediaDetected`) and achievements computed from the history (`Achievements`) |
| `internal/world` | where a persona lives (leaf): time zone, weekend, part of day, daily routine (`RoutineAt`…) for the prompt's "right now" section and routine gating |
| `internal/memory` | what personas learn about people (leaf): `Merge` (dedupe/update/cap/expiry) and `Select` for the reply prompt |
| `internal/crossctx` | cross-chat context (leaf: `model`+`memory`+`handoff`): `ModeFor`/`ShareFor` (chat → app default), sensitive `Classify` (hand-off keywords + romance/secret/money/health extras, EN+HE), `Carries` (lock/unlock/sensitive/fresh), `Select` (people in the conversation, caps), `Leak` (reply guard) |
| `internal/handoff` | keyword classifier (leaf, English + Hebrew) for messages you should answer yourself (money, health, meeting, distress, bot, legal) |
| `internal/notify` | desktop notifications for approvals, goals, hand-offs, WhatsApp drops, recaps; rate-limited, per-event settings. Backends: terminal-notifier/osascript (macOS), notify-send (Linux), a toast via Windows PowerShell (`-EncodedCommand`, XML-escaped; shown as "Windows PowerShell"); `Args` is pure and tested on every OS |
| `internal/llm` | `Provider` interface; `ollama.go`, `anthropic.go`, `openai.go`, `fake.go`; `Registry` (resolves persona/default provider, rebuilds on settings change) |
| `internal/mention` | @tags in groups (leaf, imports only `model`): member `Directory` with unique display names, `Normalize` (validate LLM "@Name" tags), `Encode` ("@<number>" + MentionedJID for WhatsApp), `Humanize` (incoming tags → "@Name"), `TagFirst`, `CleanName` |
| `internal/prompt` | system prompt composition (golden tests in `testdata/`), builder prompt; `goal.go`: the goal section (`GoalSection`, per-reply `GoalTurn`) and the plan-ahead request/parser (`Plan`, `ParsePlan`); `goal_eval_test.go` = live goal-pursuit eval (env-guarded); `cross.go` (`CrossSection`, `CrossTurn`) and `brief.go` (`Brief`, `ParseBrief`) for cross-chat context |
| `internal/goals` | goal-pursuit logic (leaf, imports only `model`+`persona`): `Resolve` (persona → chat overrides), "say the word" goals (`ParseSayWord`, `SaysWord`, `SpeakerMatches`; English + Hebrew), reply `Leak` check, planner `EvidenceFound`, API `View` |
| `internal/guard` | scored injection detector, sanitizer, character-break detector |
| `internal/persona` | `Seeds()`, `Seed(id)`, `Normalize(*Persona) error`, generated avatars |
| `internal/wa` | whatsmeow `Manager` (pairing/QR, status, reconnect, logout, presence, chats, avatars, legacy `bot.db` import) and `Fake` |
| `internal/store` | personas/chats/approvals JSON (atomic writes) + history JSONL per chat + `runtime.json` (`RunnerState`); migrates v1 per-chat `overrides` on `Open`; wave 3 files: `memories.go`, `recaps.go`, `missions.go` (read on demand, own mutex), `selfsamples.go`, `UpdateHistory`; `briefs.go` (cross-chat briefs) |
| `internal/config` | `Paths` (everything under the data dir), `Settings` (`config.json`, v5: `behavior.{triggerPrefix,private,group}` + `notifications`, `safety{handoff,reveal}`, `memory` (+ `memory.cross`, v5), `recap`, `clone` (Go field `SelfClone`); v1 `replies`/`approvals` migrated in `normalize`; older files get `V3Fields`/`V4Fields` from their preset), `Secrets` (`secrets.json`, 0600) |
| `internal/events` | `Hub`: publish/subscribe for SSE with replay ring (Last-Event-ID) + activity ring buffer + file sink |
| `internal/contract` | interfaces `WhatsApp`, `Engine`, `LLM`, `Playground`, `Builder` — the server only talks to these |
| `internal/model` | shared plain types (JSON tags are the API field names); `behavior.go` = `BehaviorProfile`/`BehaviorOverrides` |
| `web/` | vanilla ES modules + CSS, no build step; `web/embed.go` embeds it (`//go:embed *` — every subfolder must contain a file). `pages/` (one module per route), `components/` (shared views; `drawer/` = the chat panel's tabs Overview · People/Contact · Goal · Behaviour · Memory around the `chat-drawer.js` shell; `settings/` = Settings sections), `guide-content.js` (every word of the Guide, help tips and tour), `styles/` (`tokens` → `base` → `components` → `pages` → `control` → `animations` → `fun` → `realism` → `skins`), `skins.js` (the looks registry + `applyAppearance`) |
| `scripts/` | Mac: `build_app.sh`, `install_app.sh`, `uninstall_app.sh`; all: `dist.sh`, `smoke.sh` (bash + curl + jq/plutil), `install-from-source.sh`/`.ps1`, `dev.ps1`; `linux/` and `windows/` = the installers shipped in the archives; `geniconn/` (stdlib icon renderer, also `-ico`) |
| `assets/icon/icon.svg` | editable icon source; the shipped icon is rendered by `scripts/geniconn` |
| `docs/API.md` | **the authoritative HTTP/SSE contract** — keep server, frontend and this doc in sync |

## Data directory

Default (`platform.DefaultDataDir`): macOS `~/Library/Application Support/WhatsappDoppel/`,
Windows `%APPDATA%\WhatsappDoppel\`, Linux `$XDG_CONFIG_HOME|~/.config/WhatsappDoppel/` (override:
`--data-dir` or `DOPPEL_DATA_DIR`; `make run` uses `./data`, gitignored). Never use relative paths in
code: the .app starts with cwd `/`, shortcuts and .desktop files with other folders. Contents: `config.json`, `secrets.json`, `personas.json`,
`chats.json`, `approvals.json`, `runtime.json` (per-chat reply timestamps, last incoming/reply,
proactive log/due time, queued wake-up, goal progress — best effort), `whatsapp.db` (whatsmeow session), `avatars/`, `cache/`,
`history/<chatKey>.jsonl`, `memories/<chatKey>.json`, `briefs/<chatKey>.json` (cross-chat summaries), `recaps.json`, `missions.json`, `cache/self-samples.jsonl`
(only with the Clone consent), `logs/server.log`, `logs/activity-YYYY-MM-DD.jsonl`,
`instance.json` (`{pid, port, token, startedAt, version}` while serving), `server.lock`.

On first start, if `whatsapp.db` is missing, `wa.New` copies the first existing legacy DB
from: `--legacy-db`, `$DOPPEL_LEGACY_DB`, `./bot.db`, `<devProjectDir>/bot.db`,
`./whatsapp_session.db`, `<devProjectDir>/whatsapp_session.db`.

## Contracts and conventions

- The server depends on `contract.*` interfaces, `config`, `store`, `events`, `model`, `behavior`, `goals`, `crossctx` only —
  never on `engine`/`wa`/`llm` directly. `internal/app` is the only place that imports everything.
- Import rules: `model` has no deps; `behavior`, `mention`, `mission`, `world`, `memory`, `handoff` import only `model`
  (+ stdlib); `crossctx` imports `model`+`memory`+`handoff`; `goals` imports `model`+`persona`; `platform` imports only stdlib + `x/sys`; `config` imports
  `behavior`+`crossctx`+`model`+`platform`; `store` imports `config`+`behavior`+`model`(+`world`); `notify` imports
  `config`+`model`+`platform`. OS-specific code goes only in `platform` (build-tagged files), never
  `runtime.GOOS` switches elsewhere except for pure decisions (notify backends, tests). Prompt text for every feature lives in
  `internal/prompt/<feature>.go` so the goldens stay in one package.
- Vibe dials (Speed, Chattiness, Boldness; 5 levels each) are **derived, never stored**: `behavior.DialTable()` is the
  single source of truth (served in `GET /api/behavior/presets` → `dials`); moving a dial writes its fields through the
  normal Settings/PATCH paths, `behavior.ReadDials` (mirrored in `web/components/vibe-dials.js`) reads a profile back
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
- Copy and pictures: user-facing words are plain English for non-technical people; **no emoji anywhere** in UI or
  activity text — pictures are inline SVG (`art.js`, `glyphs.js`, `guide-art.js`, `dial-face.js`, `badges.js`). Guide,
  help-tip and tour copy lives only in `web/guide-content.js` (`GUIDE_SECTIONS`, `TIPS`, `TOUR_STEPS`, `PAGE_SECTION`);
  `helpTip(id)` (`components/help-tip.js`) puts a "?" popover with "Learn more" next to a complex control; the topbar
  "?" opens `#/guide/<section>` for the current page; the tour (`components/tour.js`) starts once right after
  onboarding (`localStorage doppel.tour`) and is replayable from the Guide. Escape all user/WhatsApp text (the `html`
  tag does; only trusted static markup goes through `raw`). Respect `prefers-reduced-motion` (global rule in
  `animations.css`; avoid JS-driven motion).
- Progressive disclosure: simple controls first, details behind Advanced/"More details" (Settings › Behaviour, the chat
  Behaviour tab, the persona editor's optional fields, the Goal tab's wording under an active mission).
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
  ./internal/prompt -run 'TestGoalEval$' -v -timeout 3h` (needs Ollama with llama3.1:8b; ~30 min).
- Cross-chat context is not behaviour (like memory and goals): `settings.memory.cross` (v5) → `ChatAssignment.cross`
  (mode/share; nil = default) → `PersonPrefs.cross` (groups, per person) → `Memory.scope` (`local` lock / `shared`
  unlock) → `crossctx.ModeFor`/`Carries`. Same persona only; sources are memories + the per-chat brief, never messages.
  Groups default to Discreet (the prompt section is background only), DMs to Open; sensitive topics never cross on
  their own. `engine.promptOptions` returns the `*crossUse` that `guardedReply` checks with `crossctx.Leak` (retry with
  `prompt.CrossRetryNote`, then without the section) — like `goals.Leak`. Build closures take
  `(prompt.GoalTurn, prompt.CrossTurn)`. The planner/goal check never get cross input. Activity texts never contain
  the notes. Tune with the live eval: `DOPPEL_CROSS_EVAL=1 CROSS_EVAL_OUT=<dir> CROSS_EVAL_LABEL=x CROSS_EVAL_MODE=discreet
  CROSS_EVAL_GUARD=1 go test ./internal/engine -run 'TestCrossEval$' -v -timeout 3h` (Ollama llama3.1:8b; ~10 min per config).
- Engine rules: all randomness goes through `Engine.rng` (`behavior.Sampler`; tests inject `behavior.Fixed`),
  all waiting through `Engine.clock` (`e.sleep`, `Runner.armLocked`/`afterLocked`). Phase transitions run under
  `Runner.mu` and collect side effects (`effects`) that run after unlocking; typing on/off goes through
  `typingFxLocked` (sequence-ordered). Goroutines that do work are counted in `Engine.busy` (tests wait for 0).
- Activity `text` is plain English and never contains emoji or reason codes (codes go in `meta.reason`,
  a reaction's emoji in `meta.emoji`).
- Mutating API requests need `X-Doppel-Token`; GETs don't. `Host` must be
  `127.0.0.1:<port>`/`localhost:<port>` (the port the request arrived on, so both listeners
  work during a port change). Errors are `{"error","code"}`.
- `PUT /api/settings` is a deep merge of partial JSON; `port`/`version` are ignored there
  (ports change via `POST /api/system/port`, which starts a second listener, persists the
  port, rewrites `instance.json`, publishes `system{kind:"port_changed"}` and closes the old
  listener after 3 s).
- Chat keys: `dm:<phone>`, `group:<id>`, `lid:<lid>`; URL-encode them in paths.
- `POST /api/chats/{key}/simulate` is only allowed with `--fake-wa`, for the user's own
  chat, or in approval mode (otherwise a real person would receive the reply).
- SSE: on connect the server sends the current `wa.status` (no `id`), then the backlog after
  `Last-Event-ID`, then live events; `: ping` every 15 s.
- Persist JSON via `config.WriteJSONAtomic` (temp file + rename). Secrets: mode 0600, only
  returned masked (`config.SecretView`).
- Keep code gofmt'd and `go vet` clean; prefer the stdlib (the icon generator and avatar
  resampler are stdlib-only on purpose).

## Testing

- Unit tests use in-package fakes: `internal/server` has tiny fakes of the contract
  interfaces (`fakes_test.go`) and runs a real listener; `internal/engine` uses a fake clock
  (`h.advance(d)` steps through due timers and waits for the engine to settle) and an
  Instant-like profile by default (`instantLike`); realistic-timing tests override fields;
  `llm.NewFake()` records requests; `wa.NewFake(hub)` provides synthetic contacts/groups and
  a "You (message yourself)" chat.
- `--fake-wa --fake-llm` run the whole app with no phone and no model — use it for UI work
  and `scripts/smoke.sh`.
- `DOPPEL_NO_BROWSER=1` stops `launch`/`serve --open` from opening a browser (and `launch` from showing
  an error dialog) in scripts.
- CI (`.github/workflows/ci.yml`) runs vet/gofmt/race tests/smoke on ubuntu, macos and windows,
  cross-compiles the six release targets and installs → launches (fake) → quits → uninstalls with
  each OS's installer; `release.yml` publishes `make dist` output on a `v*` tag.
- Tests must not assume Unix: no `/usr/bin/...`, no permission-bit checks on Windows, paths via
  `filepath`/`t.TempDir()` (the launcher test re-executes its own binary as a helper).
- Real WhatsApp testing: use a throwaway `--data-dir` to exercise pairing; assign a persona
  to "You (message yourself)" and send `1 hi` from the phone (`triggerPrefix`).

## Gotchas

- Restart semantics: `runtime.json` keeps rate-limit counters, cooldown, proactive due time and a queued
  wake-up (re-armed on start); in-flight phases (noticing/typing…) are dropped by design — the messages are
  already in history and the next message triggers a reply covering them.
- `--fake-wa` with the default *Natural* preset is deliberately slow (notice 3–25 s, wait 8 s, think…);
  set an Instant-like profile for quick manual tests (see `scripts/smoke.sh`).

- `//go:embed *` in `web/embed.go` fails to compile if any `web/` subfolder is empty.
- The .app is `LSUIElement` (no Dock icon): the launcher exits after opening the browser and
  the server keeps running detached; failures surface as a native alert (`platform.Alert`).
- Windows release exe is GUI subsystem (`-H=windowsgui`, no console on double-click);
  `platform.AttachParentConsole` lets `status`/`quit`/`version` print in cmd/PowerShell (PowerShell
  only waits when the output is piped: `& exe status | Out-Host`). Dev builds (`go build`) are console exes.
- Windows icon/version resources: `rsrc_windows_*.syso` generated at dist/install time by
  `go run github.com/tc-hib/go-winres@v0.3.3` (not in go.mod), deleted after the build, gitignored.
- `.gitattributes` forces LF (CRLF for `.cmd`/`.ps1`); keep `.ps1` files ASCII (Windows PowerShell 5.1
  reads BOM-less files as ANSI). Never put `:` or other Windows-invalid characters in file names.
- Don't enable WAL on `whatsapp.db`: `-wal`/`-shm` sidecars look like "old bot still running" to `migrateLegacy`.
- Running the legacy bot with the same session at the same time causes `StreamReplaced`
  ping-pong (WA state `replaced`).
- `make install` writes to `/Applications` and the Desktop — only with the user's go-ahead.
