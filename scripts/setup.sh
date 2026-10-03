#!/bin/sh
# Installs what every example needs, then builds them.
#
#   ./scripts/setup.sh              everything except Android
#   ./scripts/setup.sh --all        Android as well (about 5GB more)
#   ./scripts/setup.sh --no-build   install only
#
# There is one of these per platform, and this runs them: each is small, and
# you can run a single one if you only care about a single example.
#
#   scripts/setup-macos.sh      Go and Xcode              (checks only)
#   scripts/setup-ios.sh        Xcode, a Simulator        (checks, downloads a runtime)
#   scripts/setup-linux.sh      colima, docker
#   scripts/setup-windows.sh    qemu, the .NET SDK, an ISO you fetch yourself
#   scripts/setup-freebsd.sh    qemu
#   scripts/setup-netbsd.sh     qemu
#   scripts/setup-openbsd.sh    qemu
#   scripts/setup-plan9.sh      qemu
#   scripts/setup-dragonfly.sh  qemu
#   scripts/setup-illumos.sh    qemu
#   scripts/setup-wasm.sh       wasmtime, node
#   scripts/setup-android.sh    the Android SDK, a JDK, Kotlin
#
# Safe to re-run: anything already present is left alone.
set -e

ROOT=$(cd "$(dirname "$0")" && pwd)/..
cd "$ROOT"
ALL=no
BUILD=yes
while [ $# -gt 0 ]; do
    case $1 in
        --all)      ALL=yes; shift ;;
        --no-build) BUILD=no; shift ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
done

command -v brew >/dev/null 2>&1 || {
    echo "Homebrew is needed: https://brew.sh" >&2; exit 1; }

# macOS first: it is the only one that can stop the rest being worth doing,
# because without Go there is no worker for any of them.
sh "$ROOT/scripts/setup-macos.sh" || exit 1

FAILED=""
for os in ios linux windows freebsd netbsd openbsd dragonfly illumos plan9 wasm; do
    echo
    echo "==> $os"
    sh "$ROOT/scripts/setup-$os.sh" || FAILED="$FAILED $os"
done

if [ "$ALL" = yes ]; then
    echo
    echo "==> android"
    sh "$ROOT/scripts/setup-android.sh" || FAILED="$FAILED android"
else
    echo
    echo "==> android: skipped, about 5GB.  scripts/setup-android.sh does it."
fi

# colima can report "running" while the docker socket is not reachable.
docker info >/dev/null 2>&1 || { echo; echo "==> starting colima"; colima start; }

if [ "$BUILD" = yes ]; then
    echo
    echo "==> building"
    "$ROOT/scripts/build-all.sh"
fi

[ -n "$FAILED" ] && { echo; echo "these did not finish:$FAILED"; }

cat <<'TXT'

run the examples with:
  ./scripts/run.sh            macOS, Linux and Windows at once
  ./scripts/run-linux.sh      Linux on its own, in a container
  ./scripts/run-windows.sh    Windows on its own, in a VM
  ./scripts/run-freebsd.sh    FreeBSD on its own, in a VM
  ./scripts/run-netbsd.sh     NetBSD on its own, in a VM
  ./scripts/run-openbsd.sh    OpenBSD on its own, in a VM
  ./scripts/run-web.sh        a browser, with the worker compiled in
  ./scripts/run-ios.sh        the iOS Simulator
  ./scripts/run-android.sh    the Android emulator
  ./scripts/run-dragonfly.sh  DragonFly on its own, in a VM
  ./scripts/run-illumos.sh    illumos on its own, in a VM
  ./scripts/run-plan9.sh      Plan 9 on its own, in a VM (--test for the test suite)
TXT
exit 0
