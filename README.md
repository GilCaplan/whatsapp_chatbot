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

### Architecture

One Go binary serves a local web UI (vanilla ES modules, no build step, embedded with
`go:embed`) and a JSON + Server-Sent-Events API on `127.0.0.1`. See **[CLAUDE.md](CLAUDE.md)**
for package boundaries and conventions, and **[docs/API.md](docs/API.md)** for the full HTTP/SSE contract.

```
main.go               launch | serve | status | quit
internal/app          wiring + process lifecycle (lock, instance.json, signals, notifier)
internal/launcher     single instance: instance.json, server.lock, health probe, detached spawn
internal/platform     OS differences: data dir, file lock, open URL/folder, spawn, alerts, console
internal/server       HTTP API, SSE, security middleware, static UI
internal/engine       per-chat reply pipeline (notice → seen → wait → think → typing → bubbles),
                      group decisions, people rules, goals/openers, approvals/co-pilot, hand-off,
                      reveal, memory & recap jobs, check-ins, playground
internal/behavior     behaviour profiles: presets, dials, ranges, resolver, planner, sampler, typos
internal/prompt       system prompt sections (identity, world, people, memory, agenda, expression)
internal/llm          providers: Ollama, Anthropic (SDK), OpenAI, Fake; registry
internal/goals        goal logic (style, reached detection)    internal/mission   templates, achievements
internal/mention      @tag directory and encoding               internal/memory    merge/select memories
internal/handoff      serious-topic classifier (EN + HE)        internal/notify    desktop notifications
internal/world        time zones, weekends, routines           internal/guard     injection + character checks
internal/persona      built-in personas, validation, avatars    internal/wa        whatsmeow lifecycle, Fake
internal/store        JSON stores + per-chat history JSONL     internal/config    data dir, settings, secrets
internal/events       pub/sub hub feeding SSE + activity        internal/contract  interfaces between layers
internal/model        shared data types
web/                  the UI (served at /)
scripts/              build_app.sh, install_app.sh, uninstall_app.sh (Mac), dist.sh, smoke.sh,
                      install-from-source.sh/.ps1, dev.ps1, linux/, windows/ (installers), geniconn/
```

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
