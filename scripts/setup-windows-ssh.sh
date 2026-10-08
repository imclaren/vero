#!/bin/sh
# Sets up SSH into vero's Windows VM, so that scripts - test-repo.sh --vm
# windows among them - can run commands there and copy files in, instead
# of typing at its screen. A VM installed with run-windows.sh --install
# has SSH already; this is for one installed before it did.
#
#   scripts/setup-windows-ssh.sh            # port 2222 on this Mac reaches the VM
#   scripts/setup-windows-ssh.sh --port 2223
#   scripts/setup-windows-ssh.sh --type       # types at the VM even if SSH works
#
# It boots the VM with no window, with a disc holding the virtio drivers
# (without which Windows on ARM has no network in qemu), Microsoft's
# OpenSSH for Windows and your key, ~/.ssh/vero-windows, which it makes
# the first time. Then it types one command at the VM, as an
# administrator: open the Run box, run the disc's setup-ssh.ps1, and say
# yes when Windows asks. It checks a screenshot before each step, waits
# until SSH answers, and shuts the VM down. Running it again does no harm.
#
# Afterwards, scripts/run-windows.sh --ssh 2222 starts the VM with SSH on
# that port:
#
#   ssh -i ~/.ssh/vero-windows -p 2222 vero@127.0.0.1
set -e
VERO=$(cd "$(dirname "$0")/.." && pwd)
. "$VERO/scripts/lib/windows-disc.sh"
PORT=2222 TYPE=""
while [ $# -gt 0 ]; do
    case $1 in
        --port) PORT=${2:?--port needs a number}; shift 2 ;;
        --type) TYPE=yes; shift ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
done
VM=${VM:-$HOME/vm/vero-windows}
[ -f "$VM/disk.qcow2" ] || { echo "no Windows VM in $VM: scripts/run-windows.sh --install makes one" >&2; exit 1; }
windows_running && { echo "the Windows VM is running already: stop it first" >&2; exit 1; }

DISC="$HOME/.cache/vero/windows/ssh-disc"
windows_ssh_disc "$DISC" "$VERO"
QMP=/tmp/vero-qmp.sock
Q="python3 $VERO/scripts/lib/qmp.py $QMP"
SHOTS="$HOME/.cache/vero/windows/setup-shots"
rm -rf "$SHOTS" && mkdir -p "$SHOTS"
n=0
# shot NAME: a screenshot, as PNG, for looking at when a step goes wrong.
shot() {
    n=$((n + 1))
    $Q shot "$SHOTS/$n.ppm" && sips -s format png "$SHOTS/$n.ppm" --out "$SHOTS/$n-$1.png" >/dev/null && rm "$SHOTS/$n.ppm"
}
# settled: waits until the screen stops changing, which is the desktop
# once Windows has logged in.
settled() {
    prev="" same=0 i=0
    while [ $i -lt 120 ]; do
        $Q shot "$SHOTS/now.ppm" 2>/dev/null || true
        h=$(md5 -q "$SHOTS/now.ppm" 2>/dev/null || echo none)
        if [ "$h" = "$prev" ]; then same=$((same + 1)); else same=0; fi
        prev=$h
        [ $same -ge 4 ] && return 0
        sleep 5; i=$((i + 1))
    done
    return 1
}
stop() { windows_stop; }

echo "booting the VM with the setup disc (about a minute)"
"$VERO/scripts/run-windows.sh" --headless --payload "$DISC" --ssh "$PORT" >"$SHOTS/qemu.log" 2>&1 &
sleep 10
if [ -z "$TYPE" ] && windows_wait "$PORT" 60; then
    echo "SSH answers already; making sure it's all set up"
    windows_ssh "$PORT" "powershell -NoProfile -ExecutionPolicy Bypass -File D:/vero/setup-ssh.ps1" >/dev/null 2>&1 || true
else
    settled || { shot desktop; echo "the VM's screen never settled: see $SHOTS" >&2; stop; exit 1; }
    shot desktop
    # The display may be asleep: a key wakes it. Then the Run box, the
    # command, and Ctrl+Shift+Enter, which runs it as an administrator.
    # The VM's keyboard is a UK one, so the command has no \ " @ or |.
    $Q keys shift; sleep 1
    $Q keys meta_l-r; sleep 2
    $Q text "powershell -ExecutionPolicy Bypass -File D:/vero/setup-ssh.ps1"; sleep 1
    shot run-box
    $Q keys ctrl-shift-ret; sleep 4
    shot asked
    # Windows asks whether PowerShell may make changes: yes.
    $Q keys alt-y
    echo "setting up the drivers and SSH in the VM"
    windows_wait "$PORT" 300 || { shot failed; echo "SSH never answered: see $SHOTS, and C:\\vero-ssh.log in the VM" >&2; stop; exit 1; }
    # Done when the script says so: SSH may answer before it finishes.
    sleep 15
    i=0
    until windows_ssh "$PORT" "if (Test-Path C:/vero-ssh-done.txt) { exit 0 } else { exit 1 }" >/dev/null 2>&1; do
        i=$((i + 1)); [ $i -lt 60 ] || { echo "the setup never finished: see C:\\vero-ssh.log in the VM" >&2; stop; exit 1; }
        sleep 5
    done
fi
windows_ssh "$PORT" "Get-Content C:/vero-ssh.log"
echo "SSH works: ssh -i $WINDOWS_KEY -p $PORT vero@127.0.0.1"
echo "shutting the VM down"
stop
