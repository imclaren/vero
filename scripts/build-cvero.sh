#!/bin/sh
# Builds the C archive that the Swift package links, from this checkout,
# into dist/CVero.xcframework, which Package.swift then uses in place of the
# last release's. It is for working on main between releases, when the
# Swift code needs C functions that the release's archive lacks. Remove
# dist/CVero.xcframework to go back to the release's.
#
#   scripts/build-cvero.sh
set -eu
ROOT=$(cd "$(dirname "$0")/.." && pwd)
DIST="$ROOT/dist"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$DIST" "$TMP/hdr"
# As build-all.sh does: Go 1.26 keeps the archive linkable on macOS 12.
export GOTOOLCHAIN=${MACOS_GOTOOLCHAIN:-go1.26.0}
export MACOSX_DEPLOYMENT_TARGET=${MACOSX_DEPLOYMENT_TARGET:-11.0}
cd "$ROOT"
CGO_ENABLED=1 GOARCH=arm64 go build -buildmode=c-archive -o "$TMP/libvero-arm64.a" ./cshim
CGO_ENABLED=1 GOARCH=amd64 CC="clang -arch x86_64 -mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET" \
    go build -buildmode=c-archive -o "$TMP/libvero-amd64.a" ./cshim
lipo -create "$TMP/libvero-arm64.a" "$TMP/libvero-amd64.a" -output "$TMP/libvero.a"
cp "$ROOT/Sources/CVero/include/CVero.h" "$TMP/hdr/"
printf 'module CVero {\n    header "CVero.h"\n    export *\n}\n' > "$TMP/hdr/module.modulemap"
rm -rf "$DIST/CVero.xcframework"
xcodebuild -create-xcframework -library "$TMP/libvero.a" -headers "$TMP/hdr" -output "$DIST/CVero.xcframework" >/dev/null
echo "built $DIST/CVero.xcframework, which the Swift package now uses"
