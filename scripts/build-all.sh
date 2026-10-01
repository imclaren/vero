#!/bin/sh
# Builds vero for every platform it runs on, from a Mac.
#
#   scripts/build-all.sh
#
# Everything lands in dist/.  Almost all of it is a plain cross-compile: the
# worker supervises itself, so a frontend spawns it rather than loading a C
# library, and there is nothing to build with a foreign C toolchain.
#
# The exception is macOS, where the Swift package links the Go archive into
# the application, and cgo builds that here natively.
set -e

# Work from the module we are in, so this script also works copied into
# someone else's project alongside their go.mod.
if [ -f "$PWD/go.mod" ]; then
    ROOT=$PWD
else
    ROOT=$(cd "$(dirname "$0")/.." && pwd)
fi
cd "$ROOT"
DIST="$ROOT/dist"
mkdir -p "$DIST"

WORKER=${WORKER:-./example/worker}
# The C shim, for the one target that links Go into the frontend.  In your own
# project this is the import path instead:
#   CSHIM=github.com/imclaren/vero/cshim WORKER=./cmd/worker ./build-all.sh
CSHIM=${CSHIM:-./cshim}
SKIPPED=""
skip() { SKIPPED="$SKIPPED
  $1"; echo "  skipped: $1"; }

# The platforms a release carries a worker for.  Everything Go builds works -
# 35 of its ports do - so this is a choice about what to ship rather than what
# is possible.  TARGETS= overrides it.
TARGETS=${TARGETS:-"darwin/arm64 darwin/amd64
windows/amd64 windows/arm64
linux/amd64 linux/arm64
freebsd/amd64 freebsd/arm64
openbsd/amd64 openbsd/arm64
netbsd/amd64 netbsd/arm64
dragonfly/amd64
illumos/amd64
solaris/amd64
aix/ppc64
plan9/amd64
wasip1/wasm"}

echo "the worker: no C toolchain needed for any of these"
for t in $TARGETS; do
    os=${t%/*}; arch=${t#*/}
    ext=""
    [ "$os" = windows ] && ext=".exe"
    # A WASI runtime is handed a file to run, and expects it to say what it is.
    [ "$os" = wasip1 ] && ext=".wasm"
    CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
        go build -o "$DIST/worker-$os-$arch$ext" "$WORKER"
    echo "  worker-$os-$arch$ext"
done
if [ -f "$DIST/worker-darwin-arm64" ] && [ -f "$DIST/worker-darwin-amd64" ]; then
    lipo -create "$DIST/worker-darwin-arm64" "$DIST/worker-darwin-amd64" \
         -output "$DIST/worker-macos-universal"
    echo "  worker-macos-universal"
fi

echo "macOS: a universal C archive for Swift to link"
# The Go version decides the minimum macOS this archive supports, whatever
# MACOSX_DEPLOYMENT_TARGET says: go1.27 stamps darwin/arm64 objects macOS 13,
# and an application targeting 12 then fails to link against them.  Pinned
# here so a release cannot quietly drop macOS 12 users because of whichever Go
# happened to be on the PATH.
export GOTOOLCHAIN=${MACOS_GOTOOLCHAIN:-go1.26.0}
export MACOSX_DEPLOYMENT_TARGET=${MACOSX_DEPLOYMENT_TARGET:-11.0}
CGO_ENABLED=1 GOARCH=arm64 \
    go build -buildmode=c-archive -o "$DIST/libvero-arm64.a" "$CSHIM"
CGO_ENABLED=1 GOARCH=amd64 CC="clang -arch x86_64 -mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET" \
    go build -buildmode=c-archive -o "$DIST/libvero-amd64.a" "$CSHIM"
lipo -create "$DIST/libvero-arm64.a" "$DIST/libvero-amd64.a" -output "$DIST/libvero.a"
echo "  libvero.a ($(lipo -info "$DIST/libvero.a" | sed 's/.*are: //'))"

unset GOTOOLCHAIN

echo "Windows: the WPF example, published for win-arm64"
DOTNET=$(command -v dotnet 2>/dev/null || true)
[ -z "$DOTNET" ] && [ -x "$HOME/.dotnet/dotnet" ] && DOTNET="$HOME/.dotnet/dotnet"
if [ -n "$DOTNET" ] && [ -d "$ROOT/example/wpf-app" ]; then
    ( cd "$ROOT/example/wpf-app" && "$DOTNET" publish -c Release -r win-arm64 \
        --self-contained -p:EnableWindowsTargeting=true -o "$DIST/wpf-arm64" -v quiet ) >/dev/null
    echo "  wpf-arm64/VeroExample.exe"
else
    skip "wpf-arm64 (no dotnet SDK, or no example/wpf-app)"
fi

# One canonical header beside the archive, for anyone linking it by hand.
[ -f "$DIST/libvero-arm64.h" ] && cp "$DIST/libvero-arm64.h" "$DIST/vero.h"
rm -f "$DIST"/libvero-arm64.a "$DIST"/libvero-amd64.a "$DIST"/libvero-*.h
echo
echo "in $DIST:"
ls "$DIST" | sed 's/^/  /'
[ -n "$SKIPPED" ] && printf '\nskipped:%s\n' "$SKIPPED"
exit 0
