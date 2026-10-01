#!/bin/sh
# Builds the Android example and runs it in the emulator.
#
#   ./build.sh            build, install, launch
#   ./build.sh --build    build only
#
# No Gradle: the pieces are the same ones it would call, and spelling them out
# is shorter than the project files that drive it.  What this needs from the
# Android SDK is in scripts/setup-android.sh.
set -e
cd "$(dirname "$0")"

SDK=${ANDROID_HOME:-$HOME/Library/Android/sdk}
API=${ANDROID_API:-35}
ABI=arm64-v8a
PKG=com.imclaren.vero.example
BUILD=.build

TOOLS=$(ls -d "$SDK"/build-tools/* 2>/dev/null | sort -V | tail -1)
JAR="$SDK/platforms/android-$API/android.jar"
[ -x "$TOOLS/aapt2" ] || { echo "no build-tools in $SDK - run scripts/setup-android.sh" >&2; exit 1; }
[ -f "$JAR" ] || { echo "no android-$API platform in $SDK - run scripts/setup-android.sh" >&2; exit 1; }

rm -rf "$BUILD"
mkdir -p "$BUILD/lib/$ABI" "$BUILD/classes" "$BUILD/dex"

echo "building the worker"
# Android runs executables from the application's own native library
# directory, and only files named lib*.so are put there - so the worker is
# called libworker.so whatever it is called in the Go module.  Pure Go, so no
# NDK: CGO_ENABLED=0 is what makes GOOS=android build without one.
CGO_ENABLED=0 GOOS=android GOARCH=arm64 \
    go build -o "$BUILD/lib/$ABI/libworker.so" ../worker

echo "compiling the frontend"
kotlinc -nowarn -classpath "$JAR" \
    ../../bindings/kotlin/Vero.kt src/com/imclaren/vero/example/MainActivity.kt \
    -d "$BUILD/classes" 2>&1 | grep -v "^warning:" || true

echo "dexing"
# The Kotlin standard library goes in too: Android has no Kotlin runtime of
# its own, and an application that leaves it out dies on its first line with
# ClassNotFoundException: kotlin.jvm.internal.Intrinsics.
STDLIB=$(dirname "$(readlink "$(command -v kotlinc)" || command -v kotlinc)")/../lib/kotlin-stdlib.jar
[ -f "$STDLIB" ] || STDLIB=/opt/homebrew/opt/kotlin/libexec/lib/kotlin-stdlib.jar
"$TOOLS/d8" --lib "$JAR" --min-api 24 --output "$BUILD/dex" \
    "$STDLIB" $(find "$BUILD/classes" -name '*.class')

echo "packaging"
"$TOOLS/aapt2" link -I "$JAR" \
    --manifest AndroidManifest.xml \
    --min-sdk-version 24 --target-sdk-version "$API" \
    -o "$BUILD/app.apk"
( cd "$BUILD" && zip -q app.apk classes.dex -j dex/classes.dex && \
  zip -q app.apk "lib/$ABI/libworker.so" )

# A debug key, made once and kept, so reinstalling over an earlier build does
# not fail on a changed signature.
KEYSTORE=$HOME/.android/debug.keystore
if [ ! -f "$KEYSTORE" ]; then
    mkdir -p "$(dirname "$KEYSTORE")"
    keytool -genkeypair -keystore "$KEYSTORE" -storepass android -keypass android \
        -alias androiddebugkey -dname "CN=Android Debug,O=Android,C=US" \
        -keyalg RSA -keysize 2048 -validity 10000 >/dev/null
fi
"$TOOLS/zipalign" -f 4 "$BUILD/app.apk" "$BUILD/app-aligned.apk"
"$TOOLS/apksigner" sign --ks "$KEYSTORE" --ks-pass pass:android \
    --out "$BUILD/vero-example.apk" "$BUILD/app-aligned.apk"
echo "  $BUILD/vero-example.apk"

[ "$1" = "--build" ] && exit 0

ADB="$SDK/platform-tools/adb"
"$ADB" wait-for-device
"$ADB" install -r "$BUILD/vero-example.apk"
# am start, not monkey: monkey looks the application up in a package list
# that has not caught up with the install this line follows, and quietly does
# nothing.
"$ADB" shell am start -n "$PKG/.MainActivity" >/dev/null
echo "running on $("$ADB" shell getprop ro.product.model | tr -d '\r')"
