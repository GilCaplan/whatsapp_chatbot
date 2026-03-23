# WhatsApp Persona Bot

A Go-based WhatsApp bot that impersonates a custom persona using a local LLM ([Ollama](https://ollama.com)). Responds to messages in private chats or group chats, with natural group behaviour (not every message, skips messages aimed at others).

---

## Prerequisites

- [Go 1.21+](https://golang.org/dl/)
- [Ollama](https://ollama.com) running locally
- WhatsApp mobile app (to scan QR on first run)

---

## Setup

### 1. Clone & install dependencies

```bash
git clone https://github.com/GilCaplan/whatsapp_chatbot.git
cd whatsapp_chatbot
go mod tidy
```

### 2. Pull a model into Ollama

```bash
ollama pull llama3.2:latest
```

Check available models anytime with `ollama list`.

### 3. Configure `.env`

Copy the example and fill in your values:

```bash
cp .env.example .env
```

**.env for a private chat:**
```env
TARGET_TYPE=individual
DB_PATH=bot.db
TARGET_PHONE=15551234567
```

**.env for a group chat:**
```env
TARGET_TYPE=group
DB_PATH=bot.db
TARGET_GROUP_NAME=My Group Chat
# Or use the JID directly for faster startup:
# TARGET_GROUP_JID=123456789-987654321@g.us
```

> Phone numbers: any format works — spaces, dashes, and `+` are stripped automatically.

### 4. Customize the persona

Edit the `IDENTITY` constant in `bot.go`. Define the character's name, background, communication style, and tone. Update `PERSONA_NAME` to match.

---

## Run

```bash
go run bot.go
```

On first run a QR code appears in the terminal — scan it with WhatsApp. The session is saved to `DB_PATH` so you won't need to scan again.

---

## Running Two Instances (private + group simultaneously)

Each instance needs its own `.env` and database file. The simplest way is two separate directories:

```bash
mkdir instance_group instance_private

# copy source into each
cp bot.go go.mod go.sum instance_group/
cp bot.go go.mod go.sum instance_private/

# create .env for each
echo "TARGET_TYPE=group
DB_PATH=bot.db
TARGET_GROUP_NAME=My Group Chat" > instance_group/.env

echo "TARGET_TYPE=individual
DB_PATH=bot.db
TARGET_PHONE=15551234567" > instance_private/.env

# run in separate terminals
cd instance_group && go run bot.go
cd instance_private && go run bot.go
```

Each instance will show its own QR code on first run.

---

## Testing

Send a message **from your own WhatsApp** in the target chat prefixed with `1`:

```
1 hey what's up?
```

The bot treats it as if the other person sent `"hey what's up?"` and replies immediately. The `1` prefix is stripped before being passed to the LLM.

---

## Group Chat Behaviour

In group mode the bot doesn't reply to every message — it behaves more like a real participant:

| Situation | Action |
|-----------|--------|
| Message contains the persona's name | Always replies |
| Message @mentions someone else | Skips |
| General message | LLM decides if it's worth jumping in (with 40% fallback) |
| You send `1 [message]` | Always replies immediately |

---

## Key Files

| File | Purpose |
|------|---------|
| `bot.go` | All bot logic |
| `export_contacts.go` | Export WhatsApp contacts + LIDs to JSON |
| `.env` | Your configuration (never committed) |
| `.env.example` | Template to copy from |
| `bot.db` | WhatsApp session (auto-created, never committed) |
| `persona.go` | Blank persona template |

---

## Security

- Anti-jailbreak rules prevent persona manipulation
- Prompt injection attempts are silently ignored and not added to history
- `.env` and `bot.db` are gitignored — credentials never leave your machine
