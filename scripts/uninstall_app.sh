#!/usr/bin/env bash
# Removes what install_app.sh created: the installed app(s), the Desktop alias
# and the project symlink. Your data (settings, personas, WhatsApp session) is
# kept unless you pass --purge or answer "y" when asked.
#   scripts/uninstall_app.sh [--purge | --keep-data]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="WhatsappDoppel"
ALIAS_NAME="WhatsApp Doppel"
DATA_DIR="${DOPPEL_DATA_DIR:-$HOME/Library/Application Support/WhatsappDoppel}"
MODE="ask"
case "${1:-}" in
  --purge) MODE="purge" ;;
  --keep-data) MODE="keep" ;;
  "") ;;
  *) echo "usage: $0 [--purge | --keep-data]" >&2; exit 2 ;;
esac

step() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }

for app in "/Applications/$APP_NAME.app" "$HOME/Applications/$APP_NAME.app"; do
  if [[ -d "$app" ]]; then
    if [[ -x "$app/Contents/MacOS/$APP_NAME" ]]; then
      step "Stopping the server"
      "$app/Contents/MacOS/$APP_NAME" quit >/dev/null 2>&1 || true
    fi
    step "Removing $app"
    rm -rf "$app"
  fi
done

for alias in "$HOME/Desktop/$ALIAS_NAME" "$HOME/Desktop/$APP_NAME.app alias" "$HOME/Desktop/$APP_NAME alias"; do
  if [[ -f "$alias" && ! -L "$alias" ]]; then
    step "Removing Desktop alias"
    rm -f "$alias"
  fi
done

if [[ -L "$ROOT/$APP_NAME.app" ]]; then
  step "Removing project symlink"
  rm -f "$ROOT/$APP_NAME.app"
fi

if [[ -d "$DATA_DIR" ]]; then
  if [[ "$MODE" == "ask" && -t 0 ]]; then
    read -r -p "Also delete your data in \"$DATA_DIR\" (personas, settings, WhatsApp link)? [y/N] " answer
    [[ "$answer" =~ ^[Yy]$ ]] && MODE="purge"
  fi
  if [[ "$MODE" == "purge" ]]; then
    step "Deleting $DATA_DIR"
    rm -rf "$DATA_DIR"
  else
    echo "Kept your data in: $DATA_DIR"
  fi
fi
step "Uninstalled."
