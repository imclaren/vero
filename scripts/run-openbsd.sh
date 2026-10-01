#!/bin/sh
# Runs the GTK example on OpenBSD, in a VM on your Mac.
#
#   scripts/run-openbsd.sh              # watch it, live, over VNC
#   scripts/run-openbsd.sh --record out.gif
#   scripts/run-openbsd.sh --no-open    # start it but do not open a viewer
#   scripts/run-openbsd.sh --shell      # a shell in the VM, for poking about
#   scripts/run-openbsd.sh --reset      # throw the VM away and install again
#
# Needs qemu:  scripts/setup-openbsd.sh
#
# OpenBSD publishes an installer rather than a ready-made disk image, so the
# first run installs it: autoinstall(8) reads a response file this script
# writes and serves, and answers every question from it.  That takes about
# ten minutes, once.  After it the VM boots in seconds.
#
# The same app as Linux, FreeBSD and NetBSD, unchanged.
set -e

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

VM=${VM:-$HOME/vm/vero-openbsd}
RELEASE=${RELEASE:-7.9}
PORT=${PORT:-5904}          # one past NetBSD's
SSH_PORT=${SSH_PORT:-2224}
HTTP_PORT=${HTTP_PORT:-8124}
MIRROR=${MIRROR:-cdn.openbsd.org}
PACKAGES=${PACKAGES:-"gtk+4 py3-gobject3 tigervnc ImageMagick"}
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

command -v qemu-system-aarch64 >/dev/null 2>&1 || {
    echo "qemu is missing - scripts/setup-openbsd.sh" >&2; exit 1; }
FW=$(brew --prefix)/share/qemu
mkdir -p "$VM"
VERSION=$(echo "$RELEASE" | tr -d .)

if [ ! -f "$VM/install$VERSION.img" ]; then
    echo "downloading the OpenBSD $RELEASE installer (once; about 630MB)"
    curl -fL "https://$MIRROR/pub/OpenBSD/$RELEASE/arm64/install$VERSION.img" \
        -o "$VM/install$VERSION.img"
fi

[ "$RESET" = yes ] && rm -f "$VM/disk.qcow2" "$VM/vars.fd"
INSTALL=no
if [ ! -f "$VM/disk.qcow2" ]; then
    qemu-img create -f qcow2 "$VM/disk.qcow2" 12G >/dev/null
    INSTALL=yes
fi
[ -f "$VM/vars.fd" ] || cp "$FW/edk2-arm-vars.fd" "$VM/vars.fd"
[ -f "$VM/key" ] || ssh-keygen -q -t ed25519 -N "" -f "$VM/key" -C vero

SSH="ssh -i $VM/key -p $SSH_PORT -o StrictHostKeyChecking=no \
     -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o IdentitiesOnly=yes root@127.0.0.1"

if [ -f "$VM/qemu.pid" ] && kill -0 "$(cat "$VM/qemu.pid")" 2>/dev/null; then
    echo "stopping the VM that is already running"
    $SSH "halt -p" >/dev/null 2>&1 || true
    n=0
    while kill -0 "$(cat "$VM/qemu.pid")" 2>/dev/null; do
        n=$((n + 1))
        [ $n -gt 45 ] && { kill "$(cat "$VM/qemu.pid")" 2>/dev/null; sleep 2; break; }
        sleep 1
    done
fi
rm -f "$VM/qemu.pid" "$VM/console.sock"

echo "building the OpenBSD worker"
CGO_ENABLED=0 GOOS=openbsd GOARCH=arm64 go build -o "$VM/worker" ./example/worker

# The response file autoinstall(8) reads.  It has to be served over HTTP:
# the installer fetches it by URL, and qemu's user networking puts this Mac
# at 10.0.2.2.  Written every run, because it carries the ssh key.
cat > "$VM/auto_install.conf" <<EOF
System hostname = vero
Password for root account = $(openssl rand -base64 18)
Public ssh key for root account = $(cat "$VM/key.pub")
Allow root ssh login = prohibit-password
Start sshd(8) by default = yes
Do you expect to run the X Window System = no
Change the default console to com0 = yes
Which speed should com0 use = 115200
Setup a user = no
What timezone are you in = UTC
Which disk is the root disk = sd0
Use (W)hole disk MBR, whole disk (G)PT or (E)dit = whole
Use (A)uto layout, (E)dit auto layout, or create (C)ustom layout = auto
Which network interface do you wish to configure = done
IPv4 address for vio0 = dhcp
IPv6 address for vio0 = none
Location of sets = http
HTTP Server = $MIRROR
Server directory = pub/OpenBSD/$RELEASE/arm64
Set name(s) = done
Directory does not contain SHA256.sig. Continue without verification = yes
EOF

pkill -f "http.server $HTTP_PORT" 2>/dev/null || true
python3 -m http.server "$HTTP_PORT" --bind 0.0.0.0 --directory "$VM" >/dev/null 2>&1 &
HTTP=$!

boot() {   # $1: extra -drive for the installer
    # ipv6=off: qemu offers an IPv6 default route that goes nowhere, and a
    # resolver that prefers AAAA records then waits for a timeout on every
    # fetch - which makes pkg_add look like it has hung.
    qemu-system-aarch64 \
        -machine virt,highmem=on -accel hvf -cpu host -smp 4 -m 2048 \
        -drive if=pflash,format=raw,readonly=on,file="$FW/edk2-aarch64-code.fd" \
        -drive if=pflash,format=raw,file="$VM/vars.fd" \
        -drive if=virtio,format=qcow2,file="$VM/disk.qcow2" \
        $1 \
        -device virtio-gpu-pci -device qemu-xhci -device usb-kbd -device usb-tablet \
        -netdev "user,id=n0,ipv6=off,hostfwd=tcp::$SSH_PORT-:22,hostfwd=tcp::$PORT-:5900" \
        -device virtio-net-pci,netdev=n0 \
        -display none \
        -serial unix:"$VM/console.sock",server,nowait \
        -pidfile "$VM/qemu.pid" -daemonize
}

cleanup() {
    kill $HTTP 2>/dev/null || true
    [ -f "$VM/qemu.pid" ] || return 0
    $SSH "halt -p" >/dev/null 2>&1 || true
    n=0
    while kill -0 "$(cat "$VM/qemu.pid")" 2>/dev/null; do
        n=$((n + 1)); [ $n -gt 30 ] && { kill "$(cat "$VM/qemu.pid")" 2>/dev/null; break; }
        sleep 1
    done
    rm -f "$VM/qemu.pid"
}
trap cleanup EXIT INT TERM

if [ "$INSTALL" = yes ]; then
    echo "installing OpenBSD (once; about ten minutes)"
    boot "-drive if=virtio,format=raw,file=$VM/install$VERSION.img"
    python3 - "$VM/console.sock" "$HTTP_PORT" <<'PY'
import socket, sys, time

sock, port = sys.argv[1], sys.argv[2]
s = socket.socket(socket.AF_UNIX)
for _ in range(60):
    try:
        s.connect(sock); break
    except OSError:
        time.sleep(1)
else:
    raise SystemExit("qemu never opened its console socket")
s.settimeout(1)

def until(text, timeout, nudge=None):
    end, seen = time.time() + timeout, ""
    while time.time() < end:
        try:
            seen += s.recv(4096).decode('utf-8', 'replace')
        except socket.timeout:
            if nudge:
                s.sendall(nudge)
            continue
        if text in seen:
            return seen
    raise SystemExit("OpenBSD never said %r" % text)

# The bootloader waits five seconds for a command; a newline starts it.
until("boot>", 180, nudge=b"")
s.sendall(b"\n")

# The installer's first question, and the only two answers this needs: the
# rest of them are in the response file.
until("(A)utoinstall", 600, nudge=b"\n")
time.sleep(1)
s.sendall(b"a\n")
until("Response file location?", 120)
time.sleep(1)
s.sendall(b"http://10.0.2.2:%s/auto_install.conf\n" % port.encode())

until("CONGRATULATIONS", 1800)
print("    installed; rebooting")
until("login:", 600)
PY
    # It reboots into the installed system by itself; the installer image is
    # not attached again, so nothing can boot from it by accident.
    kill "$(cat "$VM/qemu.pid")" 2>/dev/null || true
    sleep 3
    rm -f "$VM/qemu.pid" "$VM/console.sock"
fi

echo "booting the VM"
boot ""

printf "waiting for ssh"
n=0
until $SSH true 2>/dev/null; do
    n=$((n + 1))
    [ $n -gt 90 ] && { echo; echo "the VM never came up" >&2; exit 1; }
    printf "."
    sleep 2
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
    kill $HTTP 2>/dev/null || true
    echo "the VM is up.  ssh in with:"
    echo "  $SSH"
    echo "stop it with:  kill \$(cat $VM/qemu.pid)"
    exit 0
fi

if ! $SSH "pkg_info -e py3-gobject3" >/dev/null 2>&1; then
    echo "installing GTK4 and Python in the VM (once; a few minutes)"
    $SSH "pkg_add -I $PACKAGES" >/dev/null 2>&1 ||
        { echo "the packages did not install - the VM is still up, ssh in with:" >&2
          echo "  $SSH" >&2; trap - EXIT INT TERM; exit 1; }
fi

echo "starting the example on a virtual display"
# GSK_RENDERER=cairo: GTK 4.22 asks for Vulkan first, there is no GPU in the
# VM, and on OpenBSD it gives up rather than falling back.
$SSH "cat > /root/run-app.sh" <<'APP'
#!/bin/sh
pkill Xvnc 2>/dev/null
pkill -f main.py 2>/dev/null
sleep 1
/usr/local/bin/Xvnc :99 -geometry 480x440 -depth 24 -SecurityTypes None -AlwaysShared \
    >/root/xvnc.log 2>&1 &
sleep 4
cd /root/app
DISPLAY=:99 GTK_A11Y=none GSK_RENDERER=cairo exec /usr/local/bin/python3 main.py
APP
$SSH "chmod +x /root/run-app.sh; rm -f /root/app/gtk.log;
      nohup /root/run-app.sh >/root/app/gtk.log 2>&1 </dev/null &" >/dev/null 2>&1
sleep 12

if [ -n "$RECORD" ]; then
    echo "recording 20 frames"
    # import, not magick: OpenBSD packages ImageMagick 6.
    $SSH 'cd /root/app && rm -rf frames && mkdir frames && i=1
          while [ $i -le 20 ]; do
              n=$(printf "%02d" $i)
              DISPLAY=:99 /usr/local/bin/import -window vero frames/f$n.png 2>/dev/null
              sleep 0.4
              i=$((i + 1))
          done' >/dev/null 2>&1
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
    kill $HTTP 2>/dev/null || true
    echo "the VM is up, VNC on $PORT.  stop it with:  kill \$(cat $VM/qemu.pid)"
fi
