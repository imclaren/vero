#!/bin/sh
# Checks what the iOS example needs: Xcode, and a Simulator runtime.
#
#   scripts/setup-ios.sh
#
# No signing identity and no device: the Simulator takes an unsigned bundle,
# which is why the example is a script rather than an Xcode project.
set -e

xcrun --sdk iphonesimulator --show-sdk-path >/dev/null 2>&1 || {
    echo "the iOS Simulator SDK is missing.  Install Xcode from the App Store," >&2
    echo "then: sudo xcode-select -s /Applications/Xcode.app" >&2
    echo "and:  xcodebuild -downloadPlatform iOS" >&2
    exit 1
}

if ! xcrun simctl list devices available 2>/dev/null | grep -q iPhone; then
    echo "no iPhone simulator is installed.  Getting one:"
    xcodebuild -downloadPlatform iOS
fi

echo "$(xcrun simctl list devices available | grep -c iPhone) iPhone simulators, SDK $(xcrun --sdk iphonesimulator --show-sdk-version)"
echo "done.  run it with:  scripts/run-ios.sh"
