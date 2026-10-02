#!/usr/bin/env bash
# Installs WhatsApp Doppel for the current user (no sudo needed):
#   ~/.local/bin/whatsapp-doppel                         the app
#   ~/.local/share/applications/whatsapp-doppel.desktop  app menu entry
#   ~/.local/share/icons/hicolor/*/apps/whatsapp-doppel.png
#   ~/Desktop/whatsapp-doppel.desktop                    Desktop shortcut (if you have a Desktop folder)
#   ~/.local/share/whatsapp-doppel/uninstall.sh           to remove all of the above later
# Run it from the extracted folder:  ./install.sh [--launch | --no-launch] [--no-desktop-shortcut]
# Safe to run again (updates in place). Your data (~/.config/WhatsappDoppel) is never touched.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_BIN="$HERE/whatsapp-doppel"
BIN_DIR="$HOME/.local/bin"
DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
APPS_DIR="$DATA_HOME/applications"
ICON_DIR="$DATA_HOME/icons/hicolor"
BIN="$BIN_DIR/whatsapp-doppel"
DESKTOP_FILE="whatsapp-doppel.desktop"

LAUNCH="ask"
DESKTOP_SHORTCUT=1
for arg in "$@"; do
  case "$arg" in
    --launch) LAUNCH="yes" ;;
    --no-launch) LAUNCH="no" ;;
    --no-desktop-shortcut) DESKTOP_SHORTCUT=0 ;;
    -h | --help) sed -n '2,9p' "$0"; exit 0 ;;
    *) echo "unknown option: $arg (try --help)" >&2; exit 2 ;;
  esac
done

step() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarning:\033[0m %s\n' "$*" >&2; }

[[ -f "$SRC_BIN" ]] || { echo "whatsapp-doppel not found next to install.sh ($SRC_BIN). Extract the whole archive first." >&2; exit 1; }
chmod 755 "$SRC_BIN" 2>/dev/null || true

# Stop a running copy so it can be replaced (the browser tab reconnects later).
if [[ -x "$BIN" ]]; then
  step "Stopping the running copy (if any)"
  "$BIN" quit >/dev/null 2>&1 || true
fi

step "Installing $BIN"
mkdir -p "$BIN_DIR"
cp -f "$SRC_BIN" "$BIN.new"
chmod 755 "$BIN.new"
mv -f "$BIN.new" "$BIN"

step "Adding the icon and the app menu entry"
for size in 256 512; do
  src="$HERE/whatsapp-doppel.png"
  [[ "$size" == 512 ]] && src="$HERE/whatsapp-doppel-512.png"
  if [[ -f "$src" ]]; then
    mkdir -p "$ICON_DIR/${size}x${size}/apps"
    cp -f "$src" "$ICON_DIR/${size}x${size}/apps/whatsapp-doppel.png"
  fi
done
# Exec= wants the path double-quoted with \ " ` $ escaped, and % doubled.
exec_path="$(printf '%s' "$BIN" | sed -e 's/\\/\\\\\\\\/g' -e 's/["`$]/\\\\&/g' -e 's/%/%%/g')"
mkdir -p "$APPS_DIR"
template="$HERE/$DESKTOP_FILE"
if [[ ! -f "$template" ]]; then
  echo "$DESKTOP_FILE not found next to install.sh" >&2
  exit 1
fi
while IFS= read -r line || [[ -n "$line" ]]; do
  if [[ "$line" == Exec=* ]]; then line="Exec=\"$exec_path\""; fi
  printf '%s\n' "$line"
done <"$template" >"$APPS_DIR/$DESKTOP_FILE"
chmod 644 "$APPS_DIR/$DESKTOP_FILE"
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$APPS_DIR" >/dev/null 2>&1 || true
command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -q -t -f "$ICON_DIR" >/dev/null 2>&1 || true

if [[ -f "$HERE/uninstall.sh" ]]; then
  mkdir -p "$DATA_HOME/whatsapp-doppel"
  if [[ "$HERE/uninstall.sh" != "$DATA_HOME/whatsapp-doppel/uninstall.sh" ]]; then
    cp -f "$HERE/uninstall.sh" "$DATA_HOME/whatsapp-doppel/uninstall.sh"
  fi
  chmod 755 "$DATA_HOME/whatsapp-doppel/uninstall.sh"
fi

if [[ "$DESKTOP_SHORTCUT" == 1 ]]; then
  desk="$(xdg-user-dir DESKTOP 2>/dev/null || true)"
  [[ -n "$desk" && "$desk" != "$HOME" ]] || desk="$HOME/Desktop"
  if [[ -d "$desk" ]]; then
    step "Adding a Desktop shortcut"
    cp -f "$APPS_DIR/$DESKTOP_FILE" "$desk/$DESKTOP_FILE"
    chmod 755 "$desk/$DESKTOP_FILE"
    # GNOME only starts Desktop launchers marked as trusted.
    command -v gio >/dev/null 2>&1 && gio set "$desk/$DESKTOP_FILE" metadata::trusted true >/dev/null 2>&1 || true
  fi
fi

if ! command -v notify-send >/dev/null 2>&1; then
  warn "notify-send is missing, so Doppel can't show notifications. Install it with your package manager (Ubuntu/Debian: sudo apt install libnotify-bin, Fedora: sudo dnf install libnotify, Arch: sudo pacman -S libnotify)."
fi
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "Tip: add $BIN_DIR to your PATH to run 'whatsapp-doppel status' or 'whatsapp-doppel quit' in a terminal." ;;
esac

step "Installed. Open WhatsApp Doppel from your app menu or your Desktop."
echo "To uninstall later: $DATA_HOME/whatsapp-doppel/uninstall.sh"

if [[ "$LAUNCH" == "ask" ]]; then
  if [[ -t 0 && -t 1 ]]; then
    read -r -p "Open WhatsApp Doppel now? [Y/n] " answer || answer="n"
    [[ "$answer" =~ ^[Nn] ]] && LAUNCH="no" || LAUNCH="yes"
  elif [[ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]]; then
    # Started by double-click ("Run as a Program") without a terminal: show that it worked.
    LAUNCH="yes"
  else
    LAUNCH="no"
  fi
fi
if [[ "$LAUNCH" == "yes" ]]; then
  "$BIN" launch || warn "could not start it; run: $BIN"
fi
