#!/usr/bin/env bash
# End-to-end smoke test against a throwaway server with fake WhatsApp + fake LLM.
# Exercises health, settings, personas, chats, the SSE stream, token/host
# guards, assigning a chat, a simulated incoming message → "sent" activity,
# and quit. Exits non-zero on the first failure.
#   scripts/smoke.sh        (or: make smoke)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/doppel-smoke.XXXXXX")"
BIN="$WORK/whatsapp-doppel"
DATA="$WORK/data"
LOG="$WORK/serve.log"
PID=""

pass() { printf '  \033[32m✓\033[0m %s\n' "$*"; }
fail() {
  printf '  \033[31m✗ %s\033[0m\n' "$*" >&2
  if [[ -f "$LOG" ]]; then
    echo "--- server log (tail) ---" >&2
    tail -n 40 "$LOG" >&2 || true
  fi
  exit 1
}
cleanup() {
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then kill "$PID" 2>/dev/null || true; fi
  rm -rf "$WORK"
}
trap cleanup EXIT

# json <path> : extract a value from JSON on stdin (plutil understands JSON).
json() { plutil -extract "$1" raw -o - - 2>/dev/null; }

free_port() {
  local p
  for _ in $(seq 1 50); do
    p=$((20000 + RANDOM % 20000))
    if ! nc -z 127.0.0.1 "$p" 2>/dev/null; then echo "$p"; return; fi
  done
  echo 0
}

echo "Building…"
CGO_ENABLED=1 go build -o "$BIN" . || fail "build failed"
PORT="$(free_port)"
[[ "$PORT" != 0 ]] || fail "no free port"
BASE="http://127.0.0.1:$PORT"

echo "Starting server on $BASE (data: $DATA)"
DOPPEL_NOTIFY=dry "$BIN" serve --fake-wa --fake-llm --data-dir "$DATA" --port "$PORT" >"$LOG" 2>&1 &
PID=$!

for _ in $(seq 1 100); do
  if curl -fsS "$BASE/api/health" >/dev/null 2>&1; then break; fi
  kill -0 "$PID" 2>/dev/null || fail "server exited during startup"
  sleep 0.2
done

HEALTH="$(curl -fsS "$BASE/api/health")" || fail "health"
TOKEN="$(json token <<<"$HEALTH")"
[[ -n "$TOKEN" ]] || fail "no token in /api/health"
[[ "$(json fakeWA <<<"$HEALTH")" == "true" ]] || fail "fakeWA not reported"
pass "health ($(json version <<<"$HEALTH"))"

CODE=""
RESP=""
api() { # api METHOD PATH [BODY] → sets CODE and RESP (call directly, not in $(...))
  local method="$1" path="$2" body="${3:-}" out="$WORK/resp"
  if [[ -n "$body" ]]; then
    CODE=$(curl -sS -o "$out" -w '%{http_code}' -X "$method" -H "X-Doppel-Token: $TOKEN" \
      -H 'Content-Type: application/json' --data "$body" "$BASE$path")
  else
    CODE=$(curl -sS -o "$out" -w '%{http_code}' -X "$method" -H "X-Doppel-Token: $TOKEN" "$BASE$path")
  fi
  RESP="$(cat "$out")"
}

curl -fsS "$BASE/" | grep -q "$TOKEN" || fail "index.html does not carry the token"
pass "index.html templated"

api GET /api/settings

SETTINGS="$RESP"
[[ "$CODE" == 200 && "$(json behavior.private.waitForMoreSec <<<"$SETTINGS")" =~ ^[0-9]+$ ]] || fail "GET /api/settings ($CODE)"
[[ "$(json behavior.group.preset <<<"$SETTINGS")" == natural ]] || fail "default group preset is not natural"
[[ "$(json version <<<"$SETTINGS")" == 4 && "$(json recap.time <<<"$SETTINGS")" == 21:00 ]] || fail "settings are not v4"
pass "settings (v4)"

api GET /api/missions
[[ "$CODE" == 200 && "$RESP" == *'"achievements"'* ]] || fail "GET /api/missions ($CODE)"
api POST /api/system/notify-test # DOPPEL_NOTIFY=dry: logged, not shown
[[ "$CODE" == 200 && "$(json backend <<<"$RESP")" == dry-run ]] || fail "POST /api/system/notify-test ($CODE): $RESP"
pass "wave 3 endpoints answer"

api GET /api/behavior/presets
[[ "$CODE" == 200 && "$(json presets.0.id <<<"$RESP")" == natural && "$(json ranges.replyPercent.max <<<"$RESP")" == 100 ]] \
  || fail "GET /api/behavior/presets ($CODE)"
[[ "$(json dials.speed.levels.4.label <<<"$RESP")" == Instant && "$(json dials.boldness.levels.2.group.typoPercent <<<"$RESP")" == 5 ]] \
  || fail "GET /api/behavior/presets: vibe dials missing"
pass "behavior presets + vibe dials"

api POST /api/behavior/sample '{"kind":"group","samples":3}'
[[ "$CODE" == 200 && "$(json summary.maxTotalSec <<<"$RESP")" =~ ^[0-9.]+$ && "$(json samples.0.phases.0.name <<<"$RESP")" == notice ]] \
  || fail "POST /api/behavior/sample ($CODE): $RESP"
pass "behavior sample"

api GET /api/personas

PERSONAS="$RESP"
PERSONA_ID="$(json 0.id <<<"$PERSONAS")"
[[ "$CODE" == 200 && -n "$PERSONA_ID" ]] || fail "GET /api/personas ($CODE)"
pass "personas (first: $PERSONA_ID)"

api POST "/api/personas/$PERSONA_ID/expression-preview" '{"persona":{"name":"Smoke","emoji":{"usage":"none","favorites":[]},"messageLength":"short"},"count":2}'
[[ "$CODE" == 200 && -n "$(json samples.1.reply <<<"$RESP")" && "$(json samples.0.emoji <<<"$RESP")" == 0 ]] \
  || fail "POST /api/personas/{id}/expression-preview ($CODE): $RESP"
pass "expression preview"

api GET /api/chats
[[ "$CODE" == 200 ]] || fail "GET /api/chats ($CODE)"
pass "chats"

api GET '/api/wa/chats?tab=recent&limit=20'

WACHATS="$RESP"
JID="$(json items.0.jid <<<"$WACHATS")"
[[ "$CODE" == 200 && -n "$JID" ]] || fail "GET /api/wa/chats ($CODE)"
pass "wa chats (first: $JID)"

SSE="$(curl -sN --max-time 2 "$BASE/api/events" || true)"
grep -q '^event: wa.status' <<<"$SSE" || fail "SSE did not start with wa.status"
pass "SSE stream"

# Guards
CODE=$(curl -s -o /dev/null -w '%{http_code}' -X PUT -H 'Content-Type: application/json' --data '{}' "$BASE/api/settings")
[[ "$CODE" == 401 ]] || fail "missing token accepted ($CODE)"
CODE=$(curl -s -o /dev/null -w '%{http_code}' -X PUT -H "X-Doppel-Token: nope" -H 'Content-Type: application/json' --data '{}' "$BASE/api/settings")
[[ "$CODE" == 401 ]] || fail "wrong token accepted ($CODE)"
CODE=$(curl -s -o /dev/null -w '%{http_code}' -H "Host: evil.example:$PORT" "$BASE/api/settings")
[[ "$CODE" == 403 ]] || fail "foreign Host accepted ($CODE)"
CODE=$(curl -s -o /dev/null -w '%{http_code}' -H "Origin: http://evil.example" "$BASE/api/settings")
[[ "$CODE" == 403 ]] || fail "foreign Origin accepted ($CODE)"
pass "token / host / origin guards"

# Fast replies for the test (an Instant-like profile for both kinds).
FAST='{"noticeMinSec":0,"noticeMaxSec":0,"waitForMoreSec":1,"burstCapSec":3,"thinkMinSec":0,"thinkMaxSec":0,"distractedPercent":0,"typingMinSec":0,"typingMaxSec":1,"splitPercent":0,"replyPercent":100}'
api PUT /api/settings "{\"behavior\":{\"private\":$FAST,\"group\":$FAST}}"
[[ "$CODE" == 200 && "$(json behavior.private.waitForMoreSec <<<"$RESP")" == 1 && "$(json behavior.private.preset <<<"$RESP")" == custom ]] \
  || fail "PUT /api/settings ($CODE): $RESP"
api PUT /api/settings '{"behavior":{"private":{"replyPercent":101}}}'
[[ "$CODE" == 400 ]] || fail "out-of-range behavior accepted ($CODE)"
pass "settings update + validation"

api POST /api/chats "{\"jid\":\"$JID\",\"personaId\":\"$PERSONA_ID\",\"approvalMode\":false}"

CHAT="$RESP"
[[ "$CODE" == 201 || "$CODE" == 200 ]] || fail "assign chat ($CODE): $CHAT"
KEY="$(json key <<<"$CHAT")"
pass "assigned $KEY → $PERSONA_ID"
api POST /api/chats "{\"jid\":\"$JID\",\"personaId\":\"$PERSONA_ID\"}"
[[ "$CODE" == 409 ]] || fail "double assignment not rejected ($CODE)"
pass "double assignment rejected"

EKEY="${KEY//:/%3A}"
api PATCH "/api/chats/$EKEY" '{"behavior":{"lengthBias":"shorter"}}'
[[ "$CODE" == 200 && "$(json behavior.lengthBias <<<"$RESP")" == shorter ]] || fail "PATCH behavior ($CODE): $RESP"
api GET "/api/chats/$EKEY/behavior"
[[ "$CODE" == 200 && "$(json sources.lengthBias <<<"$RESP")" == chat && "$(json sources.replyPercent <<<"$RESP")" == default \
   && "$(json available <<<"$RESP")" == true ]] || fail "GET chat behavior ($CODE): $RESP"
api PATCH "/api/chats/$EKEY" '{"behavior":{"lengthBias":null}}'
[[ "$CODE" == 200 && "$(json behavior.lengthBias <<<"$RESP" || true)" == "" ]] || fail "PATCH behavior null ($CODE): $RESP"
pass "per-chat behavior override / inherit"

api PATCH "/api/chats/$EKEY" '{"people":{"people":[{"jid":"'"$JID"'","notes":"smoke test contact"}]}}'
[[ "$CODE" == 200 ]] || fail "PATCH people ($CODE): $RESP"
api GET "/api/chats/$EKEY/people"
[[ "$CODE" == 200 && "$(json members.0.notes <<<"$RESP")" == "smoke test contact" ]] || fail "GET people ($CODE): $RESP"
api PATCH "/api/chats/$EKEY" '{"people":{"mode":"sometimes"}}'
[[ "$CODE" == 400 ]] || fail "invalid people mode accepted ($CODE)"
pass "people notes + validation"

api POST /api/missions/start "{\"chatKey\":\"$KEY\",\"templateId\":\"say-word\",\"blanks\":{\"name\":\"them\",\"word\":\"banana\"}}"
[[ "$CODE" == 200 && "$(json missionId <<<"$RESP")" == say-word ]] || fail "POST /api/missions/start ($CODE): $RESP"
api GET "/api/chats/$EKEY/behavior"
[[ "$CODE" == 200 && "$(json dials.speed.level <<<"$RESP")" =~ ^[1-5]$ ]] || fail "chat behavior dials ($CODE)"
api POST "/api/missions/$EKEY/abandon"
[[ "$CODE" == 200 && "$(json missionId <<<"$RESP")" == "" ]] || fail "POST /api/missions/{key}/abandon ($CODE): $RESP"
pass "mission started and abandoned"

# Memory of people + Clone yourself (wave 3, realism).
api POST "/api/chats/$EKEY/memories" '{"text":"is training for the Tel Aviv marathon","person":"Smoke"}'
MEMID="$(json id <<<"$RESP")"
[[ "$CODE" == 200 && -n "$MEMID" && "$(json source <<<"$RESP")" == user ]] || fail "POST memories ($CODE): $RESP"
api PATCH "/api/chats/$EKEY/memories/$MEMID" '{"pinned":true}'
[[ "$CODE" == 200 && "$(json pinned <<<"$RESP")" == true ]] || fail "PATCH memory ($CODE): $RESP"
api GET "/api/chats/$EKEY/memories"
[[ "$CODE" == 200 && "$(json items.0.text <<<"$RESP")" == "is training for the Tel Aviv marathon" && "$(json enabled <<<"$RESP")" == true ]] \
  || fail "GET memories ($CODE): $RESP"
api POST "/api/chats/$EKEY/memories/extract"
[[ "$CODE" == 200 && "$(json added <<<"$RESP")" =~ ^[0-9]+$ ]] || fail "POST memories/extract ($CODE): $RESP"
api POST /api/clone/draft '{"name":"Me"}'
[[ "$CODE" == 400 && "$(json code <<<"$RESP")" == too_few_samples ]] || fail "clone draft without samples ($CODE): $RESP"
pass "memories CRUD + extract, clone guard"

api POST "/api/chats/$EKEY/simulate" '{"text":"hey, how was your weekend?"}'
[[ "$CODE" == 200 ]] || fail "simulate ($CODE)"
pass "simulated incoming message"

SENT=""
for _ in $(seq 1 60); do
  api GET '/api/activity?types=sent&limit=5'
  ACT="$RESP"
  SENT="$(json items.0.text <<<"$ACT" || true)"
  [[ -n "$SENT" ]] && break
  sleep 0.5
done
[[ -n "$SENT" ]] || fail "no 'sent' activity within 30 s"
pass "reply sent: \"$SENT\""
api GET '/api/activity?types=seen,waiting&limit=5'
[[ "$(json items.0.type <<<"$RESP")" == seen ]] || fail "pipeline did not narrate 'seen': $RESP"
pass "pipeline narrated (seen → waiting → sent)"

api GET "/api/chats/$EKEY/history?limit=10"

HIST="$RESP"
[[ "$CODE" == 200 && -n "$(json 0.text <<<"$HIST")" ]] || fail "history empty ($CODE)"
pass "history recorded"

# Hand-off: "are you a bot?" pauses the chat until you resume it.
api POST "/api/chats/$EKEY/simulate" '{"text":"wait, are you a bot?"}'
[[ "$CODE" == 200 ]] || fail "simulate hand-off ($CODE)"
for _ in $(seq 1 20); do
  api GET /api/chats
  [[ "$(json 0.handoff.category <<<"$RESP" || true)" == bot ]] && break
  sleep 0.25
done
[[ "$(json 0.handoff.category <<<"$RESP" || true)" == bot ]] || fail "hand-off did not pause the chat: $RESP"
api POST "/api/chats/$EKEY/handoff/resume"
[[ "$CODE" == 200 && -z "$(json handoff.category <<<"$RESP" || true)" ]] || fail "resume hand-off ($CODE): $RESP"
pass "hand-off paused and resumed"

# Co-pilot: three drafts wait in Approvals, never auto-sent.
api PATCH "/api/chats/$EKEY" '{"mode":"copilot"}'
[[ "$CODE" == 200 && "$(json mode <<<"$RESP")" == copilot ]] || fail "PATCH mode copilot ($CODE): $RESP"
api POST "/api/chats/$EKEY/simulate" '{"text":"dinner tonight?"}'
for _ in $(seq 1 40); do
  api GET /api/approvals
  [[ -n "$(json 0.drafts.2.text <<<"$RESP" || true)" ]] && break
  sleep 0.25
done
[[ -n "$(json 0.drafts.2.text <<<"$RESP" || true)" && -z "$(json 0.autoSendAt <<<"$RESP" || true)" ]] || fail "co-pilot drafts: $RESP"
pass "co-pilot: 3 drafts (\"$(json 0.drafts.1.text <<<"$RESP")\")"

api POST /api/recaps/generate "{\"chatKey\":\"$KEY\"}"
[[ "$CODE" == 200 && -n "$(json items.0.headline <<<"$RESP")" ]] || fail "recap ($CODE): $RESP"
pass "recap on demand"

api POST "/api/chats/$EKEY/reveal" '{}'
[[ "$CODE" == 200 && -n "$(json text <<<"$RESP")" ]] || fail "reveal ($CODE): $RESP"
api GET /api/chats
[[ "$(json 0.enabled <<<"$RESP")" == false && -n "$(json 0.revealedAt <<<"$RESP")" ]] || fail "reveal did not pause the chat: $RESP"
api POST "/api/chats/$EKEY/reveal" '{}'
[[ "$CODE" == 409 ]] || fail "second reveal not rejected ($CODE)"
pass "reveal sent and chat paused"

api POST /api/system/quit
[[ "$CODE" == 200 ]] || fail "quit ($CODE)"
for _ in $(seq 1 50); do
  kill -0 "$PID" 2>/dev/null || break
  sleep 0.2
done
if kill -0 "$PID" 2>/dev/null; then fail "server still running 10 s after quit"; fi
[[ ! -f "$DATA/instance.json" ]] || fail "instance.json left behind"
PID=""
pass "quit (instance.json removed)"

printf '\n\033[1;32mSmoke test passed.\033[0m\n'
