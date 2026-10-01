#!/bin/sh
# Installs what is needed to run the examples for every platform on a
# Mac, then builds it.
#
#   ./scripts/setup.sh
#
# Safe to re-run: anything already present is left alone.
set -e

ROOT=$(cd "$(dirname "$0")" && pwd)/..
cd "$ROOT"

command -v brew >/dev/null 2>&1 || {
    echo "Homebrew is needed: https://brew.sh" >&2; exit 1; }

echo "==> toolchains"
# Only what is actually missing.  Re-installing a working formula can leave it
# unlinked, and then the command it provides disappears from the PATH.
# Go is deliberately not in this list.  Installing a second one shadows
# whatever is already on the PATH, and the Go version decides the minimum macOS
# the archive supports - 1.27 stamps darwin/arm64 objects macOS 13, which
# silently drops macOS 12 users.  Bring your own.
command -v go >/dev/null 2>&1 || {
    echo "Go is missing: https://go.dev/dl - not installing one, because the" >&2
    echo "version decides the minimum macOS your build supports." >&2
    exit 1
}

# Docker runs the Linux example; qemu runs the Windows and FreeBSD ones.
# Nothing here is needed to *build*: the worker supervises itself, so every
# target but macOS is a plain Go cross-compile with no C toolchain at all.
for pair in colima:colima docker:docker qemu-system-aarch64:qemu; do
    cmd=${pair%%:*}; formula=${pair#*:}
    command -v "$cmd" >/dev/null 2>&1 && continue
    echo "    installing $formula"
    brew install "$formula" >/dev/null 2>&1 || true
    command -v "$cmd" >/dev/null 2>&1 ||
        brew link --overwrite "$formula" >/dev/null 2>&1 || true
    command -v "$cmd" >/dev/null 2>&1 ||
        { echo "    $formula installed but $cmd is not on the PATH" >&2; }
done

# colima can report "running" while the docker socket is not reachable.
docker info >/dev/null 2>&1 || { echo "==> starting colima"; colima start; }

echo "==> building"
"$ROOT/scripts/build-all.sh"

cat <<'TXT'

run the examples with:
  ./scripts/run.sh            macOS, Linux and Windows at once
  ./scripts/run-linux.sh      Linux on its own, in a container
  ./scripts/run-windows.sh    Windows on its own, in a VM
  ./scripts/run-freebsd.sh    FreeBSD on its own, in a VM
  ./scripts/run-web.sh        a browser, with the worker compiled in
  ./scripts/run-ios.sh        the iOS Simulator - needs Xcode
  ./scripts/run-android.sh    the Android emulator - run setup-android.sh first
  ./scripts/run-plan9.sh      vero's tests on 9front, in a VM

The Windows VM needs a Windows 11 ARM64 ISO the first time:
  ./scripts/run-windows.sh --iso ~/Downloads/win11.iso --install

Android needs about 5GB of SDK, so it has a setup script of its own:
  ./scripts/setup-android.sh
TXT
