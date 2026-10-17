#!/bin/sh
# Tests an app on the systems test-repo.sh knows, each on a port of its
# own, and then shows every result on one page:
# ~/.cache/vero/test-results/index.html.
#
#   scripts/test-all.sh --quick                          every day: Debian and Windows
#   scripts/test-all.sh                                  the full run, for a release
#   scripts/test-all.sh --app path/to/vero-app.toml      your app; vero's example without it
#       [--quick]             Debian and Windows only, without the update
#                             test and without recordings
#       [--only "debian fedora freebsd"]   just these, at either level
#       [--skip "illumos dragonfly"]       all but these
#       [--no-vms]            only the containers and the Flatpak
#       [--update]            the update test, from 1.0.0 to 1.0.1, in a
#                             quick run too; the full run always has it
#       [--record]            a recording of each system; the full run
#                             makes one when the app's [test] has record = true
#       [--again]             every system, even one that passed before
#                             with nothing changed
#       [--fresh]             each system from itself, not from its
#                             ready-made copy (test-repo.sh --fresh)
#       [--vms 2]             how many VMs at once (2)
#       [--containers N]      how many containers at once (all of them)
#       [--port 8700]         the first port to try for the sites
#       [--prune]             remove vero's stale test caches, and stop
#       [--status]            print the status of the run going now, and stop
#
# The systems: debian debian12 ubuntu fedora opensuse arch alpine chimera
# void flatpak freebsd netbsd openbsd dragonfly illumos windows, and mac
# for vero's example (an app of your own records the Mac with
# record-mac.sh). DragonFly and illumos are emulated, which is slow, so
# they run only in the full run or when --only names them.
#
# A system is skipped when it passed before with nothing changed: the
# same commit of the app and of vero, each with no changes beyond it, and
# the same ready-made system (test-repo.sh says what that is). It says so,
# and --again runs it anyway.
#
# The packages are built once for every system, a family at a time, while
# the systems start: the Linux packages together, the BSDs' together,
# Windows' and the Flatpak. Each system then builds its own small site,
# since each reaches this Mac by its own address.
#
# The containers all run at once, and at most two VMs beside them, the
# slowest first. Before each system, the free space on the disks that
# hold vero's caches, images and VMs is checked (scripts/lib/space.sh),
# and a system that would leave less than VERO_MIN_FREE_GB (10) is
# refused after vero's own stale copies are pruned.
#
# A VM is made the first time it is needed, which takes from minutes to
# more than an hour, and needs room: run scripts/doctor.sh to see what is
# missing. Each system's log is in ~/.cache/vero/test-runs/, and a line for
# each says PASS, FAIL or skipped as it finishes; in a terminal, a status
# of every system is drawn in place instead, with its stage, its times
# and an estimate of the time left. The status is also in
# ~/.cache/vero/test-runs/status.txt, rewritten as it changes, and on the
# results page, which refreshes itself while the run goes; --status prints
# it. It exits 1 when any system failed.
set -e
VERO=$(cd "$(dirname "$0")/.." && pwd)
APP="" QUICK="" ONLY="" SKIP="" NOVMS="" FIRST=8700 UPDATES="" RECORD="" AGAIN="" FRESH="" VMS=2 CONTAINERS=0 PRUNE="" STATUS=""
while [ $# -gt 0 ]; do
    case $1 in
        --app) APP=$2; shift ;;
        --quick) QUICK=yes ;;
        --full) QUICK="" ;;
        --only) ONLY=$2; shift ;;
        --skip) SKIP=$2; shift ;;
        --no-vms) NOVMS=yes ;;
        --update) UPDATES=yes ;;
        --record) RECORD=yes ;;
        --again) AGAIN=yes ;;
        --fresh) FRESH=yes ;;
        --vms | --parallel) VMS=$2; shift ;;
        --containers) CONTAINERS=$2; shift ;;
        --port) FIRST=$2; shift ;;
        --prune) PRUNE=yes ;;
        --status) STATUS=yes ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
    shift
done
CACHE="${VERO_CACHE:-$HOME/.cache/vero}"
VERO_CACHE=$CACHE
export VERO_CACHE
. "$VERO/scripts/lib/space.sh"
if [ -n "$STATUS" ]; then
    LOGS="$CACHE/test-runs"
    # Each run going now has its running.PID; status.txt shows them all.
    live=""
    for r in "$LOGS"/running.*; do [ -f "$r" ] && kill -0 "${r##*.}" 2>/dev/null && live=yes; done
    if [ -n "$live" ]; then
        cat "$LOGS/status.txt" 2>/dev/null || echo "The run has only just started."
    elif [ -f "$LOGS/status.txt" ]; then
        echo "No run is going now. The last run ended like this:"
        cat "$LOGS/status.txt"
    else
        echo "No run is going now, and none has left a status."
    fi
    exit 0
fi
if [ -n "$PRUNE" ]; then
    vero_prune
    echo "Pruned what vero no longer needs. scripts/doctor.sh shows what is left."
    exit 0
fi

# name, kind, and test-repo.sh's arguments for it. The kinds are a
# container, a VM that runs at the Mac's own speed, and an emulated one.
all="debian container --image debian:trixie
debian12 container --image debian:bookworm
ubuntu container --image ubuntu:24.04
fedora container --image fedora:latest
opensuse container --image opensuse/tumbleweed
arch container --image archlinux
alpine container --image alpine
chimera container --image chimeralinux/chimera
void container --image ghcr.io/void-linux/void-glibc
flatpak container --flatpak
freebsd vm --vm freebsd
netbsd vm --vm netbsd
openbsd vm --vm openbsd
dragonfly emulated --vm dragonfly
illumos emulated --vm illumos
windows vm --vm windows
mac mac --mac"

# How long each takes, in seconds, for running the slowest first: the
# last run's, or these, measured with an app with a web view and ffmpeg.
TIMES="$CACHE/test-times"
expected() {
    t=$(awk -v s="$1" '$1 == s { print $2 }' "$TIMES" 2>/dev/null | tail -1)
    [ -n "$t" ] && { echo "$t"; return; }
    case $1 in
        illumos) echo 3300 ;; dragonfly) echo 900 ;; windows) echo 600 ;; void) echo 700 ;;
        opensuse) echo 500 ;; openbsd) echo 400 ;; freebsd | netbsd | arch | chimera) echo 300 ;;
        *) echo 250 ;;
    esac
}

LOGS="$CACHE/test-runs"
mkdir -p "$LOGS" "$CACHE/bin"
# vero-repo once for every system, rather than one build each at once.
if [ -z "$VERO_REPO" ]; then
    go build -C "$VERO/cmd/vero-repo" -o "$CACHE/bin/vero-repo.$$" . && mv -f "$CACHE/bin/vero-repo.$$" "$CACHE/bin/vero-repo"
    VERO_REPO="$CACHE/bin/vero-repo"
fi
export VERO_REPO
TOML=${APP:-$VERO/example/vero-app.toml}
eval "$("$VERO_REPO" show --app "$TOML")" || exit 1

# The level: the full run has the update test, and a recording when the
# app asks for one; a quick run has neither unless asked.
if [ -z "$QUICK" ]; then
    UPDATES=yes
    [ "$APP_RECORD" = yes ] && RECORD=yes
fi

# state DIR: the commit of DIR's checkout, or "changed" when it has
# changes beyond it, which never match.
state() {
    c=$(git -C "$1" rev-parse --short HEAD 2>/dev/null) || { echo changed; return; }
    [ -n "$(git -C "$1" status --porcelain 2>/dev/null | head -1)" ] && { echo changed; return; }
    echo "$c"
}
APPSTATE=$(state "$APP_DIR") VEROSTATE=$(state "$VERO")
PASSES="$CACHE/test-passes/$APP_NAME"
mkdir -p "$PASSES"
WANT=$(mktemp) JOBS=$(mktemp) SAID=$(mktemp)
SHARED="$CACHE/packages-run.$$"
PORTS="$HOME/.cache/vero/ports"
mkdir -p "$PORTS"
release_ports() { for p in $(grep -lx "$$" "$PORTS"/*/pid 2>/dev/null); do rm -rf "$(dirname "$p")"; done; }
finish() {
    # Packaging still going in the background stops with the run.
    for p in $(jobs -p); do pkill -P "$p" 2>/dev/null || true; kill "$p" 2>/dev/null || true; done
    rm -f "$WANT" "$JOBS" "$SAID" "$LOGS/running.$$" "$LOGS/status.$$.txt"
    # A run that was killed leaves its running.PID, which goes now.
    for r in "$LOGS"/running.*; do [ -f "$r" ] && ! kill -0 "${r##*.}" 2>/dev/null && rm -f "$r"; done
    rm -rf "$SHARED"
    release_ports
}
trap finish EXIT
trap 'exit 130' INT TERM

# Which systems, and which are skipped because they passed before with
# nothing changed.
echo "$all" | while read -r name kind args; do
    case " $SKIP " in *" $name "*) continue ;; esac
    if [ -n "$ONLY" ]; then
        case " $ONLY " in *" $name "*) ;; *) continue ;; esac
    elif [ -n "$QUICK" ]; then
        case $name in debian | windows) ;; *) continue ;; esac
    fi
    case $kind in vm | emulated) [ -n "$NOVMS" ] && continue ;; esac
    if [ "$name" = mac ] && [ -n "$APP" ]; then
        echo "mac: an app of your own records the Mac with scripts/record-mac.sh" >&2
        continue
    fi
    if [ "$name" = windows ] && [ ! -f "$HOME/vm/vero-windows/disk.qcow2" ]; then
        echo "windows: there is no Windows VM, so it is left out. scripts/run-windows.sh --install makes one." >&2
        continue
    fi
    # shellcheck disable=SC2086
    key=$(sh "$VERO/scripts/test-repo.sh" --app "$TOML" $args --key 2>/dev/null | tail -1)
    if [ -z "$AGAIN" ] && [ -f "$PASSES/$name" ] && [ "$APPSTATE" != changed ] && [ "$VEROSTATE" != changed ] &&
        [ "$(sed -n 1p "$PASSES/$name")" = "$APPSTATE $VEROSTATE $key" ]; then
        had=" $(sed -n 2p "$PASSES/$name") "
        # A pass without the update test, or without a recording, does
        # not stand for a run that asks for one.
        ok=yes
        [ -n "$UPDATES" ] && case $had in *" update "*) ;; *) ok="" ;; esac
        [ -n "$RECORD" ] && case $had in *" record "*) ;; *) ok="" ;; esac
        if [ -n "$ok" ]; then
            echo "skipped $name: it passed on $(date -r "$PASSES/$name" '+%Y-%m-%d at %H:%M') and nothing has changed since (--again runs it)" | tee -a "$SAID" >&2
            continue
        fi
    fi
    echo "$(expected "$name") $name $kind $args"
done >"$WANT"
if [ ! -s "$WANT" ]; then
    grep -q '^skipped' "$SAID" && { echo "Nothing to test: every system passed before, and nothing has changed since."; exit 0; }
    echo "no systems to test" >&2
    exit 2
fi

# A port each, reserved as a folder holding this run's process id, so
# that two runs started together never take the same one: one that is
# only about to listen on it is as taken as one that listens. A
# reservation whose run has gone is taken over.
reserve() {
    if mkdir "$PORTS/$1" 2>/dev/null; then
        echo $$ >"$PORTS/$1/pid"
        return 0
    fi
    owner=$(cat "$PORTS/$1/pid" 2>/dev/null)
    [ -n "$owner" ] && kill -0 "$owner" 2>/dev/null && return 1
    echo $$ >"$PORTS/$1/pid"
}
port=$((FIRST - 1))
# The slowest first.
sort -rn "$WANT" | while read -r secs name kind args; do
    port=$((port + 1))
    while lsof -nP -iTCP:$port -sTCP:LISTEN >/dev/null 2>&1 || ! reserve $port; do port=$((port + 1)); done
    echo "$secs $name $kind $port $args"
done >"$JOBS"

# The disk, before anything starts: a run that would begin below the
# margin is refused at once. Each system checks again, for what it needs.
free_mac=$(vero_free_gb "$CACHE")
if [ "$free_mac" -lt "$VERO_MIN_FREE_GB" ]; then
    vero_prune
    free_mac=$(vero_free_gb "$CACHE")
fi
if [ "$free_mac" -lt "$VERO_MIN_FREE_GB" ]; then
    echo "Not enough disk space: this Mac's disk has $free_mac GB free, which is less than the $VERO_MIN_FREE_GB GB that VERO_MIN_FREE_GB keeps free. scripts/doctor.sh shows what vero keeps." >&2
    exit 1
fi

# family SYSTEM: the family whose packages SYSTEM installs, and its
# target for vero-repo package.
family() {
    case $1 in
        debian | debian12 | ubuntu) echo "linux deb" ;;
        fedora | opensuse) echo "linux rpm" ;;
        arch) echo "linux pacman" ;;
        alpine | chimera | void) echo "linux $1" ;;
        flatpak) echo "flatpak flatpak" ;;
        freebsd | netbsd | openbsd | dragonfly | illumos) echo "bsd $1" ;;
        windows) echo "windows windows${APP_MSIX:+,msix}" ;;
    esac
}
# Only for the processors of the systems tested: ARM64, as this Mac and its
# VMs are, and x86-64, which Arch, DragonFly and illumos are emulated as.
# vero release still makes every one the app asks for.
VERO_PACKAGE_GOARCHES=${VERO_PACKAGE_GOARCHES:-arm64,amd64}
export VERO_PACKAGE_GOARCHES
# The packages, once per family, in the background: each system waits for
# its own family's, in $SHARED/FAMILY/VERSION, which VERSION.done marks.
mkdir -p "$SHARED"
for f in linux bsd windows flatpak; do
    targets=$(cut -d' ' -f2 "$JOBS" | while read -r n; do family "$n"; done |
        awk -v f=$f '$1 == f { print $2 }' | sort -u | paste -sd, -)
    [ -n "$targets" ] || continue
    mkdir -p "$SHARED/$f"
    (
        for v in 1.0.0 ${UPDATES:+1.0.1}; do
            # Windows' VM is ARM64, and each processor costs a publish.
            [ $f = windows ] && VERO_PACKAGE_GOARCHES=arm64
            if sh "$VERO/scripts/package.sh" --app "$TOML" --version "$v" --targets "$targets" \
                --out "$SHARED/$f/$v" >"$SHARED/$f/$v.log" 2>&1; then
                touch "$SHARED/$f/$v.done"
            else
                touch "$SHARED/$f/$v.failed"
            fi
        done
    ) &
done

level="the full run"
[ -n "$QUICK" ] && level="a quick run"
# The status, kept up to date in the background until the run is over.
. "$VERO/scripts/lib/status.sh"
RUN_BEGAN=$(date +%s)
TERMINAL=""
[ -t 1 ] && TERMINAL=yes
while read -r secs name rest; do rm -f "$LOGS/$name.began"; done <"$JOBS"
echo $$ >"$LOGS/running.$$"
VERO_TEST_LOGS=$LOGS
export VERO_TEST_LOGS
status_loop &
STATUS_LOOP=$!
echo "testing $(cut -d' ' -f2 "$JOBS" | tr '\n' ' ')in $level${UPDATES:+, with the update test}${RECORD:+, recorded}; logs in $LOGS"

# one NAME KIND PORT ARGUMENT...: one system's test, a line saying how
# it went, and on a pass, the key that lets the next run skip it.
one() {
    name=$1 kind=$2 port=$3
    shift 3
    log="$LOGS/$name.log"
    began=$(date +%s)
    : >"$log"
    echo "$began" >"$LOGS/$name.began"
    fam=$(family "$name" | cut -d' ' -f1)
    upd=--no-update rec=--no-record
    [ -n "$UPDATES" ] && upd=--update
    [ -n "$RECORD" ] && rec=--record
    # shellcheck disable=SC2086
    if sh "$VERO/scripts/test-repo.sh" --app "$TOML" "$@" --port "$port" \
        $upd $rec ${FRESH:+--fresh} \
        ${fam:+--packages "$SHARED/$fam"} >"$log" 2>&1; then
        took=$(($(date +%s) - began))
        echo "PASS $name (${took}s)"
        key=$(sed -n 's/^system key: //p' "$log" | head -1)
        printf '%s\n%s\n' "$APPSTATE $VEROSTATE $key" "${UPDATES:+update} ${RECORD:+record}" >"$PASSES/$name"
        # Only a run that made nothing new says how long the system takes.
        if ! grep -q 'which this key needs once' "$log"; then
            echo "$name $took" >>"$TIMES"
            record_stages "$name" "$took"
        fi
    else
        echo "FAIL $name ($(($(date +%s) - began))s): $(grep "FAIL" "$log" | head -1 | cut -c1-160)"
    fi
}
# lanes N KIND...: the jobs of those kinds shared among N lanes, a line
# of jobs for each, the slowest first and each to the lane with the least
# to do so far. A job is its fields joined by "|".
lanes() {
    n=$1
    shift
    awk -v n="$n" -v kinds=" $* " '
        index(kinds, " " $3 " ") {
            best = 1
            for (i = 2; i <= n; i++) if (load[i] < load[best]) best = i
            load[best] += $1
            job = $2 "|" $3 "|" $4
            for (j = 5; j <= NF; j++) job = job "|" $j
            lane[best] = lane[best] " " job
        }
        END { for (i = 1; i <= n; i++) if (lane[i] != "") print lane[i] }' "$JOBS"
}
# run_lanes: each line of jobs on its standard input run in turn, every
# line at once, until all are done.
run_lanes() {
    while read -r jobs; do
        (
            for job in $jobs; do
                # shellcheck disable=SC2046
                one $(echo "$job" | tr '|' ' ')
            done
        ) &
    done
    wait
}
count=$(awk '$3 == "container" || $3 == "mac"' "$JOBS" | wc -l | tr -d ' ')
[ "$CONTAINERS" -gt 0 ] 2>/dev/null || CONTAINERS=$count
[ "$CONTAINERS" -gt 0 ] || CONTAINERS=1
# In a terminal the status shows each line; elsewhere each is printed.
run_all() {
    lanes "$VMS" vm emulated | run_lanes &
    lanes "$CONTAINERS" container mac | run_lanes &
    wait
}
if [ -n "$TERMINAL" ]; then (run_all) >>"$SAID"; else run_all | tee -a "$SAID"; fi
rm -f "$LOGS/running.$$"
wait "$STATUS_LOOP" 2>/dev/null || true
write_status
if [ -n "$TERMINAL" ]; then
    printf '\033[H\033[J'
    cat "$LOGS/status.txt"
fi
sh "$VERO/scripts/lib/results-page.sh" "$CACHE/test-results"
echo "every result: $CACHE/test-results/index.html"
# It fails when any system did.
! grep -q '^FAIL' "$SAID"
