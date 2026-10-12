#!/bin/sh
# Gives vero's Windows VM a 1024 by 768 screen, once, so that an app's
# window has room in its recordings. The VM's firmware starts at 800 by 600,
# and Windows keeps whatever it started with:
#
#   1. In the firmware's setup (Device Manager, OVMF Platform Configuration),
#      the preferred resolution becomes 1024x768, the largest it offers,
#      chosen with key presses over QMP.
#   2. In Windows, the signed-in user's display is set to 1024 by 768 and
#      kept. Without this, Windows stretches its 800 by 600 to fill the
#      screen, and an app still has only 800 by 600 of room.
#
# The VM must not be running already. It needs SSH into the VM
# (scripts/setup-windows-ssh.sh).
#
#   scripts/setup-windows-display.sh
set -eu
VERO=$(cd "$(dirname "$0")/.." && pwd)
. "$VERO/scripts/lib/windows-disc.sh"
PORT=2230
windows_running && { echo "the Windows VM is running already: stop it first" >&2; exit 1; }
EMPTY=$(mktemp -d)
trap 'rm -rf "$EMPTY"' EXIT
qmp() { python3 "$VERO/scripts/lib/qmp.py" /tmp/vero-qmp.sock "$@" >/dev/null; }
key() { qmp keys "$1"; sleep 0.5; }
quit() {
    python3 - /tmp/vero-qmp.sock <<'PY'
import json, socket, sys
s = socket.socket(socket.AF_UNIX); s.connect(sys.argv[1]); f = s.makefile("rw"); f.readline()
for c in ("qmp_capabilities", "quit"):
    f.write(json.dumps({"execute": c}) + "\n"); f.flush(); f.readline()
PY
    while windows_running; do sleep 1; done
}

echo "== the firmware's preferred resolution"
rm -f /tmp/vero-qmp.sock
"$VERO/scripts/run-windows.sh" --headless --payload "$EMPTY" >/dev/null 2>&1 &
i=0; while [ ! -S /tmp/vero-qmp.sock ] && [ $i -lt 40 ]; do sleep 0.25; i=$((i + 1)); done
# Escape, while the firmware starts, opens its setup.
i=0; while [ $i -lt 30 ]; do qmp keys esc 2>/dev/null || true; sleep 0.3; i=$((i + 1)); done
sleep 1
key down; key ret                        # Device Manager
sleep 1
key down; key down; key down; key ret    # OVMF Platform Configuration
sleep 1
key ret; key down; key down; key ret     # the list of resolutions: 1024x768, the last
key down; key ret                        # Commit Changes and Exit
sleep 1
quit

echo "== Windows' own display"
"$VERO/scripts/run-windows.sh" --headless --payload "$EMPTY" --ssh "$PORT" >/dev/null 2>&1 &
trap 'windows_stop; rm -rf "$EMPTY"' EXIT
windows_wait "$PORT" 300 || { echo "FAIL: the VM never answered over SSH" >&2; exit 1; }
windows_scp "$PORT" "$VERO/scripts/lib/windows-display.ps1" vero@127.0.0.1:C:/Users/vero/vero-display.ps1
windows_ssh "$PORT" '$a = New-ScheduledTaskAction -Execute powershell.exe -Argument "-NoProfile -ExecutionPolicy Bypass -File C:\Users\vero\vero-display.ps1"; $p = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive; Register-ScheduledTask -TaskName vero-display -Action $a -Principal $p -Force | Out-Null; Start-ScheduledTask -TaskName vero-display; Start-Sleep 10; Unregister-ScheduledTask -TaskName vero-display -Confirm:$false; Get-Content C:\Users\vero\vero-display.txt' | tr -d '\r' | tee "$EMPTY/result"
grep -q 'display: 0' "$EMPTY/result" || { echo "FAIL: Windows did not change its display" >&2; exit 1; }
echo "PASS: the VM's screen is 1024 by 768"
