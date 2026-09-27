#!/bin/sh
# Builds vero for macOS, Windows and Linux, from a Mac.
#
#   scripts/build-all.sh
#
# Everything lands in dist/.  Targets whose toolchain is missing are skipped
# with a note rather than failing the run, so this is useful before you have
# installed all of them:
#
#   brew install mingw-w64                     # Windows x64
#   brew install colima docker && colima start # Linux
#   llvm-mingw from github.com/mstorsjo/llvm-mingw, unpacked with bin/ on PATH,
#   or in ~/toolchains/llvm-mingw               # Windows ARM64
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
# The C shim.  In your own project this is the import path instead:
#   CSHIM=github.com/imclaren/vero/cshim WORKER=./cmd/worker ./build-all.sh
CSHIM=${CSHIM:-./cshim}
SKIPPED=""
skip() { SKIPPED="$SKIPPED
  $1"; echo "  skipped: $1"; }

# The ARM64 Windows compiler is not in Homebrew, so look where it usually ends up.
ARM_CC=$(command -v aarch64-w64-mingw32-clang 2>/dev/null || true)
[ -z "$ARM_CC" ] && [ -x "$HOME/toolchains/llvm-mingw/bin/aarch64-w64-mingw32-clang" ] \
    && ARM_CC="$HOME/toolchains/llvm-mingw/bin/aarch64-w64-mingw32-clang"

echo "the worker: no C toolchain needed for any of these"
for t in darwin/arm64 darwin/amd64 windows/amd64 windows/arm64 linux/amd64 linux/arm64; do
    os=${t%/*}; arch=${t#*/}
    ext=""; [ "$os" = windows ] && ext=".exe"
    CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
        go build -o "$DIST/worker-$os-$arch$ext" "$WORKER"
    echo "  worker-$os-$arch$ext"
done
lipo -create "$DIST/worker-darwin-arm64" "$DIST/worker-darwin-amd64" \
     -output "$DIST/worker-macos-universal"
echo "  worker-macos-universal"

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

echo "Windows: one DLL per architecture"
if command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
    CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc \
        go build -buildmode=c-shared -o "$DIST/vero-amd64.dll" "$CSHIM"
    echo "  vero-amd64.dll"
else
    skip "vero-amd64.dll (brew install mingw-w64)"
fi
if [ -n "$ARM_CC" ]; then
    CGO_ENABLED=1 GOOS=windows GOARCH=arm64 CC="$ARM_CC" \
        go build -buildmode=c-shared -o "$DIST/vero-arm64.dll" "$CSHIM"
    echo "  vero-arm64.dll"
else
    skip "vero-arm64.dll (llvm-mingw not found)"
fi

echo "Linux: a container, because c-shared on macOS emits a Mach-O dylib"
if docker info >/dev/null 2>&1; then
    # $PWD has to be under $HOME: colima shares nothing else.
    # Only the native architecture by default: the emulated one takes minutes
    # and the Go compiler segfaults in it now and then.  ALL_ARCHES=1 for both.
    ARCHES=$(uname -m | sed 's/^arm64$/arm64/; s/^x86_64$/amd64/')
    [ -n "$ALL_ARCHES" ] && ARCHES="arm64 amd64"
    for arch in $ARCHES; do
        # Three goes: under emulation the Go compiler segfaults occasionally.
        n=0
        while :; do
            n=$((n + 1))
            docker run --rm --platform "linux/$arch" \
                -v "$ROOT":/src:ro -v "$DIST":/out -w /src \
                -e GOCACHE=/tmp/gocache -e GOPATH=/tmp/go -e GOTOOLCHAIN=auto \
                golang:1.24-bookworm \
                sh -c "CGO_ENABLED=1 go build -buildmode=c-shared -o /out/libvero-$arch.so $CSHIM" \
                >/tmp/vero-linux-build.log 2>&1 && { echo "  libvero-$arch.so"; break; }
            [ $n -ge 3 ] && {
                skip "libvero-$arch.so: $(tail -1 /tmp/vero-linux-build.log | cut -c1-72)"
                break
            }
        done
    done
else
    skip "libvero-*.so (colima start)"
fi

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

# One canonical header: cgo writes the same declarations for every target.
for h in "$DIST"/libvero-arm64.h "$DIST"/libvero.h "$DIST"/vero-arm64.h; do
    [ -f "$h" ] && { cp "$h" "$DIST/vero.h"; break; }
done
rm -f "$DIST"/libvero-arm64.a "$DIST"/libvero-amd64.a "$DIST"/libvero*.h "$DIST"/vero-*.h
echo
echo "in $DIST:"
ls "$DIST" | sed 's/^/  /'
[ -n "$SKIPPED" ] && printf '\nskipped:%s\n' "$SKIPPED"
exit 0
