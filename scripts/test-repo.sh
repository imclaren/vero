#!/bin/sh
# Tests vero's packaging from start to finish on the example app, as a
# user would meet it: builds the .deb, builds the site with a throwaway
# key, serves it from this Mac, and in a clean Debian container adds the
# repository the way the site says, installs the example, and checks its
# worker. Then it releases 1.0.1 into the same site and checks that apt
# upgrade brings it.
#
#   scripts/test-repo.sh [--image debian:bookworm] [--port 8642]
#
# Needs Docker and colima, as package-linux.sh does. Everything it makes
# is in ~/.cache/vero, the key included, so nothing in it is anyone's.
set -e
VERO=$(cd "$(dirname "$0")/.." && pwd)
. "$VERO/scripts/lib/docker.sh"
IMAGE=debian:bookworm PORT=8642
while [ $# -gt 0 ]; do
    case $1 in
        --image) IMAGE=$2 ;; --port) PORT=$2 ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
    shift 2
done
vero_docker
CACHE="$HOME/.cache/vero"
KEY="$CACHE/example-key" SITE="$CACHE/example-site" PACKAGES="$CACHE/example-packages"
# The container reaches this Mac by this name: colima and Docker Desktop
# both answer it.
URL="http://host.docker.internal:$PORT"
NAME=vero-test-repo
mkdir -p "$CACHE/bin"
go build -C "$VERO/cmd/vero-repo" -o "$CACHE/bin/vero-repo" .
REPO="$CACHE/bin/vero-repo"
[ -f "$KEY/private.asc" ] || "$REPO" key --dir "$KEY" --name "vero example" --email example@example.com
rm -rf "$SITE"

release() {
    rm -rf "$PACKAGES"
    (cd "$VERO" && scripts/package.sh --app example/vero-app.toml --version "$1" --targets linux --out "$PACKAGES")
    "$REPO" build --app "$VERO/example/vero-app.toml" --packages "$PACKAGES" --key "$KEY" --url "$URL" --out "$SITE"
}
in_container() { docker exec "$NAME" sh -c "$1"; }
cleanup() {
    docker rm -f "$NAME" >/dev/null 2>&1 || true
    [ -n "$SERVER" ] && kill "$SERVER" 2>/dev/null || true
}
trap cleanup EXIT

echo "== releasing 1.0.0"
release 1.0.0
go build -o "$CACHE/bin/serve" "$VERO/scripts/lib/serve.go"
"$CACHE/bin/serve" "$SITE" "$PORT" &
SERVER=$!
docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --name "$NAME" --add-host=host.docker.internal:host-gateway "$IMAGE" sleep infinity >/dev/null

echo "== installing it as the site says, in $IMAGE"
in_container "apt-get update -qq && apt-get install -y -qq curl ca-certificates > /dev/null"
in_container "curl -fsSL $URL/install.sh | sh"
got=$(in_container "/usr/lib/vero-example/worker -version")
[ "$got" = 1.0.0 ] || { echo "FAIL: the installed worker says $got, not 1.0.0" >&2; exit 1; }
in_container "command -v vero-example && ls /usr/share/applications/dev.vero.example.desktop /usr/share/metainfo/dev.vero.example.metainfo.xml /usr/lib/vero-example/vero.py" >/dev/null ||
    { echo "FAIL: the package lacks its command, menu entry, metadata or binding" >&2; exit 1; }
echo "ok: 1.0.0 installed from the repository, and its worker answers"

echo "== releasing 1.0.1 into the same site"
release 1.0.1
in_container "apt-get update -qq && apt-get upgrade -y -qq > /dev/null"
got=$(in_container "/usr/lib/vero-example/worker -version")
[ "$got" = 1.0.1 ] || { echo "FAIL: after apt upgrade the worker says $got, not 1.0.1" >&2; exit 1; }
echo "ok: apt upgrade brought 1.0.1"
echo "PASS"
