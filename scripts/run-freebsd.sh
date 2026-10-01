#!/bin/sh
# Runs the GTK example on FreeBSD, in a VM on your Mac.
#
#   scripts/run-freebsd.sh              # watch it, live, over VNC
#   scripts/run-freebsd.sh --record out.gif
#   scripts/run-freebsd.sh --no-open    # start it but do not open a viewer
#   scripts/run-freebsd.sh --shell      # a shell in the VM, for poking about
#
# Needs qemu:  brew install qemu
#
# The first run downloads FreeBSD's own cloud image and installs GTK4 in it,
# which takes a few minutes.  After that the VM has both and boots in seconds.
#
# Why a VM rather than a container, as Linux uses: there is no FreeBSD
# container runtime on macOS.  Why so little of this is about building: the
# worker supervises itself and the Python binding spawns it, so there is no
# shared library to compile on the target - the worker cross-compiles here,
# like every other platform.
set -e

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

VM=${VM:-$HOME/vm/vero-freebsd}
RELEASE=${RELEASE:-15.0}
PORT=${PORT:-5902}          # VNC, one past the Linux example's
SSH_PORT=${SSH_PORT:-2222}
RECORD=""
OPEN=yes
SHELL_ONLY=no
while [ $# -gt 0 ]; do
    case $1 in
        --record)  RECORD=${2:?--record needs a filename}; shift 2 ;;
        --no-open) OPEN=no; shift ;;
        --shell)   SHELL_ONLY=yes; shift ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
done

command -v qemu-system-aarch64 >/dev/null 2>&1 || { echo "qemu is missing - brew install qemu" >&2; exit 1; }
FW=$(brew --prefix)/share/qemu
mkdir -p "$VM"

IMAGE="FreeBSD-$RELEASE-RELEASE-arm64-aarch64-BASIC-CLOUDINIT-ufs.qcow2"
URL="https://download.freebsd.org/releases/VM-IMAGES/$RELEASE-RELEASE/aarch64/Latest/$IMAGE.xz"

if [ ! -f "$VM/disk.qcow2" ]; then
    echo "downloading FreeBSD $RELEASE (once; about 500MB)"
    curl -fL "$URL" -o "$VM/image.qcow2.xz"
    xz -d "$VM/image.qcow2.xz"
    mv "$VM/image.qcow2" "$VM/disk.qcow2"
    # Room for packages: the image ships barely larger than its contents.
    qemu-img resize "$VM/disk.qcow2" 20G >/dev/null
    echo "made $VM/disk.qcow2"
fi
[ -f "$VM/vars.fd" ] || cp "$FW/edk2-arm-vars.fd" "$VM/vars.fd"

# cloud-init reads this on first boot: a user to ssh in as, and the key to let
# in.  The image has no password for anyone, which is the point of it.
if [ ! -f "$VM/seed.iso" ]; then
    [ -f "$VM/key" ] || ssh-keygen -q -t ed25519 -N "" -f "$VM/key" -C vero
    SEED=$(mktemp -d)
    cat > "$SEED/meta-data" <<EOF
instance-id: vero
local-hostname: vero
EOF
    cat > "$SEED/user-data" <<EOF
#cloud-config
users:
  - name: vero
    groups: wheel
    shell: /bin/sh
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
      - $(cat "$VM/key.pub")
EOF
    hdiutil makehybrid -iso -joliet -default-volume-name cidata \
        -o "$VM/seed.iso" "$SEED" -quiet
    rm -rf "$SEED"
    echo "made $VM/seed.iso"
fi

SSH="ssh -i $VM/key -p $SSH_PORT -o StrictHostKeyChecking=no \
     -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR vero@127.0.0.1"

echo "building the FreeBSD worker"
# Pure Go: nothing here needs a FreeBSD toolchain.
CGO_ENABLED=0 GOOS=freebsd GOARCH=arm64 \
    go build -o "$VM/worker" ./example/worker

echo "booting the VM"
qemu-system-aarch64 \
    -machine virt,highmem=on -accel hvf -cpu host -smp 4 -m 4096 \
    -drive if=pflash,format=raw,readonly=on,file="$FW/edk2-aarch64-code.fd" \
    -drive if=pflash,format=raw,file="$VM/vars.fd" \
    -drive if=virtio,format=qcow2,file="$VM/disk.qcow2" \
    -drive if=virtio,format=raw,readonly=on,file="$VM/seed.iso" \
    -device virtio-gpu-pci -device qemu-xhci -device usb-kbd -device usb-tablet \
    -netdev user,id=n0,hostfwd=tcp::"$SSH_PORT"-:22 -device virtio-net-pci,netdev=n0 \
    -display none -vnc ":$((PORT - 5900))" \
    -pidfile "$VM/qemu.pid" -daemonize

cleanup() {
    [ -f "$VM/qemu.pid" ] && kill "$(cat "$VM/qemu.pid")" 2>/dev/null || true
    rm -f "$VM/qemu.pid"
}
trap cleanup EXIT INT TERM

printf "waiting for ssh"
n=0
until $SSH true 2>/dev/null; do
    n=$((n + 1))
    [ $n -gt 120 ] && { echo; echo "the VM never came up" >&2; exit 1; }
    printf "."
    sleep 2
done
echo

# GTK and Python, once.  They stay in the image, so this is the slow part of
# the first run and nothing at all afterwards.
if ! $SSH "pkg info -e py311-gobject3" 2>/dev/null; then
    echo "installing GTK4 and Python in the VM (once; a few minutes)"
    $SSH "sudo pkg install -y gtk4 py311-gobject3 xorg-vfbserver x11vnc python3" >/dev/null
fi

echo "copying the example in"
$SSH "mkdir -p app" >/dev/null
scp -q -i "$VM/key" -P "$SSH_PORT" -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR \
    "$VM/worker" "$ROOT/example/gtk-app/main.py" "$ROOT/bindings/python/vero.py" \
    vero@127.0.0.1:app/
$SSH "chmod +x app/worker app/main.py" >/dev/null

if [ "$SHELL_ONLY" = yes ]; then
    trap - EXIT INT TERM
    echo "the VM is up.  ssh in with:"
    echo "  $SSH"
    echo "stop it with:  kill \$(cat $VM/qemu.pid)"
    exit 0
fi

echo "starting the example on a virtual display"
$SSH "pkill Xvfb; pkill x11vnc; true" >/dev/null 2>&1 || true
$SSH "daemon -f Xvfb :99 -screen 0 480x440x24; sleep 2;
      cd app && DISPLAY=:99 daemon -f python3.11 main.py; sleep 4;
      daemon -f x11vnc -display :99 -forever -nopw -listen 0.0.0.0" >/dev/null 2>&1

if [ -n "$RECORD" ]; then
    echo "recording 20 frames"
    $SSH "pkg info -e ImageMagick7 >/dev/null 2>&1 || sudo pkg install -y ImageMagick7" >/dev/null 2>&1
    $SSH 'cd app && rm -rf frames && mkdir frames &&
          for i in $(seq -w 1 20); do
              DISPLAY=:99 magick import -window root frames/f$i.png 2>/dev/null
              sleep 0.5
          done' >/dev/null 2>&1
    OUT=$(mktemp -d)
    scp -q -i "$VM/key" -P "$SSH_PORT" -o StrictHostKeyChecking=no \
        -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR \
        "vero@127.0.0.1:app/frames/*.png" "$OUT/"
    ffmpeg -loglevel error -y -framerate 2 -i "$OUT/f%02d.png" \
        -vf "scale=480:-1:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse" \
        -loop 0 "$RECORD"
    rm -rf "$OUT"
    echo "wrote $RECORD"
    exit 0
fi

if [ "$OPEN" = yes ]; then
    echo "opening Screen Sharing on port $PORT"
    open "vnc://localhost:$PORT"
    echo "close the viewer, then press Enter to stop the VM"
    read -r _
else
    trap - EXIT INT TERM
    echo "the VM is up, VNC on $PORT.  stop it with:  kill \$(cat $VM/qemu.pid)"
fi
