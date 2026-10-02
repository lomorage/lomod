#!/usr/bin/env bash
#
# Builds Lomorage.app inside --dest-dir from the static template at installers/macos/Lomorage.app
# (Info.plist + lomorage-tray.swift), compiling the tray app as a universal (arm64+x86_64)
# Mach-O binary -- see lomorage-tray.swift's header comment for why this whole feature is a
# compiled Swift binary and not JXA (an earlier version was JXA/osascript, matching this
# project's general "ships as plain interpreted source" philosophy, but that turned out to
# have a confirmed, reproducible, unfixable bug in JXA's own bridge into AppKit's
# NSStatusItem). swiftc doesn't support building a universal binary in one invocation the way
# clang does (-arch x86_64 -arch arm64) -- compile each architecture as its own slice, then
# lipo them together. Also generates Contents/Resources/AppIcon.icns from
# installers/macos/AppIcon-1024.png (macOS's icon format needs a full .iconset --
# multiple resolutions bundled via iconutil -- not just a single PNG). That PNG is produced
# by scripts/icons/generate-app-icons.py.
set -euo pipefail

DEST_DIR=""
SIGN_IDENTITY=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dest-dir) DEST_DIR="$2"; shift 2 ;;
        --sign-identity) SIGN_IDENTITY="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [[ -z "${DEST_DIR}" ]]; then
    echo "usage: $0 --dest-dir <dir> [--sign-identity <codesign identity>]" >&2
    exit 1
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TEMPLATE="${REPO_ROOT}/installers/macos/Lomorage.app"
APP_DIR="${DEST_DIR}/Lomorage.app"
LOGO="${REPO_ROOT}/installers/macos/AppIcon-1024.png"

rm -rf "${APP_DIR}"
mkdir -p "${APP_DIR}/Contents/MacOS" "${APP_DIR}/Contents/Resources"
cp "${TEMPLATE}/Contents/Info.plist" "${APP_DIR}/Contents/Info.plist"

echo "==> Compiling lomorage-tray.swift (universal arm64+x86_64)"
SWIFT_BUILD="$(mktemp -d)"
ICONSET="$(mktemp -d)"
trap 'rm -rf "${SWIFT_BUILD}" "${ICONSET}"' EXIT
swiftc -target arm64-apple-macos11.0 -O -o "${SWIFT_BUILD}/lomorage-tray-arm64" \
    "${TEMPLATE}/Contents/MacOS/lomorage-tray.swift"
swiftc -target x86_64-apple-macos10.13 -O -o "${SWIFT_BUILD}/lomorage-tray-x86_64" \
    "${TEMPLATE}/Contents/MacOS/lomorage-tray.swift"
lipo -create "${SWIFT_BUILD}/lomorage-tray-arm64" "${SWIFT_BUILD}/lomorage-tray-x86_64" \
    -output "${APP_DIR}/Contents/MacOS/lomorage-launcher"
chmod +x "${APP_DIR}/Contents/MacOS/lomorage-launcher"

echo "==> Generating AppIcon.icns from AppIcon-1024.png"
mkdir -p "${ICONSET}/AppIcon.iconset"
for spec in "16:icon_16x16" "32:icon_16x16@2x" "32:icon_32x32" "64:icon_32x32@2x" \
            "128:icon_128x128" "256:icon_128x128@2x" "256:icon_256x256" \
            "512:icon_256x256@2x" "512:icon_512x512" "1024:icon_512x512@2x"; do
    size="${spec%%:*}"
    name="${spec##*:}"
    sips -z "${size}" "${size}" "${LOGO}" --out "${ICONSET}/AppIcon.iconset/${name}.png" >/dev/null
done
iconutil -c icns "${ICONSET}/AppIcon.iconset" -o "${APP_DIR}/Contents/Resources/AppIcon.icns"

if [[ -n "${SIGN_IDENTITY}" ]]; then
    echo "==> Codesigning Lomorage.app with ${SIGN_IDENTITY} (hardened runtime + secure timestamp)"
    codesign --force --options runtime --timestamp --sign "${SIGN_IDENTITY}" "${APP_DIR}"
else
    echo "==> Codesigning Lomorage.app ad-hoc (no --sign-identity given)"
    codesign --force --sign - "${APP_DIR}"
fi

echo "==> Built ${APP_DIR}"
