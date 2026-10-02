# WhatsApp Doppel — local HTTP / SSE API

The server binds **127.0.0.1 only** and serves the embedded web UI at `/`.
All payloads are JSON. Go types live in `internal/model/model.go`, `internal/model/behavior.go`
and `internal/config/config.go` — JSON field names below match their tags exactly.

## Conventions

- **Auth for mutating requests** (`POST`/`PUT`/`PATCH`/`DELETE`):
  header `X-Doppel-Token: <token>`. The token is embedded in `index.html` as
  `<meta name="doppel-token" content="...">` (the server templates it in) and is also returned by `GET /api/health`.
  JSON bodies must send `Content-Type: application/json` (the avatar upload uses `multipart/form-data`).
- **Origin/Host guard**: `Host` must be `127.0.0.1:<port>` or `localhost:<port>`; if `Origin` is present it must
  match one of those; `Sec-Fetch-Site: cross-site` is rejected. (DNS-rebinding + CSRF protection.)
- **Errors**: non-2xx with `{"error":"Human readable message","code":"snake_case"}`.
- Timestamps are RFC 3339 strings. Optional values are `null`.

## Shapes (abridged; see Go types)

```
Settings      { version, port, theme:"system|light|dark", onboardingCompleted,
                llm:{ defaultProvider:"ollama|anthropic|openai", defaultModel, ollamaURL, ollamaModel,
                      anthropicModel, openaiModel, openaiBaseURL, temperature, replyMaxTokens, ollamaNumCtx },
                behavior:{ triggerPrefix, private: BehaviorProfile, group: BehaviorProfile },
                notifications:{ enabled, approvals, goals, handoff, whatsapp, recap, sound },          // (wave 3)
                safety:{ handoff:{ enabled, money, health, meeting, distress, bot, legal, aiCheck },
                         reveal:{ template (≤ 600 chars; {persona} and {me} are filled in) } },        // (wave 3)
                memory:{ enabled, cross: CrossSettings }, recap:{ enabled, time:"HH:MM", keepDays (1–365) }, // (wave 3; cross v5)
                clone:{ collectSamples, maxSamples (50–5000) } }                                         // (wave 3)
                // defaultModel is derived from the per-provider model of defaultProvider; set ollamaModel/anthropicModel/openaiModel.
                // version is 5. v1 files ("replies"/"approvals" blocks) are migrated on load; those top-level keys are ignored by PUT.
                // v2 files get the v3 behaviour fields (marked † below) from the preset each profile is labelled with;
                // v3 files get the v4 fields (marked ‡) the same way, and the default notifications/safety/memory/recap/clone blocks.
                // Defaults: notifications all on except recap; every hand-off category on; memory on; recap off at 21:00, 30 days;
                // clone sampling off, 500 samples.
                // v4 files get the default memory.cross block (v5); its bools can't be told apart from unset either.
CrossSettings { enabled, groupMode:"off|discreet|open", dmMode:"off|discreet|open",
                sensitive:{ money, health, meeting, distress, bot, legal, romance, secret } (true = never crosses),
                freshDays (1–90), maxPeople (1–8), maxItems (1–16) }
                // Defaults: on, groups "discreet", private chats "open", every sensitive topic kept local, 14 days, 4 people, 8 items.
Persona       { id, builtIn, name, tagline, avatar:{kind:"generated|upload", gradient:[c1,c2], glyph, initials, version},
                bio, personality, style, vocabulary, rules, language, emoji:{usage:"none|rare|some|lots", favorites:[]},
                messageLength:"short|medium|long", goal, decisionHint, fallbackReply, advancedPrompt,
                goalStyle:"subtle|balanced|direct", goalPlanAhead: bool (null = true), goalAfterReached:"relax|continue",
                llm: null | {provider, model}, world: World, createdAt, updatedAt }
World         { city, country, timezone ("" = this computer, else IANA), routine:[RoutineBlock] (never null, ≤ 12) }
RoutineBlock  { label (≤ 24), days:["mon".."sun"] (empty = every day), from:"HH:MM", to:"HH:MM", reach:"normal|slow|unreachable" }
                // to <= from wraps past midnight; from == to is invalid. Times are in World.timezone. An unknown zone,
                // a bad time or day → 400 invalid_persona; labels are trimmed, days sorted, reach defaults to "normal".
                // The reply prompt gets "RIGHT NOW: Thursday 1 October, 20:11 — evening in Tel Aviv (your local time)…",
                // the weekend for World.country (Israel/Gulf: Fri–Sat), the current/just-finished/next routine block,
                // the people's time when the zone differs from this computer's, and — when answering ≥ 10 min late across a
                // slow/unreachable block (≥ 45 min otherwise) — a late-reply note. Unreachable blocks hold new messages
                // until they end (activity `deferred`, reason `routine`); slow blocks triple the notice delay.
ChatAssignment{ key:"dm:<phone>|group:<id>|lid:<lid>", kind:"dm|group", jid, altJid, name, personaId, enabled,
                mode:"auto|approve|copilot", approvalMode, goalOverride, goalStyle: null|"subtle|balanced|direct", goalPlanAhead: null|bool,
                behavior: BehaviorOverrides, people: PeopleConfig, snoozedUntil: null|time,
                memory: null|bool, handoff: null|HandoffState, missionId, cross: CrossContext, revealedAt: null|time, lastActivityAt, createdAt }
                // goalStyle / goalPlanAhead: null = use the persona's setting.
                // mode (wave 3) replaces approvalMode, which stays as a derived bool (mode != "auto") for one version;
                // old files get mode from approvalMode. memory: null = settings.memory.enabled.
CrossContext  { mode: ""|"off"|"discreet"|"open" ("" = the app default for the chat kind), share: null|bool (null = on) }
HandoffState  { category:"money|health|meeting|distress|bot|legal", excerpt, sender, messageId, at, how:"keyword|ai" }
                // set while a sensitive message has paused the persona in this chat (see Hand-off below)
ChatGoal      { text, source:"chat|persona|default", style, styleSource:"chat|persona", planAhead,
                planAheadSource:"chat|persona", afterReached:"relax|continue", state:"working|reached",
                reachedAt: null|time, how:"said_word|ai", evidence, lastPlan, lastPlanAt }
                // "overrides" (v1) is gone: old chats.json files are migrated into "behavior" on start.
ChatItem      { key, kind, jid, altJid, name, phone, participantCount, lastMessageAt, isSelf, assigned }
PeopleConfig  { mode:"auto|everyone|selected" ("" = auto), people:[PersonPrefs] }
PersonPrefs   { jid, name, respond: null|bool (null = follow the mode), priority, notes (≤ 300 characters),
                cross: null|bool (groups: use the persona's private chat with this person here; null = on) }
Participant   { jid, phone, lid, name, isAdmin, isSelf }   // a group member; jid is the address tags must use (lid or phone)
Message       { id, ts, speaker:"them|me", name, text, fromBot, senderJid?, mentions?, kind?, corrected?, waId?, crossUsed? }
                // crossUsed: names of the people whose context from other chats informed this persona bubble.
                // (wave 3) kind: "fix" (the "*word" bubble after a typo) | "edited" | "reveal"; corrected: the intended
                // text of a bubble sent with a typo; waId: WhatsApp message id of a persona bubble.
                // senderJid: who wrote a "them" message (groups). mentions: display names tagged with "@" in text
                // ("@Dana" — incoming "@<number>" tags are shown as "@Name"). Both absent in older history.
PendingReply  { id, chatKey, chatName, personaId, personaName, text, context:[Message], createdAt, stale, autoSendAt, provider, model, mentions?, opener, drafts?, crossUsed? }
                // drafts (wave 3, co-pilot): [{tone:"brief|warm|playful", text, mentions?}] ×3; text == drafts[0].text.
                // mentions: the display names the reply tags (text contains "@Name" for each).
ActivityEvent { id, ts, type, chatKey, chatName, personaName, text, meta:{...} }
Memory        { id, chatKey, personJid?, person?, text (≤ 160), kind:"fact|preference|event|relationship|other",
                source:"learned|user", pinned, confidence (0–100), evidence?, createdAt, updatedAt, lastUsedAt, expiresAt,
                sensitive?: "money|health|meeting|distress|bot|legal|romance|secret", scope?: "local|shared" }  // (wave 3; sensitive/scope: cross-chat)
MemoryItem    Memory & { shares (used in other chats right now), effectiveSensitive ("" or the stored/classified topic) }
Brief         { chatKey, personaId, kind, topics:[≤4], commitments:[≤3], tone, people:[{name, jid?, note}] (groups, ≤6),
                sensitive:[topics the window touched], from, to, messageCount, generatedAt }                    // cross-chat
CrossView     { enabled, kind, mode, modeSource:"chat|default", share, shareSource:"chat|default",
                sources:[{chatKey, name, kind, person, personJid?, items, hasBrief, blocked?:"handoff|revealed|share_off|person_off|memory_off|mode_off"}],
                brief: null|Brief, briefPending }
Recap         { id, chatKey, chatName, personaName, date:"YYYY-MM-DD", from, to, messageCount, headline, topics:[],
                goalProgress, toKnow:[], mood, generatedAt, onDemand }                                                    // (wave 3)
MissionTemplate { id, title, blurb, goal (with {blank} placeholders), category:"words|vibes|curious|share|plans",
                blanks:[{key, label, placeholder}], difficulty (1–3), badge, detect:""|"say_word"|"media:image"|"media:audio" }
MissionRecord { id, chatKey, chatName, personaId, personaName, goal, templateId, style, startedAt, reachedAt: null|time,
                how:"said_word|ai|media", evidence }
Achievement   { id, title, blurb, badge, unlocked, unlockedAt: null|time, progress, target }
WAStatus      { state, me:null|{jid, lid, pushName, phone, avatarUrl}, lastError, since }
ModelInfo     { id, label, meta }
```

### Reply behaviour

How a persona replies (timing, chance, message shape) — never what it says. Three layers:
app **Private** profile and app **Group** profile (`settings.behavior.private|group`) → per-chat
overrides (`ChatAssignment.behavior`, a missing field inherits) → effective profile.

```
BehaviorProfile {
  preset: "instant|natural|busy|slow|nightowl|custom",   // label only; the server relabels to "custom"
                                                        // when values no longer match the named preset
  // Responsiveness
  replyPercent, replyWhenNameMentioned*, replyWhenAtMentioned*, skipWhenOthersMentioned*,
  aiJudgement*, chimeInPercent*, ignoreLinks, reactPercent, pauseWhenYouReply,
  maxRepliesPerHour (0 = unlimited), maxRepliesPerDay (0 = unlimited), cooldownSec,
  historyMessages, historyChars, staleAfterMin,
  // Who it answers
  respondToAllMaxMembers*† (0–1024; groups up to this size answer everyone by default, 0 = never),
  answerAnyoneWhoAddressesIt*†, maxStreak† (0–50, 0 = unlimited), triggerWords† [≤ 20 × ≤ 40 chars], muteWords† [same],
  // Timing
  noticeMinSec, noticeMaxSec, markRead, waitForMoreSec, burstCapSec, thinkMinSec, thinkMaxSec,
  distractedPercent, distractedMinSec, distractedMaxSec, typingIndicator,
  typingCharsPerSec, typingJitterPercent, typingMinSec, typingMaxSec,
  // Message shape
  splitPercent, splitMaxParts, bubbleGapMinSec, bubbleGapMaxSec, quoteReplyPercent*,
  allowMentions*†, mentionMax*† (1–5), tagReplyPercent*† (0–100),
  lengthBias: "shorter|normal|longer|match",   // see "Emoji, length and reactions"
  typoPercent‡ (0–30; chance one bubble of an automatic reply has a typo — never approvals, trigger replies,
  manual sends or reveals), typoFixStyle‡: "correction" ("*word" bubble) | "edit" (WhatsApp edit, falls back to
  correction) | "none",
  // Blocks (overridden per chat as a whole)
  availability: { enabled, timezone ("" = this computer, "persona" = the chat persona's World.timezone, else IANA), week:[DayHours ×7 mon..sun],
                  outsideHours:"queue|silent", catchUpMaxMin },
  proactive:    { enabled, afterHours, maxPerDay, spreadMinutes },
  // Safety
  autoSendSeconds (approval auto-send, 0 = off), injectionFilter:"strict|balanced|off" }
  // * = only used in group chats; † = added in settings v3; ‡ = added in settings v4 (Natural 5 % correction, Instant 0)
  // Word lists replace wholesale (in PUT /api/settings and as per-chat overrides); they are trimmed and de-duplicated.
DayHours  { day:"mon|tue|wed|thu|fri|sat|sun", ranges:[{from:"HH:MM", to:"HH:MM"}] }
            // to <= from wraps past midnight ("18:00"→"02:30"); from == to = whole day; "24:00" allowed as "to";
            // an empty ranges array = off that day. The server always stores 7 entries mon..sun.
BehaviorOverrides { any subset of BehaviorProfile's fields (same names), preset? }
            // absent = inherit. availability/proactive are whole blocks.
```

Semantics worth knowing for the UI:
- **Left on read** is `replyPercent < 100` with `markRead` on (the persona opens the chat, then sometimes doesn't answer).
- **"Reply to everything"** in groups is `chimeInPercent: 100` + `aiJudgement: false` (no separate field).
- `reactPercent` only applies when the persona would otherwise stay quiet; the emoji is chosen by what the message
  means (see "Emoji, length and reactions").
- Min/max pairs (`notice*`, `think*`, `distracted*`, `typing*Sec`, `bubbleGap*`) are never rejected for
  min > max: settings swap them, per-chat resolution uses `max = max(min, max)`.
- `typingIndicator: false` hides "typing…" but the typing time still passes.
- Ranges/enums come from `GET /api/behavior/presets` (single source of truth).

### Who it answers and @tags

Per group message, in this order (your own messages are never filtered):
1. **Mute words** (`muteWords`, whole word/phrase, any case; DMs too) → not answered, kept as context (`mute_word`).
2. **Who it answers** (groups): the sender is answerable when their `PersonPrefs.respond` says so, else by the chat's
   `people.mode` — `everyone`, `selected` (only people marked `respond:true`), or `auto` (= everyone when the group has at
   most `respondToAllMaxMembers` members, else selected; an unknown size counts as small). Someone not answerable still
   gets the normal rules when the message says the persona's name or @tags you and `answerAnyoneWhoAddressesIt` is on;
   otherwise it is kept as context (`not_selected`). People are matched by the user part of their phone or lid JID.
3. **Priority people** (`priority:true`, groups; always answerable in step 2) → reply without the reply-chance /
   chime-in / AI rolls (`priority_person`; availability and reply limits still apply).
4. **Trigger words** (`triggerWords`; DMs too) → reply without those rolls (`trigger_word`).
5. **Streak guard** (`maxStreak`): after that many replies in a row to the same person while nobody else spoke (you
   count as someone), stop (`streak_limit`). Starts over when someone else speaks or after 30 minutes of quiet.
6. The normal group decision (name / @mention / AI judgement / chime-in) and reply chance.

`PersonPrefs.notes` go into the prompt: groups get `ABOUT THE PEOPLE HERE` (members with notes, at most 15); a DM gets
`ABOUT THE PERSON YOU'RE TALKING TO`.

Tags: with `allowMentions` the group prompt lists the members (`PEOPLE IN THIS GROUP`, recent speakers first, at most 40)
and the persona may write `@Name`. The server keeps at most `mentionMax` tags of real members (unknown names lose the
"@", tags of yourself are removed), and — when at least two people spoke in the last 10 messages — `tagReplyPercent` of
the time starts the reply with `@Name` of the person it answers. On WhatsApp each tag becomes `@<number>` plus a real
mention (the member gets a notification); history, approvals and activity show `@Name`. Approved (possibly edited) and
manually sent texts may tag any member. Incoming `@<number>` tags are shown as `@Name`.

(wave 3) Media: a photo, video, voice note, audio file or sticker reaches the persona as the text `[photo]`,
`[video]`, `[voice note]`, `[audio]` or `[sticker]` (a caption follows: `[photo] look at this`), so personas react
to it and media missions can be reached.

### Emoji, length and reactions

`Persona.emoji.usage`, `Persona.messageLength` and `lengthBias` are prompt instructions **and** are enforced on every
generated reply (auto, approval drafts and regenerations, check-ins / "start the conversation", playground, expression
preview); texts you write or edit, and the persona's `fallbackReply`, are sent as written.

- Emoji (clusters incl. ZWJ sequences, skin tones, flags, keycaps): `none` → all removed; `rare` → at most 1, and none
  when one of the persona's last 3 messages had one; `some` → at most 2; `lots` → any. An emoji-free reply gets one that
  suits the message (a fitting favourite, else a calm favourite for everyday messages, else a default for good news /
  jokes / thanks / surprises) with chance 15% (`rare`), 60% (`some`), 85% (`lots`) — measured targets with
  llama3.1:8b: about 1 in 5, about half, most replies. Replies to serious / sad messages never keep or get emoji;
  replies to venting lose laughing emoji and get none added. A reply that was only emoji becomes a short word reply.
- Tone is a keyword heuristic over the messages being answered (English + Hebrew): serious, vent, good, love, funny,
  surprise, plan, question, greeting, neutral.
- Length: word budget short 12 / medium 30 / long 60; `shorter` ×0.6, `longer` ×1.6, ×1.25 when the message answered
  has more than 25 words; `match` = their words + 3 (at least 4, at most the `longer` budget). A reply over
  budget + max(3, budget/3) words is asked for again once (only when the first draft took under 20 s and there is time
  left), then cut at a sentence end; a first sentence up to 1.5× that limit stays whole, a longer one is cut after a
  comma or between words with "…" (never after a function word); words, @tags and URLs are never split.
- Reactions: a favourite emoji that suits the message's tone, else a default for it (😂 jokes; ❤️ 🎉 🔥 🙌 good news;
  ❤️ 🙏 😢 serious; …; only WhatsApp's classic six for `usage: "none"`). Never laughing at serious or venting messages.
- Activity meta: `sent` / `approval.queued` may carry `lengthRetried`, `trimmed`, `emojiRemoved` (count), `emojiAdded`;
  `reacted` carries `emoji` and `tone`, and its text describes the reaction in words
  ("Reacted to Dana's joke with a laugh instead of replying").

### Goals

What a persona steers towards (`Persona.goal`, per chat `ChatAssignment.goalOverride`) and how — never timing.
Goal settings are **not** part of the behaviour profile:

- `goalStyle`: `subtle` (default — the goal is a secret agenda: never stated or hinted, the target word is never
  written, rapport first, one small nudge per message, set-ups where it happens by itself), `balanced` (may raise the
  topic with a natural question, never says why), `direct` (pursues it openly, still never mentions "a goal").
- `goalPlanAhead` (default true): before each reply a private LLM call (JSON mode) reads the chat and returns
  `{situation, achieved, evidence, next}`; `next` goes into the reply prompt as a private note ("YOUR NEXT MOVE"),
  never into WhatsApp or history. Its latency is hidden by the think/typing delays; failures are ignored.
- `goalAfterReached` (persona only): `relax` (default — stop pursuing, just chat) or `continue` (keep gently pursuing).
- Reached detection: "say the word" goals (`get Josh to say the word apple`, `make them say "good morning"`,
  `לגרום ליוסי להגיד את המילה תפוח`) are checked on every incoming message — whole word, case/punctuation-insensitive,
  English plurals, Hebrew prefixes; in groups only the named person counts. Other goals are reached when the plan-ahead
  step's narrow "has it already happened?" check (a second small JSON call run alongside the planner) says so
  **and** its quote is found in the other people's messages since the goal was set (needs plan ahead on).
- A reply that gives the goal away (meta talk like "I need him to say…", "my mission", or the target word in
  subtle/balanced style) is rewritten once with a reminder, then — if it still leaks — written without the agenda.
- Progress (`GoalStatus`) is kept per chat in `runtime.json` and belongs to one goal text: changing the goal starts over.

WA `state` ∈ `disconnected | connecting | awaiting_qr | pairing | connected | logged_out | error | replaced | banned | outdated`.

Activity `type` ∈ `incoming | decision.skip | decision.reply | blocked_injection | noticing | seen | waiting | thinking |
generating | typing | sent | deferred | reacted | proactive | approval.queued | approval.sent | approval.discarded |
goal.reached | error | wa.status | system` and (wave 3) `handoff | handoff.resumed | reveal | memory | recap`. `text` is always plain English (never emoji; a reaction's emoji is only in `meta.emoji`).

A normal reply narrates: `incoming` → `noticing` → `seen` → `waiting` → `thinking` → `typing` → `sent`
(`typing`/`sent` once per bubble when the reply is split).

| type | example text | meta |
|---|---|---|
| `noticing` | "Hasn't looked at the chat yet — will see it in 14s" | `delaySeconds` |
| `seen` | "Seen (2 messages)" / "Looked at the chat (2 messages) — read receipts off" | `count`, `readReceipts` |
| `waiting` | "Waiting 8s for more messages", "More messages — waiting 5s", "Burst cap reached — replying now", "New message while thinking — starting over" | `waitSeconds`, `resetCount`, `reason` (`burst_cap`, `new_messages`, `cooldown`) |
| `thinking` | "Thinking for 6s" · distraction: "Got distracted — back in about 2 minutes" · plan ahead: "Planned the next move" · cross-chat: "Keeping in mind 2 things from your private chat with Dana — discreet" | `delaySeconds`, `distracted` (true for the distraction event); plan ahead: `stage:"plan"`, `plan` (the private next move), `latencyMs`; cross-chat: `stage:"cross"`, `crossContext` (below; never the notes themselves) |
| `generating` | "Writing a reply…" (trigger prefix and approval mode, which skip "thinking") | `model`, `messages` |
| `typing` | "Typing for 12s (84 characters, part 1 of 2)" (single bubble: "Typing for 12s (84 characters)") | `typingSeconds`, `chars`, `part`, `parts`, `approvalId` (approved replies) |
| `sent` | the message text | `provider`, `model`, `latencyMs`, `jid`, `part`, `parts`, `quoted`, `proactive`, `manual`, `goalRewritten` (the first draft gave the goal away and was rewritten), `crossContext` `{mode, people, items, sources}` (context from other chats was used), `crossRewritten` (a draft gave away something from another chat and was rewritten), `crossDropped` (in the end it was written without other chats), `mentions` (display names tagged in this bubble) |
| `goal.reached` | "Goal reached — Josh said apple" / "Goal reached — find out what Dana is doing this weekend" / "Goal reached — Dana sent a photo" | `goal`, `how` (`said_word`, `ai`, `media`), `evidence`, `afterReached`, `missionId?` |
| `deferred` | "Outside active hours — will reply around 08:30" / "Away until 09:00 — will reply then" | `resumeAt` (time), `reason` (`outside_hours`, `snoozed`) |
| `reacted` | "Reacted to Dana's message instead of replying" | `emoji`, `messageId` |
| `proactive` | "Will check in within 3 hours — quiet for 2 days" / "Checking in after 2 days of silence" / "Starting a conversation because you asked — about: the new bakery" | `stage` (`scheduled`, `starting`, `manual`), `silenceHours`, `dueAt`, `hint` |
| `decision.skip` | e.g. "Decided not to answer this one — left on read", "Staying quiet — the AI thinks Leo wouldn't jump in here", "Reply limit reached (6 per hour) — not answering", "You replied yourself — standing down" | `reason` (below) |
| `decision.reply` | e.g. "Joining in — someone said Leo's name", "Joining in — you were @mentioned", "Chiming in by chance" | `reason` (below) |
| `approval.sent` | the message text (one event per bubble) | `approvalId`, `edited`, `jid`, `provider`, `model`, `part`, `parts` |
| `handoff` | "Dana asked if they're talking to a bot — paused so you can take over" | `category`, `excerpt`, `how` (`keyword\|ai`), `sender`, `match?` |
| `handoff.resumed` | "Back on — Leo is answering in Dana again" | `category` |
| `reveal` | "Revealed Leo to Dana and paused this chat" | `message`, `jid`, `again` |
| `memory` (wave 3) | "Remembered 2 new things about Dana" | `added`, `updated` |
| `recap` | "Daily recap ready for Dana and Friends" | `chats`, `onDemand`, `headline` (first recap) |

(wave 3) Typos are `sent` events with meta `typo:true` (the bubble had a typo), `fix:true` (the "*word" bubble) or
`edited:true` (the typo was edited away); `approval.queued` carries `drafts` (count) in co-pilot mode;
`decision.skip` gains reasons `handoff`, `routine`; `deferred` gains reason `routine` (+ `block`).

Texts never contain reason codes; the code is always in `meta.reason`.
`decision.skip` reasons: `link`, `stale`, `mentions_other`, `llm_no`, `llm_error`, `chime_out`, `reply_chance`
(+ `percent`, `react:true` when it reacts instead), `outside_hours`, `snoozed`, `rate_limit` (+ `limit`,
`window:"hour|day"`), `you_replied`, `mute_word` (+ `word`), `not_selected` (+ `sender`; "Not answering Josh — this
group only answers people you picked"), `streak_limit` (+ `limit`; "Taking a break — 6 replies in a row to Josh with
nobody else joining in").
`decision.reply` reasons: `name_mentioned`, `mentioned_me`, `always_reply` (chime 100 % without AI judgement),
`llm_yes`, `chime_in` (random chime-in; replaces v1 `random_override`), `llm_error_random`, `priority_person`
(+ `sender`), `trigger_word` (+ `word`).
`incoming`, `approval.queued` and `approval.sent` also carry `mentions` (display names) when the text tags someone.
Other common `meta` keys: `provider`, `model`, `latencyMs`, `score`, `matches`, `percent`.

## System

| Method & path | Body | Response |
|---|---|---|
| `GET /api/health` | – | `{ok, version, port, token, startedAt, dataDir, onboardingCompleted, fakeWA, fakeLLM, platform:"darwin\|linux\|windows"}` (`platform` = the server's OS, for OS-specific wording in the UI) |
| `GET /api/system/ports?from=7000&to=9999&limit=20` | – | `{current, free:[int]}` |
| `POST /api/system/port` | `{port}` | `{url}` then SSE `system {kind:"port_changed", url}`; old port closes ~3 s later. 409 `port_in_use`, 400 `invalid_port` |
| `POST /api/system/quit` | – | `{ok:true}`, SSE `system {kind:"quitting"}`, process exits |
| `POST /api/system/open-data-dir` | – | `{ok:true}` (opens the data folder in Finder / File Explorer / the file manager via `xdg-open`); 503 `unavailable` when the server has no folder opener, 500 `open_failed` |
| `POST /api/system/notify-test[?dry=1]` | – | `{ok, backend:"terminal-notifier\|osascript\|notify-send\|powershell\|dry-run\|none", events}` — shows "Notifications are working" (`dry=1`: only reports the backend; `events:false` = fake-WhatsApp run, only tests are shown). 502 `notify_failed`. `DOPPEL_NOTIFY=dry\|off\|on` (env) logs instead / turns them off / allows activity notifications with `--fake-wa` |
| `GET /api/events` | – | SSE stream (below) |

## Settings & LLM

| Method & path | Body | Response |
|---|---|---|
| `GET /api/settings` | – | `Settings & {secrets:{anthropic:{set,hint}, openai:{set,hint}}}` |
| `PUT /api/settings` | partial Settings (deep-merged; `port`/`version` are ignored — use `POST /api/system/port`) | same as GET; SSE `settings.changed` |
| `PUT /api/settings/secrets` | `{anthropicKey?, openaiKey?}` (`""` clears; omitted = unchanged) | `{anthropic:{set,hint}, openai:{set,hint}}` |
| `POST /api/llm/test` | `{provider, model?}` | `LLMTestResult {ok, latencyMs, sample, provider, model, error}` (always 200) |
| `GET /api/llm/models?provider=ollama\|anthropic\|openai` | – | `{models:[ModelInfo]}` |
| `GET /api/ollama/status` | – | `{reachable, version, models:[ModelInfo], error}` |
| `POST /api/ollama/pull` | `{name}` | `{jobId}`; progress via SSE `ollama.pull` `{jobId,name,status,completed,total,percent,done,error}` |

`PUT /api/settings` with `behavior`: objects deep-merge (`{"behavior":{"group":{"replyPercent":80}}}` changes one
field), arrays replace wholesale — always send the full `availability.week`. Every profile is validated against the
ranges/enums below (400 `invalid_settings`, message names the field, e.g. `behavior.private.replyPercent must be
between 0 and 100`); `behavior.triggerPrefix` ≤ 8 characters, no spaces (`""` disables the trigger).

## Reply behaviour

| Method & path | Body | Response |
|---|---|---|
| `GET /api/behavior/presets` | – | `{presets:[{id, label, description, private: BehaviorProfile, group: BehaviorProfile}], ranges:{field:{min,max}}, enums:{field:[...]}, defaults:{private, group}, dials:{speed\|chattiness\|boldness: DialDef}, dialOrder:["speed","chattiness","boldness"]}` |
| `POST /api/behavior/sample` | `{kind:"dm\|group" ("private" accepted), profile: BehaviorProfile (partial ok: missing fields = defaults for kind; clamped, never 400), text?, samples?:1–10 (default 5), approval?:bool}` | `{samples:[{totalSec, phases:[{name, sec, startSec}], bubbles:[{text, typingSec}], quoted}], summary:{minTotalSec, medianTotalSec, maxTotalSec, splitShare}}` |

- Presets (`id`): `natural` (default for both kinds), `instant`, `busy`, `slow`, `nightowl`; `custom` is the label
  for anything else. `ranges` keys are JSON names; nested ones are `availability.catchUpMaxMin`,
  `proactive.afterHours`, `proactive.maxPerDay`, `proactive.spreadMinutes`. `enums` keys: `preset`,
  `injectionFilter`, `lengthBias`, `availability.outsideHours`, `availability.week.day`.
- Vibe dials (`behavior/dials.go`): `DialDef {id, label, blurb, fields:[owned JSON names], levels:[DialLevel ×5]}`,
  `DialLevel {level 1–5, label, blurb, privateBlurb?, private:{field:value}, group:{field:value}}`. A dial is derived,
  never stored: moving it writes that level's fields for the chat kind (Settings: `PUT /api/settings`
  `behavior.private|group`; a chat: `PATCH /api/chats/{key}` `behavior`). Each field belongs to one dial; group-only
  fields are absent from `private`. Speed owns `notice*`, `waitForMoreSec`, `burstCapSec`, `think*`, `distracted*`,
  `typingCharsPerSec`, `typingJitterPercent`, `typingMin/MaxSec`; Chattiness owns `replyPercent`, `chimeInPercent`*,
  `aiJudgement`*, `maxRepliesPerHour/Day`, `cooldownSec`; Boldness owns `reactPercent`, `quoteReplyPercent`*,
  `tagReplyPercent`*, `mentionMax`*, `typoPercent`. Every field is monotone across the levels, and every preset lands
  exactly on a level of each dial (Natural 3/3/3, Instant 5/3/1, Busy 2/1/3, Slow 1/2/3, Night owl 4/3/3).
  `DialPos {level, exact, matches:[levels equal exactly]}`: `exact` = every owned field equals the level; otherwise
  the nearest level (log-scaled distance, "0 = no limit" as the far end). Private Chattiness 3–5 coincide (`matches`
  lists all; `level` prefers 3). Settings v4 nudged three preset numbers so this holds (Slow distracted 25→35 %,
  Busy reactions 5/25→0/15 %, Slow 0/10→0/15 %); profiles/chats still holding the whole old Busy/Slow preset were moved
  to the new numbers once on upgrade so they keep their label.
- Sample: assumes ONE incoming message and simulates the engine's planner with fresh randomness each call (so "re-roll"
  = call again). `phases[].name` ∈ `notice | seen | wait | think | distracted | gap | typing | send` in time order;
  `seen` (only when `markRead`) and `send` are instants (`sec` 0); zero-length phases are omitted. `startSec` is the
  offset from the incoming message. `text` defaults to a sample reply whose length follows `lengthBias`.
  `quoted` = the first bubble would quote the message (groups only). `splitShare` = fraction of samples with > 1 bubble.
  With `approval:true`: no think/distraction, typing and bubble gaps capped at 3 s (the human review time is not
  included). This is also how approved replies (`POST /api/approvals/{id}/approve`, auto-send) are delivered.

## WhatsApp

| Method & path | Body | Response |
|---|---|---|
| `GET /api/wa/status` | – | `WAStatus` |
| `POST /api/wa/pair` | – | `{ok:true}`; QR frames on SSE `wa.qr` `{png:"data:image/png;base64,…", expiresInSec}` |
| `GET /api/wa/qr.png` | – | current QR PNG or 204 |
| `POST /api/wa/reconnect` | – | `{ok:true}` |
| `POST /api/wa/disconnect` | – | `{ok:true}` |
| `POST /api/wa/logout` | – | `{ok:true}` (state → `logged_out`, then pairing can start again) |
| `GET /api/wa/chats?q=&kind=all\|dm\|group&tab=recent\|assigned\|contacts\|groups&limit=50&offset=0` | – | `{items:[ChatItem], total}` (`tab=recent` always includes a "You (message yourself)" item with `isSelf:true`) |
| `POST /api/wa/chats/refresh` | – | `{ok:true}` |
| `GET /api/wa/avatar?jid=` | – | image bytes, or **204** when none (UI shows generated fallback) |

## Chat assignments

| Method & path | Body | Response |
|---|---|---|
| `GET /api/chats` | – | `[ChatAssignment & {personaName, pendingCount, historyCount, goal: ChatGoal, nextCheckInAt: null\|time}]` (`nextCheckInAt` = scheduled automatic check-in, from runtime.json) |
| `POST /api/chats` | `{jid, personaId, mode?, approvalMode?, enabled?, behavior?: BehaviorOverrides}` (mode wins over approvalMode) | `ChatAssignment` (201; server canonicalizes key/jid/altJid/name); 409 `already_assigned`; 400 `invalid_behavior` |
| `PATCH /api/chats/{key}` | any of `{enabled, personaId, mode, approvalMode, goalOverride, goalStyle, goalPlanAhead, name, behavior, snoozedUntil, people, memory, missionId, cross}` | `ChatAssignment`; 400 `invalid_behavior` / `invalid_snooze` / `invalid_goal` / `invalid_people` / `invalid_mode` / `invalid_memory` / `invalid_cross`. `approvalMode:true` keeps co-pilot, `false` = auto; `memory:null` = the app setting; `cross: {mode?: string\|null, share?: bool\|null}` (absent = keep, null = the app default; `cross:null` resets both); `people.people[].cross: bool\|null` |
| `GET /api/chats/{key}/people` | – | `{kind, mode:"auto\|everyone\|selected", effectiveMode:"everyone\|selected", memberCount, threshold, answerAnyoneWhoAddressesIt, members:[{jid, name, phone, lid, isAdmin, isSelf, respond (effective), respondSource:"person\|mode", priority, notes, lastSpokeAt: null\|time, left, cross (effective), crossSource:"person\|default", dmChatKey ("" = no private chat with this persona), dmShares}], membersError?}` — group members (cached by WhatsApp, 10 min) merged with the chat's preferences; people with preferences who are no longer members are listed with `left:true`. Order: recent speakers, then by name, then you, then people who left. DMs list the single contact (for notes). |
| `GET /api/chats/{key}/behavior` | – | `{kind:"dm\|group", effective: BehaviorProfile, sources:{field:"chat"\|"default"}, overrides: BehaviorOverrides, defaults: BehaviorProfile, snoozedUntil: null\|time, available: bool, nextChangeAt: null\|time, goal: ChatGoal, dials:{speed, chattiness, boldness: DialPos}}` |
| `POST /api/chats/{key}/initiate` | `{hint?: string ≤ 300}` | `{ok:true, approval:bool}` — the persona starts a conversation now (below). 409 `chat_disabled` / `busy`, 400 `invalid_hint`, 404 |
| `POST /api/chats/{key}/goal/reset` | – | `ChatGoal` — forgets the goal progress (reached / planned moves); the persona pursues the goal again. SSE `chats.changed` |
| `DELETE /api/chats/{key}` | – | `{ok:true}` |
| `GET /api/chats/{key}/history?limit=100` | – | `[Message]` |
| `DELETE /api/chats/{key}/history` | – | `{ok:true}` |
| `POST /api/chats/{key}/send` | `{text}` | `{ok:true}` — sends as the persona |
| `POST /api/chats/{key}/reveal` | `{text?, force?}` | `{ok, text}` — sends the reveal message as yours (text "" = `safety.reveal.template` with `{persona}`/`{me}` filled in; no typos, no delays; history `kind:"reveal"`), discards pending replies, clears `handoff`, pauses the chat (`enabled:false`, `revealedAt`). 409 `already_revealed` while revealed and paused (then `force:true`); 400 `text_too_long` (> 2000); 502 `reveal_failed` |
| `POST /api/chats/{key}/handoff/resume` | – | `ChatAssignment` — clears `handoff` (no-op when not paused); activity `handoff.resumed`; the next message is answered normally |
| `GET /api/chats/{key}/memories` | – | `{enabled, crossEnabled (what is learned here may reach other chats), items:[MemoryItem] (pinned first, then newest), pending (messages since the last extraction), lastExtractedAt: null\|time}` |
| `POST /api/chats/{key}/memories` | `{text, person?, personJid?, pinned?, scope?, sensitive?}` | `Memory` (`source:"user"`, never rewritten by the extractor; `sensitive` classified when not given); 400 `invalid_memory` (empty or > 160 chars, unknown scope/topic) |
| `PATCH /api/chats/{key}/memories/{id}` | `{text?, pinned?, person?, scope?: "local"\|"shared"\|null, sensitive?: ""\|topic}` | `Memory`; changing the text makes it `source:"user"`; `scope:"local"` = never used in other chats, `"shared"` = your unlock (crosses even when sensitive), `null` = follow the settings; 400 `invalid_memory`; 404 |
| `DELETE /api/chats/{key}/memories/{id}` | – | `{ok:true}`; 404 |
| `DELETE /api/chats/{key}/memories` | – | `{ok:true}` — forget everything learned in this chat |
| `POST /api/chats/{key}/memories/extract` | – | `{added, updated}` — reads the messages since the last extraction now (waits for the background slot); 502 `extract_failed` |

Memory (wave 3): after 6 new messages from the other people and 90 s of quiet, a background job (one at a time,
yields to replies; 45 s, ≤ 350 tokens, JSON) asks the persona's model for durable facts about the **other** people
(never the persona). Each one must name someone who wrote in the chat and be backed by their words (`evidence`),
otherwise it is dropped; moods and "what I did today" are skipped. Near-repeats update the existing memory (pins and
ids kept), at most 60 per chat (oldest unpinned learned ones go first), events are forgotten 7 days after their date.
Replies get up to 12 memories (pinned first, then recent, on-topic and upcoming ones). Every change publishes
`memories.changed`; the chat's `memory` switch (null = `settings.memory.enabled`) turns learning and use off.

### Cross-chat context

| Method & path | Body | Response |
|---|---|---|
| `GET /api/chats/{key}/cross` | – | `CrossView` — the chat's effective mode and sharing, the other chats of the same persona it may draw on (and why some are not used), its own brief |
| `POST /api/chats/{key}/cross/refresh` | – | `Brief` — rewrites this chat's brief now (waits for the background slot); 502 `brief_failed`; 404 |

A persona can use what it learned in its **other chats with the same people** (same persona only). In a group, the
private chats with the people taking part right now (the person answered, recent speakers, people tagged lately;
`maxPeople`) — **Discreet** by default: the prompt section is background only ("never mention, quote or hint at it;
never imply you chat privately"); **Open** may refer to it lightly with that person, never in front of others. In a
private chat, the groups the contact shares with the persona (roster, else their messages there) — **Open** by default:
what *they* said or did there and what the persona promised there, never other members' things. **Off** keeps chats apart.
Only summaries cross: the source chat's memories about that person and its **brief** (`briefs/<chatKey>.json`: written
by a background JSON call after 8 new messages and 3 min of quiet, only for chats that can be a source; ≤ 300 tokens).
Never crossing, whatever the mode: memories whose topic is sensitive (stored `sensitive`, else a keyword classifier in
English and Hebrew over text and evidence; topics switched on in `memory.cross.sensitive`), memories learned from a
sensitive message, `scope:"local"` memories, unpinned memories and briefs older than `freshDays`, past events, a brief's
topics and notes when its window touched a sensitive topic (its promises are checked one by one), and anything from a
chat that is paused for a hand-off (its brief is deleted on hand-off), revealed (deleted on reveal), has learning off,
sharing off (`cross.share:false`) or — for one person in a group — `people[].cross:false`. Every reply that used other
chats is checked (`crossctx.Leak`: distinctive words or three-word runs of a note that weren't said in this chat, or
phrases like "you told me", "in our private chat"); a leaky draft is written again with a reminder, then — if it still
leaks — without the other chats (activity `crossRewritten`/`crossDropped`). Co-pilot drafts that leak are dropped.
No extra model call per reply otherwise. Playground and preview chats never draw on real chats. The prompt preview
shows the section.
| `POST /api/chats/{key}/simulate` | `{text, fromMe?, senderJid?}` (groups: the member it comes from; absent = a random member, else "Tester") | `{ok:true}` — injects an incoming message into the real pipeline. Allowed only with `--fake-wa`, for your own (self) chat, or when the chat is in approval mode; otherwise 403 `simulate_not_allowed` (a fake message must never trigger a real reply to a real person) |

`{key}` must be URL-encoded (`encodeURIComponent`) — keys contain `:`.
Mutations publish SSE `chats.changed`.

PATCH `behavior` semantics:
- key absent → overrides unchanged; `"behavior": null` → remove every override (inherit everything).
- object → partial: each present field overrides, a field set to `null` inherits again, absent fields keep their
  current override. `availability` and `proactive` are replaced (or nulled) as whole blocks — send the full block.
- `"preset": "busy"` may accompany the values of a preset to label the chat; the effective label is recomputed
  (it becomes the matching preset or `custom`).
- Unknown fields, wrong types or out-of-range values → 400 `invalid_behavior` and nothing is saved.

PATCH `people`: `{mode?, people?: [{jid, name?, respond?: true|false|null, priority?, notes?}]}` — entries are upserted by
JID (phone and lid forms of the same user match); absent fields keep their value, `respond:null` follows the mode again.
Entries left without any preference are dropped. `"people": null` forgets every preference. Unknown fields, a bad mode,
an address without "@", duplicates or notes over 300 characters → 400 `invalid_people`.

PATCH `snoozedUntil` ("Away"): RFC 3339 time, or `null` to clear (a time not in the future also clears). While
snoozed the chat behaves like "outside active hours" using its effective `availability.outsideHours`
(`queue`: messages are answered after the snooze ends; `silent`: kept as context only).

`POST /api/chats/{key}/initiate` ("Start the conversation"): the persona writes an opener through the normal reply
cycle from *thinking* on (think time, plan ahead, typing, bubbles per the chat's behaviour) and sends it — or, in
approval mode, queues it in Approvals (`approval:true`). Because it is an explicit request it ignores active hours,
Away and reply limits, but needs the chat enabled (409 `chat_disabled`) and idle (409 `busy`). The opener prompt
(`prompt.Initiate` + `Opener`) knows whether the chat is new, how long it has been quiet, whether it's a group, the
optional `hint` (topic) and the chat goal (a natural first step, never revealed); plan ahead runs in "opening" mode.
Activity: `proactive` with `stage:"manual"`, text "Starting a conversation because you asked — about: …", `hint`.
Openers count as check-ins (`proactive` in runtime.json, `sent` meta `proactive:true`); a queued opener has
`PendingReply.opener:true` and Regenerate writes a new opener (not a reply).

PATCH `goalStyle` / `goalPlanAhead`: absent = keep, `null` = use the persona's setting, a value overrides it for this
chat. `goalStyle` must be `subtle|balanced|direct` (case-insensitive), `goalPlanAhead` a bool; anything else → 400
`invalid_goal`. Changing `goalOverride` (or the persona's goal) starts the progress over automatically.

`GET /api/chats/{key}/behavior`: `sources` has one entry per BehaviorProfile field plus `preset`
(`availability`/`proactive` are single entries). `defaults` is the app profile for the chat's kind.
`available` is false while snoozed or outside active hours; `nextChangeAt` is when that flips (opening time when
closed, closing time when open; `null` = never, e.g. active hours disabled).

## Personas

| Method & path | Body | Response |
|---|---|---|
| `GET /api/personas` | – | `[Persona]` |
| `POST /api/personas` | Persona (id ignored) | `Persona` (201) |
| `GET /api/personas/{id}` | – | `Persona` |
| `PUT /api/personas/{id}` | Persona | `Persona` |
| `DELETE /api/personas/{id}` | – | `{ok:true}` (assigned chats get disabled) |
| `POST /api/personas/{id}/duplicate` | – | `Persona` (new id, name "X copy", builtIn false) |
| `POST /api/personas/{id}/reset` | – | `Persona` (built-ins only: restore seed text) |
| `PUT /api/personas/{id}/avatar` | multipart field `file` (png/jpg/gif ≤ 5 MB; server crops to square and stores 512px PNG) | `Persona` (avatar.kind="upload", version bumped) |
| `DELETE /api/personas/{id}/avatar` | – | `Persona` (back to generated) |
| `GET /api/personas/{id}/avatar?v=` | – | PNG |
| `POST /api/personas/draft` | `{description, samples, provider?, model?}` | `{persona: Persona (id ""), raw}` — AI builder, not saved |
| `GET /api/personas/{id}/prompt-preview?chatKey=` | – | `{system}` |
| `POST /api/personas/{id}/expression-preview` | `{persona?: Persona (unsaved editor fields; required when id is "new"), lengthBias?: "shorter\|normal\|longer\|match" ("" = the Private profile's), count?: 1-3 (default 3), roll?: int (re-roll)}` | `{samples:[{incoming, reply, words, emoji, budget, tone, adjusted}], provider, model, latencyMs}` — sample DM replies at the persona's emoji/length settings through the real reply path (no goal agenda). 400 `missing_persona` / `invalid_persona` / `invalid_length_bias`, 404, 502 `preview_failed` |

Mutations publish SSE `personas.changed`.

## Playground

| Method & path | Body | Response |
|---|---|---|
| `POST /api/playground` | `{personaId}` | `{sessionId}` |
| `POST /api/playground/{id}/messages` | `{text, group?:bool, goal?:string, initiate?:bool}` (with `initiate:true` the persona speaks first; `text` is then an optional topic and may be empty) | `PlaygroundReply {reply, decision:null\|{wouldReply,reason}, latencyMs, blocked, blockReason, provider, model, goal:{text, style, plan, reached, evidence, rewritten}, speaker?, mentions?}` |
| `DELETE /api/playground/{id}` | – | `{ok:true}` |

Playground group mode: each user message is attributed to a small rotating cast (`speaker`: Dana, Noam, Maya, Eitan) so
tagging can be tried; `mentions` lists the display names the reply tags.

Playground `goal`: present = the session's "Goal for this test" (`""` = back to the persona's goal; absent = unchanged).
The reply's `goal.plan` is the private plan-ahead move (only shown in the playground), `goal.reached` the session's
progress, `goal.rewritten` true when the first draft gave the goal away.

## Approvals

| Method & path | Body | Response |
|---|---|---|
| `GET /api/approvals` | – | `[PendingReply]` |
| `POST /api/approvals/{id}/approve` | `{text?, draft?}` (edited text; co-pilot: 0-based draft index, `text` wins when both are sent) | `{ok:true}` after delivery (typing ≤ 3 s per bubble, split per the chat's behaviour); 400 `invalid_draft` |
| `POST /api/approvals/{id}/regenerate` | – | `PendingReply` |
| `DELETE /api/approvals/{id}` | – | `{ok:true}` |

Co-pilot chats (`mode:"copilot"`): one JSON call writes three drafts (brief, warm, playful); each is checked like a
normal reply (character breaks, goal leaks, speaking as someone else) and gets the expression rules (word budget, emoji
level) and @mention rules. They are never auto-sent (`autoSendAt` stays null); regenerate = "More ideas". When fewer
than two drafts survive, a single normal reply is queued (`drafts` empty).

## Hand-off

Every message from them is scanned (before the people gates and the group decision, so it also fires when the persona
would stay quiet) for the categories switched on in `safety.handoff`, in English and Hebrew. A strong keyword ("are you
a bot?", "send me money", "I'm in hospital", "אתה בוט?") pauses the chat at once; a weak one ("I owe you", "meet you
at the gym lol") is confirmed by one small JSON call when `aiCheck` is on (any error = not paused). Pausing sets
`ChatAssignment.handoff`, ends the reply in progress (typing off), stops pending replies of the chat from auto-sending
(marked stale), keeps the message as context and emits `handoff` (+ a notification). While paused, messages are kept as
context (one `decision.skip` reason `handoff` per 10 minutes) and no check-ins are sent; your trigger-prefix messages
and approvals you send yourself still work. `POST …/handoff/resume` lets the persona answer again.

## Daily recap

At `recap.time` (this computer's zone) when `recap.enabled`, every enabled chat with at least 3 messages since the last
scheduled recap (at most 24 h back) gets one recap (one JSON call per chat, one chat at a time, at most 20; postponed 2
minutes while a reply is being written). A recap for the same chat and day replaces the earlier one; recaps older than
`recap.keepDays` are dropped. Changing the recap settings re-arms the timer right away.

| Method & path | Body | Response |
|---|---|---|
| `GET /api/recaps?chat=&limit=30` | – | `{items:[Recap]}` newest first (`chat` = one chat key, absent = all) |
| `POST /api/recaps/generate` | `{chatKey?}` (absent = every enabled chat with a message in the last 24 h) | `{items:[Recap]}` — `[]` = nothing to recap; a single quiet chat recaps its latest messages; 404 unknown chat; 502 `recap_failed` |

## Missions

A mission is a goal picked from a template with blanks ("Get {name} to say "{word}""), pursued by the normal goal
engine (style, plan ahead, evidence check, "say the word" detection). Starting one sets the chat's `goalOverride` +
`missionId` (the template id), starts the goal status over and records the start in `missions.json` (newest first, at
most 500; a chat has at most one open record). When any chat goal is reached, the engine completes the chat's open
record for that goal, or adds one, so hand-written goals count too (activity `goal.reached` gains `meta.missionId`;
SSE `missions.changed`). Media missions (`detect: "media:image" | "media:audio"`) are also reached when they send a
photo / voice note while the mission is the chat's goal (`how: "media"`). Editing the goal text by hand
(`PATCH goalOverride` without `missionId`) drops `missionId`. Achievements are computed from the history, never stored:
`first-mission`, `three-missions`, `word-smith` (3 say-the-word), `social-butterfly` (3 chats), `smooth-operator`
(subtle style), `night-shift` (00:00–05:00 on this computer), `photo-finish`, `planner` (a "make plans" mission),
`streak-week` (3 within 7 days), `ten-missions`; locked ones carry `progress`/`target`.

| Method & path | Body | Response |
|---|---|---|
| `GET /api/missions` | – | `{templates:[MissionTemplate], active:[{chatKey, chatName, kind, jid, personaId, personaName, missionId, goal: ChatGoal}], history:[MissionRecord], achievements:[Achievement]}` — `active` = chats with a goal still being worked on (`missionId` "" for a hand-written or persona goal); `history` includes open records (`reachedAt` null) |
| `POST /api/missions/start` | `{chatKey, templateId?, blanks?:{key:value}, goal?}` (a template with every blank filled, ≤ 60 chars each; or a free-form `goal` ≤ 500 chars) | `ChatAssignment`; 404 unknown chat; 400 `unknown_mission` / `invalid_blanks` / `invalid_goal` |
| `POST /api/missions/{chatKey}/abandon` | – | `ChatAssignment` — clears `goalOverride`/`missionId` (the persona's own goal applies again), resets the goal status, drops the open record; 404 |

## Clone yourself (wave 3)

Only with consent (`settings.clone.collectSamples`, off by default): messages you type yourself on WhatsApp (live
and from history sync; never what Doppel sends for a persona, never trigger-prefix messages) are kept in
`cache/self-samples.jsonl` (0600, FIFO `clone.maxSamples`) with links, e-mail addresses and phone numbers replaced
by `[link]`/`[email]`/`[number]`; a bare photo/sticker is skipped (a caption is kept).

| Method & path | Body | Response |
|---|---|---|
| `GET /api/clone/samples` | – | `{enabled (settings clone.collectSamples), count, since: null\|time, preview:[last 15 texts]}` |
| `DELETE /api/clone/samples` | – | `{ok:true}` |
| `POST /api/clone/draft` | `{name?, extra?, provider?, model?}` | `{persona: Persona (id ""), raw, sampleCount}` — the builder reads up to 150 recent, deduplicated samples; not saved (the UI opens it in the persona editor); 400 `too_few_samples` (< 20); 502 `draft_failed` |

Wave 3 endpoints whose feature has not landed yet answer **501** `{"code":"not_implemented"}`; the UI shows a
friendly "coming soon" state for them.

## Activity

| Method & path | Body | Response |
|---|---|---|
| `GET /api/activity?since=0&chat=&types=a,b&limit=200` | – | `{items:[ActivityEvent] (oldest first), lastId}` |
| `DELETE /api/activity` | – | `{ok:true}` |

## SSE — `GET /api/events`

`EventSource('/api/events')` (no token needed for GET). Frames:

```
id: 42
event: activity
data: {...json...}
```

Heartbeat comment `: ping` every 15 s. Reconnects send `Last-Event-ID` and receive missed events (except `wa.qr`).
On connect the server immediately sends one `wa.status` event with the current status.

| event | data |
|---|---|
| `wa.status` | `WAStatus` |
| `wa.qr` | `{png, expiresInSec}` |
| `activity` | `ActivityEvent` |
| `approval` | `{action:"new|updated|removed", pending: PendingReply}` |
| `chats.changed` | `{}` |
| `personas.changed` | `{}` |
| `settings.changed` | `Settings` (no secrets) |
| `ollama.pull` | `PullProgress` |
| `system` | `{kind:"port_changed", url}` or `{kind:"quitting"}` |
| `memories.changed` (wave 3) | `{chatKey}` — also after a chat's brief (cross-chat context) was rewritten |
| `recaps.changed` (wave 3) | `{}` |
| `missions.changed` | `{}` |
