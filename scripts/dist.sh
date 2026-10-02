#!/usr/bin/env bash
# Builds the release archives into dist/:
#   WhatsappDoppel-<ver>-macos-universal.zip         the .app (Apple silicon + Intel), needs a Mac to build
#   whatsapp-doppel-<ver>-linux-{amd64,arm64}.tar.gz  binary, icons, .desktop, install.sh, uninstall.sh
#   WhatsappDoppel-<ver>-windows-{amd64,arm64}.zip    GUI exe with icon, install/uninstall .ps1 + .cmd
#   SHA256SUMS.txt
#
#   scripts/dist.sh [all | mac | linux | windows ...]     (or: make dist, make dist-linux, ...)
# Env: VERSION (default: git describe). Pure Go (CGO_ENABLED=0); the Windows
# icon/version resources come from `go run github.com/tc-hib/go-winres` (needs
# network the first time; nothing is added to go.mod).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
# Numeric a.b.c for Info.plist and the Windows file version ("v1.2.3-4-gabc" → 1.2.3).
if [[ "$VERSION" =~ ^v?([0-9]+)\.([0-9]+)\.([0-9]+) ]]; then
  SHORT_VERSION="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.${BASH_REMATCH[3]}"
else
  SHORT_VERSION="0.0.0"
fi
FILE_VERSION="$SHORT_VERSION.0"
WINRES="github.com/tc-hib/go-winres@v0.3.3"

DIST="$ROOT/dist"
STAGE="$ROOT/build/dist-stage"
LDFLAGS="-s -w -X 'main.version=$VERSION'" # no devProjectDir: it means nothing on another computer

step() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
die() { echo "dist: $*" >&2; exit 1; }

targets=("$@")
[[ ${#targets[@]} -gt 0 ]] || targets=(all)
if [[ " ${targets[*]} " == *" all "* ]]; then
  targets=(linux windows)
  if [[ "$(uname -s)" == Darwin ]]; then
    targets=(mac "${targets[@]}")
  else
    echo "note: the macOS .app can only be packaged on a Mac; skipping it" >&2
  fi
fi

cleanup() { rm -f "$ROOT"/rsrc_windows_*.syso; }
trap cleanup EXIT

rm -rf "$STAGE"
mkdir -p "$STAGE" "$DIST"

gobuild() { # gobuild GOOS GOARCH OUT [extra ldflags]
  step "Compiling $1/$2"
  CGO_ENABLED=0 GOOS="$1" GOARCH="$2" go build -trimpath -ldflags "$LDFLAGS ${4:-}" -o "$3" .
}

icons() {
  local f fresh=1
  for f in icon_256.png icon_512.png icon.ico; do
    [[ -f "$ROOT/build/$f" && "$ROOT/build/$f" -nt "$ROOT/scripts/geniconn/main.go" ]] || fresh=0
  done
  [[ "$fresh" == 1 ]] && return
  step "Rendering icons"
  go run ./scripts/geniconn -size 256 -o build/icon_256.png >/dev/null
  go run ./scripts/geniconn -size 512 -o build/icon_512.png >/dev/null
  go run ./scripts/geniconn -ico build/icon.ico >/dev/null
}

# crlf FILE: Windows line endings (cmd.exe and Notepad users).
crlf() { awk '{ sub(/\r$/, ""); printf "%s\r\n", $0 }' "$1" >"$1.tmp" && mv "$1.tmp" "$1"; }

tar_gz() { # tar_gz OUT DIR (normalised owner, no macOS metadata)
  local opts=()
  if tar --version 2>/dev/null | grep -q 'GNU tar'; then
    opts=(--owner=0 --group=0 --numeric-owner)
  else
    opts=(--uid 0 --gid 0)
  fi
  COPYFILE_DISABLE=1 tar "${opts[@]}" -C "$(dirname "$2")" -czf "$1" "$(basename "$2")"
}

zip_dir() { # zip_dir OUT DIR
  command -v zip >/dev/null || die "zip is required"
  rm -f "$1"
  (cd "$(dirname "$2")" && zip -q -r -X "$1" "$(basename "$2")")
}

dist_mac() {
  [[ "$(uname -s)" == Darwin ]] || die "the macOS .app can only be packaged on a Mac"
  local out="$STAGE/mac"
  ARCHS="arm64 amd64" DIST=1 APP_OUT="$out" VERSION="$VERSION" SHORT_VERSION="$SHORT_VERSION" \
    "$ROOT/scripts/build_app.sh"
  local zip="$DIST/WhatsappDoppel-$VERSION-macos-universal.zip"
  rm -f "$zip"
  # No resource forks / xattrs / ACLs: otherwise the zip carries ._* AppleDouble files.
  ditto -c -k --norsrc --noextattr --noacl --keepParent "$out/WhatsappDoppel.app" "$zip"
  step "Wrote $zip"
}

dist_linux() {
  icons
  local arch name dir
  for arch in amd64 arm64; do
    name="whatsapp-doppel-$VERSION-linux-$arch"
    dir="$STAGE/$name"
    mkdir -p "$dir"
    gobuild linux "$arch" "$dir/whatsapp-doppel"
    cp build/icon_256.png "$dir/whatsapp-doppel.png"
    cp build/icon_512.png "$dir/whatsapp-doppel-512.png"
    cp scripts/linux/whatsapp-doppel.desktop scripts/linux/install.sh scripts/linux/uninstall.sh scripts/linux/README.txt "$dir/"
    chmod 755 "$dir/whatsapp-doppel" "$dir/install.sh" "$dir/uninstall.sh"
    chmod 644 "$dir"/*.png "$dir"/*.desktop "$dir/README.txt"
    tar_gz "$DIST/$name.tar.gz" "$dir"
    step "Wrote dist/$name.tar.gz"
  done
}

dist_windows() {
  icons
  step "Generating Windows icon + version resources (go-winres)"
  go run "$WINRES" simply --arch amd64,arm64 --out rsrc --manifest gui \
    --icon build/icon.ico --product-name "WhatsApp Doppel" --file-description "WhatsApp Doppel" \
    --original-filename WhatsappDoppel.exe --copyright "PolyForm Noncommercial 1.0.0" \
    --product-version "$VERSION" --file-version "$FILE_VERSION"
  local arch name dir f
  for arch in amd64 arm64; do
    name="WhatsappDoppel-$VERSION-windows-$arch"
    dir="$STAGE/$name"
    mkdir -p "$dir"
    # -H=windowsgui: no console window on double-click (the CLI attaches to the parent console).
    gobuild windows "$arch" "$dir/WhatsappDoppel.exe" "-H=windowsgui"
    cp scripts/windows/install.ps1 scripts/windows/uninstall.ps1 scripts/windows/install.cmd \
      scripts/windows/uninstall.cmd scripts/windows/README.txt "$dir/"
    for f in install.ps1 uninstall.ps1 install.cmd uninstall.cmd README.txt; do crlf "$dir/$f"; done
    zip_dir "$DIST/$name.zip" "$dir"
    step "Wrote dist/$name.zip"
  done
  cleanup
}

for t in "${targets[@]}"; do
  case "$t" in
    mac) dist_mac ;;
    linux) dist_linux ;;
    windows) dist_windows ;;
    *) die "unknown target $t (all, mac, linux, windows)" ;;
  esac
done

step "Checksums"
(
  cd "$DIST"
  rm -f SHA256SUMS.txt
  files=()
  for f in *.zip *.tar.gz; do [[ -e "$f" ]] && files+=("$f"); done
  if command -v sha256sum >/dev/null; then sha256sum "${files[@]}"; else shasum -a 256 "${files[@]}"; fi >SHA256SUMS.txt
)
ls -l "$DIST"
