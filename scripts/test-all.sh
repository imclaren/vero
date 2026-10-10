#!/bin/sh
# Tests an app on every system test-repo.sh knows, a few at a time, each
# on a port of its own, and then shows every result on one page:
# ~/.cache/vero/test-results/index.html.
#
#   scripts/test-all.sh                                  vero's example, everywhere
#   scripts/test-all.sh --app path/to/vero-app.toml      your app
#       [--parallel 3]        how many at once (3; VMs are slow and share the Mac)
#       [--only "debian fedora freebsd"]   just these
#       [--skip "illumos dragonfly"]       all but these
#       [--no-vms]            only the containers and the Flatpak
#
# The systems: debian ubuntu fedora opensuse arch alpine chimera void
# flatpak freebsd netbsd openbsd dragonfly illumos windows, and mac for
# vero's example (an app of your own records the Mac with record-mac.sh).
# A VM is made the first time it is needed, which takes from minutes to
# more than an hour (cmd/vero-repo/README.md says how long each takes),
# and needs room: run scripts/doctor.sh to see what is missing.
#
# Each system's log is in ~/.cache/vero/test-runs/, and a line for each
# says PASS or FAIL as it finishes. It exits 1 when any system failed.
set -e
VERO=$(cd "$(dirname "$0")/.." && pwd)
APP="" PARALLEL=3 ONLY="" SKIP="" NOVMS=""
while [ $# -gt 0 ]; do
    case $1 in
        --app) APP=$2; shift ;;
        --parallel) PARALLEL=$2; shift ;;
        --only) ONLY=$2; shift ;;
        --skip) SKIP=$2; shift ;;
        --no-vms) NOVMS=yes ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
    shift
done

# name, then test-repo.sh's arguments for it.
all="debian --image debian:bookworm
ubuntu --image ubuntu:24.04
fedora --image fedora:latest
opensuse --image opensuse/tumbleweed
arch --image archlinux
alpine --image alpine
chimera --image chimeralinux/chimera
void --image ghcr.io/void-linux/void-glibc
flatpak --flatpak
freebsd --vm freebsd
netbsd --vm netbsd
openbsd --vm openbsd
dragonfly --vm dragonfly
illumos --vm illumos
windows --vm windows
mac --mac"

LOGS="$HOME/.cache/vero/test-runs"
mkdir -p "$LOGS"
JOBS=$(mktemp)
trap 'rm -f "$JOBS"' EXIT
port=8700
echo "$all" | while read -r name args; do
    case " $SKIP " in *" $name "*) continue ;; esac
    [ -n "$ONLY" ] && case " $ONLY " in *" $name "*) ;; *) continue ;; esac
    case $args in --vm*) [ -n "$NOVMS" ] && continue ;; esac
    [ "$name" = mac ] && [ -n "$APP" ] && { echo "mac: an app of your own records the Mac with scripts/record-mac.sh" >&2; continue; }
    port=$((port + 1))
    echo "$name $port $args"
done >"$JOBS"
[ -s "$JOBS" ] || { echo "no systems to test" >&2; exit 2; }
echo "testing $(cut -d' ' -f1 "$JOBS" | tr '\n' ' ')$PARALLEL at a time; logs in $LOGS"

# One system: its test, and a line saying how it went.
export VERO APP LOGS
one='name=$1 port=$2; shift 2
log="$LOGS/$name.log"
began=$(date +%s)
if sh "$VERO/scripts/test-repo.sh" ${APP:+--app "$APP"} "$@" --port "$port" >"$log" 2>&1; then
    echo "PASS $name ($(($(date +%s) - began))s)"
else
    echo "FAIL $name ($(($(date +%s) - began))s): $(grep "FAIL" "$log" | head -1 | cut -c1-120)"
fi'
SAID=$(mktemp)
trap 'rm -f "$JOBS" "$SAID"' EXIT
xargs -P "$PARALLEL" -L 1 sh -c "$one" sh <"$JOBS" | tee "$SAID"
sh "$VERO/scripts/lib/results-page.sh" "$HOME/.cache/vero/test-results"
echo "every result: $HOME/.cache/vero/test-results/index.html"
# It fails when any system did.
! grep -q '^FAIL' "$SAID"
