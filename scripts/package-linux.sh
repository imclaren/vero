#!/bin/sh
# Packages a vero app with a GTK front end as .deb files for Debian and
# Ubuntu, amd64 and arm64, from a Mac: the worker cross-compiled
# (build-all.sh), the package made by dpkg-deb in a Debian container.
#
# From beside your app's go.mod:
#
#   path/to/vero/scripts/package-linux.sh --name myapp --version 1.2.3 \
#       --app linux --entry myapp.py --worker ./cmd/worker --worker-name myapp-worker \
#       --icon icon.png --summary "One line about it" --description "A paragraph." \
#       --maintainer "Your Name <you@example.com>" [--homepage URL] [--id org.example.myapp] \
#       [--depends "python3-gi, gir1.2-gtk-4.0"] [--recommends "..."] \
#       [--categories "Utility;"] [--section utils] [--out dist/packages]
#
# Each package puts the app's folder (its Python, vero.py, and the worker)
# in /usr/lib/NAME, NAME on the PATH, a menu entry, and the icon at the
# sizes a desktop uses. The worker is built with main.version set.
#
# --ldflags adds to the worker's build flags: what your app builds into it.
#
# Nothing here names anyone: the maintainer, homepage and the rest are
# what you pass.
set -e
VERO=$(cd "$(dirname "$0")/.." && pwd)
. "$VERO/scripts/lib/docker.sh"
ROOT=$PWD
[ -f "$ROOT/go.mod" ] || { echo "run this from beside your app's go.mod" >&2; exit 2; }

NAME="" VERSION="" APP="" ENTRY="" WORKER="" WORKER_NAME="" ICON="" SUMMARY="" DESCRIPTION=""
MAINTAINER="" HOMEPAGE="" ID="" DEPENDS="python3 (>= 3.10), python3-gi, gir1.2-gtk-4.0"
RECOMMENDS="" CATEGORIES="Utility;" SECTION=utils OUT="$ROOT/dist/packages"
while [ $# -gt 0 ]; do
    case $1 in
        --name) NAME=$2 ;; --version) VERSION=$2 ;; --app) APP=$2 ;; --entry) ENTRY=$2 ;;
        --worker) WORKER=$2 ;; --worker-name) WORKER_NAME=$2 ;; --icon) ICON=$2 ;;
        --summary) SUMMARY=$2 ;; --description) DESCRIPTION=$2 ;; --maintainer) MAINTAINER=$2 ;;
        --homepage) HOMEPAGE=$2 ;; --id) ID=$2 ;; --depends) DEPENDS=$2 ;; --recommends) RECOMMENDS=$2 ;;
        --categories) CATEGORIES=$2 ;; --section) SECTION=$2 ;; --out) OUT=$2 ;;
        --ldflags) LDEXTRA=$2 ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
    shift 2
done
for v in NAME VERSION APP ENTRY WORKER WORKER_NAME ICON SUMMARY MAINTAINER; do
    eval "[ -n \"\$$v\" ]" || { echo "--$(echo $v | tr 'A-Z_' 'a-z-') is needed" >&2; exit 2; }
done
ID=${ID:-$NAME}
APP=$(cd "$APP" && pwd)
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)
vero_docker

echo "building the worker"
DIST="$ROOT/dist"
WORKER=$WORKER CSHIM=none WPF_APP=none TARGETS="linux/amd64 linux/arm64" \
    LDFLAGS="-s -w -X main.version=$VERSION ${LDEXTRA:-}" "$VERO/scripts/build-all.sh" >/dev/null

for arch in amd64 arm64; do
    STAGE=$(mktemp -d "$HOME/.cache/vero-deb.XXXXXX")
    P="$STAGE/${NAME}_${VERSION}_$arch"
    LIB="$P/usr/lib/$NAME"
    mkdir -p "$P/DEBIAN" "$LIB" "$P/usr/bin" "$P/usr/share/applications"
    # The app's own files, not what a run or a test left beside them.
    (cd "$APP" && find . -type f ! -name "$WORKER_NAME" ! -name core ! -path '*/__pycache__/*' ! -name '*.pyc' \
        | while read -r f; do mkdir -p "$LIB/$(dirname "$f")"; cp "$f" "$LIB/$f"; done)
    cp "$DIST/worker-linux-$arch" "$LIB/$WORKER_NAME"
    chmod 755 "$LIB/$WORKER_NAME" "$LIB/$ENTRY"
    printf '#!/bin/sh\nexec python3 /usr/lib/%s/%s "$@"\n' "$NAME" "$ENTRY" > "$P/usr/bin/$NAME"
    chmod 755 "$P/usr/bin/$NAME"
    for size in 64 128 256 512; do
        mkdir -p "$P/usr/share/icons/hicolor/${size}x${size}/apps"
        sips -z $size $size "$ICON" --out "$P/usr/share/icons/hicolor/${size}x${size}/apps/$ID.png" >/dev/null
    done
    cat > "$P/usr/share/applications/$ID.desktop" <<DESKTOP
[Desktop Entry]
Type=Application
Name=$NAME
Comment=$SUMMARY
Exec=$NAME
Icon=$ID
Categories=$CATEGORIES
StartupNotify=true
DESKTOP
    {
        echo "Package: $NAME"
        echo "Version: $VERSION"
        echo "Architecture: $arch"
        echo "Maintainer: $MAINTAINER"
        echo "Depends: $DEPENDS"
        [ -n "$RECOMMENDS" ] && echo "Recommends: $RECOMMENDS"
        echo "Section: $SECTION"
        echo "Priority: optional"
        [ -n "$HOMEPAGE" ] && echo "Homepage: $HOMEPAGE"
        echo "Description: $SUMMARY"
        # Debian's long description: each line indented, a blank one a dot.
        [ -n "$DESCRIPTION" ] && printf '%s\n' "$DESCRIPTION" | fold -s -w 72 | sed 's/^$/./; s/^/ /'
    } > "$P/DEBIAN/control"
    docker run --rm -v "$STAGE":/build -w /build debian:bookworm-slim \
        dpkg-deb --root-owner-group --build "${NAME}_${VERSION}_$arch" >/dev/null
    mv "$STAGE/${NAME}_${VERSION}_$arch.deb" "$OUT/"
    rm -rf "$STAGE"
    echo "built $OUT/${NAME}_${VERSION}_$arch.deb"
done
