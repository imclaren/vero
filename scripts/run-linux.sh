#!/bin/sh
# Runs a GTK app on Linux - the example, or your own - and shows it in a
# window on your Mac.
#
#   scripts/run-linux.sh              # watch it, live, over VNC
#   scripts/run-linux.sh --record out.gif
#   scripts/run-linux.sh --no-open    # start it but do not open a viewer
#   scripts/run-linux.sh --shot out.png --wait 10   # a screenshot, and stop
#
# Your own app, from beside its go.mod:
#
#   path/to/vero/scripts/run-linux.sh --app linux --entry app.py \
#       --worker ./cmd/worker --worker-name app-worker \
#       [--base debian:trixie-slim] [--packages "gir1.2-webkit-6.0"] \
#       [--data ~/some/folder] [--env NAME=value]... [--size 1280x800]
#
#   --app DIR          the app's folder, in the project; its entry file runs
#   --entry FILE       the Python file that starts it
#   --worker PKG       the worker's package, cross-compiled into DIR
#   --worker-name N    what the app expects the worker to be called
#   --base IMAGE       the Debian to run on; --packages, more to install
#   --data DIR         mounted as /data, for test files and settings: colima
#                      shares only your home folder, so it must be in it
#   --env NAME=value   set for the app; repeat for more
#
# Needs Docker:  brew install colima docker && colima start
set -e

VERO=$(cd "$(dirname "$0")/.." && pwd)
# The project being run: the one we are in, when it is not vero itself.
if [ -f "$PWD/go.mod" ]; then ROOT=$PWD; else ROOT=$VERO; fi
cd "$ROOT"
. "$VERO/scripts/lib/docker.sh"

PORT=${PORT:-5901}
APP=example/gtk-app
ENTRY=main.py
WORKER=./example/worker
WORKER_NAME=worker
BASE=debian:bookworm-slim
EXTRA=""
DATA=""
ENVS=""
SIZE=480x440
RECORD=""
SHOT=""
WAIT=5
OPEN=yes
while [ $# -gt 0 ]; do
    case $1 in
        --record) RECORD=${2:?--record needs a filename}; shift 2 ;;
        --shot) SHOT=${2:?--shot needs a filename}; shift 2 ;;
        --wait) WAIT=${2:?--wait needs seconds}; shift 2 ;;
        --no-open) OPEN=no; shift ;;
        --app) APP=${2:?}; shift 2 ;;
        --entry) ENTRY=${2:?}; shift 2 ;;
        --worker) WORKER=${2:?}; shift 2 ;;
        --worker-name) WORKER_NAME=${2:?}; shift 2 ;;
        --base) BASE=${2:?}; shift 2 ;;
        --packages) EXTRA=${2:?}; shift 2 ;;
        --data) DATA=${2:?}; shift 2 ;;
        --env) ENVS="$ENVS -e ${2:?}"; shift 2 ;;
        --size) SIZE=${2:?}; shift 2 ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
done
APP=$(cd "$APP" && pwd)

vero_docker

echo "building the Linux worker"
# Pure Go, so it cross-compiles here: the worker hosts itself, and the Python
# binding spawns it rather than loading a C library.  There is nothing left
# that has to be built on Linux.
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o "$APP/$WORKER_NAME" "$WORKER"

# One image for each Debian and set of packages asked for.
IMAGE=vero-gtk-$(printf '%s %s' "$BASE" "$EXTRA" | shasum | cut -c1-10)
echo "building $IMAGE"
docker build -q -t "$IMAGE" --build-arg BASE="$BASE" --build-arg EXTRA="$EXTRA" \
    -f "$VERO/scripts/linux.Dockerfile" "$VERO/scripts" >/dev/null

# The whole project, so that an app can reach beside its folder (the
# example's Python binding is two up), and the app's folder within it.
case "$APP/" in
    "$ROOT"/*) WORKDIR=/src/${APP#"$ROOT"/} ;;
    *) echo "--app must be in the project, $ROOT" >&2; exit 2 ;;
esac
MOUNTS="-v $ROOT:/src"
[ -n "$DATA" ] && MOUNTS="$MOUNTS -v $(cd "$DATA" && pwd):/data"
# GTK_A11Y: no accessibility bus in a container. WebKit's sandbox needs
# namespaces an unprivileged container does not have; on a Linux desktop
# it runs as it should.
RUN="docker run --rm $MOUNTS -w $WORKDIR -e GTK_A11Y=none -e WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1 $ENVS"
START="Xvfb :99 -screen 0 ${SIZE}x24 >/dev/null 2>&1 & sleep 2; export DISPLAY=:99; python3 $ENTRY >/tmp/app.log 2>&1 &"

if [ -n "$SHOT" ]; then
    OUT=$(mktemp -d "$HOME/.cache/vero-shot.XXXXXX")
    $RUN -v "$OUT":/out "$IMAGE" sh -c "$START sleep $WAIT; import -window root /out/shot.png; cp /tmp/app.log /out/"
    mv "$OUT/shot.png" "$SHOT"
    [ -s "$OUT/app.log" ] && { echo "the app said:"; tail -20 "$OUT/app.log"; }
    rm -rf "$OUT"
    echo "wrote $SHOT"
    exit 0
fi

if [ -n "$RECORD" ]; then
    echo "recording 20 frames"
    OUT=$(mktemp -d "$ROOT/.gifframes.XXXXXX")
    $RUN -v "$OUT":/frames "$IMAGE" sh -c "$START sleep 5
        geom=\$(xwininfo -root -children | awk '/\"/ {print \$0}' | grep -o '[0-9]*x[0-9]*+[0-9]*+[0-9]*' | head -1)
        for i in \$(seq -w 1 20); do import -window root -crop \"\$geom\" +repage /frames/f\$i.png; sleep 0.5; done
        convert -delay 50 -loop 0 /frames/f*.png /frames/out.gif"
    mv "$OUT/out.gif" "$RECORD"
    rm -rf "$OUT"
    echo "wrote $RECORD"
    exit 0
fi

echo "starting the app on a virtual display, shared on :$PORT"
CID=$($RUN -d -p "$PORT":5900 "$IMAGE" sh -c "$START sleep 3; x11vnc -display :99 -forever -nopw -listen 0.0.0.0 -quiet")
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
