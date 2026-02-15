# WhatsApp Persona Bot

A privacy-focused WhatsApp automation bot powered by **Go** and **Ollama**. Adopts a specific persona to reply to messages with automatic contact management and anti-jailbreak protection.

## ⚡ Prerequisites

* **Go** (1.25+)
* **Ollama** (running locally with `llama3`)
* **WhatsApp Mobile App** (to scan QR code)

## 🛠️ Setup

1.  **Clone & Install**
    ```bash
    git clone <your-repo-url>
    cd whatsapp_doppel_go
    go mod tidy
    ```

2.  **Start Ollama**
    ```bash
    ollama pull llama3:latest
    ollama serve
    ```

3.  **Configure Target**
    Create/edit `.env` file:
    ```bash
    TARGET_PHONE=972 54-637-1966
    ```
    *(Any phone format works - spaces, dashes, + symbol all auto-sanitized)*

4.  **Export Contacts**
    ```bash
    go run export_contacts.go
    ```
    Creates `whatsapp_contacts.json` with all your WhatsApp contacts and their JIDs/LIDs.

## 🚀 Run

```bash
go run bot.go
```

On first run, scan the QR code with WhatsApp. The bot automatically:
- Loads target from `.env`
- Finds contact in `whatsapp_contacts.json`
- Resolves LID if missing
- Starts responding

## 🎭 Persona System

Edit the `IDENTITY` constant in `bot.go` to change personas. Security rules are separate in `ANTI_JAILBREAK_RULES` - no need to copy them.

**Current persona:** Chad "The Shred" Remington (Gym trainer)

See `PERSONA_GUIDE.md` for templates and examples.

## 🛡️ Security Features

- **5-layer anti-jailbreak protection** blocks prompt injection attempts
- Aggressive content filtering removes dangerous phrases
- Injection attempts silently ignored (not added to conversation history)
- Character lock prevents persona manipulation

See `SECURITY_DEFENSES.md` for details.

## 📁 Key Files

| File | Purpose |
|------|---------|
| `bot.go` | Main bot code |
| `export_contacts.go` | Contact/LID exporter |
| `.env` | Target phone configuration |
| `whatsapp_contacts.json` | Auto-generated contact database |
| `bot.db` | WhatsApp session data |
| `persona.go` | Persona template |

## 🔧 Switching Targets

Just update `.env` and restart:
```bash
TARGET_PHONE=1-555-123-4567
```

## 🎯 Testing Mode

Send messages with prefix `"1"` to test without waiting for target:
```
1 Hey what's up?
```
Bot responds immediately as if target sent the message (without prefix).

## 📝 Notes

- Contact exports may take 2-5 minutes for LID resolution
- LIDs auto-update on first message if not in export
- Injection attempts are logged but silently ignored
- Clean, minimal codebase - no unnecessary dependencies
