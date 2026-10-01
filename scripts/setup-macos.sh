#!/bin/sh
# Checks what the macOS example needs: Go, and Xcode.
#
#   scripts/setup-macos.sh
#
# Neither is installed for you.  Go because the version decides the minimum
# macOS your build supports - go1.27 stamps darwin/arm64 objects macOS 13,
# which silently drops macOS 12 users - and Xcode because it is 15GB from the
# App Store.
set -e

ok=yes
command -v go >/dev/null 2>&1 || {
    echo "Go is missing: https://go.dev/dl" >&2; ok=no; }
xcrun --sdk macosx --show-sdk-path >/dev/null 2>&1 || {
    echo "Xcode is missing: install it from the App Store, then run" >&2
    echo "  sudo xcode-select -s /Applications/Xcode.app" >&2; ok=no; }
[ "$ok" = yes ] || exit 1

echo "go $(go version | awk '{print $3}'), $(xcodebuild -version | head -1)"
echo "done.  run it with:  example/menubar-app/build.sh"
