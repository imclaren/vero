#!/bin/sh
# Runs the GTK example on illumos, in a VM on your Mac.
#
#   scripts/run-illumos.sh              # watch it, live, over VNC
#   scripts/run-illumos.sh --record out.gif
#   scripts/run-illumos.sh --no-open    # start it but do not open a viewer
#   scripts/run-illumos.sh --shell      # a shell in the VM, for poking about
#   scripts/run-illumos.sh --install    # install OpenIndiana into a new disk
#
# Needs qemu:  scripts/setup-illumos.sh
#
# illumos is x86 only, so on an Apple Silicon Mac this is emulation rather
# than virtualisation.  The install takes about half an hour and is driven
# through OpenIndiana's text installer over the serial console; after it, the
# VM boots in a couple of minutes and runs the same application the BSDs run,
# unchanged.
#
# Three things about this VM are not obvious, and all three cost hours to
# find:
#
#   * It needs a VGA device.  With -vga none the kernel resets in a loop
#     before printing anything at all.
#   * The kernel only talks to the serial port when the loader is told to put
#     the console there, which is a menu item, per boot.
#   * The installer finishes, reboots, and leaves a system that cannot boot:
#     no bootloader and no boot archive.  Both are written here afterwards,
#     from the install media, which is what --install does at the end.
set -e

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

VM=${VM:-$HOME/vm/vero-illumos}
RELEASE=${RELEASE:-20260430}
PORT=${PORT:-5906}
SSH_PORT=${SSH_PORT:-2226}
PYTHON=${PYTHON:-/usr/bin/python3.9}
PACKAGES=${PACKAGES:-"library/desktop/gtk4 library/python/pygobject-39 \
library/python/pycairo-39 library/desktop/gobject/gobject-introspection \
x11/server/xvnc image/imagemagick system/font/truetype/dejavu"}
RECORD=""
OPEN=yes
SHELL_ONLY=no
INSTALL=no
while [ $# -gt 0 ]; do
    case $1 in
        --record)  RECORD=${2:?--record needs a filename}; shift 2 ;;
        --no-open) OPEN=no; shift ;;
        --shell)   SHELL_ONLY=yes; shift ;;
        --install) INSTALL=yes; shift ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
done

command -v qemu-system-x86_64 >/dev/null 2>&1 || {
    echo "qemu is missing - scripts/setup-illumos.sh" >&2; exit 1; }
mkdir -p "$VM"
[ -f "$VM/key" ] || ssh-keygen -q -t ed25519 -N "" -f "$VM/key" -C vero

SSH="ssh -i $VM/key -p $SSH_PORT -o StrictHostKeyChecking=no \
     -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o IdentitiesOnly=yes root@127.0.0.1"

if [ ! -f "$VM/disk.qcow2" ] && [ "$INSTALL" = no ]; then
    echo "no VM yet.  Install one first, which takes about half an hour:" >&2
    echo "  scripts/run-illumos.sh --install" >&2
    exit 1
fi

if [ "$INSTALL" = yes ]; then
    if [ ! -f "$VM/oi-text.iso" ]; then
        echo "downloading OpenIndiana (once; about 690MB)"
        curl -fL "https://dlc.openindiana.org/isos/hipster/$RELEASE/OI-hipster-text-$RELEASE.iso" \
            -o "$VM/oi-text.iso"
    fi
    rm -f "$VM/disk.qcow2"
    # 32G: an app with GTK and ffmpeg from pkgsrc fills 14G. The file
    # takes only what the VM uses. (The installer's crash on its Users
    # screen, once blamed on the size, was a race in reading Esc-2, which
    # illumos-install.py now sends as F2's own key code.)
    qemu-img create -f qcow2 "$VM/disk.qcow2" "${DISK_SIZE:-32G}" >/dev/null
fi

boot() {   # $1: extra arguments, e.g. the install media
    rm -f "$VM/console.sock"
    # -vga std, even though nothing looks at it: see the note at the top.
    qemu-system-x86_64 -accel tcg -cpu max -smp 2 -m 4096 \
        -drive if=virtio,format=qcow2,file="$VM/disk.qcow2" \
        $1 \
        -netdev "user,id=n0,ipv6=off,hostfwd=tcp::$SSH_PORT-:22,hostfwd=tcp::$PORT-:5900" \
        -device virtio-net-pci,netdev=n0 \
        -display none -vga std \
        -chardev socket,id=con,path="$VM/console.sock",server=on,wait=off,logfile="$VM/console.log" \
        -serial chardev:con \
        -pidfile "$VM/qemu.pid" -daemonize
}

stop() {
    [ -f "$VM/qemu.pid" ] || return 0
    $SSH "poweroff" >/dev/null 2>&1 || true
    n=0
    while kill -0 "$(cat "$VM/qemu.pid")" 2>/dev/null; do
        n=$((n + 1)); [ $n -gt 40 ] && { kill "$(cat "$VM/qemu.pid")" 2>/dev/null; break; }
        sleep 1
    done
    rm -f "$VM/qemu.pid"
}
stop
trap stop EXIT INT TERM

echo "building the illumos worker"
CGO_ENABLED=0 GOOS=illumos GOARCH=amd64 go build -o "$VM/worker" ./example/worker

if [ "$INSTALL" = yes ]; then
    echo "installing OpenIndiana (about half an hour, emulated)"
    boot "-drive if=ide,media=cdrom,file=$VM/oi-text.iso -boot d"
    python3 "$ROOT/scripts/lib/illumos-install.py" "$VM/console.sock" "$(cat "$VM/key.pub")"
    stop
    echo "installed"
fi

echo "booting the VM"
boot ""

printf "waiting for ssh"
n=0
until $SSH true 2>/dev/null; do
    n=$((n + 1))
    [ $n -gt 180 ] && { echo; echo "the VM never came up - see $VM/console.log" >&2; exit 1; }
    printf "."
    sleep 3
done
echo

echo "copying the example in"
$SSH "mkdir -p /root/app" >/dev/null
scp -q -i "$VM/key" -P "$SSH_PORT" -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o IdentitiesOnly=yes \
    "$VM/worker" "$ROOT/example/gtk-app/main.py" "$ROOT/bindings/python/vero.py" \
    root@127.0.0.1:/root/app/
$SSH "chmod +x /root/app/worker /root/app/main.py" >/dev/null

if [ "$SHELL_ONLY" = yes ]; then
    trap - EXIT INT TERM
    echo "the VM is up.  ssh in with:"
    echo "  $SSH"
    echo "stop it with:  kill \$(cat $VM/qemu.pid)"
    exit 0
fi

# The font package is in the list for a reason: a text install has no fonts at
# all, every label then measures zero high, and GTK asks X to make a window of
# no height - which a strict server refuses with BadAlloc, three steps away
# from anything that mentions fonts.
if ! $SSH "pkg info library/python/pygobject-39" >/dev/null 2>&1; then
    echo "installing GTK4 and Python in the VM (once; the best part of an hour, emulated)"
    $SSH "pkg install -q $PACKAGES" >/dev/null 2>&1 ||
        { echo "the packages did not install - the VM is still up, ssh in with:" >&2
          echo "  $SSH" >&2; trap - EXIT INT TERM; exit 1; }
fi

echo "starting the example on a virtual display"
scp -q -i "$VM/key" -P "$SSH_PORT" -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o IdentitiesOnly=yes \
    /dev/stdin root@127.0.0.1:/root/run-app.sh <<APP
#!/bin/sh
pkill Xvnc 2>/dev/null
pkill -f main.py 2>/dev/null
sleep 1
/usr/bin/Xvnc :99 -geometry 1024x768 -depth 24 -SecurityTypes None -AlwaysShared \
    >/root/xvnc.log 2>&1 &
sleep 6
cd /root/app
DISPLAY=:99 GTK_A11Y=none GSK_RENDERER=cairo exec $PYTHON main.py
APP
$SSH "chmod +x /root/run-app.sh; rm -f /root/app/gtk.log;
      nohup /root/run-app.sh >/root/app/gtk.log 2>&1 </dev/null &" >/dev/null 2>&1
sleep 30

if [ -n "$RECORD" ]; then
    echo "recording 20 frames"
    scp -q -i "$VM/key" -P "$SSH_PORT" -o StrictHostKeyChecking=no \
        -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o IdentitiesOnly=yes \
        /dev/stdin root@127.0.0.1:/root/shoot.sh <<'SHOOT'
#!/bin/sh
cd /root/app
rm -rf frames; mkdir frames
i=1
while [ $i -le 20 ]; do
    n=$(printf "%02d" $i)
    DISPLAY=:99 /usr/bin/magick import -window vero frames/f$n.png 2>/dev/null
    sleep 0.4
    i=$((i + 1))
done
SHOOT
    $SSH "sh /root/shoot.sh" >/dev/null 2>&1
    OUT=$(mktemp -d)
    scp -q -i "$VM/key" -P "$SSH_PORT" -o StrictHostKeyChecking=no \
        -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o IdentitiesOnly=yes \
        "root@127.0.0.1:/root/app/frames/*.png" "$OUT/"
    ffmpeg -loglevel error -y -framerate 2 -i "$OUT/f%02d.png" \
        -vf "scale=480:-1:flags=lanczos,pad=480:252:0:(oh-ih)/2:white,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse" \
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
