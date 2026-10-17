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
#   scripts/test-repo.sh --vm illumos             # pkg, from an IPS repository, in vero's OpenIndiana VM
#   scripts/test-repo.sh --mac                    # the disk image, on this Mac, and the appcast
#   scripts/test-repo.sh --vm windows             # the installer and the MSIX, in vero's Windows VM, over SSH
#   [--app path/to/vero-app.toml] [--port 8642] [--no-launch] [--limit SECONDS]
#   [--no-update] [--no-record] [--fresh] [--packages DIR] [--key]
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
# --no-record plays the steps without recording them, and --no-update
# leaves out releasing 1.0.1 and updating to it.
#
# Each system starts from a ready-made copy with the app's dependencies
# installed, and the system's own tools for the test, so that a run does
# not install GTK afresh: a saved Docker image (vero-ready-SYSTEM:KEY) for
# a container, and a qcow2 snapshot (~/vm/vero-SYSTEM/ready-KEY/) for a VM.
# The test then runs on a throwaway copy of it: a new container, or a qcow2
# overlay that is deleted afterwards. KEY is a hash of the system, of what
# is installed in it and of what the app needs from it, and of the week (a
# month for a VM), so that each is made again as its system moves on.
# The first run with a new KEY makes it, which takes as long as a run did
# without one. --fresh starts from the system itself instead, as a person
# would, and --key prints the system's KEY and stops.
#
# --packages DIR takes the packages from DIR/1.0.0 and DIR/1.0.1, which
# test-all.sh builds once for every system it tests, waiting for
# DIR/VERSION.done, instead of building them here.
#
# A VM test starts the system's VM with its run script (scripts/run-*.sh
# --shell), which makes it the first time, and stops it at the end; the
# system's own packages - GTK and Python - come from its usual mirrors.
# The VM's own disk is not changed by a test: its snapshot and each run's
# overlay are made on top of it.
#
# The container tests need Docker and colima. Everything it makes
# is in ~/.cache/vero, the key included, so nothing in it is anyone's. The
# Flatpak test keeps GNOME's runtime, which it downloads from Flathub the
# first time, in a Docker volume called vero-test-flatpak; remove it with
# docker volume rm vero-test-flatpak.
set -e
VERO=$(cd "$(dirname "$0")/.." && pwd)
. "$VERO/scripts/lib/docker.sh"
IMAGE=debian:trixie PORT=8642 FLATPAK="" VMSYS="" MAC="" LAUNCH=yes LIMIT=600 APP=""
UPDATES=yes RECORD=yes FRESH="" PREBUILT="" KEYONLY=""
while [ $# -gt 0 ]; do
    case $1 in
        --image) IMAGE=$2; shift ;; --port) PORT=$2; shift ;;
        --flatpak) FLATPAK=yes ;;
        --vm) VMSYS=$2; shift ;;
        --mac) MAC=yes ;;
        --no-launch) LAUNCH="" ;;
        --limit) LIMIT=$2; shift ;;
        --app) APP=$2; shift ;;
        --update) UPDATES=yes ;; --no-update) UPDATES="" ;;
        --record) RECORD=yes ;; --no-record) RECORD="" ;;
        --fresh) FRESH=yes ;;
        --packages) PREBUILT=$2; shift ;;
        --key) KEYONLY=yes ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
    shift
done
if [ -n "$APP" ] && [ -n "$MAC" ]; then
    echo "--app can't test the Mac yet" >&2
    exit 2
fi
[ -n "$VMSYS$MAC" ] || vero_docker
# Everything it makes is in vero's cache, which VERO_CACHE can move, so
# that two checkouts of vero can test at once without sharing files.
CACHE="${VERO_CACHE:-$HOME/.cache/vero}"
VERO_CACHE=$CACHE
. "$VERO/scripts/lib/space.sh"
# Each stage, numbered, with the time so far: a run can take an hour.
BEGAN=$(date +%s) STAGE=0
stage() {
    STAGE=$((STAGE + 1)) t=$(($(date +%s) - BEGAN))
    printf '== [%d/%s] %s (%d:%02d)\n' "$STAGE" "${STAGES:-?}" "$1" $((t / 60)) $((t % 60))
}
mkdir -p "$CACHE/bin"
# vero-repo: the one VERO_REPO names (vero ship sets it), or built from
# this checkout.
if [ -n "$VERO_REPO" ]; then [ "$VERO_REPO" -ef "$CACHE/bin/vero-repo" ] || cp "$VERO_REPO" "$CACHE/bin/vero-repo"; else go build -C "$VERO/cmd/vero-repo" -o "$CACHE/bin/vero-repo" .; fi
REPO="$CACHE/bin/vero-repo"
# package.sh uses the same one, rather than building its own.
export VERO_REPO="$REPO"
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
# The port is in each folder's name, so that two runs at once never share
# one; a run that passes removes its site and packages afterwards.
KEY="$CACHE/$P-key" SITE="$CACHE/$P-site-$LABEL-$PORT" PACKAGES="$CACHE/$P-packages-$LABEL-$PORT"
# The container reaches this Mac by this name: colima and Docker Desktop
# both answer it.
URL="http://host.docker.internal:$PORT"
# The port is in the name, so that two runs at once never share one.
NAME=vero-test-$LABEL-$PORT
[ -f "$KEY/private.asc" ] || "$REPO" key --dir "$KEY" --name "$APP_DISPLAY test" --email test@example.com

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
            UPDATE="pkg upgrade -y > /dev/null"
            # FreeBSD's packages roll on, so a snapshot is made from an
            # upgraded system.
            REFRESH="pkg upgrade -y > /dev/null" ;;
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
            # Emulated, it takes minutes to boot, so each run carries on
            # from its snapshot's saved state, with the clock set again, in
            # the form of date that RESUME gives.
            RESUME=+%Y%m%d%H%M.%S
            INSTALL="fetch -q -o - $URL/install.sh | sh"
            # Just the example: an upgrade of everything takes in
            # whatever DragonFly's own repository is changing that day,
            # which has been known to remove packages the example needs.
            XSETUP="pkg install -y tigervnc-server ImageMagick7 > /dev/null"
            UPDATE="pkg upgrade -y $APP_NAME > /dev/null" ;;
        illumos) SSH_PORT=2226 PREFIX=/usr PY=/usr/bin/python3.9
            RESUME=+%m%d%H%M%Y.%S
            # OpenIndiana's own packages, from which the app's IPS package
            # takes GTK 4, libadwaita and PyGObject.
            INSTALL="curl -fsSL $URL/install.sh | sh"
            # OpenIndiana's own Xvnc and ImageMagick, as run-illumos.sh has.
            XSETUP="pkg install -q x11/server/xvnc image/imagemagick; true"
            # 4 is pkg's "nothing to do".
            UPDATE="pkg update -q application/$APP_NAME; s=\$?; [ \$s -eq 0 ] || [ \$s -eq 4 ]"
            # OpenIndiana rolls on, and a system whose incorporation lags
            # its repository cannot install what a newer package needs, so
            # a snapshot is made from one brought up to date, restarted
            # into its new boot environment when the update made one.
            REFRESH="pkg update -q --accept; s=\$?; [ \$s -eq 4 ] && exit 0; [ \$s -eq 0 ] && touch /var/tmp/vero-reboot; exit \$s" ;;
        *) echo "unknown VM: $VMSYS (freebsd, dragonfly, netbsd, openbsd or illumos)" >&2; exit 2 ;;
    esac
    : "${SETUP:=true}"
    # A VM keeps what earlier tests added: the repositories of their sites,
    # long gone, which make the system's updates fail. Each is removed
    # first, and this test's site adds its own again.
    case $VMSYS in
        # What was installed from them goes too, as on illumos below: a
        # copy left from an earlier run has the same version, so pkg would
        # keep it in place of the build under test.
        freebsd|dragonfly) FORGET="for f in /usr/local/etc/pkg/repos/*.conf; do grep -qs 10.0.2.2 \$f || continue; r=\$(basename \$f .conf); for n in \$(pkg query -a '%R %n' | awk -v r=\$r '\$1 == r {print \$2}'); do pkg delete -y \$n; done; rm -f \$f; done; true" ;;
        # The app's own package from an earlier run goes too, since one of
        # the same version is kept in place of the build under test.
        netbsd) FORGET="[ -f /usr/pkg/etc/pkgin/repositories.conf ] && sed -i.vero '/10.0.2.2/d' /usr/pkg/etc/pkgin/repositories.conf; /usr/sbin/pkg_delete $APP_NAME 2>/dev/null; true" ;;
        openbsd) FORGET="pkg_delete $APP_NAME 2>/dev/null; true" ;;
        # The packages installed from them go too: one that is left can
        # hold back every later update, once its dependencies move on. The
        # app goes as well, even when its publisher was removed before, since
        # a newer copy from an earlier run is kept in place of the build
        # under test.
        illumos) FORGET="for p in \$(pkg publisher -H | awk '\$NF ~ /10.0.2.2/ {print \$1}'); do for f in \$(pkg list -Hv | sed -n \"s|^\\(pkg://\$p/[^@]*\\)@.*|\\1|p\"); do pkg uninstall \$f; done; pkg unset-publisher \$p; done; pkg uninstall application/$APP_NAME > /dev/null 2>&1; true" ;;
        *) FORGET=true ;;
    esac
    SETUP="$FORGET; $SETUP"
    WORKER="$PREFIX/lib/$APP_NAME/$APP_WORKER -version"
    APPCMD="$PREFIX/bin/$APP_NAME"
    CHECK="PATH=$PREFIX/bin:\$PATH; command -v $APP_NAME && ls $PREFIX/share/applications/$APP_ID.desktop $PREFIX/lib/$APP_NAME/vero.py && $PY -c \"$GTK\""
    ICON_CHECK="$PY -c \"$ICONS\" $PREFIX"
elif [ -n "$FLATPAK" ]; then
    IMAGE=debian:trixie TARGETS=flatpak
    # The volume keeps GNOME's runtime between runs; the example, and
    # the remote its .flatpakref added, go, so that it's installed afresh.
    # Flatpak itself goes in the ready-made system; the volume's runtime
    # stays, and the app and its remote are removed at every run.
    SETUP="apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq flatpak curl ca-certificates > /dev/null"
    EACH="flatpak uninstall -y --noninteractive $APP_ID > /dev/null 2>&1 || true
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
            DEPS_IN="pacman -U --noconfirm /tmp/vero-ready/*-x86_64.pkg.tar.zst > /dev/null 2>&1 && pacman -Rdd --noconfirm $APP_NAME > /dev/null && rm -rf /var/cache/pacman/pkg/*"
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
            UPDATE="apk upgrade -U > /dev/null"
            # apk removes what nothing installed asked for, so the app's
            # dependencies are asked for by name before it goes.
            DEPS_IN="apk add -q --allow-untrusted /tmp/vero-ready/*-\$(apk --print-arch).apk && apk add -q \$(apk info -qR $APP_NAME) && apk del -q $APP_NAME" ;;
        *chimera*)
            # Chimera's image is a minimal install, without the fetch the
            # others have. Its sleep, FreeBSD's, takes only a number.
            TARGETS=chimera SLEEP=86400
            SETUP="apk add chimerautils-extra > /dev/null"
            INSTALL="fetch -qo - $URL/install.sh | sh"
            XSETUP="apk add -q sway grim"
            UPDATE="apk upgrade -U > /dev/null"
            DEPS_IN="apk add -q --allow-untrusted /tmp/vero-ready/*-\$(apk --print-arch).apk > /dev/null && apk add -q \$(apk info -qR $APP_NAME) > /dev/null && apk del -q $APP_NAME > /dev/null" ;;
        *void*)
            TARGETS=void
            SETUP="xbps-install -Syu xbps > /dev/null && xbps-install -y curl > /dev/null"
            XSETUP="xbps-install -y xorg-server-xvfb xwd xdpyinfo > /dev/null"
            UPDATE="xbps-install -Syu > /dev/null"
            # xbps installs only from a repository: a local one, unsigned,
            # as xbps allows for a folder.
            DEPS_IN="mkdir -p /tmp/vero-repo && cp /tmp/vero-ready/*.\$(xbps-uhelper arch).xbps /tmp/vero-repo/ && xbps-rindex -a /tmp/vero-repo/*.xbps > /dev/null && xbps-install -y -R /tmp/vero-repo $APP_NAME > /dev/null && xbps-remove -y $APP_NAME > /dev/null && rm -rf /tmp/vero-repo" ;;
        opensuse*)
            TARGETS=rpm
            SETUP="zypper -n -q install curl gzip > /dev/null"
            XSETUP="zypper -n -q install xorg-x11-server-Xvfb xwd xdpyinfo > /dev/null"
            CHECK="$CHECK && rpm -qi $APP_NAME | grep -A1 '^Signature' | grep -q RSA"
            UPDATE="zypper -n -q refresh > /dev/null && zypper -n -q update > /dev/null"
            DEPS_IN="zypper -n -q --no-gpg-checks install --allow-unsigned-rpm /tmp/vero-ready/*.\$(uname -m).rpm > /dev/null && rpm -e --nodeps $APP_NAME && zypper -n -q clean" ;;
        fedora*)
            TARGETS=rpm
            SETUP="dnf install -y -q curl > /dev/null"
            XSETUP="dnf install -y -q xorg-x11-server-Xvfb xwd xdpyinfo > /dev/null"
            # dnf refuses a package or an index that isn't signed; this
            # checks that the package is.
            CHECK="$CHECK && rpm -qi $APP_NAME | grep -A1 '^Signature' | grep -q RSA"
            # --refresh: dnf looks for a new index every six hours
            # otherwise.
            UPDATE="dnf upgrade --refresh -y -q > /dev/null"
            DEPS_IN="dnf install -y -q --nogpgcheck /tmp/vero-ready/*.\$(uname -m).rpm > /dev/null && rpm -e --nodeps $APP_NAME && dnf clean packages > /dev/null" ;;
        *)
            TARGETS=deb
            SETUP="apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq curl ca-certificates > /dev/null"
            XSETUP="DEBIAN_FRONTEND=noninteractive apt-get install -y -qq xvfb x11-apps x11-utils > /dev/null"
            UPDATE="apt-get update -qq && apt-get upgrade -y -qq > /dev/null"
            DEPS_IN="DEBIAN_FRONTEND=noninteractive apt-get install -y -qq /tmp/vero-ready/*_\$(dpkg --print-architecture).deb > /dev/null && dpkg --purge $APP_NAME > /dev/null && apt-get clean" ;;
    esac
fi

# belongs FILE: whether FILE, one of the packages that test-all.sh built
# for every system at once, is one of this system's.
belongs() {
    for t in $(echo "$TARGETS" | tr ',' ' '); do
        case $t:$1 in
            deb:*.deb | rpm:*.rpm | pacman:*.pkg.tar.zst | void:*.xbps | flatpak:*.flatpak) return 0 ;;
            msix:*.msix | windows:*-setup.exe | chimera:*-chimera-*.apk) return 0 ;;
            alpine:*-chimera-*.apk) ;;
            alpine:*.apk) return 0 ;;
            freebsd:*-freebsd-* | dragonfly:*-dragonfly-* | netbsd:*-netbsd-* | openbsd:*-openbsd-* | illumos:*-illumos-*) return 0 ;;
        esac
    done
    return 1
}

# release VERSION [BUILD [NOTES]]: the Mac's build number, when it isn't the
# version, and what's new, for the update prompt.
release() {
    rm -rf "$PACKAGES"
    if [ -n "$PREBUILT" ] && [ -z "$MAC" ]; then
        # Built once for every system by test-all.sh, perhaps still
        # building: this system's are copied out, as clones where the disk
        # can make them, since vero-repo build may change what it is given.
        n=0
        until [ -f "$PREBUILT/$1.done" ]; do
            [ -f "$PREBUILT/$1.failed" ] && { echo "FAIL: the packages for $1 did not build; see $PREBUILT/$1.log" >&2; exit 1; }
            n=$((n + 1))
            [ $n -gt 7200 ] && { echo "FAIL: the packages for $1 did not come in two hours" >&2; exit 1; }
            sleep 0.5
        done
        mkdir -p "$PACKAGES"
        for f in "$PREBUILT/$1"/*; do
            belongs "${f##*/}" || continue
            cp -c "$f" "$PACKAGES/" 2>/dev/null || cp "$f" "$PACKAGES/"
        done
        ls "$PACKAGES" | grep -q . || { echo "FAIL: $PREBUILT/$1 has no packages for $TARGETS" >&2; exit 1; }
    else
        (cd "$VERO" && scripts/package.sh --app "$APP_TOML" --version "$1" ${2:+--build "$2"} --targets "$TARGETS" --out "$PACKAGES")
    fi
    "$REPO" build --app "$APP_TOML" --packages "$PACKAGES" --key "$KEY" --url "$URL" --out "$SITE" ${3:+--notes "$3"}
}

# overlay FROM TO: a VM's disks in TO, each a qcow2 overlay on the one in
# FROM, which is not changed, and a copy of its firmware's variables.
overlay() {
    mkdir -p "$2"
    for f in disk.qcow2 local.qcow2; do
        [ -f "$1/$f" ] || continue
        qemu-img create -q -f qcow2 -b "$1/$f" -F qcow2 "$2/$f" || return 1
    done
    if [ -f "$1/vars.fd" ]; then cp -c "$1/vars.fd" "$2/" 2>/dev/null || cp "$1/vars.fd" "$2/"; fi
}
# vm_stop [clean]: the VM stopped. A run's overlay is thrown away, so qemu
# is simply stopped; a snapshot being made is shut down cleanly first, so
# that its filesystems are whole.
vm_stop() {
    [ -f "$VMDIR/qemu.pid" ] || return 0
    pid=$(cat "$VMDIR/qemu.pid")
    if [ "$1" = clean ]; then
        $SSH 'PATH=/sbin:/usr/sbin:$PATH; poweroff 2>/dev/null || shutdown -p now' >/dev/null 2>&1 || true
        n=0
        while kill -0 "$pid" 2>/dev/null && [ $n -lt 120 ]; do sleep 1; n=$((n + 1)); done
    fi
    kill "$pid" 2>/dev/null || true
    n=0
    while kill -0 "$pid" 2>/dev/null && [ $n -lt 20 ]; do sleep 0.5; n=$((n + 1)); done
    rm -f "$VMDIR/qemu.pid"
}
# windows_quit: qemu told to stop at once, for a run on a throwaway overlay.
windows_quit() {
    [ -S /tmp/vero-qmp.sock ] || return 0
    python3 - /tmp/vero-qmp.sock <<'PY' || true
import json, socket, sys
s = socket.socket(socket.AF_UNIX); s.connect(sys.argv[1]); f = s.makefile("rw"); f.readline()
for c in ("qmp_capabilities", "quit"):
    f.write(json.dumps({"execute": c}) + "\n"); f.flush(); f.readline()
PY
    i=0
    while windows_running && [ $i -lt 30 ]; do sleep 1; i=$((i + 1)); done
}

# A VM is made the first time, by its run script, and shut down cleanly,
# so that its snapshots start from a disk at rest.
if [ -n "$VMSYS" ] && [ "$VMSYS" != windows ] && [ ! -f "$VMDIR/disk.qcow2" ]; then
    [ -n "$KEYONLY" ] && { echo new; exit 0; }
    echo "== making the $VMSYS VM, the first time it is needed"
    "$VERO/scripts/run-$VMSYS.sh" --shell --no-open >/dev/null
    SSH="ssh -i $VMDIR/key -p $SSH_PORT -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o IdentitiesOnly=yes root@127.0.0.1"
    vm_stop clean
fi

# The ready-made system: its key, made from the system it starts from,
# what is installed in it and what the app needs, and the period after
# which it is made again (a week for a container, a month for a VM, whose
# own packages move more slowly). The app's name is left out, so that two
# apps that need the same share one.
: "${EACH:=true}" "${DEPS_IN:=true}"
period() { if [ -n "$VMSYS" ]; then date +%Y-%m; else date +%G-%V; fi; }
if [ -n "$MAC" ]; then
    BASE="$(sw_vers -productVersion) $(xcodebuild -version 2>/dev/null | head -1)"
elif [ -n "$VMSYS" ]; then
    BASE=$(vero_vm_disks "${VMDIR:-$HOME/vm/vero-$VMSYS}")
else
    # Fetched once, as docker run would, so that the key is the image's.
    # shellcheck disable=SC2046
    docker image inspect "$IMAGE" >/dev/null 2>&1 ||
        docker pull -q $(echo "$RUN" | grep -o -e '--platform [^ ]*') "$IMAGE" >/dev/null 2>&1 || true
    BASE=$(docker image inspect --format '{{.Id}}' "$IMAGE" 2>/dev/null || echo "$IMAGE")
fi
RKEY=$(printf '%s\n' "ready 1" "$LABEL" "$BASE" "$SETUP" "$XSETUP" "$DEPS_IN" "$REFRESH" "$APP_DEPS" "$(period)" |
    sed "s/$APP_NAME/APP/g" | shasum | cut -c1-12)
if [ -n "$KEYONLY" ]; then
    echo "$RKEY"
    exit 0
fi
echo "system key: $RKEY"
mkdir -p "$VERO_READY_USED"
READY=vero-ready-$LABEL:$RKEY
[ -n "$VMSYS" ] && READY="$VMDIR/ready-$RKEY"
if [ -n "$FRESH" ] || [ -n "$MAC" ] || [ "$VMSYS" = windows ]; then
    HAVE_READY=yes
elif [ -n "$VMSYS" ]; then
    [ -f "$READY/ok" ] && HAVE_READY=yes
else
    docker image inspect "$READY" >/dev/null 2>&1 && HAVE_READY=yes
fi
# The system's name, as test-all.sh and the space table in
# scripts/lib/space.sh have it.
case "$MAC:$VMSYS:$FLATPAK:$IMAGE" in
    yes:*) SYSNAME=mac ;;
    :[!:]*) SYSNAME=$VMSYS ;;
    ::yes:*) SYSNAME=flatpak ;;
    *:debian:bookworm*) SYSNAME=debian12 ;;
    *:debian*) SYSNAME=debian ;;
    *:ubuntu*) SYSNAME=ubuntu ;;
    *:fedora*) SYSNAME=fedora ;;
    *:opensuse*) SYSNAME=opensuse ;;
    *:archlinux*) SYSNAME=arch ;;
    *:alpine*) SYSNAME=alpine ;;
    *chimera*) SYSNAME=chimera ;;
    *void*) SYSNAME=void ;;
    *) SYSNAME=$LABEL ;;
esac
vero_space_check "$SYSNAME" "${HAVE_READY:-no}" || exit 1

if [ -n "$MAC" ] || [ "$VMSYS" = windows ]; then
    in_container() { sh -c "$1"; }
elif [ -n "$VMSYS" ]; then
    SSH="ssh -i $VMDIR/key -p $SSH_PORT -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o IdentitiesOnly=yes root@127.0.0.1"
    # Through sh, on its standard input, whatever root's shell is:
    # DragonFly's is csh, which reads sh's syntax differently.
    in_container() { printf '%s\n' "$1" | $SSH sh; }
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
        [ -n "$QEMU" ] && windows_quit
        [ -n "$RUNDIR" ] && { echo "the run's overlay took $(du -sh "$RUNDIR" | cut -f1)"; rm -rf "$RUNDIR"; }
    elif [ -n "$VMSYS" ]; then
        vm_stop
        [ -n "$RUNDIR" ] && { echo "the run's overlay took $(du -sh "$RUNDIR" | cut -f1)"; rm -rf "$RUNDIR"; }
        rm -rf "$READY.tmp"
    else
        docker rm -f "$NAME" >/dev/null 2>&1 || true
    fi
    [ -n "$SERVER" ] && kill "$SERVER" 2>/dev/null || true
    [ -n "$PASSED" ] && rm -rf "$SITE" "$PACKAGES" "$CACHE/windows-test-$PORT" "$CACHE/windows-test-$PORT-disc"
    return 0
}
trap cleanup EXIT

# make_ready: the ready-made system made, from the system itself, with
# the app's dependencies installed from its 1.0.0 packages and then the app
# removed, and the tools the test uses. A container is saved as an image;
# a VM's snapshot is an overlay on its own disk, shut down cleanly. The
# site is being served, which a VM installs from.
make_ready() {
    STAGES=$((STAGES + 1))
    if [ -n "$VMSYS" ]; then
        stage "making the VM's snapshot, which this key needs once ($READY)"
        rm -rf "$READY.tmp"
        overlay "$VMDIR" "$READY.tmp" || { echo "FAIL: could not make the snapshot's overlay" >&2; exit 1; }
        VERO_DISK_DIR="$READY.tmp" "$VERO/scripts/run-$VMSYS.sh" --shell --no-open >/dev/null
        # What earlier runs left goes first, since an update fails on a
        # repository that is no longer there.
        in_container "$SETUP" || { echo "FAIL: could not set up the VM's snapshot" >&2; exit 1; }
        if [ -n "$REFRESH" ]; then
            echo "bringing the system up to date"
            in_container "$REFRESH" || { echo "FAIL: the system did not update" >&2; exit 1; }
            if in_container "[ -f /var/tmp/vero-reboot ]"; then
                echo "restarting into the updated system"
                in_container "rm -f /var/tmp/vero-reboot; (sleep 2; reboot) > /dev/null 2>&1 &" || true
                n=0
                while $SSH true 2>/dev/null && [ $n -lt 60 ]; do sleep 5; n=$((n + 1)); done
                n=0
                until $SSH true 2>/dev/null; do
                    n=$((n + 1))
                    [ $n -gt 360 ] && { echo "FAIL: the VM did not come back after restarting" >&2; exit 1; }
                    sleep 5
                done
            fi
        fi
        # The app's own packages bring its dependencies, as a person's
        # install would; then the app and its repository go again.
        in_container "$INSTALL" && in_container "${XSETUP:-true}" && in_container "$FORGET" ||
            { echo "FAIL: could not make the VM's snapshot" >&2; exit 1; }
        vm_stop clean
        echo "the snapshot takes $(du -sh "$READY.tmp" | cut -f1)"
        vero_vm_disks "$VMDIR" >"$READY.tmp/disks"
        echo "$APP_DEPS" >"$READY.tmp/deps"
        rm -rf "$READY"
        mv "$READY.tmp" "$READY"
        touch "$READY/ok"
        # A snapshot that this one replaces, made for the same needs, goes.
        for r in "$VMDIR"/ready-*; do
            [ "$r" != "$READY" ] && [ "$(cat "$r/deps" 2>/dev/null)" = "$APP_DEPS" ] &&
                rm -rf "$r" && echo "removed the snapshot this one replaces, $r"
        done
        return
    fi
    stage "making the ready-made system, which this key needs once ($READY)"
    docker rm -f "$NAME" >/dev/null 2>&1 || true
    # shellcheck disable=SC2086
    docker run -d --name "$NAME" $RUN --add-host=host.docker.internal:host-gateway "$IMAGE" sleep "${SLEEP:-infinity}" >/dev/null
    in_container "$SETUP" && in_container "${XSETUP:-true}" || { echo "FAIL: could not set up the ready-made system" >&2; exit 1; }
    if [ "$DEPS_IN" != true ]; then
        put_dir "$PACKAGES" /tmp/vero-ready &&
            in_container "$DEPS_IN; s=\$?; rm -rf /tmp/vero-ready; exit \$s" ||
            { echo "FAIL: the app's dependencies did not install in the ready-made system" >&2; exit 1; }
    fi
    docker commit -c "LABEL dev.vero.ready=1" -c "LABEL dev.vero.system=$LABEL" -c "LABEL dev.vero.key=$RKEY" \
        -c "LABEL dev.vero.period=$(period)" -c "LABEL dev.vero.deps=$APP_DEPS" "$NAME" "$READY" >/dev/null
    docker rm -f "$NAME" >/dev/null 2>&1 || true
    # An image that this one replaces, made for the same needs, goes.
    docker image ls --filter "label=dev.vero.system=$LABEL" --filter "label=dev.vero.deps=$APP_DEPS" \
        --format '{{.Repository}}:{{.Tag}}' | while read -r old; do
        [ "$old" = "$READY" ] && continue
        docker image rm "$old" >/dev/null 2>&1 && echo "removed the ready-made system this one replaces, $old"
    done
}

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
    if [ -n "$UPDATES" ]; then
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
    else
        echo "(no update test: test-all.sh --update or a full run has one)"
    fi
    # An app with Sparkle.framework inside, signed as vero signs one:
    # nested code first, and Gatekeeper's strict check passes.
    SPARKLE_VERSION=2.6.4
    SPARKLE="$CACHE/sparkle-$SPARKLE_VERSION"
    if [ ! -d "$SPARKLE/Sparkle.framework" ]; then
        mkdir -p "$SPARKLE"
        curl -fsSL "https://github.com/sparkle-project/Sparkle/releases/download/$SPARKLE_VERSION/Sparkle-$SPARKLE_VERSION.tar.xz" | tar -xJf - -C "$SPARKLE"
    fi
    # The check is one of vero-repo's Go tests, so it needs vero's checkout.
    [ -d "$VERO/cmd/vero-repo" ] || { PASSED=yes; echo "PASS (the Sparkle signing check needs vero's checkout)"; return 0; }
    (cd "$VERO/cmd/vero-repo" && VERO_SPARKLE_FRAMEWORK="$SPARKLE/Sparkle.framework" go test -count=1 -run TestSignWithSparkle . >/dev/null) \
        || { echo "FAIL: an app with Sparkle.framework doesn't sign" >&2; exit 1; }
    echo "ok: an app with Sparkle.framework inside signs and verifies"
    PASSED=yes
echo "PASS"
}

# windows_test: the installers in vero's Windows VM, over SSH: the NSIS
# installer, installed, started on the desktop, updated to 1.0.1 and
# uninstalled; then the MSIX's files, registered, started and removed.
# windows_boot: vero's Windows VM started on a throwaway overlay on its
# own disk, which is left as it was. It starts before the packages are
# ready, since nothing reaches it on its disc, and windows_test waits for
# it.
windows_boot() {
    . "$VERO/scripts/lib/windows-disc.sh"
    VMDIR="$HOME/vm/vero-windows"
    # Not 2222, which is FreeBSD VM's, so that both can be tested at once.
    SSHPORT=2227
    [ -f "$VMDIR/disk.qcow2" ] || { echo "no Windows VM in $VMDIR: scripts/run-windows.sh --install makes one" >&2; exit 1; }
    [ -f "$WINDOWS_KEY" ] || { echo "no $WINDOWS_KEY: scripts/setup-windows-ssh.sh sets up SSH into the VM" >&2; exit 1; }
    VM=$VMDIR windows_running && { echo "the Windows VM is running already: stop it first" >&2; exit 1; }
    pgrep -qf "$VMDIR/run-" && { echo "another test is using the Windows VM: wait for it to finish" >&2; exit 1; }
    PAY="$CACHE/windows-test-$PORT"
    rm -rf "$PAY" "$PAY-disc" && mkdir -p "$PAY" "$PAY-disc"
    # The installers go in over SSH, so the VM's disc holds only a note.
    echo "vero's test copies its files in over SSH." >"$PAY-disc/README.txt"
    RUNDIR="$VMDIR/run-$PORT"
    rm -rf "$RUNDIR"
    overlay "$VMDIR" "$RUNDIR" || { echo "FAIL: could not make the run's overlay" >&2; exit 1; }
    VM=$RUNDIR
    echo "== booting the Windows VM, while the packages are made"
    "$VERO/scripts/run-windows.sh" --vm "$RUNDIR" --headless --payload "$PAY-disc" --ssh "$SSHPORT" >"$CACHE/windows-vm-$PORT.log" 2>&1 &
    QEMU=$!
}

windows_test() {
    cp "$PACKAGES"/*-setup.exe "$VERO/scripts/lib/windows-test.ps1" "$VERO/scripts/lib/launch-windows.ps1" "$PAY/"
    [ -n "$APP_MSIX" ] && cp "$PACKAGES"/*.msix "$PAY/"
    STAGES=$((3 + ${LAUNCH:+1} + ${UPDATES:+1} + 0))
    stage "waiting for the Windows VM"
    windows_wait "$SSHPORT" 300 || { echo "FAIL: the VM never answered over SSH: scripts/setup-windows-ssh.sh sets it up" >&2; exit 1; }
    windows_signed_in "$SSHPORT" || { echo "FAIL: the VM did not sign vero in to its desktop" >&2; exit 1; }
    windows_ssh "$SSHPORT" "New-Item -ItemType Directory -Force C:/vero-test | Out-Null; Remove-Item -Recurse -Force C:/vero-test/*"
    windows_scp "$SSHPORT" "$PAY"/* vero@127.0.0.1:C:/vero-test/
    # phase NAME: one of windows-test.ps1's phases, its lines printed.
    phase() {
        windows_ssh "$SSHPORT" "powershell -NoProfile -ExecutionPolicy Bypass -File C:/vero-test/windows-test.ps1 -Phase $1 -Name $APP_NAME -Exe $APP_EXE -Worker $APP_WORKER.exe ${APP_MSIX:+-Identity $APP_MSIX} -Limit $LIMIT" | tr -d '\r' | tee -a "$PAY/results.txt"
    }
    # expect LINE: windows-test.ps1 printed that line.
    expect() { grep -qx "$1" "$PAY/results.txt" || { echo "FAIL: no \"$1\" from the VM" >&2; exit 1; }; }
    stage "the NSIS installer"
    phase nsis
    for line in "nsis-worker: 1.0.0" "nsis-app: True" "nsis-startmenu: True" "nsis-uninstaller: True" "nsis-runs: True"; do expect "$line"; done
    python3 "$VERO/scripts/lib/qmp.py" /tmp/vero-qmp.sock shot "$PAY/nsis.ppm" && sips -s format png "$PAY/nsis.ppm" --out "$PAY/nsis.png" >/dev/null
    echo "ok: 1.0.0 installed and running (the desktop: $PAY/nsis.png)"
    if [ -n "$LAUNCH" ]; then
        stage "starting the app and playing its steps${RECORD:+, recorded}"
        lb=$(date +%s)
        L="$PAY/launch" out="$RESULTS/windows"
        fresh_results "$out"
        rm -rf "$L" && mkdir -p "$L"
        "$REPO" steps --app "$APP_TOML" --out "$L" >/dev/null
        [ -n "$RECORD" ] || touch "$L/no-frames"
        sed -n "s/^export \([^=]*\)='\(.*\)'\$/\1=\2/p" "$L/env.sh" >"$L/env.txt"
        windows_scp "$SSHPORT" -r "$L" vero@127.0.0.1:C:/vero-test/
        phase launch
        windows_scp "$SSHPORT" -r vero@127.0.0.1:C:/vero-test/launch "$out.tmp" && cp -R "$out.tmp/." "$out/" && rm -rf "$out.tmp"
        rm -rf "$out/files" "$out/steps.json" "$out/env.txt" "$out/no-frames"
        finish_launch "$out" "$lb"
    fi
    if [ -n "$UPDATES" ]; then
        stage "releasing 1.0.1, and installing it over 1.0.0"
        release 1.0.1
        cp "$PACKAGES"/*-1.0.1-*-setup.exe "$PAY/"
        windows_scp "$SSHPORT" "$PAY"/*-1.0.1-*-setup.exe vero@127.0.0.1:C:/vero-test/
        phase nsis-update
        expect "nsis-updated: 1.0.1"
    else
        echo "(no update test: test-all.sh --update or a full run has one)"
    fi
    phase nsis-done
    expect "nsis-uninstalled: True"; expect "nsis-uninstall-entry-gone: True"
    if [ -n "$UPDATES" ]; then echo "ok: 1.0.1 installed over it, and uninstalled cleanly"; else echo "ok: it uninstalled cleanly"; fi
    if [ -z "$APP_MSIX" ]; then
        PASSED=yes
        echo "PASS (no MSIX: the app's vero-app.toml has no [wpf.msix])"
        return
    fi
    stage "the MSIX"
    phase msix
    for line in "msix-installed: True" "msix-worker: 1.0.0" "msix-runs: True"; do expect "$line"; done
    phase msix-done
    expect "msix-removed: True"
    echo "ok: the MSIX's app ran, and was removed"
    PASSED=yes
echo "PASS"
}

# launch_test: the installed app started on a virtual display, the steps
# played in it, and the window recorded, as a GIF in the results folder.
RESULTS="$CACHE/test-results/$APP_NAME"
launch_test() {
    stage "starting the app and playing its steps${RECORD:+, recorded}"
    lb=$(date +%s)
    out="$RESULTS/$LABEL" L="$CACHE/launch-$LABEL-$PORT"
    fresh_results "$out"
    rm -rf "$L" && mkdir -p "$L"
    "$REPO" steps --app "$APP_TOML" --out "$L" >/dev/null
    cp "$VERO/scripts/lib/launch-test.sh" "$L/"
    # Without a recording, the steps are played with no frames taken.
    [ -n "$RECORD" ] || touch "$L/no-frames"
    # A ready-made system has its virtual display already.
    if [ -z "$USED_READY" ]; then
        in_container "${XSETUP:-true}" || { echo "FAIL: couldn't add a virtual display" >&2; exit 1; }
    fi
    put_dir "$L" /var/tmp/vero-launch
    in_container "sh /var/tmp/vero-launch/launch-test.sh $LIMIT $APPCMD" || true
    get_dir /var/tmp/vero-launch "$out"
    rm -f "$out/launch-test.sh" "$out/steps.json" "$out/no-frames"
    rm -rf "$out/files" "$L"
    finish_launch "$out" "$lb"
}

# fresh_results DIR: DIR emptied for this run's results. A run that does
# not record keeps the last recording, so that the results page still
# shows the app on each system.
fresh_results() {
    keep=""
    if [ -z "$RECORD" ] && [ -f "$1/app.gif" ]; then
        keep=$(mktemp -d)
        mv "$1/app.gif" "$keep/"
        [ -f "$1/app.png" ] && mv "$1/app.png" "$keep/"
    fi
    rm -rf "$1" && mkdir -p "$1"
    [ -n "$keep" ] && { mv "$keep"/* "$1/"; rmdir "$keep"; }
    return 0
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
    if [ -n "$RECORD" ] && [ -f "$out/app.gif" ]; then
        echo "ok: the app started and its steps passed; recorded in $out/app.gif"
    else
        echo "ok: the app started and its steps passed (not recorded: test-all.sh --record records it)"
    fi
}

# make_gif DIR: DIR/frames made into DIR/app.gif and DIR/app.png.
make_gif() { sh "$VERO/scripts/lib/make-gif.sh" "$1"; }

# summary: index.html in the results folder, every app and system tested
# so far.
summary() { sh "$VERO/scripts/lib/results-page.sh" "$CACHE/test-results"; }

STAGES=2
[ -n "$LAUNCH" ] && STAGES=$((STAGES + 1))
[ -n "$UPDATES" ] && STAGES=$((STAGES + 2))
rm -rf "$SITE"
[ "$VMSYS" = windows ] && windows_boot
# vm_boot: a BSD's or illumos's VM started on this run's own overlay, on
# the snapshot, or with --fresh on the VM's own disk; either is left as it
# was. With a snapshot made already, it starts in the background while
# the packages are made, and vm_wait waits for it.
vm_boot() {
    # An emulated VM's snapshot keeps its running state, saved the first
    # time a run needs it, and each run carries on from that.
    VERO_RESUME=""
    if [ -n "$RESUME" ] && [ -z "$FRESH" ]; then
        [ -f "$READY/state.gz" ] || save_state "$READY" || echo "(the VM's state was not saved, so it boots instead)"
        [ -f "$READY/state.gz" ] && VERO_RESUME="$READY/state.gz"
    fi
    export VERO_RESUME
    RUNDIR="$VMDIR/run-$PORT"
    rm -rf "$RUNDIR"
    if [ -n "$FRESH" ]; then overlay "$VMDIR" "$RUNDIR"; else USED_READY=yes; touch "$READY/ok"; overlay "$READY" "$RUNDIR"; fi ||
        { echo "FAIL: could not make the run's overlay" >&2; exit 1; }
    VERO_DISK_DIR="$RUNDIR" "$VERO/scripts/run-$VMSYS.sh" --shell --no-open >"$CACHE/vm-boot-$PORT.log" 2>&1 &
    BOOTING=$!
}
vm_wait() {
    wait "$BOOTING" || { echo "FAIL: the $VMSYS VM did not start:" >&2; tail -5 "$CACHE/vm-boot-$PORT.log" >&2; exit 1; }
    rm -f "$CACHE/vm-boot-$PORT.log"
    if [ -n "$VERO_RESUME" ]; then
        echo "carried on from the snapshot's saved state"
        in_container "date -u $(date -u "$RESUME") > /dev/null" || echo "(the VM's clock could not be set)"
    fi
}
# save_state DIR: the VM booted on the snapshot in DIR, and its running
# state saved there, compressed, as state.gz; qemu then quits, so that
# the snapshot's disks stay as they were when the state was saved.
save_state() {
    echo "saving the VM's running state, once, so that runs carry on from it"
    VERO_RESUME="" VERO_DISK_DIR="$1" "$VERO/scripts/run-$VMSYS.sh" --shell --no-open >/dev/null 2>&1 || { vm_stop; return 1; }
    python3 - "$1/qmp.sock" "$1/state.gz.tmp" <<'PY' || { vm_stop; rm -f "$1/state.gz.tmp"; return 1; }
import json, socket, sys, time
s = socket.socket(socket.AF_UNIX); s.connect(sys.argv[1]); f = s.makefile("rw"); f.readline()
def ask(c, **a):
    f.write(json.dumps({"execute": c, "arguments": a} if a else {"execute": c}) + "\n"); f.flush()
    while True:
        r = json.loads(f.readline())
        if "event" not in r:
            return r
ask("qmp_capabilities")
# Saved while it runs, not paused: a state saved paused would come back
# paused.
ask("migrate", uri="exec:gzip -1 > " + sys.argv[2])
for _ in range(1800):
    st = ask("query-migrate").get("return", {}).get("status")
    if st == "completed":
        break
    if st in ("failed", "cancelled"):
        sys.exit(1)
    time.sleep(1)
else:
    sys.exit(1)
ask("quit")
PY
    n=0
    while [ -f "$VMDIR/qemu.pid" ] && kill -0 "$(cat "$VMDIR/qemu.pid")" 2>/dev/null && [ $n -lt 60 ]; do sleep 1; n=$((n + 1)); done
    rm -f "$VMDIR/qemu.pid"
    mv "$1/state.gz.tmp" "$1/state.gz"
    echo "the saved state takes $(du -sh "$1/state.gz" | cut -f1)"
}
if [ -n "$VMSYS" ] && [ "$VMSYS" != windows ] && [ -n "$HAVE_READY" ]; then
    echo "== booting the $VMSYS VM, while the packages are made"
    vm_boot
fi
stage "releasing 1.0.0"
if [ -n "$MAC" ]; then release 1.0.0 100; else release 1.0.0; fi
# Built aside and moved into place, since other runs may be starting it.
go build -o "$CACHE/bin/serve.$$" "$VERO/scripts/lib/serve.go" && mv -f "$CACHE/bin/serve.$$" "$CACHE/bin/serve"
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
    if [ -z "$HAVE_READY" ]; then
        make_ready
        vm_boot
    fi
    vm_wait
    IMAGE="the $VMSYS VM"
else
    [ -n "$HAVE_READY" ] || make_ready
    FROM=$READY
    if [ -n "$FRESH" ]; then FROM=$IMAGE; else USED_READY=yes; touch "$VERO_READY_USED/vero-ready-$LABEL-$RKEY"; fi
    docker rm -f "$NAME" >/dev/null 2>&1 || true
    # shellcheck disable=SC2086
    docker run -d --name "$NAME" $RUN --add-host=host.docker.internal:host-gateway "$FROM" sleep "${SLEEP:-infinity}" >/dev/null
fi

from=" (ready-made)"
[ -n "$FRESH" ] && from=""
stage "installing it as the site says, in $IMAGE${FLATPAK:+, with Flatpak}$from"
# A ready-made system has had its setting up done; one from the system
# itself has it now.
if [ -n "$FRESH" ]; then in_container "$SETUP"; fi
in_container "$EACH"
in_container "$INSTALL"
got=$(in_container "$WORKER")
[ "$got" = 1.0.0 ] || { echo "FAIL: the installed worker says $got, not 1.0.0" >&2; exit 1; }
in_container "$CHECK" >/dev/null ||
    { echo "FAIL: the package lacks its command, menu entry, metadata, binding, signature or GTK" >&2; exit 1; }
in_container "$ICON_CHECK" >/dev/null 2>&1 ||
    { echo "FAIL: GTK 4 crashed or failed reading the app's icon, which the app loads when it starts" >&2; exit 1; }
echo "ok: 1.0.0 installed from the repository, and its worker answers"
[ -n "$LAUNCH" ] && launch_test

if [ -n "$UPDATES" ]; then
    stage "releasing 1.0.1 into the same site"
    release 1.0.1
    stage "updating to 1.0.1 with the system's own updates"
    in_container "$UPDATE"
    got=$(in_container "$WORKER")
    [ "$got" = 1.0.1 ] || { echo "FAIL: after updating, the worker says $got, not 1.0.1" >&2; exit 1; }
    echo "ok: the system's updates brought 1.0.1"
else
    echo "(no update test: test-all.sh --update or a full run has one)"
fi
PASSED=yes
echo "PASS"
