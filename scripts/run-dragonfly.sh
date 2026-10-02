#!/bin/sh
# Runs the GTK example on DragonFly, in a VM on your Mac.
#
#   scripts/run-dragonfly.sh              # watch it, live, over VNC
#   scripts/run-dragonfly.sh --record out.gif
#   scripts/run-dragonfly.sh --no-open    # start it but do not open a viewer
#   scripts/run-dragonfly.sh --shell      # a shell in the VM, for poking about
#   scripts/run-dragonfly.sh --reset      # throw the VM away and start again
#
# Needs qemu:  scripts/setup-dragonfly.sh
#
# DragonFly is x86 only, so on an Apple Silicon Mac this is emulation rather
# than virtualisation: it boots in about a minute and installing GTK4 the
# first time takes the best part of an hour.  Everything after that is quick
# enough to watch.
#
# The same app as Linux, FreeBSD, NetBSD and OpenBSD, unchanged.
set -e

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

VM=${VM:-$HOME/vm/vero-dragonfly}
RELEASE=${RELEASE:-6.4.2}
PORT=${PORT:-5905}          # one past OpenBSD's
SSH_PORT=${SSH_PORT:-2225}
PYTHON=${PYTHON:-/usr/local/bin/python3.11}
PACKAGES=${PACKAGES:-"gtk4 py311-pygobject tigervnc-server ImageMagick7"}
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

command -v qemu-system-x86_64 >/dev/null 2>&1 || {
    echo "qemu is missing - scripts/setup-dragonfly.sh" >&2; exit 1; }
mkdir -p "$VM"

if [ ! -f "$VM/base.img" ]; then
    echo "downloading DragonFly $RELEASE (once; about 270MB, and 1.9GB unpacked)"
    curl -fL "https://mirror-master.dragonflybsd.org/iso-images/dfly-x86_64-${RELEASE}_REL.img.bz2" \
        -o "$VM/base.img.bz2"
    bunzip2 -f "$VM/base.img.bz2"
    mv "$VM/dfly-x86_64-${RELEASE}_REL.img" "$VM/base.img" 2>/dev/null || true
fi

[ "$RESET" = yes ] && rm -f "$VM/disk.qcow2" "$VM/local.qcow2"
FIRST_BOOT=no
if [ ! -f "$VM/disk.qcow2" ]; then
    qemu-img create -f qcow2 -b "$VM/base.img" -F raw "$VM/disk.qcow2" 8G >/dev/null
    # The image's own filesystem has about 800MB free and GTK4 needs more, so
    # packages go on a disk of their own, mounted at /usr/local.
    qemu-img create -f qcow2 "$VM/local.qcow2" 8G >/dev/null
    FIRST_BOOT=yes
fi
[ -f "$VM/key" ] || ssh-keygen -q -t ed25519 -N "" -f "$VM/key" -C vero

SSH="ssh -i $VM/key -p $SSH_PORT -o StrictHostKeyChecking=no \
     -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o IdentitiesOnly=yes root@127.0.0.1"

if [ -f "$VM/qemu.pid" ] && kill -0 "$(cat "$VM/qemu.pid")" 2>/dev/null; then
    echo "stopping the VM that is already running"
    $SSH "shutdown -p now" >/dev/null 2>&1 || true
    n=0
    while kill -0 "$(cat "$VM/qemu.pid")" 2>/dev/null; do
        n=$((n + 1))
        [ $n -gt 60 ] && { kill "$(cat "$VM/qemu.pid")" 2>/dev/null; sleep 2; break; }
        sleep 1
    done
fi
rm -f "$VM/qemu.pid" "$VM/console.sock"

echo "building the DragonFly worker"
CGO_ENABLED=0 GOOS=dragonfly GOARCH=amd64 go build -o "$VM/worker" ./example/worker

# -vga none: without it DragonFly writes to the VGA console and nothing
# reaches the serial line this script drives the first boot over.
# ipv6=off: qemu offers an IPv6 route that goes nowhere and fetches hang.
qemu-system-x86_64 -accel tcg -smp 2 -m 2048 \
    -drive if=virtio,format=qcow2,file="$VM/disk.qcow2" \
    -drive if=virtio,format=qcow2,file="$VM/local.qcow2" \
    -netdev "user,id=n0,ipv6=off,hostfwd=tcp::$SSH_PORT-:22,hostfwd=tcp::$PORT-:5900" \
    -device virtio-net-pci,netdev=n0 \
    -display none -vga none \
    -chardev socket,id=con,path="$VM/console.sock",server=on,wait=off,logfile="$VM/console.log" \
    -serial chardev:con \
    -pidfile "$VM/qemu.pid" -daemonize

cleanup() {
    [ -f "$VM/qemu.pid" ] || return 0
    $SSH "shutdown -p now" >/dev/null 2>&1 || true
    n=0
    while kill -0 "$(cat "$VM/qemu.pid")" 2>/dev/null; do
        n=$((n + 1)); [ $n -gt 40 ] && { kill "$(cat "$VM/qemu.pid")" 2>/dev/null; break; }
        sleep 1
    done
    rm -f "$VM/qemu.pid"
}
trap cleanup EXIT INT TERM

# The live image has no way in: sshd is off and root has no password.  So the
# first boot is driven over the console - the only thing it is used for.
echo "first boot (emulated, so about a minute)"
python3 - "$VM/console.sock" "$(cat "$VM/key.pub")" "$FIRST_BOOT" <<'PY'
import socket, sys, time

sock, pubkey, first = sys.argv[1], sys.argv[2], sys.argv[3] == "yes"
s = socket.socket(socket.AF_UNIX)
for _ in range(90):
    try:
        s.connect(sock); break
    except OSError:
        time.sleep(1)
else:
    raise SystemExit("qemu never opened its console socket")
s.settimeout(0.5)

def drain(seconds):
    end, out = time.time() + seconds, b""
    while time.time() < end:
        try:
            out += s.recv(4096)
        except socket.timeout:
            pass
    return out.decode("utf-8", "replace")

def send(line, wait=8):
    # A character at a time: this console drops input on long lines, and the
    # kernel says so afterwards - "sio0: N more interrupt-level buffer
    # overflows" - by which point the command has already run truncated.
    for ch in line:
        s.sendall(ch.encode()); time.sleep(0.004)
    s.sendall(b"\n")
    return drain(wait)

deadline, seen = time.time() + 300, ""
while time.time() < deadline:
    seen += drain(5)
    if "login:" in seen:
        break
else:
    raise SystemExit("DragonFly never reached a login prompt")
send("root", 6)

# root's shell is csh, which has no 2>&1 - every command here goes through sh.
if first:
    send("sh -c 'newfs /dev/vbd1s0 >/dev/null 2>&1 || newfs /dev/vbd1'", 90)
    send("sh -c 'mount /dev/vbd1s0 /mnt'", 15)
    # /usr/local already holds pkg itself, so move it in rather than over it.
    send("sh -c 'cd /usr/local && tar cf - . | (cd /mnt && tar xf -)'", 120)
    send("sh -c 'umount /mnt'", 15)
send("sh -c 'mount /dev/vbd1s0 /usr/local'", 20)
send("sh -c 'dhclient vtnet0 >/dev/null 2>&1'", 30)

send("sh -c 'mkdir -p /root/.ssh'", 6)
send("sh -c \"echo '%s' > /root/.ssh/authorized_keys\"" % pubkey, 10)
send("sh -c 'chmod 700 /root/.ssh'", 6)
send("sh -c 'grep -q sshd_enable /etc/rc.conf || echo sshd_enable=YES >> /etc/rc.conf'", 8)
out = send("rcstart sshd", 60)
print("   ", "sshd started" if "Starting sshd" in out else out.strip()[-80:])
PY

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

if ! $SSH "pkg info -e py311-pygobject" >/dev/null 2>&1; then
    echo "installing GTK4 and Python in the VM (once; the best part of an hour, emulated)"
    $SSH "pkg install -y $PACKAGES" >/dev/null 2>&1 ||
        { echo "the packages did not install - the VM is still up, ssh in with:" >&2
          echo "  $SSH" >&2; trap - EXIT INT TERM; exit 1; }
fi

echo "starting the example on a virtual display"
# GSK_RENDERER=cairo: GTK 4 asks for Vulkan first and there is no GPU here.
# From a script, because a command backgrounded inside an ssh session dies
# with the session.
scp -q -i "$VM/key" -P "$SSH_PORT" -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o IdentitiesOnly=yes \
    /dev/stdin root@127.0.0.1:/root/run-app.sh <<APP
#!/bin/sh
pkill Xvnc 2>/dev/null
pkill -f main.py 2>/dev/null
sleep 1
/usr/local/bin/Xvnc :99 -geometry 480x440 -depth 24 -SecurityTypes None -AlwaysShared \
    >/root/xvnc.log 2>&1 &
sleep 5
cd /root/app
DISPLAY=:99 GTK_A11Y=none GSK_RENDERER=cairo exec $PYTHON main.py
APP
$SSH "chmod +x /root/run-app.sh; rm -f /root/app/gtk.log;
      nohup /root/run-app.sh >/root/app/gtk.log 2>&1 </dev/null &" >/dev/null 2>&1
sleep 25

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
    # By name, not the root window: the display is bigger than the window.
    DISPLAY=:99 /usr/local/bin/magick import -window vero frames/f$n.png 2>/dev/null
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
