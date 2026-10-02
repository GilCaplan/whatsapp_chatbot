# Contributing

Thanks for wanting to help. This is a personal, non-commercial project
(see [LICENSE](LICENSE) — PolyForm Noncommercial 1.0.0); contributions are accepted under the same terms.

## Getting set up

```bash
git clone https://github.com/GilCaplan/whatsapp_chatbot.git
cd whatsapp_chatbot
make dev-fake      # runs the app with a simulated WhatsApp and a scripted AI — no phone or model needed
```

Requirements: macOS, Linux or Windows with Go (version in `go.mod`; Go 1.21+ fetches it). No C
compiler is needed (SQLite is pure Go); `go test -race` alone needs cgo. Optionally
[Ollama](https://ollama.com) for real AI replies, `jq` for `make smoke` and Node.js for `node --check`.
On Windows use `scripts\dev.ps1` (`build`, `dev-fake`, `test`, `vet`, …) instead of `make`.

## Before opening a pull request

```bash
make vet test smoke
find web -name '*.js' -print0 | xargs -0 -n1 node --check
```

All four must pass. CI runs the same on Linux, macOS and Windows, cross-compiles every release
target and runs each OS's installer end to end. If you touch anything OS-specific, keep it in
`internal/platform` (one file per OS) and run `GOOS=windows go vet ./...` and `GOOS=linux go vet ./...`.

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

Use the issue templates. For bugs, include your OS, the relevant lines from the **Activity** page and
`logs/server.log` from the data folder (`~/Library/Application Support/WhatsappDoppel` on a Mac,
`%APPDATA%\WhatsappDoppel` on Windows, `~/.config/WhatsappDoppel` on Linux) — but remove names,
numbers and message text first.
