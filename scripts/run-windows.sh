#!/bin/sh
# Boots a Windows 11 ARM VM with the current build on a disc, in a window on
# your Mac.
#
#   scripts/run-windows.sh --iso ~/Downloads/win11.iso --install   # first time
#   scripts/run-windows.sh                                         # every time after
#   scripts/run-windows.sh --headless                              # no window; QMP on the socket
#   scripts/run-windows.sh --payload out                           # your own build, not the example
#
# Needs: brew install qemu, and a Windows 11 ARM64 ISO from Microsoft.
# Run scripts/build-all.sh first - the disc is made from dist/.
set -e

ROOT=$(cd "$(dirname "$0")/.." && pwd)
VM=${VM:-$HOME/vm/vero-windows}
ISO=""
INSTALL=no
PAYLOAD=""
DISPLAY_ARGS="-display cocoa"
QMP=/tmp/vero-qmp.sock          # short on purpose: unix paths cap at 104 bytes

while [ $# -gt 0 ]; do
    case $1 in
        --vm)       VM=${2:?--vm needs a directory}; shift 2 ;;
        --iso)      ISO=${2:?--iso needs a file}; shift 2 ;;
        --install)  INSTALL=yes; shift ;;
        --headless) DISPLAY_ARGS="-display none -vnc 127.0.0.1:1"; shift ;;
        --payload)  PAYLOAD=${2:?--payload needs a directory}; shift 2 ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
done

command -v qemu-system-aarch64 >/dev/null 2>&1 || { echo "qemu is missing - brew install qemu" >&2; exit 1; }
FW=$(brew --prefix)/share/qemu
mkdir -p "$VM"

[ -f "$VM/disk.qcow2" ] || { qemu-img create -f qcow2 "$VM/disk.qcow2" 40G >/dev/null; echo "made $VM/disk.qcow2"; }
[ -f "$VM/vars.fd" ]    || { cp "$FW/edk2-arm-vars.fd" "$VM/vars.fd"; echo "made $VM/vars.fd"; }

# Build the Windows pieces if they are not there, so this works on its own.
# With --payload the directory already holds them, so none of this runs.
# Rebuild when a source file is newer than what was built from it.  Checking
# only that the file exists is how a months-old dist/ ends up on the disc,
# which looks exactly like the current build until the worker reports its
# version.
stale() {
    target=$1; shift
    [ -f "$target" ] || return 0
    [ -n "$(find "$@" -newer "$target" -print -quit 2>/dev/null)" ]
}

if [ -z "$PAYLOAD" ] && stale "$ROOT/dist/worker-windows-arm64.exe" "$ROOT"/*.go "$ROOT/example/worker"; then
    echo "building the Windows worker"
    mkdir -p "$ROOT/dist"
    # Pure Go: the worker supervises itself, so there is no DLL to build and
    # no C toolchain to find.
    ( cd "$ROOT" && CGO_ENABLED=0 GOOS=windows GOARCH=arm64 \
          go build -o dist/worker-windows-arm64.exe ./example/worker )
fi
DOTNET=$(command -v dotnet 2>/dev/null || true)
[ -z "$DOTNET" ] && [ -x "$HOME/.dotnet/dotnet" ] && DOTNET="$HOME/.dotnet/dotnet"
if [ -z "$PAYLOAD" ] && [ -n "$DOTNET" ] &&
   stale "$ROOT/dist/wpf-arm64/VeroExample.exe" "$ROOT/example/wpf-app" "$ROOT/bindings/csharp"; then
    echo "publishing the WPF example (a minute or two)"
    ( cd "$ROOT/example/wpf-app" && "$DOTNET" publish -c Release -r win-arm64 \
        --self-contained -p:EnableWindowsTargeting=true -o "$ROOT/dist/wpf-arm64" -v quiet ) >/dev/null
fi

# The payload disc: how a build gets in without networking or shared folders.
STAGE=$(mktemp -d); mkdir -p "$STAGE/vero"
if [ -n "$PAYLOAD" ]; then
    [ -d "$PAYLOAD" ] || { echo "no such directory: $PAYLOAD" >&2; exit 2; }
    echo "packing $PAYLOAD onto a disc"
    cp -R "$PAYLOAD/." "$STAGE/vero/"
else
    echo "packing the build onto a disc"
    cp "$ROOT/dist/worker-windows-arm64.exe" "$STAGE/vero/worker.exe"
    if [ -d "$ROOT/dist/wpf-arm64" ]; then
        cp -R "$ROOT/dist/wpf-arm64/." "$STAGE/vero/"
        echo "  including the WPF example"
    else
        echo "  no dist/wpf-arm64: shipping worker.exe only"
    fi
fi
rm -f "$VM/payload.iso"
hdiutil makehybrid -iso -joliet -o "$VM/payload.iso" "$STAGE" -quiet
rm -rf "$STAGE"

set -- \
    -machine virt,highmem=on -accel hvf -cpu host -smp 4 -m 6144 \
    -drive if=pflash,format=raw,readonly=on,file="$FW/edk2-aarch64-code.fd" \
    -drive if=pflash,format=raw,file="$VM/vars.fd" \
    -device qemu-xhci,id=usb \
    -drive if=none,id=cd1,format=raw,readonly=on,media=cdrom,file="$VM/payload.iso" \
    -device usb-storage,drive=cd1,removable=true,bus=usb.0,bootindex=2 \
    -drive if=none,id=hd0,format=qcow2,file="$VM/disk.qcow2" \
    -device nvme,drive=hd0,serial=vero,bootindex=1 \
    -device ramfb -device usb-kbd -device usb-tablet \
    -netdev user,id=n0 -device virtio-net-pci,netdev=n0 \
    -qmp unix:"$QMP",server,nowait

if [ "$INSTALL" = yes ]; then
    [ -n "$ISO" ] || { echo "--install needs --iso windows.iso" >&2; exit 2; }
    # The answer file installs Windows without anyone sitting in front of it.
    rm -f "$VM/unattend.iso"
    hdiutil makehybrid -iso -joliet -o "$VM/unattend.iso" "$ROOT/.windows/unattend" -quiet
    set -- "$@" \
        -drive if=none,id=cd0,format=raw,readonly=on,media=cdrom,file="$ISO" \
        -device usb-storage,drive=cd0,removable=true,bus=usb.0,bootindex=0 \
        -drive if=none,id=cd2,format=raw,readonly=on,media=cdrom,file="$VM/unattend.iso" \
        -device usb-storage,drive=cd2,removable=true,bus=usb.0,bootindex=3
    echo
    echo "installing Windows. Press a key when it says 'Press any key to boot from CD'."
fi

rm -f "$QMP"
echo "booting: $VM"
[ "$INSTALL" = no ] && echo "the build is on the second CD drive - copy it to C:\\ and run VeroExample.exe"
exec qemu-system-aarch64 "$@" $DISPLAY_ARGS
