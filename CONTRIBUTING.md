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
make vet test test-py smoke
find web -name '*.js' -print0 | xargs -0 -n1 node --check
```

All four must pass. CI runs the same on Linux, macOS and Windows, cross-compiles every release
target and runs each OS's installer end to end. If you touch anything OS-specific, keep it in
`internal/host/platform` (one file per OS) and run `GOOS=windows go vet ./...` and `GOOS=linux go vet ./...`.

## Ground rules

- Read [CLAUDE.md](CLAUDE.md) for the architecture, package boundaries and conventions, and
  [docs/API.md](docs/API.md) for the HTTP contract — update it when you change an endpoint.
- **No emoji in the app's UI or activity texts.** Use the inline SVG icons (`web/icons.js`) or
  custom graphics (`web/components/art/glyphs.js`). (What a persona types in WhatsApp is its own business.)
- The web UI has no build step: vanilla ES modules, no npm, no CDN. Escape every piece of text that
  comes from WhatsApp or the user (the `html` template helper does this by default).
- Write user-facing text in plain English for non-technical people.
- Never commit anything from the data folder (`*.db`, `secrets.json`, history) — `.gitignore` is a
  whitelist, so new top-level files must be added to it explicitly.
- Add or update tests with every behaviour change; engine timing tests use the fake clock and seeded randomness.

## Keeping personal data out

[docs/SAFE_HABITS.md](docs/SAFE_HABITS.md) lists the mistakes to avoid, including everything below.

This repository is public, and anything committed stays in its history. Your WhatsApp login, chats and
keys live in the data folder, outside the project, so the normal setup can't commit them. `make dev-fake`
and `make run` use `./data`, which is ignored.

After cloning, run `make hooks` once. Every commit is then scanned for session and data files, API keys,
personal email addresses, home-folder paths and real-looking phone numbers, and CI runs the same check.
Use the 1555 range for example numbers.

- To also block your own details, list them one per line in `.git/info/privacy-words` (your number, name,
  group names). That file can't be committed.
- Your commits show your git email. To keep it private use GitHub's noreply address:
  `git config user.email ID+USERNAME@users.noreply.github.com`.
- Never paste real chats, numbers or screenshots of them into issues, PRs or test data.
- Don't bypass the check with `--no-verify`. If it stops a commit wrongly, ask in the PR.

## Reporting bugs and ideas

Use the issue templates. For bugs, include your OS, the relevant lines from the **Activity** page and
`logs/server.log` from the data folder (`~/Library/Application Support/WhatsappDoppel` on a Mac,
`%APPDATA%\WhatsappDoppel` on Windows, `~/.config/WhatsappDoppel` on Linux) — but remove names,
numbers and message text first.
