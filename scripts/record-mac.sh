#!/bin/sh
# Records a Mac app playing its vero-app.toml's [[test.step]]s, as
# test-repo.sh does on every other system: starts the app with VERO_TEST
# set, so that vero's Swift binding in it plays the steps, and captures its
# window every half second until they've finished, then makes the frames a
# GIF in ~/.cache/vero/test-results/APP/macos/, which
# ~/.cache/vero/test-results/index.html shows with every other system's.
#
#   scripts/record-mac.sh --app path/to/vero-app.toml --bundle path/to/My.app
#       [--env NAME=value]... [--clean PATH]... [--limit SECONDS]
#
# --clean removes a file or folder first, under ~/Library, such as the
# settings the copy kept from the last recording, so each one starts as a
# new install would.
# It runs on this Mac, as you, so make sure the copy it starts can't touch
# the copy you use: give it its own settings, as [test] env can, and build
# it with its own bundle ID (PRODUCT_BUNDLE_IDENTIFIER=...). Capturing a
# window needs Screen Recording permission for the terminal it runs in.
set -e
VERO=$(cd "$(dirname "$0")/.." && pwd)
TOML="" BUNDLE="" LIMIT=600 ENVS="" CLEAN=""
while [ $# -gt 0 ]; do
    case $1 in
        --app) TOML=$2; shift ;;
        --bundle) BUNDLE=$2; shift ;;
        --env) ENVS="$ENVS $2"; shift ;;
        --clean) case $2 in "$HOME"/Library/?*) CLEAN="$CLEAN
$2" ;; *) echo "--clean takes a path under ~/Library: $2" >&2; exit 2 ;; esac; shift ;;
        --limit) LIMIT=$2; shift ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
    shift
done
[ -n "$TOML" ] && [ -d "$BUNDLE" ] || { echo "usage: record-mac.sh --app vero-app.toml --bundle My.app" >&2; exit 2; }
CACHE="$HOME/.cache/vero"
mkdir -p "$CACHE/bin"
go build -C "$VERO/cmd/vero-repo" -o "$CACHE/bin/vero-repo" .
[ "$CACHE/bin/winid" -nt "$VERO/scripts/lib/winid.swift" ] || swiftc -O -o "$CACHE/bin/winid" "$VERO/scripts/lib/winid.swift"
eval "$("$CACHE/bin/vero-repo" show --app "$TOML")"
out="$CACHE/test-results/$APP_NAME/macos" L="$CACHE/mac-launch"
rm -rf "$out" "$L" && mkdir -p "$out/frames" "$L"
"$CACHE/bin/vero-repo" steps --app "$TOML" --out "$L" >/dev/null
exe="$BUNDLE/Contents/MacOS/$(defaults read "$(cd "$BUNDLE" && pwd)/Contents/Info" CFBundleExecutable)"

echo "$CLEAN" | while IFS= read -r path; do [ -n "$path" ] && rm -rf "$path"; done
echo "== starting $(basename "$BUNDLE") and playing its steps, recorded"
began=$(date +%s)
(
    . "$L/env.sh"
    for kv in $ENVS; do export "$kv"; done
    VERO_TEST="$L/steps.json" VERO_TEST_FILES="$L/files" VERO_TEST_RESULT="$L/result.json" exec "$exe"
) >"$out/app.log" 2>&1 &
app=$!
trap 'kill $app 2>/dev/null' EXIT
n=0
until win=$("$CACHE/bin/winid" $app) && [ -n "$win" ]; do
    n=$((n + 1)); [ $n -gt 60 ] && { echo "FAIL: the app showed no window" >&2; exit 1; }
    sleep 0.5
done
# In front, as somebody using it would see it.
osascript -e "tell application \"System Events\" to set frontmost of (first process whose unix id is $app) to true" >/dev/null 2>&1 || true
i=0 after=0
while kill -0 $app 2>/dev/null; do
    i=$((i + 1))
    screencapture -x -o -l"$win" "$out/frames/f$(printf %04d $i).png"
    [ -f "$L/result.json" ] && after=$((after + 1))
    [ $after -ge 6 ] && break
    [ $(($(date +%s) - began)) -ge "$LIMIT" ] && break
    [ $((i % 40)) -eq 0 ] && echo "   $(($(date +%s) - began))s, $(grep -c 'vero test: step' "$out/app.log") steps done"
    sleep 0.4
done
kill $app 2>/dev/null || true
wait $app 2>/dev/null || true
wait $app 2>/dev/null || true
cp "$L/result.json" "$out/" 2>/dev/null || true
grep 'vero test:' "$out/app.log" | sed 's/^vero test: /   /'
ffmpeg -loglevel error -y -framerate 2 -i "$out/frames/f%04d.png" \
    -vf "scale='min(640,iw)':-2:flags=lanczos,split[a][b];[a]palettegen=max_colors=128[p];[b][p]paletteuse=dither=none:diff_mode=rectangle" \
    -loop 0 "$out/app.gif"
rm -rf "$out/frames"
why=""
[ -f "$out/result.json" ] || why="the steps didn't finish in ${LIMIT}s"
[ -z "$why" ] && ! grep -Eq '"ok" ?: ?true' "$out/result.json" && why="a step failed"
printf '%s\n%s\n%s\n' "$([ -n "$why" ] && echo FAIL || echo PASS)" "$(($(date +%s) - began))" "$why" >"$out/status"
sh "$VERO/scripts/lib/results-page.sh" "$CACHE/test-results"
if [ -n "$why" ]; then echo "FAIL: $why (log in $out)" >&2; exit 1; fi
echo "ok: the steps passed; recorded in $out/app.gif"
