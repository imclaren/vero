#!/bin/sh
# Builds a release: the library and the worker, for every platform, packaged
# so nobody downloading them needs a cross toolchain of their own.
#
#   ./scripts/release.sh v0.2.0
#   ./scripts/release.sh v0.2.0 --publish     # also create the GitHub release
#
# Everything lands in dist/release/.  Linux is built for both architectures,
# which takes a few minutes: one of them runs emulated.
set -e

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

VERSION=${1:?usage: scripts/release.sh <version> [--publish]}
PUBLISH=no
[ "$2" = "--publish" ] && PUBLISH=yes
case "$VERSION" in
    v*) ;;
    *) echo "version should look like v0.2.0" >&2; exit 2 ;;
esac

OUT="$ROOT/dist/release"
rm -rf "$OUT"; mkdir -p "$OUT"

echo "==> building everything"
ALL_ARCHES=1 "$ROOT/scripts/build-all.sh" >/dev/null

DIST="$ROOT/dist"
missing=""
need() { [ -f "$DIST/$1" ] || missing="$missing $1"; }
need libvero.a
need worker-macos-universal
[ -n "$missing" ] && { echo "not releasing, these did not build:$missing" >&2; exit 1; }

echo "==> packaging"
# A worker and, on macOS, the archive Swift links.  Everywhere else the
# frontend spawns the worker rather than loading a library, so the worker is
# the whole of it.
stage() {
    name=$1; shift
    dir="$OUT/$name"; rm -rf "$dir"; mkdir -p "$dir"
    while [ $# -gt 0 ]; do
        [ -f "$DIST/$1" ] || { rm -rf "$dir"; return 1; }
        cp "$DIST/$1" "$dir/$2"
        shift 2
    done
    cp "$ROOT/LICENSE" "$dir/" 2>/dev/null || true
    cat > "$dir/README.txt" <<TXT
vero $VERSION - $name

  worker   the Go half of your application

A frontend starts it with VERO_HOST=1, which makes it supervise a second copy
of itself and speak JSON on its standard input and output.  The Python and C#
bindings do that for you; so can anything that can spawn a process.

  https://github.com/imclaren/vero
TXT
}

archive() {
    name=$1; format=$2
    ( cd "$OUT" &&
      if [ "$format" = zip ]; then zip -qr "$name.zip" "$name"; else tar -czf "$name.tar.gz" "$name"; fi )
    rm -rf "$OUT/$name"
    echo "  $name.$format"
}

# macOS carries the archive as well: the Swift package links it into the
# application rather than spawning anything.
n="vero-$VERSION-darwin-universal"
stage "$n" libvero.a libvero.a worker-macos-universal worker
cp "$DIST/vero.h" "$OUT/$n/vero.h" 2>/dev/null || true
archive "$n" tar.gz

# Everything else is the worker on its own.  One line per platform rather than
# a loop over dist/, so a release says what it ships.
for t in windows/amd64 windows/arm64 \
         linux/amd64 linux/arm64 \
         freebsd/amd64 freebsd/arm64 \
         openbsd/amd64 openbsd/arm64 \
         netbsd/amd64 dragonfly/amd64 illumos/amd64; do
    os=${t%/*}; arch=${t#*/}
    ext=""; format=tar.gz
    [ "$os" = windows ] && { ext=".exe"; format=zip; }
    n="vero-$VERSION-$os-$arch"
    if stage "$n" "worker-$os-$arch$ext" "worker$ext"; then
        archive "$n" "$format"
    else
        echo "  skipped $n (not in dist/)"
    fi
done

# The Swift package links this rather than asking every application to build
# its own: the archive is identical for all of them.
echo "==> the Swift binary target"
XCF="$OUT/CVero.xcframework"
HDR=$(mktemp -d)
cp "$ROOT/Sources/CVero/include/CVero.h" "$HDR/"
cat > "$HDR/module.modulemap" <<MOD
module CVero {
    header "CVero.h"
    export *
}
MOD
xcodebuild -create-xcframework -library "$DIST/libvero.a" -headers "$HDR" \
    -output "$XCF" >/dev/null
rm -rf "$HDR"
( cd "$OUT" && zip -qr CVero.xcframework.zip CVero.xcframework && rm -rf CVero.xcframework )
CHECKSUM=$(swift package --package-path "$ROOT" compute-checksum "$OUT/CVero.xcframework.zip")
echo "  CVero.xcframework.zip  $CHECKSUM"

( cd "$OUT" && shasum -a 256 ./* > SHA256SUMS && echo "  SHA256SUMS" )

echo
ls -lh "$OUT" | awk 'NR>1 {print "  "$9"  "$5}'

if [ "$PUBLISH" = yes ]; then
    command -v gh >/dev/null 2>&1 || { echo "gh is not installed" >&2; exit 1; }
    echo
    echo "==> creating the GitHub release $VERSION"
    gh release create "$VERSION" "$OUT"/* --title "$VERSION" --generate-notes

    # Now that the artefact has a URL, point the package at it and tag that.
    # Package.swift and the binary it names have to come from one commit, or a
    # consumer gets Swift from one version and Go from another.
    echo "==> pointing Package.swift at $VERSION"
    URL="https://github.com/imclaren/vero/releases/download/$VERSION/CVero.xcframework.zip"
    python3 - "$ROOT/Package.swift" "$URL" "$CHECKSUM" <<'PY'
import re, sys
path, url, checksum = sys.argv[1], sys.argv[2], sys.argv[3]
s = open(path).read()
s = re.sub(r'\.binaryTarget\(\s*name: "CVero".*?\)',
           '.binaryTarget(\n            name: "CVero",\n            url: "%s",\n            checksum: "%s"\n        )' % (url, checksum),
           s, flags=re.S)
if '.binaryTarget' not in s:
    s = s.replace('.target(name: "CVero"),',
                  '.binaryTarget(\n            name: "CVero",\n            url: "%s",\n            checksum: "%s"\n        ),' % (url, checksum))
open(path, "w").write(s)
PY
    git -C "$ROOT" add Package.swift
    git -C "$ROOT" commit -m "Point the Swift package at $VERSION"
    git -C "$ROOT" tag -f "$VERSION"
    git -C "$ROOT" push origin main
    git -C "$ROOT" push -f origin "$VERSION"
    echo "  tagged $VERSION with a Package.swift that matches the artefact"
else
    echo
    echo "not published. To do that:  ./scripts/release.sh $VERSION --publish"
fi
