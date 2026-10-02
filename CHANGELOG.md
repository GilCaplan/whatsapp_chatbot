# Changelog

All notable changes to WhatsApp Doppel. Dates are in YYYY-MM-DD.

## [0.1.0] — Unreleased

The command-line bot became a Mac app with a web interface. Everything below is new unless noted.

### App and setup
- Mac app (`make install`): Applications, Launchpad, Desktop icon and a project-folder shortcut; one
  double-click starts a background server and opens the browser, a second click reuses it.
- Liquid-glass web interface with light/dark mode, a first-run setup (AI brain, WhatsApp QR link,
  persona, first chat), a guided tour, an illustrated in-app Guide and "?" help tips.
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
