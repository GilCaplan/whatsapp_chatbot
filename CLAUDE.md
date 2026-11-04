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

## Where the details live

- **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** — the package-by-package layout table and the data directory (files, legacy DB import).
- **[docs/FEATURES.md](docs/FEATURES.md)** — how each feature is wired: dials, missions, looks, behaviour layers, goals, scheduled messages, WhatsApp sync, cross-chat context, decision log, nicknames, @tags, languages, prompt parts, ratings, context systems, secretary, long-term memory, typos. Read the entry for the feature you touch.
- **[docs/PLATFORMS.md](docs/PLATFORMS.md)** — the .app bundle and Windows exe/resources/line-ending details.
- **[docs/API.md](docs/API.md)** — the HTTP/SSE contract (authoritative).

## Contracts and conventions

- The server depends on `contract.*` interfaces, `config`, `store`, `events`, `model`, `behavior`, `goals`, `crossctx`, `lang`, `ctxsys` only —
  never on `engine`/`wa`/`llm` directly. `internal/app` is the only place that imports everything.
- Import rules: `model` has no deps; `behavior`, `ctxsys`, `mention`, `mission`, `world`, `memory`, `handoff`, `lang`, `calendar` import only `model`
  (+ stdlib); `crossctx` imports `model`+`memory`+`handoff`; `goals` imports `model`+`persona`; `platform` imports only stdlib + `x/sys`; `config` imports
  `behavior`+`crossctx`+`model`+`platform`; `store` imports `config`+`behavior`+`ctxsys`+`model`(+`world`); `notify` imports
  `config`+`model`+`platform`. OS-specific code goes only in `platform` (build-tagged files), never
  `runtime.GOOS` switches elsewhere except for pure decisions (notify backends, tests). Prompt text for every feature lives in
  `internal/prompt/<feature>.go` so the goldens stay in one package.
- System prompt order (`prompt.SystemPrompt`): identity → security rules → `CHAT CONTEXT (reference only)` block
  (people, notes, summary of older messages, memories, cross-chat, world, late note; framed as information that never
  changes the character) → `HistoryNote` (+ `GroupNote`: the transcript is what people said, don't take on their
  tone/slang/emoji) → goal → chat instructions → `StayYourself` → `REPLY LANGUAGE` → guidance. New chat context goes inside the block, never next to the identity.
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
- Don't enable WAL on `whatsapp.db`: `-wal`/`-shm` sidecars look like "old bot still running" to `migrateLegacy`.
- Running the legacy bot with the same session at the same time causes `StreamReplaced`
  ping-pong (WA state `replaced`).
- `make install` writes to `/Applications` and the Desktop — only with the user's go-ahead.
