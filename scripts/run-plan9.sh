#!/bin/sh
# Runs the Plan 9 example in a VM on your Mac.
#
#   scripts/run-plan9.sh                   the libdraw app, in a rio window
#   scripts/run-plan9.sh --record out.gif  the same, recorded, no window
#   scripts/run-plan9.sh --test            vero's test suite, on a text console
#
# Needs qemu:  scripts/setup-plan9.sh
#
# Plan 9 runs on x86, so on an Apple Silicon Mac this is emulation rather than
# virtualisation: about a minute to boot, and the app takes a moment to draw.
# There is no install.  9front boots from its ISO into rio, the fetch is over
# qemu's network from a server this script starts, and nothing persists.
#
# The app is example/plan9-app: Go, drawing through /dev/draw with a Go port
# of libdraw, supervising example/worker beside it as a process.
set -e

ROOT=$(cd "$(dirname "$0")/.." && pwd)
VM=${VM:-$HOME/vm/vero-plan9}
ISO=$VM/9front.iso
QMP=/tmp/vero-p9.sock            # short on purpose: unix paths cap at 104 bytes
PORT=${PORT:-8123}
RELEASE=${RELEASE:-9front-11952.amd64}
MODE=app
RECORD=""
while [ $# -gt 0 ]; do
    case $1 in
        --test)   MODE=test; shift ;;
        --record) RECORD=${2:?--record needs a filename}; shift 2 ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
done

command -v qemu-system-x86_64 >/dev/null || {
    echo "qemu is needed - scripts/setup-plan9.sh" >&2; exit 1; }

mkdir -p "$VM"
if [ ! -f "$ISO" ]; then
    echo "==> downloading $RELEASE (about 240MB, from a slow server)"
    curl -# -o "$ISO.gz" "https://9front.org/iso/$RELEASE.iso.gz"
    gunzip -f "$ISO.gz"
fi

if [ "$MODE" = test ]; then
    echo "==> building the tests for plan9/amd64"
    ( cd "$ROOT" && GOOS=plan9 GOARCH=amd64 go test -c -o "$VM/vero.test" . )
else
    echo "==> building the worker and the app for plan9/amd64"
    ( cd "$ROOT" && GOOS=plan9 GOARCH=amd64 go build -o "$VM/worker" ./example/worker )
    ( cd "$ROOT/example/plan9-app" && GOOS=plan9 GOARCH=amd64 go build -o "$VM/plan9-app" . )
fi

# The guest has no Go toolchain, so binaries arrive over the network: qemu's
# user networking puts this Mac at 10.0.2.2.
pkill -f "http.server $PORT" 2>/dev/null || true
python3 -m http.server "$PORT" --bind 0.0.0.0 --directory "$VM" >/dev/null 2>&1 &
HTTP=$!
trap 'kill $HTTP 2>/dev/null; kill $VMPID 2>/dev/null' EXIT INT TERM

# A window to watch, unless recording.  The VM has a PS/2 mouse rather than a
# tablet, which matters below: the pointer is moved with relative events.
DISPLAY_ARGS="-display default"
[ -n "$RECORD" ] || [ "$MODE" = test ] && DISPLAY_ARGS="-display none"

echo "==> booting 9front"
rm -f "$QMP"
qemu-system-x86_64 -accel tcg -m 2048 -smp 2 \
    -cdrom "$ISO" -boot d \
    -device rtl8139,netdev=n0 -netdev user,id=n0 \
    $DISPLAY_ARGS -vga std \
    -qmp unix:"$QMP",server,nowait > "$VM/console.log" 2>&1 &
VMPID=$!

python3 - "$QMP" "$PORT" "$MODE" "$VM" <<'PY'
import json, socket, sys, time

sock, port, mode, vm = sys.argv[1:5]

KEYS = {' ': 'spc', '\n': 'ret', '-': 'minus', '.': 'dot', '/': 'slash', ';': 'semicolon'}
SHIFT = {':': 'semicolon', '+': 'equal', '>': 'dot', '_': 'minus'}

class QMP:
    def __init__(self, path):
        for _ in range(60):
            try:
                self.s = socket.socket(socket.AF_UNIX); self.s.connect(path); break
            except OSError:
                time.sleep(1)
        else:
            raise SystemExit("qemu never opened its control socket")
        self.f = self.s.makefile('rw')
        self.f.readline()
        self.cmd('qmp_capabilities')

    def cmd(self, name, **args):
        self.f.write(json.dumps({'execute': name, 'arguments': args}) + '\n')
        self.f.flush()
        while True:
            reply = json.loads(self.f.readline())
            if 'event' not in reply:
                return reply

    def key(self, name, shift=False):
        keys = ([{'type': 'qcode', 'data': 'shift'}] if shift else []) + \
               [{'type': 'qcode', 'data': name}]
        for k in keys:
            self.cmd('input-send-event', events=[{'type': 'key', 'data': {'down': True, 'key': k}}])
        for k in reversed(keys):
            self.cmd('input-send-event', events=[{'type': 'key', 'data': {'down': False, 'key': k}}])

    def type(self, text):
        for ch in text:
            if ch in SHIFT:
                self.key(SHIFT[ch], shift=True)
            elif ch in KEYS:
                self.key(KEYS[ch])
            elif ch.isupper():
                self.key(ch.lower(), shift=True)
            else:
                self.key(ch)
            time.sleep(0.03)

    def move(self, dx, dy, steps):
        # Relative, a few pixels at a time: a PS/2 mouse has no absolute
        # position, and 9front moves the pointer two pixels per unit.
        for _ in range(steps):
            self.cmd('input-send-event', events=[
                {'type': 'rel', 'data': {'axis': 'x', 'value': dx}},
                {'type': 'rel', 'data': {'axis': 'y', 'value': dy}}])
            time.sleep(0.02)

    def click(self):
        self.cmd('input-send-event', events=[{'type': 'btn', 'data': {'down': True, 'button': 'left'}}])
        time.sleep(0.15)
        self.cmd('input-send-event', events=[{'type': 'btn', 'data': {'down': False, 'button': 'left'}}])

    def screen(self, path):
        self.cmd('screendump', filename=path)

q = QMP(sock)

# 9front asks four or five questions before it reaches a shell: the file
# server, the user, the screen size, and - in graphics - the monitor and the
# mouse.  The waits are generous because this is emulated.
time.sleep(45); q.type("\n")          # the default file server: the CD
time.sleep(25); q.type("\n")          # the default user: glenda
time.sleep(40)
if mode == "test":
    q.type("text\n")                  # no window system
    time.sleep(25)
    q.type("ip/ipconfig\n")
    time.sleep(12)
    q.type("hget http://10.0.2.2:%s/vero.test > /tmp/vero.test\n" % port)
    time.sleep(50)
    q.type("chmod +x /tmp/vero.test\n")
    time.sleep(5)
    print("==> running the suite (a couple of minutes under emulation)")
    q.type("/tmp/vero.test\n")
    time.sleep(90)
    q.screen("/tmp/vero-plan9.ppm")
    raise SystemExit(0)

q.type("\n")                          # 1024x768x16
time.sleep(10); q.type("\n")          # vesa
time.sleep(10); q.type("\n")          # ps2
time.sleep(60)                        # rio

# rio sends the keyboard to the window that was clicked last, so click the
# rc window: it opens at the top left, below the stats, and the pointer
# starts at the corner.
q.move(10, 9, 15)
q.click()
time.sleep(1)
q.type("ip/ipconfig\n")
time.sleep(12)
q.type("hget http://10.0.2.2:%s/worker > /tmp/worker\n" % port)
time.sleep(50)
q.type("hget http://10.0.2.2:%s/plan9-app > /tmp/app\n" % port)
time.sleep(50)
q.type("chmod +x /tmp/worker /tmp/app\n")
time.sleep(3)
q.type("cd /tmp; ./app worker\n")
time.sleep(30)

if sys.argv[3] == "app":
    import os
    frames = os.path.join(vm, "frames")
    os.makedirs(frames, exist_ok=True)
    for f in os.listdir(frames):
        os.remove(os.path.join(frames, f))
    for i in range(1, 21):
        q.screen(os.path.join(frames, "f%02d.ppm" % i))
        time.sleep(0.4)
PY

if [ "$MODE" = test ]; then
    sips -s format png /tmp/vero-plan9.ppm --out "$VM/result.png" >/dev/null 2>&1 || true
    echo
    echo "the console, as the suite finished:  $VM/result.png"
    command -v open >/dev/null && open "$VM/result.png"
    exit 0
fi

if [ -n "$RECORD" ]; then
    for f in "$VM"/frames/*.ppm; do
        sips -s format png "$f" --out "${f%.ppm}.png" >/dev/null 2>&1
    done
    # The rc window, where the app draws: at 33,130 in a 1024x768 screen,
    # less the scrollbar rio keeps down its left edge.
    ffmpeg -loglevel error -y -framerate 2 -i "$VM/frames/f%02d.png" \
        -vf "crop=584:300:48:130,scale=480:-1:flags=lanczos,pad=480:252:0:(oh-ih)/2:white,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse" \
        -loop 0 "$RECORD"
    echo "wrote $RECORD"
    exit 0
fi

echo "the app is running in the qemu window.  Close it to stop."
wait $VMPID
