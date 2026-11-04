# Changelog

All notable changes to WhatsApp Doppel. Dates are in YYYY-MM-DD.

## [0.1.0] — Unreleased

The command-line bot became a desktop app with a web interface, for macOS, Windows and Linux.
Everything below is new unless noted.

### Names, tags, languages and context
- Nicknames: give anyone a name (People tab) that is used instead of their WhatsApp name in every
  chat — in the app, in what the persona reads and when it tags them.
- Fewer @tags: a tag now has to help — not again soon after one, the person being answered only
  in a busy conversation, others only to pull them in. Tags the model adds anyway become names.
- Languages (Settings › Behaviour and per chat): the languages it speaks (English and Hebrew by
  default), stay quiet or answer other languages, reply in the language of the message or a fixed
  one, and per person in the People tab. A light built-in detector decides; "ok" and emoji never
  count as another language.
- More natural typos: seven kinds of slip (swapped, dropped, doubled or neighbouring letters, two
  words run together, a mixed-up word like "your"/"you're", a word typed twice), corrected in
  rotating ways ("*work", "work*", "**work") or left alone. In one chat a kind of slip comes back
  at most twice among the last ten and never twice in a row.
- Memory tab › What it reads shows the prompt part by part (character, safety rules, chat
  context, goal, instructions, language, guidance, conversation) with each part's share; the
  character, safety rules, "how to read the conversation" and "stay yourself" parts can be
  edited for one chat without changing the persona.
- Connection ratings (People tab, groups): once a day with at least 20 messages the persona
  privately scores, 0 to 100, how connected it felt to each person who wrote, with a line why
  and the change since last time; "Rate now" on demand. Only you see it.
- Timing curve (Behaviour › Advanced): delays drawn evenly, on a bell curve, mostly quick with
  a long tail, or always the middle of their range.
- Ignore stickers (Behaviour › Advanced, off by default): a sticker gets no reply but is still
  read as part of the conversation.
- Keep going when a message is flagged (per chat, off by default): hand-off topics are flagged
  in the feed and the Log instead of pausing the chat.
- Memory tab › What it reads: the full prompt and messages of the next reply, your instructions
  for the chat, an editable summary of older messages (summarise now, or automatically when the
  history is full) and "Start fresh".

### Decision log
- Each chat panel has a **Log** tab: every message with what the persona did about it and why
  (answered, stayed quiet, reacted, left for you, blocked, waiting for approval), the time from
  their message to the reply, and the steps on the way (seen, waited, thought, typed). On top, for
  today, 7 days or everything: messages, answer rate, typical and longest reply time, why it didn't
  answer, how fast it answers and, in groups, who it answers.

### Scheduled messages and WhatsApp sync
- Start conversations › On a schedule: an opener when replies are turned on or you join a group,
  one at each set time, and repeats with a fixed gap, an even spread, a bell curve or at random
  (Poisson), with a daily cap. A moment that can't be sent right then (WhatsApp disconnected, not
  in the group, Away, outside active hours) is skipped and never sent late, so nothing piles up.
- Every page load re-checks WhatsApp: the connection state and the groups you are in. Groups you
  left or were removed from are marked "Not in group"; being added back picks up again.
- Faster chat lists right after linking (contacts and groups are served from cache while they
  refresh) and "connected" is no longer shown while WhatsApp stops answering; sends fail fast
  with "WhatsApp is not connected" instead of hanging.

### Windows and Linux
- Runs on Windows 10/11 and 64-bit Linux as well as macOS (amd64 and arm64 everywhere).
- Downloads built by `make dist` (and by the release workflow on a `v*` tag): a universal macOS
  app zip, a Linux tar.gz with an installer (app menu entry, icon, Desktop shortcut) and a Windows
  zip with a double-click `install.cmd` (Start menu and Desktop shortcuts, Settings > Apps entry).
  Installers work per user, need no admin rights and never touch your data.
- One-command installs from source: `scripts/install-from-source.sh` (macOS, Linux) and
  `scripts/install-from-source.ps1` (Windows); `INSTALL_FOR_AGENTS.md` lets coding agents do it.
- Data folder per OS: `~/Library/Application Support/WhatsappDoppel` (unchanged),
  `%APPDATA%\WhatsappDoppel`, `~/.config/WhatsappDoppel`.
- Notifications: notify-send on Linux, toasts via Windows PowerShell on Windows.
- SQLite is now pure Go (`modernc.org/sqlite`): no Xcode tools or C compiler needed to build.
  Existing WhatsApp sessions open unchanged.
- The UI says "this computer" instead of "this Mac", and names Finder, File Explorer or your
  file manager as appropriate.

### App and setup
- Mac app (`make install`): Applications, Launchpad, Desktop icon and a project-folder shortcut; one
  double-click starts a background server and opens the browser, a second click reuses it.
- Liquid-glass web interface with light/dark mode, a first-run setup (AI brain, WhatsApp QR link,
  persona, first chat), a guided tour, an illustrated in-app Guide and "?" help tips.
- Looks: Settings › Appearance offers Liquid Glass (default), Midnight (pure black), Daylight (pure
  white), Classic, Vintage (Parchment / Candlelit study), Ocean, Forest, Neon and High contrast, each
  previewed live; light/dark or system per look (`settings.skin`).
- Port finder and live port switching; Quit from Settings.
- One WhatsApp account, many chats running at once; each chat opens a centered window with
  Overview, People, Goal, Behaviour and Memory tabs.
- AI providers: Ollama (local, default), Claude and OpenAI, selectable per persona.

### Personas
- Profile cards with uploaded photos or colour-and-graphic avatars (custom SVG, no emoji), structured
  character fields, five built-ins (Leo, Kyle, Luna, Brad, Chad), *Build with AI* from one sentence,
  and *Clone yourself* from your own messages (opt-in).
- World and routine: home city, time zone (yours or another country's), weekend days, daily blocks
  that delay replies and produce natural "sorry, was at the gym" excuses.
- Emoji habits and message length are enforced on every reply (not just suggested), with an
  expression preview in the editor and a "Match theirs" length option.

### Behaviour
- Layered profiles: defaults for private chats and for groups, overridable per chat.
- Presets plus three vibe dials (Speed, Chattiness, Boldness) and an Advanced form.
- Realistic pipeline: notice delay, blue ticks, waiting for more messages, thinking, typing time
  scaled to length, split bubbles, quote-replies, meaning-aware reactions, occasional typos with
  corrections, active hours, Away, reply limits and cooldowns, optional check-ins after silence.
- Groups: @tags of members when relevant; choose who it answers (automatic by size, everyone,
  or picked people), starred people, per-person notes, trigger and mute words, a streak limit
  that stops bots replying to each other forever.

### Goals and missions
- Secret goals pursued tactfully (subtle, balanced or direct), private plan-ahead step,
  automatic "goal reached" detection, and a guard that rewrites replies that would reveal the goal.
- Mission templates, a Missions page with achievement badges, and *Start the conversation*.

### Control and safety
- Reply modes per chat: Auto, Approve, Co-pilot (three drafts to choose from).
- Hand-off: money, health, meeting up, distress, legal and "are you a bot?" pause the chat and alert you.
- Reveal: a pre-written "it was me" message that ends the act.
- Memory of people (view, pin, edit, forget), daily recaps, Mac notifications.
- Cross-chat context: a persona remembers across its chats with the same people. Groups use private chats
  discreetly (it knows, it never tells), private chats may mention shared groups; sensitive topics never cross,
  any memory can be locked to its chat, and a leak guard rewrites replies that would give a private chat away.
- Prompt-injection filtering, character-break detection, and a guard against writing as another group member.

### Fixed (from the original bot)
- The project did not build (duplicate `main`, invalid `persona.go`).
- Configured model was not installed; a gym line from another persona was used as everyone's fallback.
- Only one chat could run at a time; history grew without limit; Ollama's context window was too small.
- The injection filter blocked normal messages; Hebrew text could be cut mid-character in logs.
- Typing indicator stayed on after errors; contacts depended on a separately exported JSON file.

### Project
- PolyForm Noncommercial 1.0.0 license, responsible-use guide, security notes, contributing guide,
  issue and pull-request templates, and a GitHub Actions workflow (vet, race tests, JS check, smoke test).
