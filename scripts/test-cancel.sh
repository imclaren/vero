#!/bin/sh
# Checks cancelling a request from Swift and from C#, against a real worker:
# each cancels a slow request after 0.2 seconds, and passes when the call
# ends at once and the worker's handler has stopped. Go's own tests check
# the worker, the supervisor and the host, and the Python binding's tests
# check Python. Swift needs the archive that scripts/build-cvero.sh builds
# until the next release has VeroCancel; C# needs the .NET SDK.
#
#   scripts/test-cancel.sh
set -eu
ROOT=$(cd "$(dirname "$0")/.." && pwd)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
cd "$ROOT"
go build -o "$TMP/worker" ./bindings/python/testdata/routerworker
failed=0
echo "== Swift"
(cd testdata/swiftcancel && swift build -q && .build/debug/SwiftCancel "$TMP/worker") || failed=1
echo "== C#"
dotnet run -v q --project bindings/csharp/testdata/cancelcheck -- "$TMP/worker" || failed=1
[ "$failed" = 0 ] && echo "PASS" || { echo "FAIL" >&2; exit 1; }
