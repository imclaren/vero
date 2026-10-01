#!/bin/sh
# Installs what the plan9 example needs: a VM, into which run-plan9.sh downloads 9front itself.
#
#   scripts/setup-plan9.sh
#
# Nothing here is needed to *build* for plan9 - the worker is pure Go and
# cross-compiles on a Mac with no C toolchain.  This is only what runs it.
set -e

command -v brew >/dev/null 2>&1 || { echo "Homebrew is needed: https://brew.sh" >&2; exit 1; }

for pair in qemu-system-x86_64:qemu; do
    cmd=${pair%%:*}; formula=${pair#*:}
    command -v "$cmd" >/dev/null 2>&1 && continue
    echo "installing $formula"
    brew install "$formula" >/dev/null 2>&1 || brew link --overwrite "$formula" >/dev/null 2>&1 || true
    command -v "$cmd" >/dev/null 2>&1 ||
        { echo "    $formula installed but $cmd is not on the PATH" >&2; exit 1; }
done
echo "done.  run it with:  scripts/run-plan9.sh"
