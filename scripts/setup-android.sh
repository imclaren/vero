#!/bin/sh
# Installs what the Android example needs, and makes an emulator to run it in.
#
#   scripts/setup-android.sh
#
# About 5GB, nearly all of it the emulator's system image.  Everything lands
# under $ANDROID_HOME (~/Library/Android/sdk by default) and nothing needs
# root: the command line tools, a JDK and the Kotlin compiler come from
# Homebrew, the rest from Google's repository.
set -e

API=${ANDROID_API:-35}
ABI=arm64-v8a
IMAGE="system-images;android-$API;google_apis;$ABI"
AVD=${AVD:-vero}
export ANDROID_HOME=${ANDROID_HOME:-$HOME/Library/Android/sdk}

need() {
    command -v "$1" >/dev/null && return 0
    echo "installing $2"
    brew install $3 "$2"
}
need java openjdk
need kotlinc kotlin
need sdkmanager android-commandlinetools --cask

# Homebrew keeps openjdk out of the way of macOS's own java, so point the
# Android tools at it rather than asking anyone to change their PATH.
JAVA_HOME=${JAVA_HOME:-/opt/homebrew/opt/openjdk}
export JAVA_HOME
PATH="$JAVA_HOME/bin:/opt/homebrew/share/android-commandlinetools/cmdline-tools/latest/bin:$PATH"
export PATH

# --sdk_root, every time: left to itself sdkmanager installs beside its own
# command line tools, which is inside the Homebrew cask - so the next
# `brew upgrade` takes the SDK with it.
echo "accepting the SDK licences"
yes 2>/dev/null | sdkmanager --sdk_root="$ANDROID_HOME" --licenses >/dev/null || true

echo "installing the SDK packages (about 5GB)"
# cmdline-tools among them: avdmanager works out where the SDK is from where
# it is itself, not from $ANDROID_HOME, so the copy in the Homebrew cask looks
# for system images there and finds none.
sdkmanager --sdk_root="$ANDROID_HOME" --install \
    "cmdline-tools;latest" "platform-tools" "platforms;android-$API" \
    "build-tools;$API.0.0" "emulator" "$IMAGE" | tail -1

AVDMANAGER="$ANDROID_HOME/cmdline-tools/latest/bin/avdmanager"
if ! "$AVDMANAGER" list avd 2>/dev/null | grep -q "Name: $AVD"; then
    echo "creating the $AVD emulator"
    echo no | "$AVDMANAGER" create avd -n "$AVD" -k "$IMAGE" --device pixel_6 >/dev/null
fi

echo
echo "done.  start the emulator and run the example with:"
echo "  scripts/run-android.sh"
