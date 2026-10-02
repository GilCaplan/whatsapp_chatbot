# Contributing

Thanks for wanting to help. This is a personal, non-commercial project
(see [LICENSE](LICENSE) — PolyForm Noncommercial 1.0.0); contributions are accepted under the same terms.

## Getting set up

```bash
git clone https://github.com/GilCaplan/whatsapp_chatbot.git
cd whatsapp_chatbot
make dev-fake      # runs the app with a simulated WhatsApp and a scripted AI — no phone or model needed
```

Requirements: macOS, Go (version in `go.mod`), Xcode command-line tools (for cgo SQLite),
optionally [Ollama](https://ollama.com) for real AI replies.

## Before opening a pull request

```bash
make vet test smoke
find web -name '*.js' -print0 | xargs -0 -n1 node --check
```

All four must pass (CI runs the same).

## Ground rules

- Read [CLAUDE.md](CLAUDE.md) for the architecture, package boundaries and conventions, and
  [docs/API.md](docs/API.md) for the HTTP contract — update it when you change an endpoint.
- **No emoji in the app's UI or activity texts.** Use the inline SVG icons (`web/icons.js`) or
  custom graphics (`web/components/glyphs.js`). (What a persona types in WhatsApp is its own business.)
- The web UI has no build step: vanilla ES modules, no npm, no CDN. Escape every piece of text that
  comes from WhatsApp or the user (the `html` template helper does this by default).
- Write user-facing text in plain English for non-technical people.
- Never commit anything from the data folder (`*.db`, `secrets.json`, history) — `.gitignore` is a
  whitelist, so new top-level files must be added to it explicitly.
- Add or update tests with every behaviour change; engine timing tests use the fake clock and seeded randomness.

## Reporting bugs and ideas

Use the issue templates. For bugs, include the relevant lines from the **Activity** page and
`~/Library/Application Support/WhatsappDoppel/logs/server.log` — but remove names, numbers and message text first.
