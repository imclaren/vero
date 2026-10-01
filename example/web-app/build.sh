#!/bin/sh
# Builds the browser example.
#
#   ./build.sh && go run serve.go
#
# Both halves are one wasm module: the frontend and the worker it supervises
# are the same program, because a page cannot start a second one.
set -e
cd "$(dirname "$0")"

GOOS=js GOARCH=wasm go build -o main.wasm .

# The glue Go ships for loading a wasm module in a browser.  Copied rather
# than vendored, so it matches the toolchain that built main.wasm.
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" .

echo "built main.wasm ($(du -h main.wasm | cut -f1)) and wasm_exec.js"
