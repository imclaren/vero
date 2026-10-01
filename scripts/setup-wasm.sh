#!/bin/sh
# Installs what the two wasm targets need.
#
#   scripts/setup-wasm.sh
#
# The browser example needs nothing but a browser - the page is served by a
# ten-line Go file.  WASI needs a runtime to run the worker in, and node is
# what the browser half's tests run under.
set -e

command -v brew >/dev/null 2>&1 || { echo "Homebrew is needed: https://brew.sh" >&2; exit 1; }

for pair in wasmtime:wasmtime node:node; do
    cmd=${pair%%:*}; formula=${pair#*:}
    command -v "$cmd" >/dev/null 2>&1 && continue
    echo "installing $formula"
    brew install "$formula" >/dev/null 2>&1 || true
    command -v "$cmd" >/dev/null 2>&1 ||
        { echo "    $formula installed but $cmd is not on the PATH" >&2; exit 1; }
done

echo "wasmtime $(wasmtime --version | awk '{print $2}'), node $(node --version)"
echo "done.  the browser example:  scripts/run-web.sh"
echo "       the WASI worker:      go test -run Wasm ."
