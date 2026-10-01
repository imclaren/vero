#!/bin/sh
# Builds the browser example and opens it.
#
#   scripts/run-web.sh
#
# Nothing to install: the frontend and the worker are one wasm module, which
# any browser runs.  The server is here because a page may only load a module
# that arrives as application/wasm, which `python3 -m http.server` does not
# send.
set -e
ROOT=$(cd "$(dirname "$0")/.." && pwd)
APP="$ROOT/example/web-app"
PORT=${PORT:-8080}

"$APP/build.sh"

# Stop a server left behind by an earlier run, so the port is ours.
PID=$(lsof -ti "tcp:$PORT" 2>/dev/null || true)
[ -n "$PID" ] && kill "$PID" 2>/dev/null && sleep 1

( cd "$APP" && go run serve.go -addr "localhost:$PORT" ) &
SERVER=$!
trap 'kill $SERVER 2>/dev/null' EXIT INT TERM

# Give it a moment to bind before the browser asks for the page.
sleep 1
echo "http://localhost:$PORT"
case $(uname) in
    Darwin) open "http://localhost:$PORT" ;;
    *)      command -v xdg-open >/dev/null && xdg-open "http://localhost:$PORT" ;;
esac

echo "ctrl-c to stop"
wait $SERVER
