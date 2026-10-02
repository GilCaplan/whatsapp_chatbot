#!/usr/bin/env bash
# Builds build/WhatsappDoppel.app: Go binary + Info.plist + AppIcon.icns, ad-hoc signed.
#   scripts/build_app.sh            (or: make app)
# Env: VERSION (default: git describe), SHORT_VERSION (default 1.0.0).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

APP_NAME="WhatsappDoppel"
BUILD="$ROOT/build"
APP="$BUILD/$APP_NAME.app"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
SHORT_VERSION="${SHORT_VERSION:-1.0.0}"
BUILD_NUMBER="$(git rev-list --count HEAD 2>/dev/null || echo 1)"

step() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }

for tool in go sips iconutil codesign plutil; do
  command -v "$tool" >/dev/null || { echo "missing required tool: $tool" >&2; exit 1; }
done

mkdir -p "$BUILD"

# ── Icon ──────────────────────────────────────────────────────
ICON_PNG="$BUILD/icon_1024.png"
if [[ ! -f "$ICON_PNG" || "$ROOT/scripts/geniconn/main.go" -nt "$ICON_PNG" ]]; then
  step "Rendering icon"
  go run ./scripts/geniconn -o "$ICON_PNG"
fi
ICONSET="$BUILD/AppIcon.iconset"
rm -rf "$ICONSET"
mkdir -p "$ICONSET"
step "Building AppIcon.icns"
for size in 16 32 128 256 512; do
  sips -z "$size" "$size" "$ICON_PNG" --out "$ICONSET/icon_${size}x${size}.png" >/dev/null
  double=$((size * 2))
  sips -z "$double" "$double" "$ICON_PNG" --out "$ICONSET/icon_${size}x${size}@2x.png" >/dev/null
done

# ── Bundle ────────────────────────────────────────────────────
step "Assembling $APP_NAME.app ($VERSION)"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/AppIcon.icns"

step "Compiling (CGO, first build can take a minute or two)"
CGO_ENABLED=1 go build -trimpath \
  -ldflags "-s -w -X 'main.version=$VERSION' -X 'main.devProjectDir=$ROOT'" \
  -o "$APP/Contents/MacOS/$APP_NAME" .

cat >"$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleExecutable</key>
  <string>$APP_NAME</string>
  <key>CFBundleIdentifier</key>
  <string>com.rockycaplan.whatsappdoppel</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>WhatsApp Doppel</string>
  <key>CFBundleDisplayName</key>
  <string>WhatsApp Doppel</string>
  <key>CFBundleIconFile</key>
  <string>AppIcon</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>$SHORT_VERSION</string>
  <key>CFBundleVersion</key>
  <string>$BUILD_NUMBER</string>
  <key>LSMinimumSystemVersion</key>
  <string>13.0</string>
  <key>LSApplicationCategoryType</key>
  <string>public.app-category.social-networking</string>
  <key>NSHighResolutionCapable</key>
  <true/>
  <key>LSUIElement</key>
  <true/>
  <key>NSHumanReadableCopyright</key>
  <string>Local build ($VERSION)</string>
</dict>
</plist>
PLIST
printf 'APPL????' >"$APP/Contents/PkgInfo"
plutil -lint "$APP/Contents/Info.plist" >/dev/null

step "Signing (ad-hoc)"
codesign --force --deep --sign - "$APP"
codesign --verify --strict "$APP"

step "Built $APP"
