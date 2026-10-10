#!/bin/sh
# Tests vero's packaging from start to finish on the example app, as a
# user would meet it: builds the packages, builds the site with a
# throwaway key, serves it from this Mac, and in a clean container adds
# the repository the way the site says, installs the example, and checks
# its worker. Then it releases 1.0.1 into the same site and checks that the
# system's own updates bring it.
#
#   scripts/test-repo.sh                          # Debian's apt
#   scripts/test-repo.sh --image ubuntu:24.04     # another apt system
#   scripts/test-repo.sh --image fedora:latest    # dnf, which checks every signature
#   scripts/test-repo.sh --image opensuse/tumbleweed  # zypper, the same
#   scripts/test-repo.sh --flatpak                # Flatpak, from the .flatpakref
#   scripts/test-repo.sh --image archlinux        # pacman, which checks every signature
#   scripts/test-repo.sh --image alpine           # apk, the same
#   scripts/test-repo.sh --image chimeralinux/chimera  # apk on Chimera
#   scripts/test-repo.sh --image ghcr.io/void-linux/void-glibc  # xbps, the same
#   scripts/test-repo.sh --vm freebsd             # pkg, in vero's FreeBSD VM
#   scripts/test-repo.sh --vm netbsd              # pkgin, in vero's NetBSD VM
#   scripts/test-repo.sh --vm openbsd             # pkg_add, in vero's OpenBSD VM
#   scripts/test-repo.sh --vm dragonfly           # pkg, in vero's DragonFly VM
#   scripts/test-repo.sh --vm illumos             # pkgin, with pkgsrc, in vero's OpenIndiana VM
#   scripts/test-repo.sh --mac                    # the disk image, on this Mac, and the appcast
#   scripts/test-repo.sh --vm windows             # the installer and the MSIX, in vero's Windows VM, over SSH
#   [--app path/to/vero-app.toml] [--port 8642] [--no-launch] [--limit SECONDS]
#
# --app tests your own app instead of vero's example, on every system but
# the Mac so far: its packages, its install, and its [[test.step]]s,
# recorded.
#
# After installing, it starts the app on a virtual display, plays the
# steps the app's vero-app.toml gives under [[test.step]] through vero's
# binding in the app, and records the window as it goes, as a GIF. Each
# run's GIF, steps and log go in ~/.cache/vero/test-results/SYSTEM/, and
# ~/.cache/vero/test-results/index.html shows every system tested so far.
# --no-launch skips that; --limit is how long the steps may take (600).
#
# A VM test starts the system's VM with its run script (scripts/run-*.sh
# --shell), which makes it the first time, and stops it at the end; the
# system's own packages - GTK and Python - come from its usual mirrors.
#
# The container tests need Docker and colima. Everything it makes
# is in ~/.cache/vero, the key included, so nothing in it is anyone's. The
# Flatpak test keeps GNOME's runtime, which it downloads from Flathub the
# first time, in a Docker volume called vero-test-flatpak; remove it with
# docker volume rm vero-test-flatpak.
set -e
VERO=$(cd "$(dirname "$0")/.." && pwd)
. "$VERO/scripts/lib/docker.sh"
IMAGE=debian:bookworm PORT=8642 FLATPAK="" VMSYS="" MAC="" LAUNCH=yes LIMIT=600 APP=""
while [ $# -gt 0 ]; do
    case $1 in
        --image) IMAGE=$2; shift ;; --port) PORT=$2; shift ;;
        --flatpak) FLATPAK=yes ;;
        --vm) VMSYS=$2; shift ;;
        --mac) MAC=yes ;;
        --no-launch) LAUNCH="" ;;
        --limit) LIMIT=$2; shift ;;
        --app) APP=$2; shift ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
    shift
done
if [ -n "$APP" ] && [ -n "$MAC" ]; then
    echo "--app can't test the Mac yet" >&2
    exit 2
fi
[ -n "$VMSYS$MAC" ] || vero_docker
CACHE="$HOME/.cache/vero"
# Each stage, numbered, with the time so far: a run can take an hour.
BEGAN=$(date +%s) STAGE=0
stage() {
    STAGE=$((STAGE + 1)) t=$(($(date +%s) - BEGAN))
    printf '== [%d/%s] %s (%d:%02d)\n' "$STAGE" "${STAGES:-?}" "$1" $((t / 60)) $((t % 60))
}
mkdir -p "$CACHE/bin"
go build -C "$VERO/cmd/vero-repo" -o "$CACHE/bin/vero-repo" .
REPO="$CACHE/bin/vero-repo"
# The app's names: APP_NAME, APP_ID, APP_WORKER, APP_TOML and the rest.
eval "$("$REPO" show --app "${APP:-$VERO/example/vero-app.toml}")" || exit 1
P=$APP_NAME
[ "$P" = vero-example ] && P=example
# Each system's own site, packages and recording folder, so that tests of
# two systems can run at once (on two --ports); the key is the app's.
LABEL=${VMSYS:+vm-$VMSYS}
[ -n "$FLATPAK" ] && LABEL=flatpak
[ -n "$MAC" ] && LABEL=macos
: "${LABEL:=$(printf '%s' "$IMAGE" | tr '/:' '--')}"
KEY="$CACHE/$P-key" SITE="$CACHE/$P-site-$LABEL" PACKAGES="$CACHE/$P-packages-$LABEL"
# The container reaches this Mac by this name: colima and Docker Desktop
# both answer it.
URL="http://host.docker.internal:$PORT"
NAME=vero-test-$LABEL
[ -f "$KEY/private.asc" ] || "$REPO" key --dir "$KEY" --name "$APP_DISPLAY test" --email test@example.com
rm -rf "$SITE"

# GTK 4 for Python, as the example starts it: the package's dependencies
# have to bring it, typelibs and all.
GTK="import gi; gi.require_version('Gtk', '4.0'); from gi.repository import Gtk"

# The icons the package puts in the hicolor theme, read as GTK 4 reads
# them, since a GTK 4 app loads its window's icon from the theme by its
# ID: GTK 4 from SmartOS's pkgsrc crashed doing that on illumos, which
# installing alone didn't show. It needs no display. Its argument is the
# prefix the package installs under.
ICONS="import gi, glob, sys; gi.require_version('Gdk', '4.0'); from gi.repository import Gdk; [Gdk.Texture.new_from_filename(f) for f in glob.glob(sys.argv[1] + '/share/icons/hicolor/*/apps/$APP_ID.png')]"

# What to build, and how to install, check and update, on this system.
if [ -n "$MAC" ]; then
    # The disk image, on this very Mac: mounted, the app copied out and
    # started, as a person would; then the appcast's next entry checked,
    # which is what the app's Sparkle would fetch.
    URL="http://127.0.0.1:$PORT" TARGETS=macos
elif [ "$VMSYS" = windows ]; then
    # Windows, in vero's VM, over SSH: scripts/lib/windows-test.ps1 and
    # the installers are copied in, and run there a phase at a time. The
    # VM needs SSH set up, which run-windows.sh --install does, or
    # scripts/setup-windows-ssh.sh in a VM installed before.
    URL="http://10.0.2.2:$PORT" TARGETS=windows
    [ -n "$APP_MSIX" ] && TARGETS=windows,msix
elif [ -n "$VMSYS" ]; then
    # A VM reaches this Mac at 10.0.2.2, as qemu's own network has it.
    URL="http://10.0.2.2:$PORT"
    TARGETS=$VMSYS
    VMDIR="$HOME/vm/vero-$VMSYS"
    case $VMSYS in
        freebsd) SSH_PORT=2222 PREFIX=/usr/local PY=python3
            INSTALL="fetch -q -o - $URL/install.sh | sh"
            XSETUP="pkg install -y tigervnc-server ImageMagick7 > /dev/null"
            UPDATE="pkg upgrade -y > /dev/null" ;;
        netbsd) SSH_PORT=2223 PREFIX=/usr/pkg PY=python3.12
            # pkgin, which the install page says to add if it's missing.
            SETUP="[ -x /usr/pkg/bin/pkgin ] || PKG_PATH=https://cdn.netbsd.org/pub/pkgsrc/packages/NetBSD/\$(uname -p)/\$(uname -r | cut -d. -f1-2)/All /usr/sbin/pkg_add pkgin"
            INSTALL="ftp -V -o - $URL/install.sh | sh"
            XSETUP="pkgin -y install tigervnc ImageMagick > /dev/null"
            UPDATE="pkgin -y -f upgrade > /dev/null" ;;
        openbsd) SSH_PORT=2224 PREFIX=/usr/local PY=python3
            # Python crashes importing GTK 4 on OpenBSD 7.9 for ARM, with
            # or without a display, whatever imports it: so here, GTK is
            # checked for, not started.
            GTK="import os; assert os.path.exists('/usr/local/lib/girepository-1.0/Gtk-4.0.typelib')"
            ICONS="pass"
            INSTALL="ftp -V -o - $URL/install.sh | sh"
            XSETUP="pkg_add -I tigervnc ImageMagick > /dev/null"
            UPDATE="PKG_PATH=$URL/openbsd/%a/:installpath pkg_add -u $APP_NAME" ;;
        dragonfly) SSH_PORT=2225 PREFIX=/usr/local PY=python3.11
            INSTALL="fetch -q -o - $URL/install.sh | sh"
            # Just the example: an upgrade of everything takes in
            # whatever DragonFly's own repository is changing that day,
            # which has been known to remove packages the example needs.
            XSETUP="pkg install -y tigervnc-server ImageMagick7 > /dev/null"
            UPDATE="pkg upgrade -y $APP_NAME > /dev/null" ;;
        illumos) SSH_PORT=2226 PREFIX=/opt/local PY=/opt/local/bin/python3.12
            # pkgsrc, as pkgsrc.smartos.org says to add it to OpenIndiana.
            SETUP="[ -x /opt/local/bin/pkgin ] || { cd /tmp && curl -fsSLO https://pkgsrc.smartos.org/packages/SmartOS/bootstrap/bootstrap-trunk-x86_64-20260811.tar.gz &&
                [ \$(/bin/digest -a sha1 bootstrap-trunk-x86_64-20260811.tar.gz) = e5e620ade4b45695aa385aea25227e94f49f078f ] &&
                gtar -zxpf bootstrap-trunk-x86_64-20260811.tar.gz -C / && /opt/local/bin/pkgin -y update > /dev/null; }"
            INSTALL="PATH=/opt/local/bin:/opt/local/sbin:\$PATH; curl -fsSL $URL/install.sh | sh"
            # OpenIndiana's own Xvnc and ImageMagick, as run-illumos.sh has.
            XSETUP="pkg install -q x11/server/xvnc image/imagemagick; true"
            UPDATE="/opt/local/bin/pkgin -y -f upgrade > /dev/null" ;;
        *) echo "unknown VM: $VMSYS (freebsd, dragonfly, netbsd, openbsd or illumos)" >&2; exit 2 ;;
    esac
    : "${SETUP:=true}"
    WORKER="$PREFIX/lib/$APP_NAME/$APP_WORKER -version"
    APPCMD="$PREFIX/bin/$APP_NAME"
    CHECK="PATH=$PREFIX/bin:\$PATH; command -v $APP_NAME && ls $PREFIX/share/applications/$APP_ID.desktop $PREFIX/lib/$APP_NAME/vero.py && $PY -c \"$GTK\""
    ICON_CHECK="$PY -c \"$ICONS\" $PREFIX"
elif [ -n "$FLATPAK" ]; then
    IMAGE=debian:trixie TARGETS=flatpak
    # The volume keeps GNOME's runtime between runs; the example, and
    # the remote its .flatpakref added, go, so that it's installed afresh.
    SETUP="apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq flatpak curl ca-certificates > /dev/null
        flatpak uninstall -y --noninteractive $APP_ID > /dev/null 2>&1 || true
        for r in \$(flatpak remotes --columns=name); do [ \$r = flathub ] || flatpak remote-delete --force \$r; done"
    INSTALL="flatpak install -y --noninteractive $URL/flatpak/$APP_NAME.flatpakref > /dev/null"
    WORKER="flatpak run --command=/app/lib/$APP_NAME/$APP_WORKER $APP_ID -version"
    CHECK="ls /var/lib/flatpak/exports/share/applications/$APP_ID.desktop /var/lib/flatpak/exports/share/icons/hicolor/128x128/apps/$APP_ID.png && flatpak run --command=python3 $APP_ID -c \"$GTK\""
    ICON_CHECK="flatpak run --command=python3 $APP_ID -c \"$ICONS\" /app"
    # /tmp is the sandbox's own, so the steps are in /var/tmp.
    APPCMD="flatpak run --filesystem=/var/tmp/vero-launch $APP_ID"
    XSETUP="DEBIAN_FRONTEND=noninteractive apt-get install -y -qq xvfb x11-apps x11-utils > /dev/null"
    UPDATE="flatpak update -y --noninteractive > /dev/null"
    RUN="--privileged -v vero-test-flatpak:/var/lib/flatpak"
else
    INSTALL="curl -fsSL $URL/install.sh | sh"
    WORKER="/usr/lib/$APP_NAME/$APP_WORKER -version"
    RUN=""
    CHECK="command -v $APP_NAME && ls /usr/share/applications/$APP_ID.desktop /usr/share/metainfo/$APP_ID.metainfo.xml /usr/lib/$APP_NAME/vero.py && python3 -c \"$GTK\""
    ICON_CHECK="python3 -c \"$ICONS\" /usr"
    APPCMD=$APP_NAME
    case $IMAGE in
        archlinux*)
            # Arch's image is x86_64 only: Docker runs it emulated here,
            # where pacman's download sandbox can't start, so it's turned
            # off. The image has no key of its own to sign others' with,
            # as an installed Arch has; pacman-key --init makes one.
            TARGETS=pacman RUN="--platform linux/amd64"
            SETUP="pacman-key --init > /dev/null 2>&1
                sed -i 's/^#DisableSandbox/DisableSandbox/' /etc/pacman.conf
                grep -q '^DisableSandbox' /etc/pacman.conf || sed -i '/^\[options\]/a DisableSandbox' /etc/pacman.conf
                pacman -Syu --noconfirm --needed curl > /dev/null 2>&1"
            # pacman refuses a package or an index that isn't signed by a
            # key it trusts; this checks that it was checked.
            CHECK="$CHECK && pacman -Qi $APP_NAME | grep -q '^Validated By *: Signature'"
            XSETUP="pacman -S --noconfirm --needed xorg-server-xvfb xorg-xwd xorg-xdpyinfo > /dev/null 2>&1"
            UPDATE="pacman -Syu --noconfirm > /dev/null" ;;
        alpine*)
            # Alpine has wget, as BusyBox's, but no curl.
            TARGETS=alpine
            SETUP=true
            INSTALL="wget -qO- $URL/install.sh | sh"
            XSETUP="apk add -q xvfb xwd xdpyinfo"
            UPDATE="apk upgrade -U > /dev/null" ;;
        *chimera*)
            # Chimera's image is a minimal install, without the fetch the
            # others have. Its sleep, FreeBSD's, takes only a number.
            TARGETS=chimera SLEEP=86400
            SETUP="apk add chimerautils-extra > /dev/null"
            INSTALL="fetch -qo - $URL/install.sh | sh"
            XSETUP="apk add -q xserver-xorg-xvfb xwd xdpyinfo"
            UPDATE="apk upgrade -U > /dev/null" ;;
        *void*)
            TARGETS=void
            SETUP="xbps-install -Syu xbps > /dev/null && xbps-install -y curl > /dev/null"
            XSETUP="xbps-install -y xorg-server-xvfb xwd xdpyinfo > /dev/null"
            UPDATE="xbps-install -Syu > /dev/null" ;;
        opensuse*)
            TARGETS=rpm
            SETUP="zypper -n -q install curl gzip > /dev/null"
            XSETUP="zypper -n -q install xorg-x11-server-Xvfb xwd xdpyinfo > /dev/null"
            CHECK="$CHECK && rpm -qi $APP_NAME | grep -A1 '^Signature' | grep -q RSA"
            UPDATE="zypper -n -q refresh > /dev/null && zypper -n -q update > /dev/null" ;;
        fedora*)
            TARGETS=rpm
            SETUP="dnf install -y -q curl > /dev/null"
            XSETUP="dnf install -y -q xorg-x11-server-Xvfb xwd xdpyinfo > /dev/null"
            # dnf refuses a package or an index that isn't signed; this
            # checks that the package is.
            CHECK="$CHECK && rpm -qi $APP_NAME | grep -A1 '^Signature' | grep -q RSA"
            # --refresh: dnf looks for a new index every six hours
            # otherwise.
            UPDATE="dnf upgrade --refresh -y -q > /dev/null" ;;
        *)
            TARGETS=deb
            SETUP="apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq curl ca-certificates > /dev/null"
            XSETUP="DEBIAN_FRONTEND=noninteractive apt-get install -y -qq xvfb x11-apps x11-utils > /dev/null"
            UPDATE="apt-get update -qq && apt-get upgrade -y -qq > /dev/null" ;;
    esac
fi

# release VERSION [BUILD [NOTES]]: the Mac's build number, when it isn't the
# version, and what's new, for the update prompt.
release() {
    rm -rf "$PACKAGES"
    (cd "$VERO" && scripts/package.sh --app "$APP_TOML" --version "$1" ${2:+--build "$2"} --targets "$TARGETS" --out "$PACKAGES")
    "$REPO" build --app "$APP_TOML" --packages "$PACKAGES" --key "$KEY" --url "$URL" --out "$SITE" ${3:+--notes "$3"}
}
if [ -n "$MAC" ] || [ "$VMSYS" = windows ]; then
    in_container() { sh -c "$1"; }
elif [ -n "$VMSYS" ]; then
    SSH="ssh -i $VMDIR/key -p $SSH_PORT -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o IdentitiesOnly=yes root@127.0.0.1"
    in_container() { $SSH "$1"; }
    # cd, not tar -C, which illumos's tar has not got.
    put_dir() { tar -C "$1" -cf - . | $SSH "rm -rf $2 && mkdir -p $2 && cd $2 && tar xf -"; }
    get_dir() { $SSH "cd $1 && tar cf - ." | tar -C "$2" -xf -; }
else
    in_container() { docker exec "$NAME" sh -c "$1"; }
    put_dir() { docker exec "$NAME" sh -c "rm -rf $2 && mkdir -p $2" && docker cp "$1/." "$NAME:$2"; }
    get_dir() { docker cp "$NAME:$1/." "$2"; }
fi
cleanup() {
    if [ -n "$MAC" ]; then
        pkill -f "$CACHE/mac-test/" 2>/dev/null || true
        hdiutil detach -quiet "$CACHE/mac-test/mnt" 2>/dev/null || true
        rm -rf "$CACHE/mac-test" "$HOME/Library/Application Support/Example"
    elif [ "$VMSYS" = windows ]; then
        [ -n "$QEMU" ] && windows_stop
    elif [ -n "$VMSYS" ]; then
        if [ -f "$VMDIR/qemu.pid" ]; then
            $SSH 'PATH=/sbin:/usr/sbin:$PATH; poweroff 2>/dev/null || shutdown -p now' >/dev/null 2>&1 || true
            sleep 15
            kill "$(cat "$VMDIR/qemu.pid")" 2>/dev/null || true
            rm -f "$VMDIR/qemu.pid"
        fi
    else
        docker rm -f "$NAME" >/dev/null 2>&1 || true
    fi
    [ -n "$SERVER" ] && kill "$SERVER" 2>/dev/null || true
}
trap cleanup EXIT

# mac_test: the disk image on this Mac, then the appcast after an update.
mac_test() {
    T="$CACHE/mac-test"
    rm -rf "$T" && mkdir -p "$T/mnt"
    DMG=$(ls "$SITE"/macos/vero-example-1.0.0-macos.dmg)
    echo "== opening $DMG on this Mac"
    hdiutil attach -quiet -nobrowse -mountpoint "$T/mnt" "$DMG"
    APP=$(ls -d "$T/mnt"/*.app)
    ditto "$APP" "$T/$(basename "$APP")"
    hdiutil detach -quiet "$T/mnt"
    APP="$T/$(basename "$APP")"
    codesign --verify --deep --strict "$APP" || { echo "FAIL: the app's signature doesn't verify" >&2; exit 1; }
    short=$(plutil -extract CFBundleShortVersionString raw "$APP/Contents/Info.plist")
    build=$(plutil -extract CFBundleVersion raw "$APP/Contents/Info.plist")
    [ "$short" = 1.0.0 ] && [ "$build" = 100 ] || { echo "FAIL: the app says $short ($build), not 1.0.0 (100)" >&2; exit 1; }
    # The installer package: it installs into /Applications, not over a
    # copy elsewhere, and hands the app to whoever installed it.
    PKG="$SITE/macos/vero-example-1.0.0-macos.pkg"
    [ -f "$PKG" ] || { echo "FAIL: no .pkg on the site" >&2; exit 1; }
    curl -fsSL "$URL/latest.json" | grep -q '"macos-pkg"' || { echo "FAIL: latest.json has no macos-pkg" >&2; exit 1; }
    pkgutil --expand "$PKG" "$T/pkg"
    INFO=$(cat "$T"/pkg/*.pkg/PackageInfo)
    case $INFO in *"<relocate>"*) case $INFO in *"<relocate/>"*) ;; *) echo "FAIL: the .pkg relocates the app" >&2; exit 1 ;; esac ;; esac
    sh -n "$T"/pkg/*.pkg/Scripts/postinstall && grep -q 'chown -R' "$T"/pkg/*.pkg/Scripts/postinstall || { echo "FAIL: the .pkg has no postinstall" >&2; exit 1; }
    echo "ok: the .pkg installs into /Applications and gives the app to its user"
    got=$("$APP/Contents/Resources/worker" -version)
    [ "$got" = 1.0.0 ] || { echo "FAIL: the bundled worker says $got, not 1.0.0" >&2; exit 1; }
    open "$APP"
    sleep 6
    pgrep -f "$APP/Contents/MacOS/" >/dev/null || { echo "FAIL: the app didn't stay running" >&2; exit 1; }
    pgrep -f "Application Support/Example/bin/worker" >/dev/null || { echo "FAIL: the app didn't start its worker" >&2; exit 1; }
    pkill -f "$APP/Contents/MacOS/"
    echo "ok: 1.0.0 installed from the disk image, and the app started its worker"
    echo "== releasing 1.0.1 into the same site"
    release 1.0.1 101 "Faster.

- One thing"
    # What Sparkle would fetch: the newest item, its signature checked
    # with the key the app would carry.
    "$REPO" check --app "$VERO/example/vero-app.toml" --url "$URL" --key "$KEY" "$SITE" >/dev/null
    newest=$(curl -fsSL "$URL/macos/appcast.xml" | grep -o '<sparkle:version>[^<]*' | head -1 | cut -d'>' -f2)
    [ "$newest" = 101 ] || { echo "FAIL: the appcast's newest build is $newest, not 101" >&2; exit 1; }
    curl -fsSL "$URL/macos/vero-example-1.0.1-notes.html" | grep -q '<li>One thing</li>' || { echo "FAIL: no release notes for 1.0.1" >&2; exit 1; }
    curl -fsSL "$URL/macos/appcast.xml" | grep -q 'releaseNotesLink>[^<]*vero-example-1.0.1-notes.html' || { echo "FAIL: the appcast doesn't link the notes" >&2; exit 1; }
    curl -fsSL "$URL/homebrew/vero-example.rb" | grep -q 'version "1.0.1"' || { echo "FAIL: the cask isn't 1.0.1" >&2; exit 1; }
    echo "ok: the appcast offers 1.0.1, signed, with its notes, and the cask has it"
    # An app with Sparkle.framework inside, signed as vero signs one:
    # nested code first, and Gatekeeper's strict check passes.
    SPARKLE_VERSION=2.6.4
    SPARKLE="$CACHE/sparkle-$SPARKLE_VERSION"
    if [ ! -d "$SPARKLE/Sparkle.framework" ]; then
        mkdir -p "$SPARKLE"
        curl -fsSL "https://github.com/sparkle-project/Sparkle/releases/download/$SPARKLE_VERSION/Sparkle-$SPARKLE_VERSION.tar.xz" | tar -xJf - -C "$SPARKLE"
    fi
    (cd "$VERO/cmd/vero-repo" && VERO_SPARKLE_FRAMEWORK="$SPARKLE/Sparkle.framework" go test -count=1 -run TestSignWithSparkle . >/dev/null) \
        || { echo "FAIL: an app with Sparkle.framework doesn't sign" >&2; exit 1; }
    echo "ok: an app with Sparkle.framework inside signs and verifies"
    echo "PASS"
}

# windows_test: the installers in vero's Windows VM, over SSH: the NSIS
# installer, installed, started on the desktop, updated to 1.0.1 and
# uninstalled; then the MSIX's files, registered, started and removed.
windows_test() {
    . "$VERO/scripts/lib/windows-disc.sh"
    VMDIR="$HOME/vm/vero-windows"
    # Not 2222, which is FreeBSD VM's, so that both can be tested at once.
    SSHPORT=2227
    [ -f "$VMDIR/disk.qcow2" ] || { echo "no Windows VM in $VMDIR: scripts/run-windows.sh --install makes one" >&2; exit 1; }
    [ -f "$WINDOWS_KEY" ] || { echo "no $WINDOWS_KEY: scripts/setup-windows-ssh.sh sets up SSH into the VM" >&2; exit 1; }
    windows_running && { echo "the Windows VM is running already: stop it first" >&2; exit 1; }
    PAY="$CACHE/windows-test"
    rm -rf "$PAY" && mkdir -p "$PAY"
    cp "$PACKAGES"/*-setup.exe "$VERO/scripts/lib/windows-test.ps1" "$VERO/scripts/lib/launch-windows.ps1" "$PAY/"
    [ -n "$APP_MSIX" ] && cp "$PACKAGES"/*.msix "$PAY/"
    echo "== booting the Windows VM (about a minute)"
    "$VERO/scripts/run-windows.sh" --headless --payload "$PAY" --ssh "$SSHPORT" >"$CACHE/windows-vm.log" 2>&1 &
    QEMU=$!
    windows_wait "$SSHPORT" 300 || { echo "FAIL: the VM never answered over SSH: scripts/setup-windows-ssh.sh sets it up" >&2; exit 1; }
    windows_ssh "$SSHPORT" "New-Item -ItemType Directory -Force C:/vero-test | Out-Null; Remove-Item -Recurse -Force C:/vero-test/*"
    windows_scp "$SSHPORT" "$PAY"/* vero@127.0.0.1:C:/vero-test/
    # phase NAME: one of windows-test.ps1's phases, its lines printed.
    phase() {
        windows_ssh "$SSHPORT" "powershell -NoProfile -ExecutionPolicy Bypass -File C:/vero-test/windows-test.ps1 -Phase $1 -Name $APP_NAME -Exe $APP_EXE -Worker $APP_WORKER.exe ${APP_MSIX:+-Identity $APP_MSIX} -Limit $LIMIT" | tr -d '\r' | tee -a "$PAY/results.txt"
    }
    # expect LINE: windows-test.ps1 printed that line.
    expect() { grep -qx "$1" "$PAY/results.txt" || { echo "FAIL: no \"$1\" from the VM" >&2; exit 1; }; }
    echo "== the NSIS installer"
    phase nsis
    for line in "nsis-worker: 1.0.0" "nsis-app: True" "nsis-startmenu: True" "nsis-uninstaller: True" "nsis-runs: True"; do expect "$line"; done
    python3 "$VERO/scripts/lib/qmp.py" /tmp/vero-qmp.sock shot "$PAY/nsis.ppm" && sips -s format png "$PAY/nsis.ppm" --out "$PAY/nsis.png" >/dev/null
    echo "ok: 1.0.0 installed and running (the desktop: $PAY/nsis.png)"
    if [ -n "$LAUNCH" ]; then
        echo "== starting the app and playing its steps, recorded"
        lb=$(date +%s)
        L="$PAY/launch" out="$RESULTS/windows"
        rm -rf "$L" "$out" && mkdir -p "$L" "$out"
        "$REPO" steps --app "$APP_TOML" --out "$L" >/dev/null
        sed -n "s/^export \([^=]*\)='\(.*\)'\$/\1=\2/p" "$L/env.sh" >"$L/env.txt"
        windows_scp "$SSHPORT" -r "$L" vero@127.0.0.1:C:/vero-test/
        phase launch
        windows_scp "$SSHPORT" -r vero@127.0.0.1:C:/vero-test/launch "$out.tmp" && cp -R "$out.tmp/." "$out/" && rm -rf "$out.tmp"
        rm -rf "$out/files" "$out/steps.json" "$out/env.txt"
        finish_launch "$out" "$lb"
    fi
    echo "== releasing 1.0.1, and installing it over 1.0.0"
    release 1.0.1
    cp "$PACKAGES"/*-1.0.1-*-setup.exe "$PAY/"
    windows_scp "$SSHPORT" "$PAY"/*-1.0.1-*-setup.exe vero@127.0.0.1:C:/vero-test/
    phase nsis-update
    expect "nsis-updated: 1.0.1"
    phase nsis-done
    expect "nsis-uninstalled: True"; expect "nsis-uninstall-entry-gone: True"
    echo "ok: 1.0.1 installed over it, and uninstalled cleanly"
    if [ -z "$APP_MSIX" ]; then
        echo "PASS (no MSIX: the app's vero-app.toml has no [wpf.msix])"
        return
    fi
    echo "== the MSIX"
    phase msix
    for line in "msix-installed: True" "msix-worker: 1.0.0" "msix-runs: True"; do expect "$line"; done
    phase msix-done
    expect "msix-removed: True"
    echo "ok: the MSIX's app ran, and was removed"
    echo "PASS"
}

# launch_test: the installed app started on a virtual display, the steps
# played in it, and the window recorded, as a GIF in the results folder.
RESULTS="$CACHE/test-results/$APP_NAME"
launch_test() {
    stage "starting the app and playing its steps, recorded"
    lb=$(date +%s)
    out="$RESULTS/$LABEL" L="$CACHE/launch-$LABEL"
    rm -rf "$out" "$L" && mkdir -p "$out" "$L"
    "$REPO" steps --app "$APP_TOML" --out "$L" >/dev/null
    cp "$VERO/scripts/lib/launch-test.sh" "$L/"
    in_container "${XSETUP:-true}" || { echo "FAIL: couldn't add a virtual display" >&2; exit 1; }
    put_dir "$L" /var/tmp/vero-launch
    in_container "sh /var/tmp/vero-launch/launch-test.sh $LIMIT $APPCMD" || true
    get_dir /var/tmp/vero-launch "$out"
    rm -f "$out/launch-test.sh" "$out/steps.json"
    rm -rf "$out/files"
    finish_launch "$out" "$lb"
}

# finish_launch DIR START: DIR's frames made a GIF, and whether the app
# stayed up and its steps passed, judged and put in the summary.
finish_launch() {
    out=$1 lb=$2
    make_gif "$out"
    # VERO_KEEP_FRAMES=1 keeps them, to see what a recording was made from.
    [ -n "$VERO_KEEP_FRAMES" ] || rm -rf "$out/frames"
    why=""
    if [ -f "$out/exited" ]; then why="the app exited ($(cat "$out/exited"))"
    elif [ ! -f "$out/result.json" ]; then why="the steps didn't finish in ${LIMIT}s"
    elif ! grep -Eq '"ok" ?: ?true' "$out/result.json"; then why="a step failed"
    fi
    secs=$(($(date +%s) - lb))
    # Which of the app's versions it was: its git commit, and whether it
    # had changes not yet committed.
    commit=$(git -C "$APP_DIR" rev-parse --short HEAD 2>/dev/null || true)
    [ -n "$commit" ] && [ -n "$(git -C "$APP_DIR" status --porcelain 2>/dev/null | head -1)" ] && commit="$commit, with changes"
    printf '%s\n%s\n%s\n%s\n' "$([ -n "$why" ] && echo FAIL || echo PASS)" "$secs" "$why" "$commit" >"$out/status"
    summary
    grep 'vero test:' "$out/app.log" | sed 's/^vero test: /   /'
    if [ -n "$why" ]; then
        echo "FAIL: $why; the app said:" >&2
        grep -v 'vero test:' "$out/app.log" | tail -15 >&2
        echo "(recording and log in $out)" >&2
        exit 1
    fi
    echo "ok: the app started and its steps passed; recorded in $out/app.gif"
}

# make_gif DIR: DIR/frames made into DIR/app.gif and DIR/app.png.
make_gif() { sh "$VERO/scripts/lib/make-gif.sh" "$1"; }

# summary: index.html in the results folder, every app and system tested
# so far.
summary() { sh "$VERO/scripts/lib/results-page.sh" "$CACHE/test-results"; }

STAGES=4
[ -n "$LAUNCH" ] && STAGES=5
stage "releasing 1.0.0"
if [ -n "$MAC" ]; then release 1.0.0 100; else release 1.0.0; fi
go build -o "$CACHE/bin/serve" "$VERO/scripts/lib/serve.go"
"$CACHE/bin/serve" "$SITE" "$PORT" &
SERVER=$!
# Up before anything asks it: the last run's may still hold the port.
n=0
until nc -z 127.0.0.1 "$PORT" 2>/dev/null && kill -0 $SERVER 2>/dev/null; do
    n=$((n + 1))
    [ $n -gt 60 ] && { echo "FAIL: the site's server isn't answering on port $PORT; is another test still running?" >&2; exit 1; }
    kill -0 $SERVER 2>/dev/null || { sleep 1; "$CACHE/bin/serve" "$SITE" "$PORT" & SERVER=$!; }
    sleep 0.5
done

if [ -n "$MAC" ]; then
    mac_test
    exit 0
fi
if [ "$VMSYS" = windows ]; then
    windows_test
    exit 0
fi
if [ -n "$VMSYS" ]; then
    "$VERO/scripts/run-$VMSYS.sh" --shell --no-open >/dev/null
    IMAGE="the $VMSYS VM"
else
    docker rm -f "$NAME" >/dev/null 2>&1 || true
    # shellcheck disable=SC2086
    docker run -d --name "$NAME" $RUN --add-host=host.docker.internal:host-gateway "$IMAGE" sleep "${SLEEP:-infinity}" >/dev/null
fi

stage "installing it as the site says, in $IMAGE${FLATPAK:+, with Flatpak}"
in_container "$SETUP"
in_container "$INSTALL"
got=$(in_container "$WORKER")
[ "$got" = 1.0.0 ] || { echo "FAIL: the installed worker says $got, not 1.0.0" >&2; exit 1; }
in_container "$CHECK" >/dev/null ||
    { echo "FAIL: the package lacks its command, menu entry, metadata, binding, signature or GTK" >&2; exit 1; }
in_container "$ICON_CHECK" >/dev/null 2>&1 ||
    { echo "FAIL: GTK 4 crashed or failed reading the app's icon, which the app loads when it starts" >&2; exit 1; }
echo "ok: 1.0.0 installed from the repository, and its worker answers"
[ -n "$LAUNCH" ] && launch_test

stage "releasing 1.0.1 into the same site"
release 1.0.1
stage "updating to 1.0.1 with the system's own updates"
in_container "$UPDATE"
got=$(in_container "$WORKER")
[ "$got" = 1.0.1 ] || { echo "FAIL: after updating, the worker says $got, not 1.0.1" >&2; exit 1; }
echo "ok: the system's updates brought 1.0.1"
echo "PASS"
