# WhatsApp Doppel

**Create AI characters that chat in your WhatsApp, with their own voice, timing and goals,
from a friendly app on your computer (Mac, Windows or Linux).**

WhatsApp Doppel links to your WhatsApp the same way WhatsApp Web does. You create *personas*
(a witty interior designer, a hyped-up gym bro, a calmer version of you) and choose, chat by
chat, which one answers. Each persona has a profile picture, a personality, a texting style
and optional secret goals, and it behaves like a real person: it notices messages late,
shows "typing…", makes the odd typo, and knows when to stay quiet.

Replies are written by an AI that runs **on your own computer** (free and private, through
[Ollama](https://ollama.com)), or by Claude or OpenAI with your own API key. There is no
account to create and no cloud service in between.

> Built for fun with friends who are in on the joke. Please read
> [Using Doppel responsibly](docs/RESPONSIBLE_USE.md) before you point it at real people.

---

## Contents

- [What you can do](#what-you-can-do)
- [Install](#install)
  - [Mac](#mac) · [Windows](#windows) · [Linux](#linux) · [From source, one command](#from-source-one-command) · [For AI coding agents](#for-ai-coding-agents)
- [First run](#first-run)
- [A tour of the app](#a-tour-of-the-app)
- [FAQ](#faq)
- [Privacy and safety](#privacy-and-safety)
- [License](#license)
- [For developers](#for-developers)
  - [Code map](#code-map): every folder and file

## What you can do

| | |
|---|---|
| **Personas** | Profile cards with a picture (upload one, or pick a colour and a custom graphic), background, personality, texting style, catch-phrases, language, emoji habits and message length. Start from five built-ins, describe a character in one line and let the AI draft it, or **clone yourself** from your own messages. |
| **Chats** | Search your contacts and groups and assign a persona to each. In groups, choose **who it answers** (automatic by group size, everyone, or only people you pick), star people it should always answer, and leave notes about them ("Josh is my brother, tease him about Arsenal"). |
| **Vibe dials** | Three dials, **Speed**, **Chattiness** and **Boldness**, set dozens of behaviours at once. Presets (Natural, Instant, Busy, Slow texter, Night owl) are one tap away, and an **Advanced** switch reveals every detail: notice and reading delays, "seen" ticks, typing speed, splitting replies into bubbles, quote-replies, reactions, @tags, typos, active hours, reply limits and more. Any chat can override any setting. |
| **Goals and missions** | Give a persona a secret aim ("get Josh to say *apple*", "plan dinner on Friday"). It steers the conversation subtly, plans its next move privately, notices when the goal is reached and never gives the game away. Pick from mission templates, collect achievement badges, and press **Start the conversation** to make the first move. |
| **Real-life feel** | Each persona lives somewhere: its own city and time zone (yours or another country's), a daily routine (gym, work, sleep) that delays replies with natural excuses, occasional typos fixed with a "*word" message, and a **memory** of the people it talks to that you can view and edit. |
| **You stay in control** | Per chat: **Auto**, **Approve** (every reply waits for your OK) or **Co-pilot** (pick one of three drafts). **Hand-off** pauses a chat and alerts you when something serious comes up (money, health, meeting in person, distress, "are you a bot?"). **Away** pauses a chat for a while, and **Reveal** sends a pre-written "it was me all along" message and stops. |
| **Stay informed** | A live **Activity** feed explains every step in plain words, **desktop notifications** tell you when you're needed, and a **daily recap** sums up each chat. |
| **Help built in** | A **Guide** page with pictures, "?" tips next to anything tricky, and a short first-run tour. |

## Install

Pick your system, download one file, run the installer, done. Everything installs for **your
user only** (no administrator password), and your data stays in one folder you can delete any
time. Downloads are on the [**Releases page**](https://github.com/GilCaplan/whatsapp_chatbot/releases/latest).

> The app is free but **not code-signed** (that costs money every year), so the
> first time you open it your computer will warn you. The steps below show exactly what to click.

### Mac

macOS 13 (Ventura) or newer, Apple silicon or Intel.

1. Download **`WhatsappDoppel-<version>-macos-universal.zip`** and double-click it (Safari usually
   unzips it for you). You get **WhatsappDoppel**.
2. Drag **WhatsappDoppel** into your **Applications** folder.
3. Open it from Applications, Launchpad or Spotlight. The first time macOS blocks it:
   - **macOS 15 (Sequoia) and newer:** click **Done**, open **System Settings → Privacy &
     Security**, scroll down to *"WhatsappDoppel" was blocked…* and click **Open Anyway**, then
     confirm with your password or Touch ID.
   - **macOS 13–14:** right-click (or Control-click) the app → **Open** → **Open**.
   - Or in Terminal: `xattr -dr com.apple.quarantine /Applications/WhatsappDoppel.app`

   You only do this once. Your browser then opens the app.

### Windows

Windows 10 or 11. Most PCs need **`amd64`**; pick **`arm64`** only for an ARM PC (Snapdragon,
Surface Pro X).

1. Download **`WhatsappDoppel-<version>-windows-amd64.zip`**.
2. Right-click it → **Extract All…** → **Extract**.
3. In the extracted folder, double-click **`install.cmd`**. If Windows warns you:
   - *"Windows protected your PC"* (SmartScreen): click **More info** → **Run anyway**.
   - *"Open File – Security Warning"*: click **Run**.

   A window shows the progress and asks whether to open the app now.
4. Afterwards, open **WhatsApp Doppel** from the **Start menu** or the **Desktop** shortcut.

Uninstall from **Settings → Apps → WhatsApp Doppel**, or with `uninstall.cmd`. Notifications show
up as coming from *Windows PowerShell*.

### Linux

Any 64-bit desktop Linux (Ubuntu, Debian, Fedora, Mint, Arch…). Most PCs need **`amd64`**;
pick **`arm64`** for ARM machines (e.g. a Raspberry Pi 4/5 with a 64-bit OS).

1. Download **`whatsapp-doppel-<version>-linux-amd64.tar.gz`** and extract it (double-click →
   *Extract*, or `tar xzf whatsapp-doppel-*-linux-amd64.tar.gz`).
2. Run the installer from the extracted folder: right-click inside it → **Open in Terminal**, then

   ```bash
   ./install.sh
   ```

   (Some file managers also offer right-click on `install.sh` → **Run as a Program**.)
3. Open **WhatsApp Doppel** from your **app menu** or the **Desktop** shortcut. (GNOME: if the
   Desktop icon says it is untrusted, right-click it → **Allow Launching**.)

For notifications install notify-send: `sudo apt install libnotify-bin` (Fedora:
`sudo dnf install libnotify`, Arch: `sudo pacman -S libnotify`). Uninstall with
`~/.local/share/whatsapp-doppel/uninstall.sh`.
On Windows with WSL, use the **Windows** download instead: inside WSL there is no browser or
notifications for the app to use.

### From source, one command

Needs [git](https://git-scm.com/downloads) and [Go](https://go.dev/dl/) 1.21 or newer (Go fetches
the exact version this project needs by itself). On a Mac, `xcode-select --install` provides git;
Go is `brew install go` or the installer from go.dev. On Windows: `winget install --id Git.Git -e`
and `winget install --id GoLang.Go -e`.

**Mac and Linux** (Terminal):

```bash
git clone https://github.com/GilCaplan/whatsapp_chatbot.git whatsapp-doppel
cd whatsapp-doppel
scripts/install-from-source.sh
```

**Windows** (PowerShell):

```powershell
git clone https://github.com/GilCaplan/whatsapp_chatbot.git whatsapp-doppel
cd whatsapp-doppel
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\install-from-source.ps1
```

The script checks for Go (and tells you how to get it if it's missing; it never installs
system packages for you), builds the app (a minute or two the first time) and installs it
exactly like the downloads: Applications + Desktop icon on a Mac (same as `make install`),
app menu + Desktop on Linux, Start menu + Desktop on Windows. Run it again after `git pull` to
update; a running copy is stopped and replaced.

### For AI coding agents

Point Claude Code, Codex, Cursor or similar at **[INSTALL_FOR_AGENTS.md](INSTALL_FOR_AGENTS.md)**:
a step-by-step runbook (detect the system, check prerequisites, install, verify, hand over)
with safety rules. Scanning the WhatsApp QR code is the one step only you can do.

### Get an AI brain (pick one)

- **Free and private:** install [Ollama](https://ollama.com/download) (Mac and Windows: the
  installer; Linux: `curl -fsSL https://ollama.com/install.sh | sh`), then run
  `ollama pull llama3.1:8b` (about 5 GB; 16 GB of memory recommended).
- **Or** use Claude or OpenAI: you paste an API key in the app.

### Where things live

| | Mac | Windows | Linux |
|---|---|---|---|
| App | `/Applications/WhatsappDoppel.app` | `%LOCALAPPDATA%\Programs\WhatsappDoppel` | `~/.local/bin/whatsapp-doppel` |
| Your data | `~/Library/Application Support/WhatsappDoppel` | `%APPDATA%\WhatsappDoppel` | `~/.config/WhatsappDoppel` |
| Uninstall | drag the app to the Trash (from source: `make uninstall`) | Settings → Apps, or `uninstall.cmd` | `~/.local/share/whatsapp-doppel/uninstall.sh` |

Uninstalling keeps your data unless you say otherwise; delete the data folder to start fresh.

## First run

1. **Open the app**: Mac: Launchpad, Spotlight or the Desktop icon; Windows: Start menu or
   Desktop; Linux: app menu or Desktop ("WhatsApp Doppel"). Your browser opens on
   `http://127.0.0.1:7788`; that page *is* the app. It keeps running quietly in the background
   (no window or Dock icon) until you quit it from Settings.
2. **Choose a brain.** The setup screen finds Ollama and its models, or lets you paste and test a
   Claude or OpenAI key.
3. **Link WhatsApp.** On your phone: WhatsApp → *Settings* → *Linked devices* → *Link a device*,
   then scan the QR code in the app.
4. **Pick a persona**, or create one with AI from a single sentence.
5. **Choose a first chat.** Tip: start with **"You (message yourself)"** in **Approve** mode. From
   your phone, send yourself `1 hi`: the `1 ` prefix tells the persona to answer you
   right away, and the reply appears under **Approvals** for your OK.

A short tour then points out the main screens; you can replay it any time from the **Guide**.

## A tour of the app

- **Dashboard**: connection status, active chats, anything that **needs you**, the live feed and the daily recap.
- **Chats**: assign personas; each chat opens a window with tabs:
  - **Overview**: persona, reply mode (Auto / Approve / Co-pilot), Away, Reveal, the latest messages and a test box.
  - **People**: who it answers in a group, starred people, notes, what it remembers about each person.
  - **Goal**: the chat's goal or mission, how subtly to pursue it, progress, and **Start the conversation**.
  - **Behaviour**: preset, vibe dials, a preview of how a reply unfolds, and the Advanced controls.
  - **Memory**: what the persona has learned (pin, edit, forget), plus recaps.
- **Personas**: the character cards, the editor (with **World & routine** and an expression preview of sample replies), *Build with AI* and *Clone yourself*.
- **Missions**: active missions, completed ones with the moment they happened, and your badges.
- **Playground**: chat with a persona privately: try a goal, let it start the conversation, simulate a group.
- **Activity**: everything that happened and why (seen, waiting, typing, skipped, blocked, goal reached…).
- **Approvals**: replies and drafts waiting for you.
- **Settings**: AI brain, Behaviour defaults, Notifications, Safety (hand-off and reveal), Memory and recap, Appearance (nine looks from Liquid Glass to pure-black Midnight, Vintage and High contrast, each with light/dark or system), WhatsApp, Data, server port, and **Quit**.
- **Guide**: the in-app manual.

## FAQ

**Ollama, Claude or OpenAI: which should I use?**
Ollama is free and nothing leaves your computer; replies take a few seconds on an 8B model (the
app hides this inside realistic "thinking" and "typing" time). Claude and OpenAI write
noticeably better replies and cost a little per message on your own key. You can choose a
default and override it per persona.

**Will it reply to everyone?**
No. It only answers in chats you assign. In groups it answers selectively: when its name comes
up, when it's tagged, when the AI thinks it would naturally chime in, and only from the people
you've chosen in bigger groups. Any chat can be paused with one switch or set to Away.

**How do I make it feel more human?**
Use the vibe dials in Settings → Behaviour (for all chats) or in a chat's Behaviour tab (for
that chat only). Give the persona a home town, time zone and routine in its World & routine
card so it knows it's late, or at the gym.

**How do goals work, and will it give itself away?**
Write a goal in the chat's Goal tab or pick a mission. By default the persona pursues it
*subtly*: it never mentions the goal, steers gradually, and a safety check rewrites any reply
that would reveal it. "Balanced" and "Direct" styles get there faster.

**Can it start a conversation?**
Yes. Press **Start the conversation** in a chat's Goal tab (optionally with a topic), or turn
on automatic check-ins after a quiet spell. Both are off unless you use them.

**What does the persona remember?**
Durable facts about the people it chats with ("Dana's exam is Friday"), learned in the
background and shown in the Memory tab, where you can edit, pin or forget them. Turn learning
off per chat or for everything in Settings.

**What is "Clone yourself"?**
With your permission, the app keeps a private sample of messages *you* write, and the AI turns
them into a persona that texts like you. Off by default; the samples stay on your computer and can be
deleted any time (Settings → Data).

**Can someone trick the persona with "ignore your instructions…"?**
Messages that look like prompt-injection attempts are ignored before they reach the AI, and
replies that break character are retried or replaced. Questions like "are you a bot?" trigger a
hand-off to you instead.

**The page says the port is in use / I want a different address.**
Settings → *Server* → *Find free ports*, pick one and press *Change*; the page moves there by itself.

**How do I quit?** Settings → *About* → **Quit Doppel**. Opening the app again starts it back up.

**I closed the browser tab. Is it still running?** Yes, until you quit. Opening the app again
brings you back to the page.

**Notifications don't show up.** Settings → Notifications → *Send a test* checks it.
- **Mac:** the first time, macOS asks whether "Script Editor" may send notifications; allow it in
  System Settings → Notifications. For clickable notifications, run `brew install terminal-notifier`.
- **Windows:** they come from "Windows PowerShell"; check Settings → System → Notifications and
  turn off Do not disturb / Focus assist.
- **Linux:** install notify-send (`sudo apt install libnotify-bin`) and reopen the app.

**I'm on WSL.** Use the Windows download: the Linux build inside WSL has no browser or
notifications to talk to.

**I updated the app. Are my settings kept?** Yes. Older settings are converted automatically,
value for value.

**Is this allowed by WhatsApp?** Automated messaging can break
[WhatsApp's Terms of Service](https://www.whatsapp.com/legal/terms-of-service), and accounts that
behave like bots can be restricted. Doppel behaves like a person (realistic timing, selective
replies, limits) and approval modes keep you in control, but the risk is yours. Unofficial
project, not affiliated with WhatsApp or Meta.

**Something went wrong.** Check the Activity page first; detailed logs are in the data folder
under `logs/server.log`. If WhatsApp shows "replaced", another program is using the same link:
close it and press *Reconnect*.

## Privacy and safety

- Everything is stored in one folder (Settings → Data shows it and opens it):
  `~/Library/Application Support/WhatsappDoppel/` on a Mac, `%APPDATA%\WhatsappDoppel\` on
  Windows, `~/.config/WhatsappDoppel/` on Linux. Delete that folder to start completely fresh.
- With Ollama nothing leaves your computer except WhatsApp itself. With Claude or OpenAI, the chat
  being answered (and memories/recaps you enable) is sent to that provider under your key.
- The local web server only listens on `127.0.0.1` and checks every request.
- Details: [SECURITY.md](SECURITY.md) · Ground rules: [docs/RESPONSIBLE_USE.md](docs/RESPONSIBLE_USE.md)

## License

Free for personal and other **non-commercial** use under the
[PolyForm Noncommercial License 1.0.0](LICENSE). You may use, change and share it for personal
projects, study, hobbies and private entertainment. Using it, or anything built from it, to make
money (selling it, offering it as a paid service, using it in a business) is not permitted.

---

## For developers

### Requirements

- Go (version in `go.mod`; any Go ≥ 1.21 fetches it). Pure Go, no C compiler: SQLite is
  `modernc.org/sqlite`, so every OS builds every target with `CGO_ENABLED=0` (only
  `go test -race` needs cgo).
- macOS: `make app`/`make install`/`make dist-mac` use Apple's `sips`, `iconutil`, `codesign`,
  `lipo`, `osascript`. Linux and Windows archives build on any OS (`make dist-linux`,
  `make dist-windows`; Windows icons via `go run github.com/tc-hib/go-winres`, nothing added to go.mod).
- Windows development: `scripts\dev.ps1 build|run|dev-fake|test|vet|gui|install|smoke`, or plain
  `go run . serve --open`.
- Optional: Ollama on `127.0.0.1:11434`; Node.js for `node --check` on the web files; `jq` for `make smoke`.

### Code map

One Go binary serves a local web UI (vanilla ES modules, no build step, embedded with
`go:embed`) and a JSON + Server-Sent-Events API on `127.0.0.1`. See **[CLAUDE.md](CLAUDE.md)**
for package boundaries and conventions, and **[docs/API.md](docs/API.md)** for the full HTTP/SSE contract.

The Go code lives in four groups, from the bottom up: **core** (data types, settings, storage,
events: everything else builds on it), **logic** (pure decisions with no I/O, each package
testable on its own), **runtime** (the moving parts: the reply engine, prompts, AI providers,
WhatsApp, notifications) and **host** (the process: HTTP server, launcher, wiring, OS
differences). A package only imports from its own group or the ones below it.

```text
whatsapp_doppel_go/
├── main.go                  subcommands: launch (default), serve, status, quit, version
├── internal/
│   ├── core/                shared foundation
│   │   ├── model/           plain data types (JSON tags = API field names)
│   │   ├── contract/        interfaces between layers (the server only talks to these)
│   │   ├── config/          config.json settings, secrets.json, data-dir paths
│   │   ├── store/           JSON files + per-chat history (personas, chats, approvals, memories…)
│   │   └── events/          pub/sub hub feeding SSE, activity ring buffer
│   ├── logic/               pure rules, no I/O
│   │   ├── behavior/        reply timing: presets, vibe dials, ranges, planner, typos, schedules
│   │   ├── calendar/        your calendars, read-only (.ics, MACalendar), free slots
│   │   ├── crossctx/        what may carry from one chat into another
│   │   ├── ctxsys/          context systems (Classic, Lean, Fast, yours): knobs and charts
│   │   ├── goals/           goal pursuit: overrides, "say the word", leak check
│   │   ├── guard/           prompt-injection and character-break detection
│   │   ├── handoff/         messages you should answer yourself (EN + HE)
│   │   ├── lang/            language detection and reply-language rules
│   │   ├── memory/          merge and pick what personas learned about people
│   │   ├── mention/         @tags in groups
│   │   ├── mission/         mission templates and achievements
│   │   ├── persona/         built-in personas, validation, generated avatars
│   │   ├── task/            tasks: templates, rules, which times to offer, the calendar leak check
│   │   └── world/           where a persona lives: time zone, weekend, daily routine
│   ├── runtime/             the moving parts
│   │   ├── engine/          per-chat runners and the reply cycle (and every feature that hooks into it)
│   │   ├── prompt/          every prompt text (golden tests in testdata/)
│   │   ├── llm/             AI providers: Ollama, Anthropic, OpenAI, Fake; registry
│   │   ├── wa/              WhatsApp via whatsmeow (pairing, chats, groups, sending) and a Fake
│   │   └── notify/          desktop notifications per OS
│   └── host/                the process
│       ├── app/             wires everything together for `serve`
│       ├── server/          HTTP API, SSE, security middleware, static UI
│       ├── launcher/        single instance: instance.json, server.lock, detached spawn
│       └── platform/        every OS difference (one file per OS)
├── web/                     the UI, served at /
│   ├── index.html, app.js   shell, routing table, boot
│   ├── pages/               one module per route (#/chats, #/personas, …)
│   ├── components/          shared views, grouped by feature (see below)
│   └── styles/              tokens → base → components → pages → … → skins
├── scripts/                 build, install, release, smoke test; linux/ and windows/ installers
├── assets/icon/             editable icon source (icon.svg)
└── docs/                    API contract, architecture, feature rules, platform notes
```

Tests sit next to the code they test (`*_test.go`). Below, every file with one line on what it
holds. Click a folder to open it.

<details>
<summary><b>main.go and internal/core/</b></summary>

```text
main.go                         subcommands; version and devProjectDir set at build time (-ldflags -X)
internal/core/model/
  model.go                      personas, chats, messages, approvals, activity types: the shared types
  behavior.go                   reply-behaviour profile and per-chat overrides
  clone.go                      "Clone yourself" types
  context.go                    chat summary and "what it reads"
  cross.go                      cross-chat context types
  ctxsys.go                     context systems, knobs, charts, sandbox runs
  goal.go                       goal settings and progress
  language.go                   language preferences
  memory.go                     memories of people
  mission.go                    missions and achievements
  people.go                     group members, mentions, per-person preferences
  rating.go                     connection ratings
  recap.go                      daily recaps
  safety.go                     hand-off and reveal state
  secretary.go                  personal secretary, calendar sources, taken notes
  task.go                       tasks: specs, slots, rules, runs, results
  world.go                      where a persona lives, daily routine
internal/core/contract/
  contract.go                   interfaces WhatsApp, Engine, LLM, Playground, Builder, Notifier
internal/core/config/
  config.go                     settings (config.json) with defaults and migrations; secrets (secrets.json, 0600)
  paths.go                      every file and folder inside the data directory
internal/core/store/
  store.go                      personas, chats, approvals (JSON, atomic writes) and history (JSONL per chat)
  runtime.go                    per-chat engine state that survives restarts (runtime.json)
  briefs.go                     chat briefs for cross-chat context
  contextsystems.go             your context systems
  decisions.go                  decision log per chat
  memories.go                   memories per chat
  missions.go                   mission history
  nicknames.go                  nicknames you give people
  ratings.go                    connection ratings per chat
  recaps.go                     daily recaps
  secretary.go                  calendars and messages taken for you
  selfsamples.go                samples of your own messages (Clone, only with consent)
  summaries.go                  compacted older messages
  tasks.go                      saved tasks (tasks.json) and task runs (taskruns.json)
  wave3.go                      helpers shared by the per-feature files
internal/core/events/
  hub.go                        publish/subscribe for SSE, replay ring (Last-Event-ID), activity buffer, file sink
```
</details>

<details>
<summary><b>internal/logic/</b></summary>

```text
behavior/
  presets.go                    the reply-behaviour presets (Instant, Natural, …)
  ranges.go                     allowed range of every field; Validate, Clamp, Normalize
  resolve.go                    app profile → chat overrides, with where each value came from
  dials.go                      vibe dials (Speed, Chattiness, Boldness) derived from the profile
  plan.go                       delivery plan: bubbles, typing time, quotes, reactions
  timing.go                     timing curves for delays
  sampler.go                    the only source of randomness (tests use Fixed)
  schedule.go                   gaps between scheduled messages
  typo.go                       typos and their corrections
  zone.go                       active hours in the persona's time zone
  people.go                     limits of a chat's people settings
  migrate.go                    v1 settings migration
  relabel.go                    one-shot relabel of old presets
calendar/
  ics.go                        .ics parser (repeats, exceptions, all-day, time zones)
  service.go                    fetch + cache sources, MACalendar client, busy times, free slots
  book.go                       booking a task's meeting: MACalendar POST /events, .ics file
crossctx/
  crossctx.go                   modes, sensitive topics, what carries, selection, reply leak check
ctxsys/
  ctxsys.go                     built-in systems, knob specs, resolve (chat → persona → app)
  chart.go                      flowchart blocks, layout, chart → knobs
goals/
  goals.go                      resolve goal settings, "say the word", leak check, evidence, API view
guard/
  injection.go                  scored prompt-injection detector and sanitizer
  breaks.go                     character-break detector
handoff/
  handoff.go                    keyword classifier: money, health, meeting, distress, bot, legal
lang/
  lang.go                       language detection, conversation language, reply-language rules
memory/
  memory.go                     merge new memories (dedupe, update, cap, expiry), pick for a reply
mention/
  mention.go                    member directory, validate/encode/humanize @tags
mission/
  mission.go                    mission templates with blanks, detectors
  achievements.go               achievements computed from completed missions
persona/
  seeds.go                      the built-in personas
  persona.go                    persona validation and defaults
  secretary.go                  secretary limits
  avatar.go                     generated avatar gradients
task/                           tasks (Find a time); the calendar stays here, never in a prompt
  spec.go                       templates, spec checks, defaults
  rules.go                      the fixed conditions and actions, the Find a time rules, Pick
  turn.go                       Decide: their reading + the rules → what the next message does
  avail.go                      when a meeting can happen: offers, checks, alternatives, windows
  leak.go                       the check on every draft: allowed times only, no calendar words, no reasons
  format.go, fallback.go        writing times (EN + HE), safe messages written by code
  request.go                    meeting requests (secretaries start Find a time)
world/
  world.go                      time zone, weekend, part of day, daily routine
```
</details>

<details>
<summary><b>internal/runtime/</b></summary>

```text
engine/                         one package; files are named by feature
  engine.go                     the Engine: routes messages to per-chat runners, Reload, Stop
  runner.go                     a Runner owns one chat: incoming messages, gates, history
  pipeline.go                   the reply cycle: idle → noticing → waiting → thinking → typing
  clock.go                      the clock interface (tests step a fake clock)
  decide.go                     group "should I reply?" decisions
  people.go                     who it answers, streak guard
  mentions.go                   @tags in replies, prompt options per chat
  language.go                   foreign-language gate, reply language
  expression.go                 emoji and length of replies
  drafts.go                     co-pilot mode: three drafts in different tones
  approvals.go                  replies waiting for your approval
  typo.go                       planting typos and corrections
  routine.go                    routine gating, "sorry, was at the gym" late notes
  proactive.go                  check-ins after a silence
  opener.go                     "Start a conversation"
  schedule.go                   scheduled messages (never sent late)
  goal.go                       goal pursuit: plan ahead, goal check, leak rewrite
  handoff.go                    pause for you, resume
  reveal.go                     "it was a persona" message
  jobs.go                       serialised background jobs
  memory.go                     memory extraction job
  recap.go                      daily recaps
  brief.go                      chat briefs for cross-chat context
  cross.go                      cross-chat context selection and prompt section
  context.go                    what the persona reads, compaction
  ctxsys.go                     the chat's context system and its knobs
  digest.go                     one background call that updates memory, brief and notes
  replycheck.go                 reply check: code checks, judge, rewrite
  replay.go                     context-system sandbox (nothing is sent)
  decisionlog.go                decision log entries per message
  ratings.go                    daily connection ratings in groups
  secretary.go                  personal secretary: notes, urgent flags, meeting requests start a task
  task.go                       tasks: read their messages, decide, confirm, book, nudge, hand over, sandbox
  clone.go                      Clone yourself builder
  playground.go                 practice chats without WhatsApp
prompt/                         every prompt text; golden files in testdata/
  compose.go                    the reply system prompt, part by part, and the message list
  goal.go                       goal section, plan-ahead request
  cross.go, brief.go            cross-chat section and brief
  context.go, memory.go         chat summary, memory extraction
  people.go, world.go           people in the chat, time and place
  language.go, expression.go    reply language, emoji and length
  opener.go, drafts.go          openers, co-pilot drafts
  handoff.go, recap.go          hand-off check, daily recap
  rating.go, secretary.go       connection ratings, secretary job and note taking
  task.go                       the task's prompt part, reading their messages, cover lines, sandbox partner
  ctxsys.go, clone.go           context-system prompts, Clone yourself
  builder.go                    AI persona builder
llm/
  provider.go                   the Provider interface and request/response types
  registry.go                   picks the provider for a persona, rebuilds on settings change
  ollama.go, anthropic.go, openai.go   the providers
  fake.go                       scripted provider for tests and --fake-llm
  meter.go                      counts AI calls (sandbox)
wa/
  manager.go                    whatsmeow lifecycle: pairing, status, reconnect, logout, sending
  chats.go, recent.go           contacts and groups, recent chats index
  membership.go, participants.go   joined groups, members
  avatars.go                    profile pictures
  keys.go                       chat keys (dm:, group:, lid:)
  db.go, migrate.go             SQLite session, legacy bot.db import
  version.go                    keeps the WhatsApp Web version current
  selfsamples.go                samples of your messages (Clone)
  log.go                        whatsmeow logging
  fake.go                       simulated WhatsApp for --fake-wa and tests
notify/
  notify.go                     desktop notifications, per-event settings, rate limits, backends per OS
```
</details>

<details>
<summary><b>internal/host/</b></summary>

```text
app/
  app.go                        Run: config → store → hub → WhatsApp → AI → engine → server; lock, port, shutdown
  selfsamples.go                wiring for Clone samples
server/
  server.go                     the HTTP server, live port change
  routes.go                     every route (Go 1.22 patterns), grouped by feature
  middleware.go                 Host/Origin guard, token, JSON errors, logging, recovery
  respond.go                    JSON responses and errors
  sse.go                        /api/events stream
  static.go                     the embedded UI with the token filled in
  ports.go                      port scan
  avatar.go                     avatar upload processing
  handlers_*.go                 one file per feature (chats, personas, settings, goal, secretary, …)
launcher/
  launch.go                     `launch`: reuse a running server or start one, open the browser
  instance.go                   instance.json, health probe, quit
  lock.go                       server.lock
platform/                       one file per OS (build tags)
  platform.go                   what lives here
  datadir.go                    default data folder per OS
  open_*.go                     open a URL or folder
  lock_*.go                     file locks
  spawn_*.go                    detached processes
  alert_*.go                    native error dialogs
  tty_*.go, console_*.go        terminal detection, Windows console
```
</details>

<details>
<summary><b>web/</b></summary>

```text
index.html                      the page; boot.js runs before first paint
app.js                          shell (sidebar, topbar), routing table, overlays
router.js, store.js, sse.js     hash router, app state, the single event stream
api.js, dom.js, ui.js, util.js  fetch wrapper, rendering core, UI primitives, helpers
system.js                       full-screen system overlays
icons.js                        inline SVG icon sprite
skins.js                        the looks and how one is applied
guide-content.js                every word of the Guide, help tips and tour
embed.go                        embeds the UI into the binary
pages/                          one module per route
  dashboard.js, chats.js, personas.js, persona-editor.js, approvals.js, activity.js,
  playground.js, missions.js, tasks.js, context.js, settings.js, guide.js, onboarding.js
components/
  common/                       building blocks: elements, controls, help-tip, word-chips, avatar,
                                mentions, status, feed, tour
  art/                          inline-SVG pictures: art, badges, dial-face, glyphs, guide-art
  chat/                         chat panel and its parts: chat-drawer, chat-picker, assign-sheet,
                                initiate-controls, people-section, cross-block, memory-list,
                                context-block, ratings-block, reveal-sheet, handoff-meta
  drawer/                       the chat panel's tabs: overview, people, goal, behaviour, memory, log;
                                banners (status above every tab), shared (helpers)
  persona/                      persona-picker, build-chooser, ai-builder, clone-sheet,
                                secretary-builder, world-card, routine-editor, language-controls
  behaviour/                    behaviour-form, behaviour-meta, preset-cards, vibe-dials,
                                reply-timeline, week-editor
  context/                      context systems: context-params, context-sandbox,
                                context-draft-sheet, flowchart
  goals/                        goal-controls, mission-sheet
  tasks/                        task-sheet (set up, try, start), rule-editor, task-labels
  whatsapp/                     qr-card, wa-sheet
  home/                         dashboard cards: notes-card, recap-card
  settings/                     Settings sections in their own modules
styles/                         tokens.css → base → components → pages → control → animations
                                → fun → realism → skins
```
</details>

<details>
<summary><b>scripts/, docs/ and the root</b></summary>

```text
scripts/
  smoke.sh                      end-to-end test against a throwaway fake server
  build_app.sh, install_app.sh, uninstall_app.sh   Mac app bundle, install, remove
  dist.sh                       release archives for every OS
  install-from-source.sh/.ps1   one-command install from a checkout
  dev.ps1                       the Makefile for Windows
  linux/, windows/              installers shipped in the archives
  geniconn/                     renders the app icon (stdlib only)
docs/
  API.md                        the HTTP/SSE contract (authoritative)
  ARCHITECTURE.md               layers and how a message flows
  FEATURES.md                   the rules of each feature
  PLATFORMS.md                  macOS, Windows, Linux notes
  RESPONSIBLE_USE.md            using Doppel responsibly
  TASKS_DESIGN.md               the design of tasks
Makefile                        build, run, test, vet, smoke, app, install, dist
CLAUDE.md, INSTALL_FOR_AGENTS.md   guidance for AI coding agents
CHANGELOG.md, CONTRIBUTING.md, SECURITY.md, LICENSE
```
</details>

### Make targets

| Target | What it does |
|---|---|
| `make dev-fake` | run with a **simulated WhatsApp and a scripted AI**: no phone, no Ollama |
| `make run` | run the real app with `./data` as the data folder and open the browser |
| `make test` | `go test -race ./...` |
| `make vet` | `go vet` + gofmt check |
| `make smoke` | end-to-end script against a throwaway fake server |
| `make app` | (Mac) build `build/WhatsappDoppel.app` (icon, Info.plist, ad-hoc signature) |
| `make install` / `make uninstall` | (Mac) install to `/Applications` (+ Desktop alias, project shortcut) / remove |
| `make dist` | release downloads in `dist/`: macOS universal zip (on a Mac), Linux tar.gz and Windows zip for amd64 + arm64, `SHA256SUMS.txt`; also `dist-mac`, `dist-linux`, `dist-windows` |
| `make build`, `make icon`, `make clean` | binary, icon render, cleanup |

### Running by hand

```bash
go run . serve --fake-wa --fake-llm --data-dir ./data/fake --port 7788 --open
go run . serve --data-dir ./data            # real WhatsApp + configured AI
go run . status --data-dir ./data
go run . quit --data-dir ./data
```

Environment overrides: `DOPPEL_DATA_DIR`, `DOPPEL_PORT`, `DOPPEL_LEGACY_DB` (old `bot.db` to
import on first start), `DOPPEL_NO_BROWSER=1` (also no error dialogs), `DOPPEL_NOTIFY=dry|off|on`.

Releases: push a tag (`git tag v1.0.0 && git push origin v1.0.0`) and
`.github/workflows/release.yml` builds every download on a macOS runner and publishes a GitHub
Release.

Live evaluations against a local model are opt-in test files (for example
`DOPPEL_GOAL_EVAL=1`, `DOPPEL_EXPR_EVAL=1`) and are skipped by default.

### Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Changes are listed in [CHANGELOG.md](CHANGELOG.md).
