# Architecture

<!-- Moved word for word from CLAUDE.md; CLAUDE.md points here. -->

## Layout

| Path | Role |
|---|---|
| `main.go` | subcommands: `launch` (default, what the .app runs), `serve`, `status`, `quit`, `version`; `version`/`devProjectDir` set via `-ldflags -X` |
| `internal/app` | `Run(Options)`: wires config → store → events hub → WA (real/fake) → llm registry → engine → server; lock, port choice, `instance.json`, graceful shutdown |
| `internal/launcher` | `instance.json` read/write, `server.lock` (via `platform.TryLockFile`), `/api/health` probe (token must match), detached spawn of `serve --from-launcher`, `Quit` |
| `internal/platform` | every OS difference (leaf: stdlib + `x/sys`), one file per OS: `DefaultDataDir` (`os.UserConfigDir`), `IsTerminal`, `OpenURL`/`OpenFolder` (open, xdg-open, ShellExecute), `TryLockFile`/`UnlockFile` (flock; LockFileEx on one byte far past EOF), `DetachAttrs`/`HideWindow`, `Alert` (osascript, zenity/kdialog/notify-send, MessageBox), `AttachParentConsole` (Windows GUI build) |
| `internal/server` | HTTP API per `docs/API.md` (Go 1.22 mux patterns), SSE `/api/events`, middleware (Host/Origin/Sec-Fetch-Site guard, `X-Doppel-Token`, JSON errors, logging, recovery), static UI with token templating, port scan + live `Rebind`, avatar processing |
| `internal/engine` | per-chat `Runner`s: reply-cycle state machine (`pipeline.go`: idle → noticing → waiting → thinking → typing, or queued outside active hours), group decisions, guard, prompt, generation, split/quoted delivery, reactions, rate limits, proactive check-ins (`proactive.go`), approvals; "who it answers" gate + streak guard (`people.go`), @tags (`mentions.go`: member directory, prompt options, tag validation/encoding); playground + AI builder; wave 3: chat modes auto/approve/co-pilot (`drafts.go`: three tone drafts), typos (`typo.go`), routine gating + late-reply notes (`routine.go`), scheduled messages (`schedule.go`: per-runner timer, skip-never-late gate, `HandleGroupChange`), serialised background jobs (`jobs.go`: memory extraction `memory.go`, daily recaps `recap.go`, chat briefs `brief.go`), cross-chat context (`cross.go`: sources, selection, prompt section, `guardedReply` leak guard in `goal.go`), decision log (`decisionlog.go`: `Engine.act` → `logActivity`, entries per message id), hand-off pause/resume (`handoff.go`), reveal (`reveal.go`), clone builder (`clone.go`), missions (`goal.go`: `publishGoalReached` → `store.CompleteMission`, `goalMedia`) |
| `internal/behavior` | reply-behaviour logic (leaf, imports only `model`): presets, ranges/enums + `Validate`/`Clamp`/`Normalize`/`ValidateOverrides`, `Resolve` (app profile → chat overrides, with sources), `NextOpen` (active hours), `Sampler`/`Fixed`, `PlanDelivery`/`PlanBubbles`/`SplitText`/`TypingDuration`/`PickReaction`, typos (`typo.go`), scheduled-message gaps (`schedule.go`: `ScheduleGap` fixed/uniform/normal/poisson, `NextTime`), v1 migration; vibe dials (`dials.go`: `Dials`/`DialTable`, `ApplyDial`, `ReadDials`) and the one-shot v4 relabel of old Busy/Slow presets (`relabel.go`) |
| `internal/ctxsys` | context systems (leaf: `model` + stdlib): built-ins `classic`/`lean`/`fast` (`ParamsFor`, `Builtin`), knob specs + `Clamp`/`Normalize`, `Resolve` (chat → persona → app), chart `Blocks` catalog, built-in `Chart`, `Layout`, `Infer` (chart → knobs), `Estimates` |
| `internal/mission` | missions (leaf, `model` + stdlib): ~15 goal templates with blanks (`Templates`, `Fill`, `Detector`/`MediaDetected`) and achievements computed from the history (`Achievements`) |
| `internal/world` | where a persona lives (leaf): time zone, weekend, part of day, daily routine (`RoutineAt`…) for the prompt's "right now" section and routine gating |
| `internal/lang` | languages (leaf: `model`): light classifier `Detect` (script, then common words for Latin languages; short/unclear = not sure), conversation language `Main`, `LanguagePrefs` rules (`Effective`, `Ignores`, `Reply`, `Validate`); `All` is mirrored in `web/components/language-controls.js` |
| `internal/calendar` | the owner's calendars, read-only (leaf: `model` + stdlib): `ParseICS` (RRULE daily/weekly/monthly/yearly, EXDATE, RECURRENCE-ID, all-day, TZID), the MACalendar client, `Service` (10-minute cache, `Busy`), `FreeSlots` |
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
| `internal/wa` | whatsmeow `Manager` (pairing/QR, status, reconnect, logout, presence, chats, avatars, legacy `bot.db` import) and `Fake`; `membership.go`: joined-groups cache as membership truth (`InGroup`, `SetGroupHandler`, diffs on refresh + live `GroupInfo`/`JoinedGroup`) |
| `internal/store` | personas/chats/approvals JSON (atomic writes) + history JSONL per chat + `runtime.json` (`RunnerState`); migrates v1 per-chat `overrides` on `Open`; wave 3 files: `memories.go`, `recaps.go`, `missions.go` (read on demand, own mutex), `selfsamples.go`, `UpdateHistory`; `briefs.go` (cross-chat briefs); `decisions.go` (decision log, `UpdateDecisions`) |
| `internal/config` | `Paths` (everything under the data dir), `Settings` (`config.json`, v5: `behavior.{triggerPrefix,private,group}` + `notifications`, `safety{handoff,reveal}`, `memory` (+ `memory.cross`, v5), `recap`, `clone` (Go field `SelfClone`); v1 `replies`/`approvals` migrated in `normalize`; older files get `V3Fields`/`V4Fields` from their preset), `Secrets` (`secrets.json`, 0600) |
| `internal/events` | `Hub`: publish/subscribe for SSE with replay ring (Last-Event-ID) + activity ring buffer + file sink |
| `internal/contract` | interfaces `WhatsApp`, `Engine`, `LLM`, `Playground`, `Builder` — the server only talks to these |
| `internal/model` | shared plain types (JSON tags are the API field names); `behavior.go` = `BehaviorProfile`/`BehaviorOverrides` |
| `web/` | vanilla ES modules + CSS, no build step; `web/embed.go` embeds it (`//go:embed *` — every subfolder must contain a file). `pages/` (one module per route), `components/` (shared views; `drawer/` = the chat panel's tabs Overview · People/Contact (nicknames, per-person language) · Goal · Behaviour (+ Languages, auto-continue) · Memory (+ What it reads) · Log around the `chat-drawer.js` shell; `settings/` = Settings sections), `guide-content.js` (every word of the Guide, help tips and tour), `styles/` (`tokens` → `base` → `components` → `pages` → `control` → `animations` → `fun` → `realism` → `skins`), `skins.js` (the looks registry + `applyAppearance`) |
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
`history/<chatKey>.jsonl`, `memories/<chatKey>.json`, `briefs/<chatKey>.json` (cross-chat summaries), `decisions/<chatKey>.json` (decision log), `summaries/<chatKey>.json` (compacted older messages), `ratings/<chatKey>.json` (connection ratings), `nicknames.json`, `calendars.json` + `notes.json` (secretary, 0600), `contextsystems.json` (your context systems), `recaps.json`, `missions.json`, `cache/self-samples.jsonl`
(only with the Clone consent), `logs/server.log`, `logs/activity-YYYY-MM-DD.jsonl`,
`instance.json` (`{pid, port, token, startedAt, version}` while serving), `server.lock`.

On first start, if `whatsapp.db` is missing, `wa.New` copies the first existing legacy DB
from: `--legacy-db`, `$DOPPEL_LEGACY_DB`, `./bot.db`, `<devProjectDir>/bot.db`,
`./whatsapp_session.db`, `<devProjectDir>/whatsapp_session.db`.
