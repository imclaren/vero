#!/bin/sh
# results-page.sh DIR: writes DIR/index.html, every recording vero's tests
# have made, an app at a time: each system's GIF, whether it passed, how
# long it took and when, which of the app's commits it was, and its log.
# Systems are in the order the install page shows them. DIR is ~/.cache/vero/test-results,
# which holds APP/SYSTEM/ folders; test-repo.sh and record-mac.sh run this
# after each recording.
dir=${1:?the results folder}
{
    echo '<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>vero test results</title>'
    echo '<style>
:root { color-scheme: light dark; --ink: #1d1d1f; --soft: #6e6e73; --line: #d2d2d7; --bg: #fff; }
@media (prefers-color-scheme: dark) { :root { --ink: #f5f5f7; --soft: #a1a1a6; --line: #424245; --bg: #1d1d1f; } }
body { margin: 0; background: var(--bg); color: var(--ink); font: 15px/1.5 system-ui, -apple-system, sans-serif; }
main { max-width: 1100px; margin: 0 auto; padding: 24px 16px 48px; }
h2 { margin: 32px 0 4px; } p { margin: 4px 0; color: var(--soft); }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; margin-top: 12px; }
figure { margin: 0; border: 1px solid var(--line); border-radius: 8px; padding: 8px; }
img { display: block; width: 100%; height: auto; } a { color: inherit; }
.PASS { color: #1a7f37; } .FAIL { color: #cf222e; } figcaption { font-size: 13px; margin-top: 6px; }
</style><main><h1>vero test results</h1>'
    echo "<p>Updated $(date '+%Y-%m-%d %H:%M').</p>"
    for app in "$dir"/*/; do
        a=$(basename "$app")
        pass=0 fail=0
        for d in "$app"*/; do
            [ -f "$d/status" ] || continue
            case $(sed -n 1p "$d/status") in PASS) pass=$((pass + 1)) ;; *) fail=$((fail + 1)) ;; esac
        done
        [ $((pass + fail)) -gt 0 ] || continue
        counts="$pass passed"
        [ $fail -gt 0 ] && counts="$counts, $fail failed"
        echo "<h2>$a</h2><p>$counts</p><div class=grid>"
        for s in $(ls "$app" | awk '{
            n = split("macos windows debian ubuntu fedora opensuse archlinux alpine chimeralinux ghcr.io-void-linux flatpak vm-freebsd vm-dragonfly vm-netbsd vm-openbsd vm-illumos", order, " ")
            rank = n + 1
            for (i = 1; i <= n; i++) if (index($0, order[i]) == 1) { rank = i; break }
            printf "%02d %s\n", rank, $0 }' | sort | cut -d" " -f2); do
            d="$app$s"
            [ -f "$d/status" ] || continue
            st=$(sed -n 1p "$d/status") secs=$(sed -n 2p "$d/status") why=$(sed -n 3p "$d/status") commit=$(sed -n 4p "$d/status")
            when=$(date -r "$d/status" '+%Y-%m-%d %H:%M')
            echo "<figure>"
            [ -f "$d/app.gif" ] && echo "<a href=\"$a/$s/app.gif\"><img src=\"$a/$s/app.gif\" alt=\"$a on $s\" loading=lazy></a>"
            echo "<figcaption><b>$s</b> <span class=$st>$st</span> $why<br>${secs}s, $when${commit:+, commit $commit} · <a href=\"$a/$s/app.log\">log</a></figcaption></figure>"
        done
        echo "</div>"
    done
    echo "</main>"
} >"$dir/index.html"
