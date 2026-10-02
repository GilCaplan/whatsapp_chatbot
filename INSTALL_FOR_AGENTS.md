# Installing WhatsApp Doppel: runbook for AI coding agents

This file is for coding agents (Claude Code, Codex, Cursor, Copilot agents and similar) asked
to "install WhatsApp Doppel" on a human's computer. Follow the steps in order and run the
commands exactly as written. Humans: the friendly version is the **Install** section of
[README.md](README.md).

Repository: `https://github.com/GilCaplan/whatsapp_chatbot` · Releases:
`https://github.com/GilCaplan/whatsapp_chatbot/releases`

## Goal and success criteria

You are done when all of these hold:

1. The app is installed for the current user with this project's own installer:
   - macOS: `/Applications/WhatsappDoppel.app` (or `~/Applications/WhatsappDoppel.app`)
   - Windows: `%LOCALAPPDATA%\Programs\WhatsappDoppel\WhatsappDoppel.exe`, plus a Start menu shortcut
   - Linux: `~/.local/bin/whatsapp-doppel`, plus `~/.local/share/applications/whatsapp-doppel.desktop`
2. The `status` command (step 5) says `WhatsApp Doppel <version> is running at http://127.0.0.1:<port>/`.
3. `GET http://127.0.0.1:<port>/api/health` returns JSON with `"ok": true`.
4. You told the human how to finish (step 6): open the app and scan the WhatsApp QR code.
   **An agent cannot do that part.**

## Safety rules (read first)

- **Ask the human before installing any system package** (Go, git, Ollama, notify-send, Homebrew,
  winget/apt/dnf/pacman packages) or running anything with `sudo`/admin rights. Propose the exact
  command; run it only after a yes. The project's installers never need `sudo` or admin.
- **Never delete, move or overwrite the data folder** (it holds the human's WhatsApp link,
  personas and settings), and never pass `--purge` / `-Purge` to an uninstaller unless the human
  explicitly asks for their data to be deleted:
  - macOS: `~/Library/Application Support/WhatsappDoppel`
  - Windows: `%APPDATA%\WhatsappDoppel`
  - Linux: `~/.config/WhatsappDoppel` (`$XDG_CONFIG_HOME/WhatsappDoppel` if that variable is set)
  - If `DOPPEL_DATA_DIR` is set in the environment, that folder is the data folder instead.
- **Never print or copy secrets or session files**: do not `cat`/upload `secrets.json`,
  `whatsapp.db*`, `config.json`, `instance.json` (it holds an access token), `.env`, contact lists,
  `history/` or `memories/`. Do not print the full `/api/health` response either (it contains the
  same token); extract only the fields named below. If you must show logs, show only the last
  few relevant lines of `logs/server.log`.
- **Never run two servers on the same data folder**, and never start a second copy of the app
  against a copy of someone's `whatsapp.db`: two connections with one WhatsApp session knock each
  other off. For experiments use `--fake-wa --fake-llm --data-dir <a new empty folder>`.
- **Stop an existing instance before reinstalling**, with the app's own `quit` command (step 4.0).
  The installers also do this; never kill the process unless `quit` failed and the human agrees.
- Do not create git tags or releases, push code, or change the repository unless asked.

## Step 1: detect the system

macOS / Linux (bash or zsh):

```bash
uname -s     # Darwin = macOS, Linux = Linux
uname -m     # x86_64 → amd64; arm64 or aarch64 → arm64
grep -qi microsoft /proc/version 2>/dev/null && echo "WSL"   # Linux only
```

Windows (PowerShell):

```powershell
$env:PROCESSOR_ARCHITECTURE                 # AMD64 → amd64; ARM64 → arm64
[Environment]::OSVersion.Version            # 10.0.x = Windows 10 or 11 (supported)
$PSVersionTable.PSVersion                   # Windows PowerShell 5.1 or newer
```

If the output says **WSL**, stop and tell the human: *"Inside WSL the app has no browser or
notifications to use. Please let me install the Windows version from a normal PowerShell window
instead."* Then continue with the Windows commands in Windows PowerShell.

Supported: macOS 13+ (Apple silicon and Intel), Windows 10/11 (amd64, arm64), 64-bit Linux
(amd64, arm64) with a desktop.

## Step 2: check prerequisites

There are two install paths. **Path A (from source)** needs git and Go. **Path B (release
download)** needs nothing beyond `curl`/`tar` (macOS, Linux) or PowerShell (Windows), but only
works once a release has been published. Prefer A when Go is present or the human agrees to
install it; otherwise try B.

| Check | Command | Expected |
|---|---|---|
| git (path A) | `git --version` | `git version 2.x…` |
| Go (path A) | `go version` | `go version go1.N… <os>/<arch>` with N ≥ 21 |
| Go toolchain policy | `go env GOTOOLCHAIN` | anything except `local` (`auto` is the default) |
| Node.js (optional, dev checks only) | `node --version` | `v18` or newer |
| Ollama (optional) | `ollama --version` then `ollama list` | a version; a table with `NAME ID SIZE MODIFIED` |
| curl (path B, macOS/Linux) | `curl --version` | `curl 7.x` or `8.x` |

About the Go version: `go.mod` needs Go `1.26.0` (read it with `awk '/^go /{print $2}' go.mod`
after step 3). Any Go **1.21 or newer** works because Go downloads the required toolchain by itself
(it prints `go: downloading go1.26.0 …`; needs network). Only if `go env GOTOOLCHAIN` prints
`local` must the installed Go be at least the `go.mod` version.

If something is missing, tell the human what it is for and **propose** the command (do not run it
without a yes):

| Missing | macOS | Windows (PowerShell) | Linux |
|---|---|---|---|
| git | `xcode-select --install` | `winget install --id Git.Git -e` | Ubuntu/Debian `sudo apt install git` · Fedora `sudo dnf install git` · Arch `sudo pacman -S git` |
| Go | `brew install go`, or the `.pkg` from https://go.dev/dl/ | `winget install --id GoLang.Go -e`, or the `.msi` from https://go.dev/dl/ | `sudo snap install go --classic` · Fedora `sudo dnf install golang` · Arch `sudo pacman -S go` · or https://go.dev/doc/install |
| Ollama (free local AI) | installer from https://ollama.com/download | installer from https://ollama.com/download | `curl -fsSL https://ollama.com/install.sh \| sh` (uses sudo) |
| notify-send (Linux notifications) | – | – | Ubuntu/Debian `sudo apt install libnotify-bin` · Fedora `sudo dnf install libnotify` · Arch `sudo pacman -S libnotify` |

After installing Go or git on Windows, open a **new** PowerShell window (PATH changes only apply
to new windows). Ollama is optional: the human can use Claude or OpenAI with an API key instead,
chosen in the app. If they want Ollama, a good first model is `ollama pull llama3.1:8b`
(about 5 GB; ask first, it is a large download).

## Step 3: get the code (path A)

```bash
git clone https://github.com/GilCaplan/whatsapp_chatbot.git whatsapp-doppel
cd whatsapp-doppel
```

Already have a checkout? `cd` into it and run `git pull`. Check that the installers exist:

```bash
ls scripts/install-from-source.sh scripts/install-from-source.ps1 scripts/windows/install.ps1 scripts/linux/install.sh
```

If they are missing, the default branch does not contain the app yet: ask the human which branch
to use (for example `git checkout feature/doppel-app`).

## Step 4: install

### 4.0 Stop a running copy (if any)

macOS:

```bash
for a in /Applications/WhatsappDoppel.app "$HOME/Applications/WhatsappDoppel.app"; do
  [ -x "$a/Contents/MacOS/WhatsappDoppel" ] && "$a/Contents/MacOS/WhatsappDoppel" quit
done; true
```

Linux:

```bash
[ -x "$HOME/.local/bin/whatsapp-doppel" ] && "$HOME/.local/bin/whatsapp-doppel" quit; true
```

Windows (PowerShell; the exe is a windowed app, so pipe its output to make PowerShell wait):

```powershell
$exe = "$env:LOCALAPPDATA\Programs\WhatsappDoppel\WhatsappDoppel.exe"
if (Test-Path $exe) { & $exe quit | Out-Host }
```

`quit` prints `WhatsApp Doppel stopped` or `WhatsApp Doppel is not running`; both are fine.

### 4A. From source (recommended for agents)

Run from the repository folder. `--no-launch` / `-NoLaunch` keeps the installer from asking
questions or opening a browser.

macOS (builds the .app, installs it to /Applications, adds a Desktop alias; same as `make install`):

```bash
scripts/install-from-source.sh --no-launch
```

The Desktop alias is made by Finder; macOS may show the human a "Terminal wants to control
Finder" prompt. If nobody is at the screen, prefix the command with `NO_DESKTOP_ALIAS=1` to skip
the alias.

Linux (installs to ~/.local/bin, app menu entry, icon, Desktop shortcut; no sudo):

```bash
scripts/install-from-source.sh --no-launch
```

Windows (PowerShell; builds a windowed exe with the app icon, installs it with Start menu and
Desktop shortcuts and a Settings > Apps entry; no admin):

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\install-from-source.ps1 -NoLaunch
```

Expected last line: `Installed. Open WhatsApp Doppel from …`. The first build takes 1–3 minutes.

### 4B. From a release download

First find the latest release tag. If this prints nothing or fails with 404, no release exists
yet: use path A.

macOS / Linux:

```bash
VER="$(curl -fsSL https://api.github.com/repos/GilCaplan/whatsapp_chatbot/releases/latest | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p')"
echo "$VER"
```

macOS (files downloaded with curl are not quarantined, so Gatekeeper does not prompt):

```bash
TMP="$(mktemp -d)"
curl -fL -o "$TMP/doppel.zip" "https://github.com/GilCaplan/whatsapp_chatbot/releases/download/$VER/WhatsappDoppel-$VER-macos-universal.zip"
ditto -x -k "$TMP/doppel.zip" "$TMP"
rm -rf /Applications/WhatsappDoppel.app
ditto "$TMP/WhatsappDoppel.app" /Applications/WhatsappDoppel.app
xattr -dr com.apple.quarantine /Applications/WhatsappDoppel.app 2>/dev/null; true
```

Linux (set `ARCH` from step 1: `amd64` or `arm64`):

```bash
ARCH=amd64
TMP="$(mktemp -d)"
curl -fL -o "$TMP/doppel.tar.gz" "https://github.com/GilCaplan/whatsapp_chatbot/releases/download/$VER/whatsapp-doppel-$VER-linux-$ARCH.tar.gz"
tar xzf "$TMP/doppel.tar.gz" -C "$TMP"
"$TMP/whatsapp-doppel-$VER-linux-$ARCH/install.sh" --no-launch
```

Windows (PowerShell):

```powershell
$ver = (Invoke-RestMethod https://api.github.com/repos/GilCaplan/whatsapp_chatbot/releases/latest).tag_name
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$name = "WhatsappDoppel-$ver-windows-$arch"
$tmp = Join-Path $env:TEMP 'doppel-install'
Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Path $tmp | Out-Null
Invoke-WebRequest -UseBasicParsing -Uri "https://github.com/GilCaplan/whatsapp_chatbot/releases/download/$ver/$name.zip" -OutFile "$tmp\$name.zip"
Expand-Archive -Path "$tmp\$name.zip" -DestinationPath $tmp
powershell -NoProfile -ExecutionPolicy Bypass -File "$tmp\$name\install.ps1" -NoLaunch
```

Optional integrity check: download `SHA256SUMS.txt` from the same release and compare with
`shasum -a 256 <file>` (macOS), `sha256sum <file>` (Linux) or `Get-FileHash <file>` (Windows).

## Step 5: verify

Start the app in the background without opening a browser (`DOPPEL_NO_BROWSER=1` also prevents
error dialogs), then check it. Starting it is safe: if it is already running, `launch` reuses the
running copy instead of starting a second one.

macOS:

```bash
APP=/Applications/WhatsappDoppel.app/Contents/MacOS/WhatsappDoppel
[ -x "$APP" ] || APP="$HOME/Applications/WhatsappDoppel.app/Contents/MacOS/WhatsappDoppel"
DOPPEL_NO_BROWSER=1 "$APP" launch
"$APP" status
DATA="${DOPPEL_DATA_DIR:-$HOME/Library/Application Support/WhatsappDoppel}"
PORT="$(sed -n 's/^ *"port": *\([0-9][0-9]*\).*/\1/p' "$DATA/instance.json")"
curl -fsS "http://127.0.0.1:$PORT/api/health" | grep -o '"ok":true'
```

Linux:

```bash
APP="$HOME/.local/bin/whatsapp-doppel"
DOPPEL_NO_BROWSER=1 "$APP" launch
"$APP" status
DATA="${DOPPEL_DATA_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/WhatsappDoppel}"
PORT="$(sed -n 's/^ *"port": *\([0-9][0-9]*\).*/\1/p' "$DATA/instance.json")"
curl -fsS "http://127.0.0.1:$PORT/api/health" | grep -o '"ok":true'
```

Windows (PowerShell):

```powershell
$exe = "$env:LOCALAPPDATA\Programs\WhatsappDoppel\WhatsappDoppel.exe"
$env:DOPPEL_NO_BROWSER = '1'; & $exe launch | Out-Host; Remove-Item Env:\DOPPEL_NO_BROWSER
& $exe status | Out-Host
$data = if ($env:DOPPEL_DATA_DIR) { $env:DOPPEL_DATA_DIR } else { Join-Path $env:APPDATA 'WhatsappDoppel' }
$port = (Get-Content (Join-Path $data 'instance.json') -Raw | ConvertFrom-Json).port
(Invoke-RestMethod "http://127.0.0.1:$port/api/health").ok
```

Expected: `status` prints `WhatsApp Doppel <version> is running at http://127.0.0.1:<port>/` and
the health check prints `"ok":true` (macOS/Linux) or `True` (Windows). The port is usually 7788
but the app moves to another free port when 7788 is taken, so always read it from
`instance.json` as shown.

Optional developer checks from the repository (they use a throwaway fake server and never touch
the human's data): `make test` and `make smoke` on macOS/Linux (`make smoke` needs `jq`, or `plutil`
on macOS); on Windows `powershell -NoProfile -ExecutionPolicy Bypass -File scripts\dev.ps1 test`.

## Step 6: hand off to the human

Tell the human, in plain words:

1. **Open WhatsApp Doppel**: macOS: Launchpad, Spotlight or the Desktop icon; Windows: Start menu
   or Desktop; Linux: app menu or Desktop. The browser opens the app (it is already running, so
   this is instant).
2. **Choose an AI brain** on the setup screen: Ollama (free, on this computer) or a Claude/OpenAI
   API key.
3. **Link WhatsApp**: on the phone, WhatsApp → Settings → Linked devices → Link a device, and scan
   the QR code shown in the app. You (the agent) cannot do this step.
4. Try it safely first: assign a persona to "You (message yourself)" in **Approve** mode and send
   `1 hi` from the phone.

If they installed a **downloaded** release by double-clicking (not via the commands above), mention
the one-time warning: macOS → System Settings → Privacy & Security → **Open Anyway**; Windows
SmartScreen → **More info** → **Run anyway**.

## Troubleshooting

| Symptom | Cause | What to do |
|---|---|---|
| Health check fails on port 7788 | 7788 was busy, the app chose another port | Read the port from `instance.json` (step 5). The human can change it in Settings → Server. |
| `another WhatsApp Doppel server is already running for this data folder` | A copy is already running | Use `status` to find it; `quit` before reinstalling. Never start a second one. |
| `WhatsApp Doppel is not running` right after `launch` | The server failed to start | Show the last ~20 lines of `<data>/logs/server.log` (not the whole file) and fix what it says. |
| Windows: "Windows protected your PC" | SmartScreen; the exe is not code-signed | **More info** → **Run anyway** (the human clicks). The installers clear the download mark on the installed exe. |
| Windows: "running scripts is disabled on this system" | PowerShell execution policy | Use the `powershell -NoProfile -ExecutionPolicy Bypass -File …` form shown above; do not change the machine policy. |
| Windows: `status`/`version` print nothing | Windowed exe; PowerShell did not wait | Pipe the output: `& $exe status \| Out-Host`. |
| macOS: "cannot be opened" / "Apple could not verify…" | Gatekeeper, app downloaded in a browser | System Settings → Privacy & Security → **Open Anyway**, or `xattr -dr com.apple.quarantine /Applications/WhatsappDoppel.app`. |
| Linux: no notifications | `notify-send` is missing | Propose `sudo apt install libnotify-bin` (or the dnf/pacman equivalent), then `quit` and open the app again. |
| Linux: Desktop icon "untrusted" / does nothing | GNOME launcher trust | Right-click the icon → **Allow Launching**, or use the app menu entry. |
| Linux: browser does not open | `xdg-open` missing | Propose installing `xdg-utils`, or open `http://127.0.0.1:<port>/` by hand. |
| Ollama not found in the app / `ollama list` fails | Ollama not installed or not running | macOS/Windows: open the Ollama app; Linux: `ollama serve` in a terminal, or (ask first) `sudo systemctl start ollama`. Then `ollama pull llama3.1:8b` (ask first). |
| WhatsApp state "outdated" / "client outdated" | WhatsApp requires a newer protocol library | Update: `git pull`, then reinstall (step 4). If it persists, the maintainers must update whatsmeow; tell the human. Do not edit `go.mod` yourself. |
| WhatsApp state "replaced" | Another program uses the same WhatsApp link | Close the other program (an old bot, a second copy); then press **Reconnect** in the app. |
| `go: downloading go1.26.0` hangs or fails | No network for the toolchain download | Ask the human to install Go ≥ the `go.mod` version, or retry with network. |
| Inside WSL | Linux build has no browser/notifications there | Install the Windows build from Windows PowerShell. |

## Uninstall

Only when the human asks. These keep the data folder; add `--purge` / `-Purge` **only** if the
human explicitly wants their data deleted too.

```bash
# macOS (from the repository folder)
scripts/uninstall_app.sh --keep-data

# Linux
"$HOME/.local/share/whatsapp-doppel/uninstall.sh" --keep-data
```

```powershell
# Windows (or Settings > Apps > WhatsApp Doppel > Uninstall)
powershell -NoProfile -ExecutionPolicy Bypass -File "$env:LOCALAPPDATA\Programs\WhatsappDoppel\uninstall.ps1" -KeepData
```
