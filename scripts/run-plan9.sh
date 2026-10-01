#!/bin/sh
# Runs vero's test suite on Plan 9, in a VM.
#
#   scripts/run-plan9.sh            download 9front if needed, boot, run the tests
#   scripts/run-plan9.sh --shell    boot and stop at the shell, for poking about
#
# There is no example application here: Plan 9 draws through libdraw, and
# nothing in this repository speaks it.  What this checks is the half that
# does exist - the worker, the supervisor, and the lock, which on Plan 9 is
# the exclusive-use bit rather than flock.
#
# Plan 9 runs on x86, so on an Apple Silicon Mac this is emulation rather than
# virtualisation.  It is slower than the other VMs here - about a minute to
# boot, and a couple more for the suite - and it does work.
set -e

ROOT=$(cd "$(dirname "$0")/.." && pwd)
VM=${VM:-$HOME/vm/vero-plan9}
ISO=$VM/9front.iso
QMP=/tmp/vero-p9.sock            # short on purpose: unix paths cap at 104 bytes
PORT=${PORT:-8123}
RELEASE=${RELEASE:-9front-11952.amd64}
SHELL_ONLY=no
[ "$1" = "--shell" ] && SHELL_ONLY=yes

command -v qemu-system-x86_64 >/dev/null || {
    echo "qemu is needed: brew install qemu" >&2; exit 1; }

mkdir -p "$VM"
if [ ! -f "$ISO" ]; then
    echo "==> downloading $RELEASE (about 240MB, from a slow server)"
    curl -# -o "$ISO.gz" "https://9front.org/iso/$RELEASE.iso.gz"
    gunzip -f "$ISO.gz"
fi

echo "==> building the tests for plan9/amd64"
( cd "$ROOT" && GOOS=plan9 GOARCH=amd64 go test -c -o "$VM/vero.test" . )

# The guest has no Go toolchain, so the binary arrives over the network:
# qemu's user networking puts this Mac at 10.0.2.2.
pkill -f "http.server $PORT" 2>/dev/null || true
python3 -m http.server "$PORT" --bind 0.0.0.0 --directory "$VM" >/dev/null 2>&1 &
HTTP=$!
trap 'kill $HTTP 2>/dev/null; kill $VMPID 2>/dev/null' EXIT INT TERM

echo "==> booting 9front"
rm -f "$QMP"
qemu-system-x86_64 -accel tcg -m 2048 -smp 2 \
    -cdrom "$ISO" -boot d \
    -device rtl8139,netdev=n0 -netdev user,id=n0 \
    -display none -vga std \
    -qmp unix:"$QMP",server,nowait > "$VM/console.log" 2>&1 &
VMPID=$!

# Everything below is typed at the console, because 9front asks three
# questions before it reaches a shell and none of them can be answered from
# the command line.  The answers are: the default file server, the default
# user, and "text" - no window system, since there is nothing to look at.
python3 - "$QMP" "$PORT" "$SHELL_ONLY" <<'PY'
import json, socket, sys, time

sock, port, shell_only = sys.argv[1], sys.argv[2], sys.argv[3] == "yes"

# What the console needs typed, in qemu's names for the keys.  The second
# map is the ones that need shift held: a missing entry here types nothing at
# all, which is how a redirection quietly becomes a second argument.
KEYS = {' ': 'spc', '\n': 'ret', '-': 'minus', '.': 'dot', '/': 'slash'}
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
            self.cmd('input-send-event',
                     events=[{'type': 'key', 'data': {'down': True, 'key': k}}])
        for k in reversed(keys):
            self.cmd('input-send-event',
                     events=[{'type': 'key', 'data': {'down': False, 'key': k}}])

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

    def screen(self, path):
        self.cmd('screendump', filename=path)

q = QMP(sock)

# bootargs, user, vgasize.  The waits are generous because this is emulated.
time.sleep(45); q.type("\n")          # the default file server: the CD
time.sleep(25); q.type("\n")          # the default user: glenda
time.sleep(40); q.type("text\n")      # no window system
time.sleep(25)

q.type("ip/ipconfig\n")
time.sleep(12)
q.type("hget http://10.0.2.2:%s/vero.test > /tmp/vero.test\n" % port)
time.sleep(50)
q.type("chmod +x /tmp/vero.test\n")
time.sleep(5)

if shell_only:
    print("9front is at a shell.  Attach with: qemu's monitor, or read "
          "$VM/console.log")
    raise SystemExit(0)

print("==> running the suite (a couple of minutes under emulation)")
q.type("/tmp/vero.test\n")
time.sleep(90)
q.screen("/tmp/vero-plan9.ppm")
PY

if [ "$SHELL_ONLY" = no ] && [ -f /tmp/vero-plan9.ppm ]; then
    # The guest has no way to hand a file back, so the result comes off the
    # screen: 9front writes it to the console, and this is a picture of it.
    sips -s format png /tmp/vero-plan9.ppm --out "$VM/result.png" >/dev/null 2>&1 || true
    echo
    echo "the console, as the suite finished:  $VM/result.png"
    command -v open >/dev/null && open "$VM/result.png"
fi
