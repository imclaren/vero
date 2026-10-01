#!/bin/sh
# Opens the example on all three platforms at once, on your Mac.
#
#   ./scripts/run.sh --iso ~/Downloads/win11.iso   # first time: installs Windows
#   ./scripts/run.sh                               # every time after
#
# macOS runs natively, Linux in a container shown over VNC, and Windows in a VM.
# FreeBSD has a script of its own, scripts/run-freebsd.sh, because its VM takes
# a few minutes to set itself up the first time.
# On a fresh clone it runs scripts/setup.sh for you first.
set -e

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"
ISO=""
OPEN=yes
while [ $# -gt 0 ]; do
    case $1 in
        --iso)     ISO=${2:?--iso needs a file}; shift 2 ;;
        --no-open) OPEN=no; shift ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
done
VM=${VM:-$HOME/vm/vero-windows}

# A fresh clone has nothing built and may have nothing installed.
if [ ! -f "$ROOT/dist/libvero.a" ]; then
    "$ROOT/scripts/setup.sh"
    echo
fi

echo "==> macOS"
( cd example/menubar-app && ./build.sh >/dev/null )
./example/menubar-app/.build/debug/MenuBarExample >/dev/null 2>&1 &
echo "    running - look in the menu bar"

echo "==> Linux"
if [ "$OPEN" = yes ]; then ./scripts/run-linux.sh; else ./scripts/run-linux.sh --no-open; fi

echo "==> Windows"
set -- --vm "$VM"
[ "$OPEN" = no ] && set -- "$@" --headless
if [ -n "$ISO" ]; then
    set -- "$@" --iso "$ISO" --install
elif [ ! -f "$VM/disk.qcow2" ]; then
    echo "    no VM yet - rerun with --iso ~/Downloads/win11.iso to install Windows"
    exit 0
fi
./scripts/run-windows.sh "$@" &

echo
echo "all three are up. stop them with:"
echo "  pkill -f MenuBarExample; pkill -f qemu-system-aarch64; docker rm -f \$(docker ps -q --filter ancestor=vero-gtk)"
wait
