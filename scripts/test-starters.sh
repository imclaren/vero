#!/bin/sh
# Makes a window app and a tray app with vero new, as a new app would be
# made, from this checkout of vero, and checks each front end of each:
#
#   macos  swift build (on a Mac)
#   wpf    dotnet build
#   gtk    run in containers, Debian 13 with libadwaita and Debian 12
#          without, with vero's step player waiting for the worker's
#          state and asking it something through the window's app
#
# With --windows it also runs each Windows app in vero's Windows VM with
# the step player, which needs the VM and SSH into it, as
# scripts/test-wpf-libs.sh does.
#
# It needs Go, and Swift, the .NET SDK and Docker for each check; a check
# whose tool is missing is skipped and said so.
#
#   scripts/test-starters.sh [--windows]
#
# VERO_NO_GTK=1 leaves out the containers, to check Windows alone.
set -eu
VERO=$(cd "$(dirname "$0")/.." && pwd)
windows=no
[ "${1:-}" = --windows ] && windows=yes
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fail=0
(cd "$VERO/cmd/vero" && go build -o "$work/vero" .)
cd "$work"
./vero new winapp --vero "$VERO" >/dev/null
./vero new trayapp --vero "$VERO" --menubar >/dev/null
grep -q '^menubar = true' trayapp/vero-app.toml || { echo "FAIL: trayapp's vero-app.toml has no menubar = true"; fail=1; }

for app in winapp trayapp; do
    if command -v swift >/dev/null 2>&1 && [ "$(uname)" = Darwin ]; then
        if (cd $app/macos && swift build >"$work/$app-swift.log" 2>&1); then echo "ok: $app macos builds"
        else echo "FAIL: $app macos"; tail -20 "$work/$app-swift.log"; fail=1; fi
    else echo "skipped: $app macos (no swift)"; fi
    if command -v dotnet >/dev/null 2>&1; then
        if (cd $app/windows && dotnet build -nologo -v q -warnaserror >"$work/$app-dotnet.log" 2>&1); then echo "ok: $app wpf builds"
        else echo "FAIL: $app wpf"; grep -E 'error|warn' "$work/$app-dotnet.log" | sort -u | head -10; fail=1; fi
    else echo "skipped: $app wpf (no dotnet)"; fi
done

# The steps: the state's jobs arrive, a request is sent, and its reply is
# seen in the state.
cat >steps.json <<'JSON'
{"steps": [{"wait": {"path": "jobs.#", "at_least": 1}, "timeout": 30},
           {"call": "restartJob", "with": {"id": 1}},
           {"wait": {"path": "jobs.0.id", "is": 1}, "timeout": 30}]}
JSON

if [ $windows = yes ]; then
    . "$VERO/scripts/lib/windows-disc.sh"
    SSHPORT=2229
    PAY="$work/windows"
    mkdir -p "$PAY/empty"
    for app in winapp trayapp; do
        (cd $app/windows && dotnet publish -nologo -v q -c Release -r win-arm64 --self-contained -o "$PAY/$app" >/dev/null)
        (cd $app && GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -o "$PAY/$app/$app-worker.exe" ./cmd/worker)
    done
    cp "$VERO/scripts/lib/starter-check.ps1" "$PAY/"
    windows_running && { echo "FAIL: the Windows VM is running already"; exit 1; }
    "$VERO/scripts/run-windows.sh" --headless --payload "$PAY/empty" --ssh "$SSHPORT" >"$PAY/vm.log" 2>&1 &
    if windows_wait "$SSHPORT" 300; then
        windows_ssh "$SSHPORT" "New-Item -ItemType Directory -Force C:/vero-starters | Out-Null; Remove-Item -Recurse -Force C:/vero-starters/*"
        for app in winapp trayapp; do windows_scp "$SSHPORT" -r "$PAY/$app" vero@127.0.0.1:C:/vero-starters/; done
        windows_scp "$SSHPORT" "$work/steps.json" "$PAY/starter-check.ps1" vero@127.0.0.1:C:/vero-starters/
        windows_ssh "$SSHPORT" "powershell -NoProfile -ExecutionPolicy Bypass -File C:/vero-starters/starter-check.ps1 -Apps winapp,trayapp" | tr -d '\r' >"$PAY/results.txt"
        for app in winapp trayapp; do
            if grep -qx "$app steps: True" "$PAY/results.txt" && grep -qx "$app running: True" "$PAY/results.txt"; then echo "ok: $app wpf runs on Windows"
            else echo "FAIL: $app wpf on Windows"; cat "$PAY/results.txt"; fail=1; fi
        done
    else
        echo "FAIL: the Windows VM never answered over SSH"; fail=1
    fi
    windows_stop
fi

[ "${VERO_NO_GTK:-}" ] && exit $fail
if ! command -v docker >/dev/null 2>if ! command -v docker >/dev/null 2>&1; then1; then
    echo "skipped: gtk (no docker)"
    exit $fail
fi
arch=$(docker info --format '{{.Architecture}}' 2>/dev/null || echo x86_64)
case $arch in aarch64|arm64) goarch=arm64 ;; *) goarch=amd64 ;; esac
for app in winapp trayapp; do
    (cd $app && GOOS=linux GOARCH=$goarch CGO_ENABLED=0 go build -o gtk/$app-worker ./cmd/worker)
done
for image in debian:trixie debian:bookworm; do
    packages="python3-gi gir1.2-gtk-4.0 xvfb xauth dbus fonts-dejavu-core"
    case $image in *trixie*) packages="$packages gir1.2-adw-1" ;; esac
    for app in winapp trayapp; do
        out=$(tar -cf - $app/gtk steps.json | docker run --rm -i "$image" sh -c "
            apt-get update -qq >/dev/null
            DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends $packages >/dev/null 2>&1
            mkdir /t && cd /t && tar xf - && cd $app/gtk
            VERO_TEST=/t/steps.json VERO_TEST_RESULT=/t/result.json timeout 20 xvfb-run -a dbus-run-session -- python3 main.py >/t/app.log 2>&1
            # 124: still running when the time ran out, as an app should be.
            echo \"exit \$?\"
            grep -A15 Traceback /t/app.log
            cat /t/result.json 2>/dev/null || { echo no result; tail -20 /t/app.log; }
        " 2>&1)
        if echo "$out" | grep -q '"ok": true' && echo "$out" | grep -q '^exit 124' && ! echo "$out" | grep -q Traceback; then echo "ok: $app gtk runs on $image"
        else echo "FAIL: $app gtk on $image"; echo "$out" | tail -20; fail=1; fi
    done
done
exit $fail
