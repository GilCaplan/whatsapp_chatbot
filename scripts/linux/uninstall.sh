#!/usr/bin/env bash
# Removes what install.sh created (a copy lives in ~/.local/share/whatsapp-doppel/). Your data (settings, personas, WhatsApp
# link) is kept unless you pass --purge or answer "y" when asked.
#   ./uninstall.sh [--purge | --keep-data]
set -euo pipefail

BIN="$HOME/.local/bin/whatsapp-doppel"
DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
DATA_DIR="${DOPPEL_DATA_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/WhatsappDoppel}"
MODE="ask"
case "${1:-}" in
  --purge) MODE="purge" ;;
  --keep-data) MODE="keep" ;;
  "") ;;
  *) echo "usage: $0 [--purge | --keep-data]" >&2; exit 2 ;;
esac

step() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }

if [[ -x "$BIN" ]]; then
  step "Stopping the server"
  "$BIN" quit >/dev/null 2>&1 || true
  step "Removing $BIN"
  rm -f "$BIN"
fi

desk="$(xdg-user-dir DESKTOP 2>/dev/null || true)"
[[ -n "$desk" && "$desk" != "$HOME" ]] || desk="$HOME/Desktop"
for f in "$DATA_HOME/applications/whatsapp-doppel.desktop" "$desk/whatsapp-doppel.desktop" \
  "$DATA_HOME/icons/hicolor/256x256/apps/whatsapp-doppel.png" "$DATA_HOME/icons/hicolor/512x512/apps/whatsapp-doppel.png"; do
  if [[ -f "$f" ]]; then
    step "Removing $f"
    rm -f "$f"
  fi
done
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$DATA_HOME/applications" >/dev/null 2>&1 || true
# The copy of this script kept by install.sh (bash has already opened it; removing is fine).
rm -rf "$DATA_HOME/whatsapp-doppel"

if [[ -d "$DATA_DIR" ]]; then
  if [[ "$MODE" == "ask" && -t 0 ]]; then
    read -r -p "Also delete your data in \"$DATA_DIR\" (personas, settings, WhatsApp link)? [y/N] " answer || answer="n"
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
