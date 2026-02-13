#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="Dotward"
DIST_DIR="${ROOT_DIR}/dist"
BUNDLE_DIR="${DIST_DIR}/${APP_NAME}.app"
CONTENTS_DIR="${BUNDLE_DIR}/Contents"
MACOS_DIR="${CONTENTS_DIR}/MacOS"
RESOURCES_DIR="${CONTENTS_DIR}/Resources"
VERSION="${VERSION:-dev}"
COMMIT="${COMMIT:-unknown}"
BUILD_DATE="${BUILD_DATE:-unknown}"
BUILT_BY="${BUILT_BY:-build_app.sh}"
PLIST_VERSION="${VERSION#v}"
if [[ ! "${PLIST_VERSION}" =~ ^[0-9]+(\.[0-9]+){0,2}$ ]]; then
  PLIST_VERSION="0.0.0"
fi
LDFLAGS="-X github.com/stefanos/dotward/internal/version.Version=${VERSION} -X github.com/stefanos/dotward/internal/version.Commit=${COMMIT} -X github.com/stefanos/dotward/internal/version.BuildDate=${BUILD_DATE} -X github.com/stefanos/dotward/internal/version.BuiltBy=${BUILT_BY}"
ICONSET_DIR="${ROOT_DIR}/AppIcons/Assets.xcassets/AppIcon.appiconset"
ICON_FILE="${RESOURCES_DIR}/AppIcon.icns"

rm -rf "${BUNDLE_DIR}"
mkdir -p "${MACOS_DIR}" "${RESOURCES_DIR}"

go build -ldflags "${LDFLAGS}" -o "${MACOS_DIR}/${APP_NAME}" "${ROOT_DIR}/cmd/app"

if [[ -d "${ICONSET_DIR}" ]] && command -v iconutil >/dev/null 2>&1; then
  tmp_root="$(mktemp -d)"
  tmp_iconset="${tmp_root}/AppIcon.iconset"
  mkdir -p "${tmp_iconset}"
  trap 'rm -rf "${tmp_root}"' EXIT

  cp "${ICONSET_DIR}/16.png" "${tmp_iconset}/icon_16x16.png"
  cp "${ICONSET_DIR}/32.png" "${tmp_iconset}/icon_16x16@2x.png"
  cp "${ICONSET_DIR}/32.png" "${tmp_iconset}/icon_32x32.png"
  cp "${ICONSET_DIR}/64.png" "${tmp_iconset}/icon_32x32@2x.png"
  cp "${ICONSET_DIR}/128.png" "${tmp_iconset}/icon_128x128.png"
  cp "${ICONSET_DIR}/256.png" "${tmp_iconset}/icon_128x128@2x.png"
  cp "${ICONSET_DIR}/256.png" "${tmp_iconset}/icon_256x256.png"
  cp "${ICONSET_DIR}/512.png" "${tmp_iconset}/icon_256x256@2x.png"
  cp "${ICONSET_DIR}/512.png" "${tmp_iconset}/icon_512x512.png"
  cp "${ICONSET_DIR}/1024.png" "${tmp_iconset}/icon_512x512@2x.png"

  iconutil -c icns "${tmp_iconset}" -o "${ICON_FILE}"
fi

cat > "${CONTENTS_DIR}/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>Dotward</string>
	<key>CFBundleDisplayName</key>
	<string>Dotward</string>
	<key>CFBundleExecutable</key>
	<string>Dotward</string>
	<key>CFBundleIdentifier</key>
	<string>com.yourname.dotward</string>
	<key>CFBundleIconFile</key>
	<string>AppIcon</string>
	<key>CFBundleVersion</key>
	<string>${PLIST_VERSION}</string>
	<key>CFBundleShortVersionString</key>
	<string>${PLIST_VERSION}</string>
	<key>LSUIElement</key>
	<true/>
	<key>NSHighResolutionCapable</key>
	<true/>
</dict>
</plist>
PLIST

echo "Built ${BUNDLE_DIR}"
