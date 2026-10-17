# Sourced by test-all.sh: the live status of a run, one line for each
# system, with its state, its stage, the time in that stage and in all,
# and an estimate of the time left. It is written to $LOGS/status.txt
# whenever it changes, drawn in place when test-all.sh runs in a
# terminal, and shown on the results page, which refreshes itself while
# a run is going. test-all.sh --status prints it.
#
# It reads what test-all.sh and test-repo.sh write already:
#   $JOBS, the systems of the run, a line each: seconds name kind port ...;
#   $LOGS/NAME.log, test-repo.sh's log, whose "== [n/m] STAGE (m:ss)" lines
#     say which stage it is in, and since when;
#   $LOGS/NAME.began, when it started, in seconds since 1970;
#   $SAID, the PASS, FAIL and skipped lines.
# The time each stage took on each system is kept in
# $CACHE/test-stage-times/APP/NAME, a line for each stage of the last run
# that passed: its seconds, then its title.

STAGE_TIMES="$CACHE/test-stage-times/$APP_NAME"

# stage_title LINE: a "== [n/m] ..." line without its numbers and times,
# and without what is in brackets, which differs from one run to the next.
stage_title() { echo "$1" | sed 's/^== \[[0-9]*\/[0-9?]*\] //; s/ ([^)]*)//g; s/^== //'; }

# stage_secs LINE: the time a "== [n/m] ... (m:ss)" line was written, in
# seconds from the start of its test.
stage_secs() { echo "$1" | sed -n 's/.*(\([0-9]*\):\([0-9][0-9]\))$/\1 \2/p' | awk '{ print $1 * 60 + $2 }'; }

# what TITLE: what a system is doing in a stage of that title.
what() {
    case $1 in
        "releasing 1.0.0"*) echo "building packages" ;;
        making*) echo "making its ready-made copy" ;;
        booting* | "waiting for the Windows VM"*) echo "starting the VM" ;;
        installing* | "the NSIS installer"* | "the MSIX"*) echo installing ;;
        "starting the app"*) echo "playing steps" ;;
        "releasing 1.0.1"* | updating*) echo updating ;;
        *) echo "$1" ;;
    esac
}

# mmss SECONDS: as m:ss.
mmss() { printf '%d:%02d' $(($1 / 60)) $(($1 % 60)); }

# record_stages NAME: the times of each stage in NAME's log, which passed,
# kept for the next run's estimates.
record_stages() {
    mkdir -p "$STAGE_TIMES"
    log="$LOGS/$1.log" total=$2
    grep '^== \[' "$log" | while read -r line; do
        echo "$(stage_secs "$line")|$(stage_title "$line")"
    done | awk -F'|' -v total="$total" '
        NR > 1 { print $1 - prev, title }
        { prev = $1; title = $2 }
        END { if (NR) print total - prev, title }' >"$STAGE_TIMES/$1"
}

# status_line NAME KIND: the line for one system.
status_line() {
    name=$1 now=$(date +%s)
    said=$(grep -E "^(PASS|FAIL|skipped) $name[ :]" "$SAID" 2>/dev/null | tail -1)
    case $said in
        PASS*) printf '%-10s PASS %s\n' "$name" "$(echo "$said" | sed 's/^PASS [^ ]* //')"; return ;;
        FAIL*) printf '%-10s FAIL %s\n' "$name" "$(echo "$said" | sed 's/^FAIL [^ ]* //' | cut -c1-110)"; return ;;
        skipped*) printf '%-10s skipped: %s\n' "$name" "$(echo "$said" | sed 's/^skipped [^:]*: //')"; return ;;
    esac
    if [ ! -f "$LOGS/$name.began" ]; then
        printf '%-10s waiting\n' "$name"
        return
    fi
    began=$(cat "$LOGS/$name.began")
    total=$((now - began))
    line=$(grep '^== ' "$LOGS/$name.log" 2>/dev/null | tail -1)
    title=$(stage_title "$line")
    num=$(echo "$line" | sed -n 's/^== \[\([0-9]*\/[0-9?]*\)\].*/\1/p')
    at=$(stage_secs "$line")
    [ -n "$at" ] || at=0
    in=$((total - at))
    [ $in -lt 0 ] && in=0
    # What is left: the rest of this stage, as long as it took last time,
    # and the stages after it; or the system's last time in all.
    left="?"
    if [ -f "$STAGE_TIMES/$name" ] && grep -q " $title\$" "$STAGE_TIMES/$name" 2>/dev/null; then
        left=$(awk -v t="$title" -v in_="$in" '
            found { rest += $1 }
            !found && substr($0, index($0, " ") + 1) == t { found = 1; this = $1 - in_; if (this < 0) this = 0 }
            END { print this + rest }' "$STAGE_TIMES/$name")
    else
        t=$(expected "$name")
        [ -n "$t" ] && [ "$t" -gt "$total" ] && left=$((t - total))
    fi
    [ "$left" != "?" ] && left="about $(mmss "$left") left"
    printf '%-10s %-27s %-6s %s for %s, %s in all, %s\n' "$name" "$(what "$title")" "${num:+$num}" \
        "$(echo "$title" | cut -c1-40)" "$(mmss $in)" "$(mmss $total)" "$left"
}

# write_status: $LOGS/status.txt written afresh, when it has changed.
write_status() {
    {
        so_far=$(mmss $(($(date +%s) - RUN_BEGAN)))
        if [ -f "$LOGS/running" ]; then
            echo "vero test run of $APP_NAME, $level, started at $(date -r "$RUN_BEGAN" '+%H:%M:%S'), $so_far so far"
        else
            echo "vero test run of $APP_NAME, $level, started at $(date -r "$RUN_BEGAN" '+%H:%M:%S'), finished after $so_far"
        fi
        grep '^skipped' "$SAID" 2>/dev/null | sed 's/^skipped \([^:]*\):.*/\1/' | while read -r n; do status_line "$n"; done
        while read -r secs name rest; do status_line "$name"; done <"$JOBS"
    } >"$LOGS/status.txt.new"
    mv -f "$LOGS/status.txt.new" "$LOGS/status.txt"
}

# status_loop: the status kept up to date while the run goes, drawn in
# place in a terminal, and the results page with it now and then.
status_loop() {
    i=0
    while [ -f "$LOGS/running" ]; do
        write_status
        if [ -n "$TERMINAL" ]; then
            printf '\033[H\033[J'
            cat "$LOGS/status.txt"
        fi
        i=$((i + 1))
        [ $((i % 3)) -eq 1 ] && sh "$VERO/scripts/lib/results-page.sh" "$CACHE/test-results" "$LOGS"
        sleep 2
    done
}
