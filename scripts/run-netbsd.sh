#!/bin/sh
# Runs the GTK example on NetBSD, in a VM on your Mac.
#
#   scripts/run-netbsd.sh              # watch it, live, over VNC
#   scripts/run-netbsd.sh --record out.gif
#   scripts/run-netbsd.sh --no-open    # start it but do not open a viewer
#   scripts/run-netbsd.sh --shell      # a shell in the VM, for poking about
#   scripts/run-netbsd.sh --reset      # throw the VM away and start again
#
# Needs qemu:  scripts/setup-netbsd.sh
#
# The first run downloads NetBSD's own arm64 image and installs GTK4 in it,
# which takes a few minutes.  After that the VM has both and boots in about a
# minute.
#
# The same app as Linux and FreeBSD, unchanged: GTK4 and the Python binding,
# driving a worker that was cross-compiled here.
set -e

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

VM=${VM:-$HOME/vm/vero-netbsd}
RELEASE=${RELEASE:-10.1}
PORT=${PORT:-5903}          # one past FreeBSD's
SSH_PORT=${SSH_PORT:-2223}
PYTHON=${PYTHON:-/usr/pkg/bin/python3.12}
PACKAGES=${PACKAGES:-"gtk4 py312-gobject3 tigervnc ImageMagick MesaLib"}
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
    echo "qemu is missing - scripts/setup-netbsd.sh" >&2; exit 1; }
FW=$(brew --prefix)/share/qemu
mkdir -p "$VM"

if [ ! -f "$VM/base.img" ]; then
    echo "downloading NetBSD $RELEASE (once; about 200MB)"
    # From the archive once a newer release has replaced it on the mirror.
    curl -fL "https://cdn.netbsd.org/pub/NetBSD/NetBSD-$RELEASE/evbarm-aarch64/binary/gzimg/arm64.img.gz" \
        -o "$VM/base.img.gz" ||
    curl -fL "https://archive.netbsd.org/pub/NetBSD-archive/NetBSD-$RELEASE/evbarm-aarch64/binary/gzimg/arm64.img.gz" \
        -o "$VM/base.img.gz"
    gunzip -f "$VM/base.img.gz"
    mv "$VM/arm64.img" "$VM/base.img" 2>/dev/null || mv "$VM/base.img.gz" "$VM/base.img" 2>/dev/null || true
    echo "made $VM/base.img"
fi

# The VM runs on an overlay, so the download stays pristine and a wrecked VM
# is thrown away with --reset rather than fetched again.  The firmware
# variables go with it: they remember which device the boot entry points at.
[ "$RESET" = yes ] && rm -f "$VM/disk.qcow2" "$VM/vars.fd"
FIRST_BOOT=no
if [ ! -f "$VM/disk.qcow2" ]; then
    qemu-img create -f qcow2 -b "$VM/base.img" -F raw "$VM/disk.qcow2" 12G >/dev/null
    FIRST_BOOT=yes
fi
[ -f "$VM/vars.fd" ] || cp "$FW/edk2-arm-vars.fd" "$VM/vars.fd"
[ -f "$VM/key" ] || ssh-keygen -q -t ed25519 -N "" -f "$VM/key" -C vero

SSH="ssh -i $VM/key -p $SSH_PORT -o StrictHostKeyChecking=no \
     -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o IdentitiesOnly=yes root@127.0.0.1"

if [ -f "$VM/qemu.pid" ] && kill -0 "$(cat "$VM/qemu.pid")" 2>/dev/null; then
    echo "stopping the VM that is already running"
    $SSH "shutdown -p now" >/dev/null 2>&1 || true
    n=0
    while kill -0 "$(cat "$VM/qemu.pid")" 2>/dev/null; do
        n=$((n + 1))
        [ $n -gt 45 ] && { kill "$(cat "$VM/qemu.pid")" 2>/dev/null; sleep 2; break; }
        sleep 1
    done
fi
rm -f "$VM/qemu.pid" "$VM/console.sock"

echo "building the NetBSD worker"
CGO_ENABLED=0 GOOS=netbsd GOARCH=arm64 go build -o "$VM/worker" ./example/worker

# ipv6=off matters.  qemu's user networking offers an IPv6 default route that
# goes nowhere, NetBSD's resolver prefers the AAAA record, and every fetch
# then waits for a timeout it will always get: pkg_add appears to hang.
qemu-system-aarch64 \
    -machine virt,highmem=on -accel hvf -cpu host -smp 4 -m 2048 \
    -drive if=pflash,format=raw,readonly=on,file="$FW/edk2-aarch64-code.fd" \
    -drive if=pflash,format=raw,file="$VM/vars.fd" \
    -drive if=virtio,format=qcow2,file="$VM/disk.qcow2" \
    -device virtio-gpu-pci -device qemu-xhci -device usb-kbd -device usb-tablet \
    -netdev "user,id=n0,ipv6=off,hostfwd=tcp::$SSH_PORT-:22,hostfwd=tcp::$PORT-:5900" \
    -device virtio-net-pci,netdev=n0 \
    -display none \
    -chardev socket,id=con,path="$VM/console.sock",server=on,wait=off,logfile="$VM/console.log" \
    -serial chardev:con \
    -pidfile "$VM/qemu.pid" -daemonize

cleanup() {
    [ -f "$VM/qemu.pid" ] || return 0
    $SSH "shutdown -p now" >/dev/null 2>&1 || true
    n=0
    while kill -0 "$(cat "$VM/qemu.pid")" 2>/dev/null; do
        n=$((n + 1)); [ $n -gt 30 ] && { kill "$(cat "$VM/qemu.pid")" 2>/dev/null; break; }
        sleep 1
    done
    rm -f "$VM/qemu.pid"
}
trap cleanup EXIT INT TERM

if [ "$FIRST_BOOT" = yes ]; then
    # The image has sshd running and root with no password, and no way in from
    # here: the key has to be put there from the console.  This is the only
    # thing the console is used for.
    echo "first boot: letting ourselves in over the console"
    python3 - "$VM/console.sock" "$(cat "$VM/key.pub")" <<'PY'
import socket, sys, time

sock, pubkey = sys.argv[1], sys.argv[2]
s = socket.socket(socket.AF_UNIX)
for _ in range(60):
    try:
        s.connect(sock); break
    except OSError:
        time.sleep(1)
else:
    raise SystemExit("qemu never opened its console socket")
s.settimeout(0.4)

def drain(seconds):
    end, out = time.time() + seconds, b""
    while time.time() < end:
        try:
            out += s.recv(4096)
        except socket.timeout:
            pass
    return out.decode('utf-8', 'replace')

def send(line, wait=2.0):
    s.sendall(line.encode() + b"\n")
    return drain(wait)

# Wait for the login prompt, which is the end of a two-minute first boot.
deadline, seen = time.time() + 300, ""
while time.time() < deadline:
    seen += drain(5)
    if "login:" in seen:
        break
else:
    raise SystemExit("NetBSD never reached a login prompt")

send("root", 4)
send("mkdir -p /root/.ssh", 2)
send("cat > /root/.ssh/authorized_keys <<'KEYEOF'", 1)
send(pubkey, 1)
send("KEYEOF", 2)
send("chmod 700 /root/.ssh; chmod 600 /root/.ssh/authorized_keys", 2)
send("sed -i -e 's/^#*PermitRootLogin.*/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config", 2)
out = send("/etc/rc.d/sshd restart", 5)
print("   ", out.strip().splitlines()[-1] if out.strip() else "sshd restarted")
PY
fi

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
    echo "the VM is up.  ssh in with:"
    echo "  $SSH"
    echo "stop it with:  kill \$(cat $VM/qemu.pid)"
    exit 0
fi

# GTK and Python, once.  MesaLib is in the list because GTK4 dlopens
# libGLESv2 and exits if it is not there, even with nothing to accelerate.
if ! $SSH "/usr/sbin/pkg_info -e py312-gobject3" >/dev/null 2>&1; then
    echo "installing GTK4 and Python in the VM (once; a few minutes)"
    $SSH "PKG_PATH=https://cdn.netbsd.org/pub/pkgsrc/packages/NetBSD/aarch64/$RELEASE/All/ \
          /usr/sbin/pkg_add -U $PACKAGES" >/dev/null 2>&1 ||
        { echo "the packages did not install - the VM is still up, ssh in with:" >&2
          echo "  $SSH" >&2; trap - EXIT INT TERM; exit 1; }
fi

echo "starting the example on a virtual display"
# From a file, and with nohup: a command backgrounded inside an ssh session
# dies with the session, and NetBSD has no daemon(8) to hand it to.
$SSH "cat > /root/run-app.sh" <<APP
#!/bin/sh
pkill Xvnc 2>/dev/null
pkill -f main.py 2>/dev/null
sleep 1
# Xvnc rather than Xvfb and x11vnc: a full X server, which GTK4 needs, and it
# serves the display itself, so there is nothing to attach afterwards.
/usr/pkg/bin/Xvnc :99 -geometry 480x440 -depth 24 -SecurityTypes None -AlwaysShared \
    >/root/xvnc.log 2>&1 &
sleep 4
cd /root/app
DISPLAY=:99 GTK_A11Y=none exec $PYTHON main.py
APP
$SSH "chmod +x /root/run-app.sh; rm -f /root/app/gtk.log;
      nohup /root/run-app.sh >/root/app/gtk.log 2>&1 </dev/null &" >/dev/null 2>&1
sleep 10

if [ -n "$RECORD" ]; then
    echo "recording 20 frames"
    $SSH 'cd /root/app && rm -rf frames && mkdir frames &&
          for i in $(jot 20 1); do
              n=$(printf "%02d" $i)
              # By name, not the root window: the display is bigger than the
              # application, and the rest of it is black.
              DISPLAY=:99 /usr/pkg/bin/magick import -window vero frames/f$n.png 2>/dev/null
              sleep 0.4
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
    echo "the VM is up, VNC on $PORT.  stop it with:  kill \$(cat $VM/qemu.pid)"
fi
