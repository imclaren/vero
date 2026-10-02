#!/bin/sh
# Builds the worker as wasm and drives it with a WASI runtime.
#
#   scripts/run-wasi.sh
#
# Needs wasmtime:  scripts/setup-wasm.sh
#
# The frontend is a terminal, because WASI has no screen.  Everything else is
# the same as any other example: the same worker, the same requests, the same
# state arriving as it changes.  The one line that differs is which program
# the supervisor starts - the runtime, with the worker as its argument.
set -e
ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

RUNTIME=${RUNTIME:-wasmtime}
command -v "$RUNTIME" >/dev/null 2>&1 || {
    echo "$RUNTIME is missing - scripts/setup-wasm.sh" >&2; exit 1; }

OUT=${OUT:-$ROOT/example/wasi-app/.build}
mkdir -p "$OUT"

echo "building the worker for wasip1"
GOOS=wasip1 GOARCH=wasm go build -o "$OUT/worker.wasm" ./example/wasi-app/worker

echo "building the frontend for this Mac"
go build -o "$OUT/wasi-app" ./example/wasi-app

echo
exec "$OUT/wasi-app" -worker "$OUT/worker.wasm" -runtime "$RUNTIME"
