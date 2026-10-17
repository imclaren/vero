# Sourced by test-all.sh, test-repo.sh and doctor.sh: the free space on
# the disks that hold vero's test caches, images and VMs, what each test
# system needs there, and the pruning of vero's own stale copies.
#
# There are two disks. One is this Mac's, which holds vero's cache
# (~/.cache/vero, or VERO_CACHE) and its VMs (~/vm/vero-*). The other is
# Docker's, inside colima's VM, which holds the containers and the
# ready-made systems.
#
# A test system is refused when it would leave less than VERO_MIN_FREE_GB
# (10) free on this Mac's disk, or less than VERO_MIN_DOCKER_FREE_GB (2) on
# Docker's. What a container needs also counts on this Mac's disk while
# colima's disk, a file on this Mac's, has still to grow to its full size. vero_prune removes only what vero made and can
# never use again:
#   - ready-made container systems (images named vero-ready-*, labelled
#     dev.vero.ready) from an earlier period, or unused for a week;
#   - stopped test containers (named vero-test-*);
#   - VM snapshots (ready-*) whose disk has changed since they were made,
#     or that have not been used for a month, and a run's overlay (run-*)
#     whose VM is no longer running;
#   - the packages of a test run that has finished, and test packages and
#     sites in vero's cache unused for a week.

: "${VERO_CACHE:=$HOME/.cache/vero}"
: "${VERO_MIN_FREE_GB:=10}" "${VERO_MIN_DOCKER_FREE_GB:=2}"
VERO_VMS="$HOME/vm"
# When each ready-made container system was last used, for every checkout
# of vero, since they all share Docker's images.
VERO_READY_USED="$HOME/.cache/vero/ready"

# What each system needs, in GB, measured with an app with a web view and
# ffmpeg, rounded up: a ready-made container image takes 1 to 2 GB of
# Docker's disk, a BSD snapshot 0.2 to 0.3 GB and each run's overlay as
# much again, and a Windows run's overlay about 1 GB. illumos's snapshot,
# which brings the system up to date, took 5 GB. The columns are:
# on Docker's disk, the ready-made system and the test container's own
# changes; on this Mac's, the packages, the site and, for a VM, its
# snapshot and the run's overlay. A ready-made system already made needs
# only the second part, the run's own.
#
#   system      docker  docker-run  mac  mac-run
vero_space_table="debian       2   1   0   1
debian12     2   1   0   1
ubuntu       2   1   0   1
fedora       2   1   0   1
opensuse     2   1   0   1
arch         3   1   0   1
alpine       1   1   0   1
chimera      2   1   0   1
void         2   1   0   1
flatpak      1   1   0   1
freebsd      0   0   1   1
netbsd       0   0   1   1
openbsd      0   0   1   1
dragonfly    0   0   1   1
illumos      0   0   6   2
windows      0   0   0   2
mac          0   0   1   1"

# vero_space_need SYSTEM COLUMN: the number in the table's COLUMN, 2 to 5,
# for SYSTEM.
vero_space_need() {
    echo "$vero_space_table" | awk -v s="$1" -v c="$2" '$1 == s { print $c; found = 1 } END { if (!found) print 1 }'
}

# vero_free_gb PATH: the free space, in whole GB, on the disk that holds
# PATH, or the nearest folder above it that exists.
vero_free_gb() {
    p=$1
    while [ ! -e "$p" ] && [ "$p" != / ]; do p=$(dirname "$p"); done
    df -k "$p" | awk 'NR == 2 { print int($4 / 1048576) }'
}

# vero_docker_free_gb: the free space, in whole GB, on Docker's disk, as a
# container sees it, or nothing when Docker is not answering.
vero_docker_free_gb() {
    img=$(docker image ls -q alpine 2>/dev/null | head -1)
    [ -n "$img" ] || img=$(docker image ls -q --filter label=dev.vero.ready 2>/dev/null | head -1)
    [ -n "$img" ] || { docker pull -q alpine >/dev/null 2>&1 && img=alpine; }
    [ -n "$img" ] || return 0
    docker run --rm --entrypoint df "$img" -k / 2>/dev/null | awk 'NR == 2 { print int($4 / 1048576) }'
}

# vero_docker_growth_gb: how far, in whole GB, colima's disk can still
# grow on this Mac's disk. Where there is no colima disk to look at, as
# with Docker Desktop, it is taken to be able to grow without limit.
vero_docker_growth_gb() {
    disk="$HOME/.colima/_lima/_disks/colima/datadisk"
    [ -f "$disk" ] || { echo 1000; return; }
    size=$(stat -f %z "$disk") used=$(du -k "$disk" | cut -f1)
    echo $(((size / 1024 - used) / 1048576))
}

# vero_space_check SYSTEM READY: says whether SYSTEM can start, and why
# not. READY is yes when its ready-made system has been made already. On
# too little space it prunes vero's stale copies first, and then refuses
# with how much is free and how much the system needs.
vero_space_check() {
    sys=$1 ready=$2
    d=$(vero_space_need "$sys" 2) dr=$(vero_space_need "$sys" 3)
    m=$(vero_space_need "$sys" 4) mr=$(vero_space_need "$sys" 5)
    [ "$ready" = yes ] && { d=0; m=0; }
    need_docker=$((d + dr)) need_mac=$((m + mr))
    # Docker's growth that would land on this Mac's disk: none once
    # colima's disk has its full size, as it keeps it.
    grow=$(vero_docker_growth_gb)
    [ "$grow" -gt "$need_docker" ] && grow=$need_docker
    need_mac=$((need_mac + grow))
    for attempt in 1 2; do
        short=""
        free_mac=$(vero_free_gb "$VERO_CACHE")
        [ $((free_mac - need_mac)) -lt "$VERO_MIN_FREE_GB" ] &&
            short="$sys needs about $need_mac GB on this Mac's disk, which has $free_mac GB free"
        if [ "$need_docker" -gt 0 ]; then
            free_docker=$(vero_docker_free_gb)
            [ -n "$free_docker" ] && [ $((free_docker - need_docker)) -lt "$VERO_MIN_DOCKER_FREE_GB" ] &&
                short="${short:+$short, and }$sys needs about $need_docker GB on Docker's disk, which has $free_docker GB free and should keep $VERO_MIN_DOCKER_FREE_GB GB (VERO_MIN_DOCKER_FREE_GB)"
        fi
        [ -z "$short" ] && return 0
        [ $attempt = 1 ] && vero_prune >&2
    done
    echo "FAIL: not enough disk space: $short. This Mac's disk keeps $VERO_MIN_FREE_GB GB free (VERO_MIN_FREE_GB). scripts/doctor.sh shows what vero keeps." >&2
    return 1
}

# vero_prune: removes vero's own stale copies, as the top of this file
# lists, and says what it removed.
vero_prune() {
    period=$(date +%G-%V)
    if docker info >/dev/null 2>&1; then
        docker ps -a --filter name=vero-test- --filter status=exited --filter status=created --format '{{.Names}}' |
            while read -r c; do
                case $c in vero-test-*) docker rm "$c" >/dev/null 2>&1 && echo "pruned the stopped container $c" ;; esac
            done
        docker image ls --filter label=dev.vero.ready --format '{{.Repository}}:{{.Tag}}' | while read -r img; do
            case $img in vero-ready-*) ;; *) continue ;; esac
            made=$(docker image inspect --format '{{index .Config.Labels "dev.vero.period"}}' "$img" 2>/dev/null)
            used="$VERO_READY_USED/$(echo "$img" | tr ':' '-')"
            if [ "$made" != "$period" ] || [ -z "$(find "$used" -mtime -7 2>/dev/null)" ]; then
                # One that a test container still runs is kept.
                [ -n "$(docker ps -q --filter ancestor="$img")" ] && continue
                docker image rm "$img" >/dev/null 2>&1 && { rm -f "$used"; echo "pruned the ready-made system $img"; }
            fi
        done
    fi
    for vm in "$VERO_VMS"/vero-*; do
        [ -d "$vm" ] || continue
        running=no
        [ -f "$vm/qemu.pid" ] && kill -0 "$(cat "$vm/qemu.pid")" 2>/dev/null && running=yes
        for r in "$vm"/run-*; do
            [ -d "$r" ] || continue
            # A run's overlay is in use while a qemu has it open.
            pgrep -qf "$r/" && continue
            rm -rf "$r" && echo "pruned the overlay $r"
        done
        [ $running = yes ] && continue
        for r in "$vm"/ready-*; do
            [ -d "$r" ] || continue
            if [ ! -f "$r/ok" ] || [ "$(cat "$r/disks" 2>/dev/null)" != "$(vero_vm_disks "$vm")" ] ||
                [ -z "$(find "$r/ok" -mtime -30 2>/dev/null)" ]; then
                rm -rf "$r" && echo "pruned the snapshot $r"
            fi
        done
    done
    # A run's packages, once the run that made them has gone.
    for d in "$VERO_CACHE"/packages-run.*; do
        [ -d "$d" ] || continue
        kill -0 "${d##*.}" 2>/dev/null && continue
        rm -rf "$d" && echo "pruned $d"
    done
    for d in "$VERO_CACHE"/*-packages-* "$VERO_CACHE"/*-site-* "$VERO_CACHE"/launch-* "$VERO_CACHE"/windows-test*; do
        [ -d "$d" ] || continue
        [ -n "$(find "$d" -maxdepth 0 -mtime -7)" ] && continue
        rm -rf "$d" && echo "pruned $d"
    done
    return 0
}

# vero_vm_disks DIR: the size and time of each of a VM's own disks, which
# change whenever the VM itself is started; a snapshot made on them is
# good only while these are the same.
vero_vm_disks() {
    for f in "$1"/disk.qcow2 "$1"/local.qcow2 "$1"/vars.fd; do
        [ -f "$f" ] && stat -f '%N %z %m' "$f"
    done | sed "s|$1/||"
}

# vero_space_report: vero's disk use by kind, for doctor.sh.
vero_space_report() {
    size() { du -sk "$@" 2>/dev/null | awk '{ s += $1 } END { printf "%.1f GB", s / 1048576 }'; }
    printf '  %-28s %s\n' "test packages and sites" "$(size "$VERO_CACHE"/*-packages* "$VERO_CACHE"/*-site* "$VERO_CACHE"/packages-run.*)"
    printf '  %-28s %s\n' "recordings and logs" "$(size "$VERO_CACHE"/test-results "$VERO_CACHE"/test-runs "$VERO_CACHE"/launch-*)"
    printf '  %-28s %s\n' "the rest of vero's cache" "$(du -sk "$VERO_CACHE" 2>/dev/null | awk '{ printf "%.1f GB in all", $1 / 1048576 }')"
    for vm in "$VERO_VMS"/vero-*; do
        [ -d "$vm" ] || continue
        n=$(basename "$vm")
        printf '  %-28s %s' "$n" "$(size "$vm")"
        snaps=$(ls -d "$vm"/ready-* 2>/dev/null | wc -l | tr -d ' ')
        runs=$(ls -d "$vm"/run-* 2>/dev/null | wc -l | tr -d ' ')
        [ "$snaps$runs" != 00 ] && printf ', of which %s in %s snapshots and %s in %s run overlays' \
            "$(size "$vm"/ready-*)" "$snaps" "$(size "$vm"/run-*)" "$runs"
        echo
    done
    if docker info >/dev/null 2>&1; then
        imgs=$(docker image ls --filter label=dev.vero.ready --format '{{.Size}}' | wc -l | tr -d ' ')
        total=$(docker image ls --filter label=dev.vero.ready -q | xargs docker image inspect --format '{{.Size}}' 2>/dev/null |
            awk '{ s += $1 } END { printf "%.1f GB", s / 1e9 }')
        printf '  %-28s %s in %s images (they share their base layers, so together they take less)\n' "ready-made containers" "$total" "$imgs"
        vol=$(docker system df -v --format '{{range .Volumes}}{{.Name}} {{.Size}}{{"\n"}}{{end}}' 2>/dev/null | awk '$1 == "vero-test-flatpak" { print $2 }')
        [ -n "$vol" ] && printf '  %-28s %s\n' "Flatpak's runtimes" "$vol"
        printf '  %-28s %s GB free\n' "Docker's disk" "$(vero_docker_free_gb)"
    fi
    printf '  %-28s %s GB free\n' "this Mac's disk" "$(vero_free_gb "$VERO_CACHE")"
}
