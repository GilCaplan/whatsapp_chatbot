# Security & privacy

WhatsApp Doppel runs entirely on your Mac. This page explains what it stores, who can reach it,
and how to report a problem.

## What stays on your Mac

Everything lives in `~/Library/Application Support/WhatsappDoppel/`:

| File / folder | Contains |
|---|---|
| `whatsapp.db` | Your linked-device WhatsApp session (like WhatsApp Web's login). Anyone with this file can act as that linked device. |
| `secrets.json` | Claude / OpenAI API keys (file mode `0600`, never sent back to the browser in full). |
| `config.json`, `personas.json`, `chats.json` | Your settings, personas and which chats they answer. |
| `history/` | The text of conversations in assigned chats, used as context for replies. |
| `runtime.json`, `approvals.json`, `logs/` | Reply timing state, replies waiting for approval, activity logs. |

To wipe everything: quit the app, unlink "WhatsApp Doppel" under WhatsApp → Settings → Linked devices,
then delete that folder (`scripts/uninstall_app.sh --purge` does the local part).

## What leaves your Mac

- **WhatsApp traffic**: the app is a normal linked device talking to WhatsApp's servers.
- **AI provider**: with **Ollama** (the default) nothing leaves your Mac. If you choose **Claude** or
  **OpenAI**, the persona prompt and recent messages of the chat being answered are sent to that
  provider under your own API key and their terms.
- Nothing else: no analytics, no telemetry, no update checks.

## The local web server

- Listens on `127.0.0.1` only (not reachable from other devices on your network).
- Every request that changes something needs a per-launch random token embedded in the page.
- `Host`, `Origin` and `Sec-Fetch-Site` checks block other websites from driving it (CSRF / DNS rebinding).

## Reporting a vulnerability

Please open a private security advisory on GitHub
(**Security → Report a vulnerability** on the repository page) rather than a public issue,
and include steps to reproduce. You should get a reply within a week.
