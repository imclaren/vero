#!/bin/sh
# Builds the iOS example and runs it in the Simulator.
#
#   ./build.sh            builds, installs and launches
#   ./build.sh --build    builds only
#
# Two halves, as always.  The difference is that both of them end up in one
# binary: iOS forbids an application starting a program, so the worker cannot
# run beside the app and runs inside it instead.
#
# A C archive is built from a single Go package, so ./archive is the shim and
# the worker together - shim.go there is a symbolic link to ../../cshim/main.go,
# the same file every other frontend links.
set -e
cd "$(dirname "$0")"

ROOT=$(cd ../.. && pwd)
BUILD=.build
APP="$BUILD/VeroExample.app"
BUNDLE_ID=com.imclaren.vero.example
DEVICE=${DEVICE:-"iPhone 17 Pro"}
MIN_IOS=17.0

SDK=$(xcrun --sdk iphonesimulator --show-sdk-path)
TARGET=arm64-apple-ios$MIN_IOS-simulator
mkdir -p "$BUILD"

echo "building libvero.a for the Simulator"
# GOOS=ios builds for a device; the flags below point the same build at the
# Simulator SDK instead, which is what makes this runnable without a phone.
CGO_ENABLED=1 GOOS=ios GOARCH=arm64 \
    CC="$(xcrun --sdk iphonesimulator --find clang)" \
    CGO_CFLAGS="-isysroot $SDK -target $TARGET" \
    CGO_LDFLAGS="-isysroot $SDK -target $TARGET" \
    go build -buildmode=c-archive -o "$BUILD/libvero.a" ./archive

echo "building the app"
# CVero as a Clang module, so `import Vero` finds the archive's symbols.  The
# package does this through SwiftPM; here it is one module map, because an
# .app for the Simulator is a bundle swiftc can produce on its own.
cp "$ROOT/Sources/CVero/include/CVero.h" "$BUILD/"
cat > "$BUILD/module.modulemap" <<MAP
module CVero {
    header "CVero.h"
    export *
}
MAP

rm -rf "$APP"
mkdir -p "$APP"

# The Vero package as its own module, so the application imports it exactly
# as it would through SwiftPM.  Not libVero.a: the Mac's filesystem does not
# distinguish that from the Go archive's libvero.a, and one silently replaces
# the other.
xcrun --sdk iphonesimulator swiftc -emit-module -emit-library -static -module-name Vero \
    -target "$TARGET" -sdk "$SDK" -O \
    -Xcc -fmodule-map-file="$PWD/$BUILD/module.modulemap" -I "$BUILD" \
    "$ROOT"/Sources/Vero/*.swift \
    -emit-module-path "$BUILD/Vero.swiftmodule" -o "$BUILD/libVeroSwift.a"

xcrun --sdk iphonesimulator swiftc -parse-as-library -target "$TARGET" -sdk "$SDK" -O \
    -Xcc -fmodule-map-file="$PWD/$BUILD/module.modulemap" -I "$BUILD" \
    Sources/IOSExample/*.swift \
    -L "$BUILD" -lVeroSwift -lvero \
    -Xlinker -syslibroot -Xlinker "$SDK" \
    -o "$APP/VeroExample"

cat > "$APP/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key><string>VeroExample</string>
  <key>CFBundleIdentifier</key><string>$BUNDLE_ID</string>
  <key>CFBundleName</key><string>vero</string>
  <key>CFBundleDisplayName</key><string>vero</string>
  <key>CFBundleVersion</key><string>1</string>
  <key>CFBundleShortVersionString</key><string>1.0</string>
  <key>LSRequiresIPhoneOS</key><true/>
  <key>UILaunchScreen</key><dict/>
  <key>MinimumOSVersion</key><string>$MIN_IOS</string>
  <key>UIDeviceFamily</key><array><integer>1</integer></array>
</dict>
</plist>
PLIST

echo "  $APP"
[ "$1" = "--build" ] && exit 0

# Boot the simulator if it is not already up, then hand it the app.
xcrun simctl boot "$DEVICE" 2>/dev/null || true
open -a Simulator
xcrun simctl install booted "$APP"
xcrun simctl launch booted "$BUNDLE_ID"
