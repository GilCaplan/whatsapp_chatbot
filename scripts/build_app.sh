#!/usr/bin/env bash
# Builds build/WhatsappDoppel.app: Go binary + Info.plist + AppIcon.icns, ad-hoc signed.
#   scripts/build_app.sh            (or: make app)
# Env: VERSION (default: git describe), SHORT_VERSION (default 1.0.0),
#      ARCHS ("arm64 amd64" = universal binary via lipo; default: this Mac's arch),
#      DIST=1 (release build: no developer project path baked in),
#      APP_OUT (output folder; default build/).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

APP_NAME="WhatsappDoppel"
BUILD="$ROOT/build"
OUT="${APP_OUT:-$BUILD}"
APP="$OUT/$APP_NAME.app"
ARCHS="${ARCHS:-$(go env GOARCH)}"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
SHORT_VERSION="${SHORT_VERSION:-1.0.0}"
BUILD_NUMBER="$(git rev-list --count HEAD 2>/dev/null || echo 1)"

step() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }

TOOLS="go sips iconutil codesign plutil"
[[ "$ARCHS" == *" "* ]] && TOOLS="$TOOLS lipo"
for tool in $TOOLS; do
  command -v "$tool" >/dev/null || { echo "missing required tool: $tool" >&2; exit 1; }
done

mkdir -p "$BUILD" "$OUT"

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

LDFLAGS="-s -w -X 'main.version=$VERSION'"
[[ "${DIST:-0}" == "1" ]] || LDFLAGS="$LDFLAGS -X 'main.devProjectDir=$ROOT'"
step "Compiling for $ARCHS (pure Go; the first build can take a minute or two)"
SLICES=()
for arch in $ARCHS; do
  out="$BUILD/$APP_NAME-darwin-$arch"
  CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build -trimpath -ldflags "$LDFLAGS" -o "$out" .
  SLICES+=("$out")
done
if [[ ${#SLICES[@]} -gt 1 ]]; then
  lipo -create -output "$APP/Contents/MacOS/$APP_NAME" "${SLICES[@]}"
else
  cp "${SLICES[0]}" "$APP/Contents/MacOS/$APP_NAME"
fi
rm -f "${SLICES[@]}"

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
