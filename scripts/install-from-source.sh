#!/usr/bin/env bash
# One-command install from a source checkout, for macOS and Linux:
#   scripts/install-from-source.sh [--no-launch | --launch]
#
#   macOS: builds WhatsappDoppel.app and installs it to /Applications with a
#          Desktop alias (same as `make install`; replaces a running copy).
#   Linux: builds the binary and runs the release installer: ~/.local/bin,
#          app menu entry, icon, Desktop shortcut (no sudo).
# Needs Go (https://go.dev/dl/); it never installs system packages for you.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
OS="$(uname -s)"

LAUNCH_ARGS=()
for arg in "$@"; do
  case "$arg" in
    --launch | --no-launch) LAUNCH_ARGS+=("$arg") ;;
    -h | --help) sed -n '2,10p' "$0"; exit 0 ;;
    *) echo "unknown option: $arg (try --help)" >&2; exit 2 ;;
  esac
done

step() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }

go_help() {
  echo "" >&2
  case "$OS" in
    Darwin)
      echo "Install Go, then run this script again:" >&2
      echo "  - the macOS installer (.pkg) from https://go.dev/dl/" >&2
      echo "  - or with Homebrew:  brew install go" >&2
      ;;
    *)
      echo "Install Go, then run this script again:" >&2
      echo "  - the official tarball from https://go.dev/dl/ (steps: https://go.dev/doc/install)" >&2
      echo "  - or Ubuntu:  sudo snap install go --classic   Fedora: sudo dnf install golang   Arch: sudo pacman -S go" >&2
      echo "    (any Go 1.21 or newer works: it downloads the exact version this project needs by itself)" >&2
      ;;
  esac
}

# Go 1.21+ is enough: it fetches the toolchain go.mod asks for (GOTOOLCHAIN=auto).
if ! command -v go >/dev/null 2>&1; then
  echo "Go is not installed (or not on your PATH)." >&2
  go_help
  exit 1
fi
GOV="$(go env GOVERSION 2>/dev/null || true)"
if [[ "$GOV" =~ ^go1\.([0-9]+) ]] && (( BASH_REMATCH[1] >= 21 )); then
  step "Using $GOV (this project needs $(awk '/^go /{print "go"$2}' go.mod); Go fetches it if needed)"
else
  echo "Go ${GOV:-?} is too old; Go 1.21 or newer is needed." >&2
  go_help
  exit 1
fi

case "$OS" in
  Darwin)
    step "Building and installing the Mac app"
    "$ROOT/scripts/install_app.sh"
    if [[ " ${LAUNCH_ARGS[*]:-} " == *" --launch "* ]]; then
      open -a "/Applications/WhatsappDoppel.app" 2>/dev/null || open -a "$HOME/Applications/WhatsappDoppel.app"
    fi
    ;;
  Linux)
    VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
    STAGE="$ROOT/build/linux-install"
    rm -rf "$STAGE"
    mkdir -p "$STAGE"
    step "Building whatsapp-doppel $VERSION (pure Go; the first build takes a minute or two)"
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X 'main.version=$VERSION'" -o "$STAGE/whatsapp-doppel" .
    step "Rendering icons"
    go run ./scripts/geniconn -size 256 -o "$STAGE/whatsapp-doppel.png" >/dev/null
    go run ./scripts/geniconn -size 512 -o "$STAGE/whatsapp-doppel-512.png" >/dev/null
    cp scripts/linux/whatsapp-doppel.desktop scripts/linux/install.sh scripts/linux/uninstall.sh "$STAGE/"
    chmod 755 "$STAGE/install.sh" "$STAGE/uninstall.sh"
    "$STAGE/install.sh" "${LAUNCH_ARGS[@]+"${LAUNCH_ARGS[@]}"}"
    ;;
  *)
    echo "Unsupported system: $OS. On Windows run scripts\\install-from-source.ps1 in PowerShell." >&2
    exit 1
    ;;
esac
