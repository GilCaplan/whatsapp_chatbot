# WhatsApp Doppel

**Create AI characters that chat in your WhatsApp, with their own voice, timing and goals,
from a friendly app on your Mac.**

WhatsApp Doppel links to your WhatsApp the same way WhatsApp Web does. You create *personas*
(a witty interior designer, a hyped-up gym bro, a calmer version of you) and choose, chat by
chat, which one answers. Each persona has a profile picture, a personality, a texting style
and optional secret goals, and it behaves like a real person: it notices messages late,
shows "typing…", makes the odd typo, and knows when to stay quiet.

Replies are written by an AI that runs **on your own Mac** (free and private, through
[Ollama](https://ollama.com)), or by Claude or OpenAI with your own API key. There is no
account to create and no cloud service in between.

> Built for fun with friends who are in on the joke. Please read
> [Using Doppel responsibly](docs/RESPONSIBLE_USE.md) before you point it at real people.

---

## Contents

- [What you can do](#what-you-can-do)
- [Install](#install)
- [First run](#first-run)
- [A tour of the app](#a-tour-of-the-app)
- [FAQ](#faq)
- [Privacy and safety](#privacy-and-safety)
- [License](#license)
- [For developers](#for-developers)

## What you can do

| | |
|---|---|
| **Personas** | Profile cards with a picture (upload one, or pick a colour and a custom graphic), background, personality, texting style, catch-phrases, language, emoji habits and message length. Start from five built-ins, describe a character in one line and let the AI draft it, or **clone yourself** from your own messages. |
| **Chats** | Search your contacts and groups and assign a persona to each. In groups, choose **who it answers** (automatic by group size, everyone, or only people you pick), star people it should always answer, and leave notes about them ("Josh is my brother, tease him about Arsenal"). |
| **Vibe dials** | Three dials, **Speed**, **Chattiness** and **Boldness**, set dozens of behaviours at once. Presets (Natural, Instant, Busy, Slow texter, Night owl) are one tap away, and an **Advanced** switch reveals every detail: notice and reading delays, "seen" ticks, typing speed, splitting replies into bubbles, quote-replies, reactions, @tags, typos, active hours, reply limits and more. Any chat can override any setting. |
| **Goals and missions** | Give a persona a secret aim ("get Josh to say *apple*", "plan dinner on Friday"). It steers the conversation subtly, plans its next move privately, notices when the goal is reached and never gives the game away. Pick from mission templates, collect achievement badges, and press **Start the conversation** to make the first move. |
| **Real-life feel** | Each persona lives somewhere: its own city and time zone (yours or another country's), a daily routine (gym, work, sleep) that delays replies with natural excuses, occasional typos fixed with a "*word" message, and a **memory** of the people it talks to that you can view and edit. |
| **You stay in control** | Per chat: **Auto**, **Approve** (every reply waits for your OK) or **Co-pilot** (pick one of three drafts). **Hand-off** pauses a chat and alerts you when something serious comes up (money, health, meeting in person, distress, "are you a bot?"). **Away** pauses a chat for a while, and **Reveal** sends a pre-written "it was me all along" message and stops. |
| **Stay informed** | A live **Activity** feed explains every step in plain words, **Mac notifications** tell you when you're needed, and a **daily recap** sums up each chat. |
| **Help built in** | A **Guide** page with pictures, "?" tips next to anything tricky, and a short first-run tour. |

## Install

You need a Mac running macOS 13 (Ventura) or newer.

**1. Install the tools (one time).** In Terminal:

```bash
xcode-select --install          # Apple's command-line tools
brew install go                 # or download Go from https://go.dev/dl/
```

(No Homebrew? Get it from [brew.sh](https://brew.sh), or install Go from go.dev directly.)

**2. Get an AI brain (pick one).**
- **Free and private:** install [Ollama](https://ollama.com), open it once, then run
  `ollama pull llama3.1:8b` (about 5 GB; a Mac with 16 GB of memory is recommended).
- **Or** use Claude or OpenAI: you'll paste an API key in the app later.

**3. Install the app.** From this folder:

```bash
make install
```

The first build takes a minute or two. It puts **WhatsApp Doppel** in your Applications
folder (and Launchpad), adds an icon to your Desktop, and a `WhatsappDoppel.app` shortcut in this
project folder. Run the same command again to update after pulling new code. To remove it:
`make uninstall` (it asks before deleting your personas and settings).

## First run

1. **Open the app** from the Desktop icon, Launchpad or Spotlight ("WhatsApp Doppel").
   Your browser opens on `http://127.0.0.1:7788`; that page *is* the app. It keeps running
   quietly in the background (no Dock icon) until you quit it from Settings.
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
- **Settings**: AI brain, Behaviour defaults, Notifications, Safety (hand-off and reveal), Memory and recap, Appearance, WhatsApp, Data, server port, and **Quit**.
- **Guide**: the in-app manual.

## FAQ

**Ollama, Claude or OpenAI: which should I use?**
Ollama is free and nothing leaves your Mac; replies take a few seconds on an 8B model (the
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
them into a persona that texts like you. Off by default; the samples stay on your Mac and can be
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

**Notifications don't show up.** The first time, macOS asks whether "Script Editor" may send
notifications; allow it in System Settings → Notifications. For clickable notifications, run
`brew install terminal-notifier`. Settings → Notifications → *Send a test* checks it.

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

- Everything is stored in `~/Library/Application Support/WhatsappDoppel/` (Settings → Data →
  *Open in Finder*). Delete that folder to start completely fresh.
- With Ollama nothing leaves your Mac except WhatsApp itself. With Claude or OpenAI, the chat
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

- Go (version in `go.mod`) with CGO: SQLite uses `mattn/go-sqlite3`, so Xcode command-line tools are needed.
- macOS tools used by the scripts: `sips`, `iconutil`, `codesign`, `osascript`, `plutil`.
- Optional: Ollama on `127.0.0.1:11434`; Node.js for `node --check` on the web files.

### Architecture

One Go binary serves a local web UI (vanilla ES modules, no build step, embedded with
`go:embed`) and a JSON + Server-Sent-Events API on `127.0.0.1`. See **[CLAUDE.md](CLAUDE.md)**
for package boundaries and conventions, and **[docs/API.md](docs/API.md)** for the full HTTP/SSE contract.

```
main.go               launch | serve | status | quit
internal/app          wiring + process lifecycle (lock, instance.json, signals, notifier)
internal/launcher     single instance: instance.json, flock, health probe, detached spawn
internal/server       HTTP API, SSE, security middleware, static UI
internal/engine       per-chat reply pipeline (notice → seen → wait → think → typing → bubbles),
                      group decisions, people rules, goals/openers, approvals/co-pilot, hand-off,
                      reveal, memory & recap jobs, check-ins, playground
internal/behavior     behaviour profiles: presets, dials, ranges, resolver, planner, sampler, typos
internal/prompt       system prompt sections (identity, world, people, memory, agenda, expression)
internal/llm          providers: Ollama, Anthropic (SDK), OpenAI, Fake; registry
internal/goals        goal logic (style, reached detection)    internal/mission   templates, achievements
internal/mention      @tag directory and encoding               internal/memory    merge/select memories
internal/handoff      serious-topic classifier (EN + HE)        internal/notify    macOS notifications
internal/world        time zones, weekends, routines           internal/guard     injection + character checks
internal/persona      built-in personas, validation, avatars    internal/wa        whatsmeow lifecycle, Fake
internal/store        JSON stores + per-chat history JSONL     internal/config    data dir, settings, secrets
internal/events       pub/sub hub feeding SSE + activity        internal/contract  interfaces between layers
internal/model        shared data types
web/                  the UI (served at /)
scripts/              build_app.sh, install_app.sh, uninstall_app.sh, smoke.sh, geniconn/
```

### Make targets

| Target | What it does |
|---|---|
| `make dev-fake` | run with a **simulated WhatsApp and a scripted AI**: no phone, no Ollama |
| `make run` | run the real app with `./data` as the data folder and open the browser |
| `make test` | `go test -race ./...` |
| `make vet` | `go vet` + gofmt check |
| `make smoke` | end-to-end script against a throwaway fake server |
| `make app` | build `build/WhatsappDoppel.app` (icon, Info.plist, ad-hoc signature) |
| `make install` / `make uninstall` | install to `/Applications` (+ Desktop alias, project shortcut) / remove |
| `make build`, `make icon`, `make clean` | binary, icon render, cleanup |

### Running by hand

```bash
go run . serve --fake-wa --fake-llm --data-dir ./data/fake --port 7788 --open
go run . serve --data-dir ./data            # real WhatsApp + configured AI
go run . status --data-dir ./data
go run . quit --data-dir ./data
```

Environment overrides: `DOPPEL_DATA_DIR`, `DOPPEL_PORT`, `DOPPEL_LEGACY_DB` (old `bot.db` to
import on first start), `DOPPEL_NO_BROWSER=1`, `DOPPEL_NOTIFY=dry|on`.

Live evaluations against a local model are opt-in test files (for example
`DOPPEL_GOAL_EVAL=1`, `DOPPEL_EXPR_EVAL=1`) and are skipped by default.

### Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Changes are listed in [CHANGELOG.md](CHANGELOG.md).
