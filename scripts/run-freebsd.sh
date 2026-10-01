#!/bin/sh
# Runs the GTK example on FreeBSD, in a VM on your Mac.
#
#   scripts/run-freebsd.sh              # watch it, live, over VNC
#   scripts/run-freebsd.sh --record out.gif
#   scripts/run-freebsd.sh --no-open    # start it but do not open a viewer
#   scripts/run-freebsd.sh --shell      # a shell in the VM, for poking about
#   scripts/run-freebsd.sh --reset      # throw the VM away and start again
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
PORT=${PORT:-5902}          # the guest's x11vnc, forwarded; one past Linux's
SSH_PORT=${SSH_PORT:-2222}
GOBJECT=${GOBJECT:-py312-pygobject}  # the GTK binding for Python, as FreeBSD names it
PYTHON=${PYTHON:-python3.12}
RECORD=""
OPEN=yes
SHELL_ONLY=no
RESET=no
while [ $# -gt 0 ]; do
    case $1 in
        --record)  RECORD=${2:?--record needs a filename}; shift 2 ;;
        --no-open) OPEN=no; shift ;;
        --shell)   SHELL_ONLY=yes; shift ;;
        --reset)   RESET=yes; shift ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
done

command -v qemu-system-aarch64 >/dev/null 2>&1 || { echo "qemu is missing - brew install qemu" >&2; exit 1; }
FW=$(brew --prefix)/share/qemu
mkdir -p "$VM"

IMAGE="FreeBSD-$RELEASE-RELEASE-arm64-aarch64-BASIC-CLOUDINIT-ufs.qcow2"
URL="https://download.freebsd.org/releases/VM-IMAGES/$RELEASE-RELEASE/aarch64/Latest/$IMAGE.xz"

if [ ! -f "$VM/base.qcow2" ]; then
    echo "downloading FreeBSD $RELEASE (once; about 500MB)"
    curl -fL "$URL" -o "$VM/base.qcow2.xz"
    xz -d "$VM/base.qcow2.xz"
    mv "$VM/$IMAGE" "$VM/base.qcow2" 2>/dev/null || true
    echo "made $VM/base.qcow2"
fi

# The VM runs on an overlay, so the download stays pristine and a VM that has
# been wrecked - a UFS that needs a manual fsck after a hard kill, say - is
# thrown away with --reset rather than fetched again.
# The firmware variables go with the disk: they remember which device the
# boot entry points at, and a VM booted once with different devices will
# otherwise land in the UEFI shell.
[ "$RESET" = yes ] && rm -f "$VM/disk.qcow2" "$VM/vars.fd"
FIRST_BOOT=no
if [ ! -f "$VM/disk.qcow2" ]; then
    qemu-img create -f qcow2 -b "$VM/base.qcow2" -F qcow2 "$VM/disk.qcow2" 20G >/dev/null
    echo "made $VM/disk.qcow2"
    FIRST_BOOT=yes
fi
[ -f "$VM/vars.fd" ] || cp "$FW/edk2-arm-vars.fd" "$VM/vars.fd"

# cloud-init reads this on first boot: a user to ssh in as, and the key to let
# in.  The image has no password for anyone, which is the point of it.
if [ ! -f "$VM/seed.iso" ]; then
    [ -f "$VM/key" ] || ssh-keygen -q -t ed25519 -N "" -f "$VM/key" -C vero
    SEED=$(mktemp -d)
    cat > "$SEED/meta-data" <<EOF
instance-id: vero-root
local-hostname: vero
EOF
    # The key goes to root rather than a new user, because FreeBSD 15 reads
    # this with nuageinit rather than cloud-init, and its pw useradd does not
    # create the home directory it then tries to write .ssh into - the failure
    # is a lua traceback on the console, not a message.  root's home exists.
    #
    # runcmd runs after the user block, so it is the right place to let that
    # key in: FreeBSD ships PermitRootLogin no.
    cat > "$SEED/user-data" <<EOF
#cloud-config
users:
  - name: root
    ssh_authorized_keys:
      - $(cat "$VM/key.pub")
runcmd:
  - sysrc sshd_enable=YES
  - sed -i '' -e 's/^#*PermitRootLogin.*/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config
  - sed -i '' -e 's/^#*UseDNS.*/UseDNS no/' /etc/ssh/sshd_config
  - service sshd restart
EOF
    hdiutil makehybrid -iso -joliet -default-volume-name cidata \
        -o "$VM/seed.iso" "$SEED" -quiet
    rm -rf "$SEED"
    echo "made $VM/seed.iso"
fi

SSH="ssh -i $VM/key -p $SSH_PORT -o StrictHostKeyChecking=no \
     -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR root@127.0.0.1"

# A VM left running from a previous go: stop it the polite way.  Killing
# qemu while the guest is writing leaves a UFS that wants a manual fsck on
# the next boot, which there is no way to answer from here.
if [ -f "$VM/qemu.pid" ] && kill -0 "$(cat "$VM/qemu.pid")" 2>/dev/null; then
    echo "stopping the VM that is already running"
    $SSH "poweroff" >/dev/null 2>&1 || true
    n=0
    while kill -0 "$(cat "$VM/qemu.pid")" 2>/dev/null; do
        n=$((n + 1))
        [ $n -gt 45 ] && { kill "$(cat "$VM/qemu.pid")" 2>/dev/null; sleep 2; break; }
        sleep 1
    done
fi
rm -f "$VM/qemu.pid"

echo "building the FreeBSD worker"
# Pure Go: nothing here needs a FreeBSD toolchain.
CGO_ENABLED=0 GOOS=freebsd GOARCH=arm64 \
    go build -o "$VM/worker" ./example/worker

boot() {   # $1: extra -netdev options
    qemu-system-aarch64 \
        -machine virt,highmem=on -accel hvf -cpu host -smp 4 -m 4096 \
        -drive if=pflash,format=raw,readonly=on,file="$FW/edk2-aarch64-code.fd" \
        -drive if=pflash,format=raw,file="$VM/vars.fd" \
        -drive if=virtio,format=qcow2,file="$VM/disk.qcow2" \
        -drive if=virtio,format=raw,readonly=on,file="$VM/seed.iso" \
        -device virtio-gpu-pci -device qemu-xhci -device usb-kbd -device usb-tablet \
        -netdev "user,id=n0,hostfwd=tcp::$SSH_PORT-:22,hostfwd=tcp::$PORT-:5900$1" \
        -device virtio-net-pci,netdev=n0 \
        -display none \
        -serial file:"$VM/console.log" \
        -pidfile "$VM/qemu.pid" -daemonize
}

wait_ssh() {
    printf "waiting for ssh"
    n=0
    until $SSH true 2>/dev/null; do
        n=$((n + 1))
        [ $n -gt 150 ] && { echo; echo "the VM never came up - see $VM/console.log" >&2; exit 1; }
        printf "."
        sleep 2
    done
    echo
}

cleanup() {
    [ -f "$VM/qemu.pid" ] || return 0
    # Ask first.  A hard kill while the guest is writing leaves a UFS that
    # wants a manual fsck on the next boot, and no way to answer it.
    $SSH "poweroff" >/dev/null 2>&1 || true
    n=0
    while kill -0 "$(cat "$VM/qemu.pid")" 2>/dev/null; do
        n=$((n + 1))
        [ $n -gt 30 ] && { kill "$(cat "$VM/qemu.pid")" 2>/dev/null; break; }
        sleep 1
    done
    rm -f "$VM/qemu.pid"
}

if [ "$FIRST_BOOT" = yes ]; then
    # Offline for the first boot on purpose.  The image patches itself the
    # first time it has a network - 2333 patches, hours of it - and nothing
    # here wants a patched system, only GTK.  Without a route it gives up at
    # once, and the firstboot marker is spent either way.
    echo "first boot (offline, so the image does not spend hours patching itself)"
    boot ",restrict=on"
    wait_ssh
    $SSH "sysrc firstboot_freebsd_update_enable=NO >/dev/null 2>&1; rm -f /firstboot; poweroff" \
        >/dev/null 2>&1 || true
    n=0
    while [ -f "$VM/qemu.pid" ] && kill -0 "$(cat "$VM/qemu.pid")" 2>/dev/null; do
        n=$((n + 1)); [ $n -gt 60 ] && break
        sleep 1
    done
    rm -f "$VM/qemu.pid"
fi

echo "booting the VM"
boot ""
trap cleanup EXIT INT TERM
wait_ssh

echo "copying the example in"
$SSH "mkdir -p app" >/dev/null
scp -q -i "$VM/key" -P "$SSH_PORT" -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR \
    "$VM/worker" "$ROOT/example/gtk-app/main.py" "$ROOT/bindings/python/vero.py" \
    root@127.0.0.1:app/
$SSH "chmod +x app/worker app/main.py" >/dev/null

if [ "$SHELL_ONLY" = yes ]; then
    trap - EXIT INT TERM
    echo "the VM is up.  ssh in with:"
    echo "  $SSH"
    echo "stop it with:  kill \$(cat $VM/qemu.pid)"
    exit 0
fi

# GTK and Python, once.  They stay in the image, so this is the slow part of
# the first run and nothing at all afterwards.  Not fatal: a missing package
# should leave the VM up to be looked at, not tear it down.
if ! $SSH "pkg info -e $GOBJECT" 2>/dev/null; then
    echo "installing GTK4 and Python in the VM (once; a few minutes)"
    $SSH "pkg install -y gtk4 $GOBJECT tigervnc-server ImageMagick7" ||
        { echo "the packages did not install - the VM is still up, ssh in with:" >&2
          echo "  $SSH" >&2; trap - EXIT INT TERM; exit 1; }
fi

echo "starting the example on a virtual display"
$SSH "pkill Xvnc; pkill -f main.py; true" >/dev/null 2>&1 || true
# Xvnc rather than Xvfb and x11vnc: it is a full X server, which GTK4 needs -
# Xvfb is spare enough that GTK4 falls over on it - and it serves the display
# itself, so there is nothing to attach afterwards.
$SSH "daemon -f Xvnc :99 -geometry 480x440 -depth 24 -SecurityTypes None -AlwaysShared;
      sleep 3;
      cd /root/app && DISPLAY=:99 daemon -f -o /root/app/gtk.log $PYTHON main.py" >/dev/null 2>&1
sleep 6

if [ -n "$RECORD" ]; then
    echo "recording 20 frames"
    $SSH 'cd /root/app && rm -rf frames && mkdir frames &&
          for i in $(seq -w 1 20); do
              # By name, not the root window: the display is bigger than
              # the application, and the rest of it is black.
              DISPLAY=:99 magick import -window vero frames/f$i.png 2>/dev/null
              sleep 0.5
          done' >/dev/null 2>&1
    OUT=$(mktemp -d)
    scp -q -i "$VM/key" -P "$SSH_PORT" -o StrictHostKeyChecking=no \
        -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR \
        "root@127.0.0.1:/root/app/frames/*.png" "$OUT/"
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
