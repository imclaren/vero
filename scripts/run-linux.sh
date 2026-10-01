#!/bin/sh
# Runs the GTK example on Linux and shows it in a window on your Mac.
#
#   scripts/run-linux.sh              # watch it, live, over VNC
#   scripts/run-linux.sh --record out.gif
#   scripts/run-linux.sh --no-open    # start it but do not open a viewer
#
# Needs Docker:  brew install colima docker && colima start
set -e

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"
PORT=${PORT:-5901}
IMAGE=vero-gtk
RECORD=""
OPEN=yes
while [ $# -gt 0 ]; do
    case $1 in
        --record) RECORD=${2:?--record needs a filename}; shift 2 ;;
        --no-open) OPEN=no; shift ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
done

docker info >/dev/null 2>&1 || { echo "docker is not running - try: colima start" >&2; exit 1; }

echo "building the Linux worker"
# Pure Go, so it cross-compiles here: the worker hosts itself, and the Python
# binding spawns it rather than loading a C library.  There is nothing left
# that has to be built on Linux.
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
    go build -o "$ROOT/example/gtk-app/worker" ./example/worker

echo "building $IMAGE"
docker build -q -t "$IMAGE" -f scripts/linux.Dockerfile scripts >/dev/null 2>&1

if [ -n "$RECORD" ]; then
    echo "recording 20 frames"
    OUT=$(mktemp -d "$ROOT/.gifframes.XXXXXX")
    docker run --rm -v "$ROOT":/src -v "$OUT":/frames -w /src/example/gtk-app "$IMAGE" sh -c '
        Xvfb :99 -screen 0 480x440x24 >/dev/null 2>&1 &
        sleep 2
        export DISPLAY=:99
        python3 main.py >/dev/null 2>&1 &
        sleep 5
        geom=$(xwininfo -root -children | awk "/\"vero\"/ {print \$0}" | grep -o "[0-9]*x[0-9]*+[0-9]*+[0-9]*" | head -1)
        for i in $(seq -w 1 20); do import -window root -crop "$geom" +repage /frames/f$i.png; sleep 0.5; done
        convert -delay 50 -loop 0 /frames/f*.png /frames/out.gif'
    mv "$OUT/out.gif" "$RECORD"
    rm -rf "$OUT"
    echo "wrote $RECORD"
    exit 0
fi

echo "starting the app on a virtual display, shared on :$PORT"
CID=$(docker run -d --rm -p "$PORT":5900 -v "$ROOT":/src -w /src/example/gtk-app "$IMAGE" sh -c '
    Xvfb :99 -screen 0 480x440x24 >/dev/null 2>&1 &
    sleep 2
    export DISPLAY=:99
    python3 main.py >/dev/null 2>&1 &
    sleep 3
    x11vnc -display :99 -forever -nopw -listen 0.0.0.0 -quiet')
trap 'docker rm -f "$CID" >/dev/null 2>&1' INT TERM

n=0
while ! nc -z localhost "$PORT" 2>/dev/null; do
    n=$((n+1)); [ $n -gt 40 ] && { echo "VNC never came up" >&2; docker logs "$CID"; exit 1; }
    sleep 0.5
done

[ "$OPEN" = yes ] && open "vnc://localhost:$PORT"
echo
echo "the GTK app is at vnc://localhost:$PORT - it is live, you can click it"
echo "stop it with:  docker rm -f $CID"
