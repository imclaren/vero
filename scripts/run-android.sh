#!/bin/sh
# Builds the Android example and runs it in the emulator.
#
#   scripts/run-android.sh            start the emulator if needed, then run
#   scripts/run-android.sh --headless the same, with no emulator window
#
# scripts/setup-android.sh installs what this needs.
set -e
ROOT=$(cd "$(dirname "$0")/.." && pwd)
export ANDROID_HOME=${ANDROID_HOME:-$HOME/Library/Android/sdk}
export JAVA_HOME=${JAVA_HOME:-/opt/homebrew/opt/openjdk}
PATH="$JAVA_HOME/bin:$ANDROID_HOME/platform-tools:$PATH"
export PATH
AVD=${AVD:-vero}

[ -x "$ANDROID_HOME/platform-tools/adb" ] || {
    echo "the Android SDK is not installed - run scripts/setup-android.sh" >&2
    exit 1
}

if ! adb devices | grep -q "device$"; then
    echo "starting the $AVD emulator"
    WINDOW=""
    [ "$1" = "--headless" ] && WINDOW="-no-window"
    # -no-snapshot-save: the emulator is a build artefact here, and a saved
    # snapshot is what makes the next boot differ from this one.
    "$ANDROID_HOME/emulator/emulator" -avd "$AVD" $WINDOW -no-snapshot-save \
        > "$ROOT/.android-emulator.log" 2>&1 &
    adb wait-for-device
    echo "waiting for it to finish booting"
    until [ "$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ]; do
        sleep 2
    done
fi

"$ROOT/example/android-app/build.sh"
