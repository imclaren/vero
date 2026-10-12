#!/bin/sh
# Runs the WPF library's checks (bindings/csharp/testdata/wpfcheck) in
# vero's Windows VM, which is Windows 11 on ARM and so runs every build
# that vero makes for Windows:
#
#   arm64         Windows on ARM
#   arm64-toasts  the same, built for Windows' own notifications, with buttons
#   x64           most PCs, which Windows on ARM runs through its emulation
#   x86           32-bit Windows, the same
#
# Each runs the window, tray and single-copy scenarios on the VM's desktop.
# It needs the VM (scripts/run-windows.sh --install), SSH into it
# (scripts/setup-windows-ssh.sh), the .NET SDK and Go. The VM must not be
# running already.
#
#   scripts/test-wpf-libs.sh [build ...]
set -eu
VERO=$(cd "$(dirname "$0")/.." && pwd)
. "$VERO/scripts/lib/windows-disc.sh"
VMDIR="$HOME/vm/vero-windows"
SSHPORT=2228
builds=${*:-"arm64 arm64-toasts x64 x86"}
[ -f "$VMDIR/disk.qcow2" ] || { echo "no Windows VM in $VMDIR: scripts/run-windows.sh --install makes one" >&2; exit 1; }
[ -f "$WINDOWS_KEY" ] || { echo "no $WINDOWS_KEY: scripts/setup-windows-ssh.sh sets up SSH into the VM" >&2; exit 1; }
windows_running && { echo "the Windows VM is running already: stop it first" >&2; exit 1; }

PAY="$HOME/.cache/vero/wpf-check"
rm -rf "$PAY" && mkdir -p "$PAY"
for build in $builds; do
    case $build in
        arm64) rid=win-arm64 goarch=arm64 toasts=false ;;
        arm64-toasts) rid=win-arm64 goarch=arm64 toasts=true ;;
        x64) rid=win-x64 goarch=amd64 toasts=false ;;
        x86) rid=win-x86 goarch=386 toasts=false ;;
        *) echo "unknown build $build" >&2; exit 2 ;;
    esac
    echo "== building $build"
    dotnet publish "$VERO/bindings/csharp/testdata/wpfcheck" -nologo -v q -c Release -r "$rid" --self-contained \
        -p:Toasts=$toasts -o "$PAY/$build" >/dev/null
    (cd "$VERO" && GOOS=windows GOARCH=$goarch CGO_ENABLED=0 go build -o "$PAY/$build/worker.exe" ./example/worker)
done
cp "$VERO/scripts/lib/wpf-check.ps1" "$PAY/"
mkdir -p "$PAY/empty"

echo "== booting the Windows VM (about a minute)"
"$VERO/scripts/run-windows.sh" --headless --payload "$PAY/empty" --ssh "$SSHPORT" >"$PAY/vm.log" 2>&1 &
trap 'windows_stop' EXIT
windows_wait "$SSHPORT" 300 || { echo "FAIL: the VM never answered over SSH" >&2; exit 1; }
windows_ssh "$SSHPORT" "New-Item -ItemType Directory -Force C:/vero-wpf | Out-Null; Remove-Item -Recurse -Force C:/vero-wpf/*"
echo "== copying the builds in"
for build in $builds; do windows_scp "$SSHPORT" -r "$PAY/$build" vero@127.0.0.1:C:/vero-wpf/; done
windows_scp "$SSHPORT" "$PAY/wpf-check.ps1" vero@127.0.0.1:C:/vero-wpf/
windows_ssh "$SSHPORT" "powershell -NoProfile -ExecutionPolicy Bypass -File C:/vero-wpf/wpf-check.ps1 -Builds $(echo $builds | tr ' ' ',')" \
    | tr -d '\r' | tee "$PAY/results.txt"
python3 "$VERO/scripts/lib/qmp.py" /tmp/vero-qmp.sock shot "$PAY/desktop.ppm" >/dev/null 2>&1 && sips -s format png "$PAY/desktop.ppm" --out "$PAY/desktop.png" >/dev/null 2>&1 || true
want=$(( $(echo $builds | wc -w) * 3 ))
got=$(grep -c '^SCENARIO OK' "$PAY/results.txt" || true)
if [ "$got" -ne "$want" ]; then
    echo "FAIL: $got of $want scenarios passed (results in $PAY/results.txt)" >&2
    exit 1
fi
echo "PASS: all $want scenarios, on $builds"
