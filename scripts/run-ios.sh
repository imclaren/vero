#!/bin/sh
# Builds the iOS example and runs it in the Simulator.
#
#   scripts/run-ios.sh           build, install, launch
#   scripts/run-ios.sh --test    run vero's test suite inside the Simulator
#
# Needs Xcode, for the Simulator and its SDK.  No signing identity and no
# device: the Simulator takes an unsigned bundle.
set -e
ROOT=$(cd "$(dirname "$0")/.." && pwd)
DEVICE=${DEVICE:-"iPhone 17 Pro"}

SDK=$(xcrun --sdk iphonesimulator --show-sdk-path)
TARGET=arm64-apple-ios17.0-simulator

if [ "$1" = "--test" ]; then
    shift                       # the rest goes to the test binary
    echo "building the tests for the Simulator"
    TEST=$(mktemp -d)/vero-ios.test
    ( cd "$ROOT" && CGO_ENABLED=1 GOOS=ios GOARCH=arm64 \
        CC="$(xcrun --sdk iphonesimulator --find clang)" \
        CGO_CFLAGS="-isysroot $SDK -target $TARGET" \
        CGO_LDFLAGS="-isysroot $SDK -target $TARGET" \
        go test -c -o "$TEST" . )

    xcrun simctl boot "$DEVICE" 2>/dev/null || true
    echo "running them there"
    # The Simulator is macOS underneath, so the tests that start a worker as a
    # program pass as well.  A phone cannot do that, which is what the
    # in-process tests - TestAWorkerInThisProcess... - are for.
    xcrun simctl spawn booted "$TEST" -test.v "$@" | tail -40
    exit 0
fi

DEVICE="$DEVICE" "$ROOT/example/ios-app/build.sh"
