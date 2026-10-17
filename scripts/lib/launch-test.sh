#!/bin/sh
# Runs on the system test-repo.sh is testing, from a folder it copied
# there holding this, steps.json and files/: starts the installed app on a
# virtual display with VERO_TEST set, so that vero's binding plays the
# steps, and takes a frame of the display every half second until the
# steps have finished (and a few frames more), the app has exited, or the
# time is up. test-repo.sh fetches the folder and makes the frames a GIF.
# When the folder holds a file called no-frames, the steps are played and
# no frames are taken.
#
#   launch-test.sh SECONDS COMMAND [ARGUMENT...]
#
# It needs Xvnc, or Xvfb where there is no Xvnc (GTK 4 falls over on Xvfb
# on FreeBSD, but not in the Linux containers), and xwd, or
# ImageMagick's import. Frames are xwd's, gzipped, or import's PNGs.
# Where there is neither X server, as on Chimera, which no longer packages
# one, the app runs on Wayland in sway with no screen, and grim takes the
# frames as PNGs.
dir=$(cd "$(dirname "$0")" && pwd)
limit=${1:?seconds}
shift
rm -rf "$dir/frames" "$dir/result.json" "$dir/exited" "$dir/timeout"
mkdir -p "$dir/frames"

wayland=no
if ! command -v Xvnc >/dev/null 2>&1 && ! command -v Xvfb >/dev/null 2>&1 && command -v sway >/dev/null 2>&1; then
    wayland=yes
    export XDG_RUNTIME_DIR="$dir/runtime"
    mkdir -p "$XDG_RUNTIME_DIR" && chmod 700 "$XDG_RUNTIME_DIR"
    printf 'output HEADLESS-1 mode 1280x800\n' >"$dir/sway.conf"
    # A copy, since sway's own file carries a capability that a
    # container is not allowed to grant, and so cannot be run.
    cp "$(command -v sway)" "$dir/sway"
    WLR_BACKENDS=headless WLR_RENDERER=pixman WLR_LIBINPUT_NO_DEVICES=1 LIBSEAT_BACKEND=noop \
        "$dir/sway" -c "$dir/sway.conf" >"$dir/display.log" 2>&1 &
elif ! command -v Xvnc >/dev/null 2>&1 && command -v Xvfb >/dev/null 2>&1; then
    Xvfb :99 -screen 0 1280x800x24 -nolisten tcp >"$dir/display.log" 2>&1 &
else
    # Not -localhost: on FreeBSD that leaves GTK nothing it can reach. The
    # systems tested are behind their VM's or container's own network.
    Xvnc :99 -geometry 1280x800 -depth 24 -SecurityTypes None >"$dir/display.log" 2>&1 &
fi
display=$!
trap 'kill $app $display 2>/dev/null || true' EXIT
export DISPLAY=:99
# The display up before the app, which cannot start without it: up to 30
# seconds, since a Mac busy with other tests can be that slow to start one.
if [ $wayland = yes ]; then
    unset DISPLAY
    n=0
    until [ -S "$XDG_RUNTIME_DIR/wayland-1" ]; do
        n=$((n + 1))
        [ $n -ge 120 ] && { echo "the virtual display did not start" >"$dir/app.log"; echo 1 >"$dir/exited"; exit 0; }
        sleep 0.25
    done
    export WAYLAND_DISPLAY=wayland-1 GDK_BACKEND=wayland
    echo "display $WAYLAND_DISPLAY from sway, with no screen" >>"$dir/display.log"
elif command -v xdpyinfo >/dev/null 2>&1; then
    n=0
    until xdpyinfo >/dev/null 2>&1; do
        n=$((n + 1))
        [ $n -ge 120 ] && { echo "the virtual display did not start" >"$dir/app.log"; echo 1 >"$dir/exited"; exit 0; }
        sleep 0.25
    done
else
    sleep 5
fi
# What the display is, for when the app cannot use it.
[ $wayland = yes ] || echo "display $DISPLAY from $(command -v Xvnc || command -v Xvfb), xdpyinfo $(xdpyinfo >/dev/null 2>&1 && echo answers || echo "does not answer")" >>"$dir/display.log"

if [ -f "$dir/no-frames" ]; then
    frame() { :; }
elif [ $wayland = yes ]; then
    frame() { grim "$dir/frames/f$1.png" 2>/dev/null; }
elif command -v xwd >/dev/null 2>&1 && command -v gzip >/dev/null 2>&1; then
    frame() { xwd -root -silent 2>/dev/null | gzip -1 >"$dir/frames/f$1.xwd.gz"; }
elif command -v xwd >/dev/null 2>&1; then
    # Void's container has no gzip: the frames as they are.
    frame() { xwd -root -silent >"$dir/frames/f$1.xwd" 2>/dev/null; }
elif command -v magick >/dev/null 2>&1; then
    frame() { magick import -window root "$dir/frames/f$1.png" 2>/dev/null; }
else
    frame() { import -window root "$dir/frames/f$1.png" 2>/dev/null; }
fi

# What the app's [test] env says, then no accessibility bus, GTK's
# software renderer (a virtual display has no GPU: without it, GTK 4 aborts
# in Fedora's container for want of OpenGL ES, and Xvfb runs out of memory
# on Alpine), and the steps and their files.
[ -f "$dir/env.sh" ] && . "$dir/env.sh"
GSK_RENDERER=${GSK_RENDERER:-cairo} GTK_A11Y=none NO_AT_BRIDGE=1 VERO_TEST="$dir/steps.json" VERO_TEST_FILES="$dir/files" \
    VERO_TEST_RESULT="$dir/result.json" "$@" >"$dir/app.log" 2>&1 &
app=$!

i=0 after=0 start=$(date +%s)
while :; do
    i=$((i+1))
    frame "$(printf %04d $i)"
    if ! kill -0 $app 2>/dev/null; then
        wait $app
        echo $? >"$dir/exited"
        break
    fi
    [ -f "$dir/result.json" ] && after=$((after+1))
    [ $after -ge 6 ] && break
    if [ $(($(date +%s) - start)) -ge "$limit" ]; then
        touch "$dir/timeout"
        break
    fi
    # A line now and then, so a long run shows it's going.
    [ $((i % 40)) -eq 0 ] && echo "   $(($(date +%s) - start))s, $(grep -c 'vero test: step' "$dir/app.log" 2>/dev/null) steps done" >&2
    sleep 0.5
done
