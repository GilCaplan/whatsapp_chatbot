#!/usr/bin/env bash
# Builds and installs WhatsApp Doppel:
#   • /Applications/WhatsappDoppel.app   (falls back to ~/Applications)
#   • a "WhatsApp Doppel" alias on the Desktop
#   • <project>/WhatsappDoppel.app  → symlink to the installed app
# Safe to run again: every step replaces what a previous run created.
# Env: SKIP_BUILD=1 (use the existing build/WhatsappDoppel.app), NO_DESKTOP_ALIAS=1
#      (skip the Finder alias, e.g. on a headless CI machine).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="WhatsappDoppel"
ALIAS_NAME="WhatsApp Doppel"
SRC="$ROOT/build/$APP_NAME.app"

step() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarning:\033[0m %s\n' "$*" >&2; }

if [[ "${SKIP_BUILD:-0}" != "1" ]]; then
  "$ROOT/scripts/build_app.sh"
fi
[[ -d "$SRC" ]] || { echo "build output missing: $SRC" >&2; exit 1; }

# Stop a running server so the binary can be replaced cleanly (the browser tab
# reconnects once the new version starts).
for existing in "/Applications/$APP_NAME.app" "$HOME/Applications/$APP_NAME.app"; do
  if [[ -x "$existing/Contents/MacOS/$APP_NAME" ]]; then
    "$existing/Contents/MacOS/$APP_NAME" quit >/dev/null 2>&1 || true
  fi
done

DEST_DIR="/Applications"
if [[ ! -w "$DEST_DIR" ]]; then
  DEST_DIR="$HOME/Applications"
  mkdir -p "$DEST_DIR"
  warn "/Applications is not writable; installing to $DEST_DIR"
fi
DEST="$DEST_DIR/$APP_NAME.app"

step "Installing to $DEST"
if ! rm -rf "$DEST" 2>/dev/null || ! ditto "$SRC" "$DEST" 2>/dev/null; then
  if [[ "$DEST_DIR" == "/Applications" ]]; then
    DEST_DIR="$HOME/Applications"
    mkdir -p "$DEST_DIR"
    DEST="$DEST_DIR/$APP_NAME.app"
    warn "could not write to /Applications; installing to $DEST"
    rm -rf "$DEST"
    ditto "$SRC" "$DEST"
  else
    exit 1
  fi
fi
# Nudge Finder/Launch Services so the new icon shows up immediately.
touch "$DEST"
LSREGISTER="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
[[ -x "$LSREGISTER" ]] && "$LSREGISTER" -f "$DEST" >/dev/null 2>&1 || true

DESKTOP="$HOME/Desktop"
if [[ "${NO_DESKTOP_ALIAS:-0}" != "1" ]]; then
  step "Creating Desktop alias"
  # Replace only a previous alias file (a regular file), never a real folder or app.
  for old in "$DESKTOP/$ALIAS_NAME" "$DESKTOP/$APP_NAME.app alias" "$DESKTOP/$APP_NAME alias"; do
    if [[ -f "$old" && ! -L "$old" ]]; then rm -f "$old"; fi
  done
  if [[ -e "$DESKTOP/$ALIAS_NAME" ]]; then
    warn "$DESKTOP/$ALIAS_NAME already exists and is not an alias; leaving it alone"
  else
    osascript >/dev/null <<OSA || warn "could not create the Desktop alias (Finder automation permission?)"
tell application "Finder"
  set theAlias to make alias file to (POSIX file "$DEST" as alias) at (path to desktop folder)
  set name of theAlias to "$ALIAS_NAME"
end tell
OSA
  fi
fi

step "Linking $ROOT/$APP_NAME.app"
LINK="$ROOT/$APP_NAME.app"
if [[ -e "$LINK" && ! -L "$LINK" ]]; then
  warn "$LINK exists and is not a symlink; leaving it alone"
else
  ln -sfn "$DEST" "$LINK"
fi

step "Done. Open \"$ALIAS_NAME\" from your Desktop, Launchpad or Spotlight."
