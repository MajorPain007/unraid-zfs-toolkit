#!/usr/bin/env bash

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

PASS=0
FAIL=0
SKIP=0

ok()   { printf '  \033[32mPASS\033[0m %s\n' "$1"; PASS=$((PASS + 1)); }
bad()  { printf '  \033[31mFAIL\033[0m %s\n' "$1"; [ -n "${2:-}" ] && printf '       %s\n' "$2"; FAIL=$((FAIL + 1)); }
skip() { printf '  \033[33mSKIP\033[0m %s\n' "$1"; SKIP=$((SKIP + 1)); }
group() { printf '\n\033[1m%s\033[0m\n' "$1"; }

group "Shell syntax"
for f in src/scripts/*.sh src/event/* scripts/*.sh tests/*.sh; do
    [ -f "$f" ] || continue
    if err=$(bash -n "$f" 2>&1); then ok "$f"; else bad "$f" "$err"; fi
done

group "PHP syntax"
if command -v php >/dev/null 2>&1; then
    for f in src/scripts/*.php src/*.page src/ZFSToolkitPage.php; do
        [ -f "$f" ] || continue
        if err=$(php -l "$f" 2>&1); then ok "$f"; else bad "$f" "$err"; fi
    done
else
    skip "php not installed"
fi

group "Unraid compatibility rules"

if hits=$(grep -rnE '\b(str_starts_with|str_ends_with|str_contains)\s*\(' src/ 2>/dev/null); then
    bad "no PHP 8 only string helpers" "$hits"
else
    ok "no PHP 8 only string helpers"
fi

if hits=$(grep -rnE 'function [a-zA-Z_]+\([^)]*\b[a-z]+\|[a-z]+ ' src/scripts/*.php 2>/dev/null); then
    bad "no union types in signatures" "$hits"
else
    ok "no union types in signatures"
fi

if hits=$(grep -rn 'ob_start(' src/scripts/*.php 2>/dev/null | grep -vE ':[[:space:]]*(\*|//|#)'); then
    bad "no ob_start() in endpoints" "$hits"
else
    ok "no ob_start() in endpoints"
fi

for f in src/scripts/setup_cron.sh src/scripts/setup_snapshots.sh src/scripts/setup_send.sh; do
    [ -f "$f" ] || { bad "$f exists"; continue; }
    if grep -q 'zdc_valid_cron' "$f"; then
        ok "$(basename "$f") validates its cron expression"
    else
        bad "$(basename "$f") validates its cron expression" \
            "An invalid line makes busybox crond drop the whole crontab."
    fi
done

if grep -qE '^\s*\(\s*crontab -l' src/scripts/setup_*.sh 2>/dev/null; then
    bad "setup scripts use /etc/cron.d" "Direct 'crontab -' writes are lost when update_cron runs."
else
    ok "setup scripts use /etc/cron.d"
fi

if hits=$(grep -rnE "rsync -a[^HAX'\"[:alnum:]-]" src/scripts/ 2>/dev/null \
          | grep -v RSYNC_OPTS | grep -vE ':[[:space:]]*(\*|//|#)'); then
    bad "rsync preserves hardlinks/ACLs/xattrs" "$hits"
else
    ok "rsync preserves hardlinks/ACLs/xattrs"
fi

group "Behaviour: cron expression validator"
if ZDC_TMP_DIR="$(mktemp -d)" . src/scripts/zdc_common.sh 2>/dev/null; then
    cron_case() {
        local expr="$1" want="$2" got
        if zdc_valid_cron "$expr"; then got="valid"; else got="reject"; fi
        if [ "$got" = "$want" ]; then ok "cron [$expr] -> $want"
        else bad "cron [$expr] -> expected $want, got $got"; fi
    }
    cron_case '*/15 * * * *'        valid
    cron_case '0 2 * * *'           valid
    cron_case '0 */6 * * MON-FRI'   valid
    cron_case '30 3 1,15 * *'       valid
    cron_case '*/15 * * *'          reject
    cron_case '*/15 * * * * *'      reject
    cron_case ''                    reject
    cron_case '* * * * * rm -rf /'  reject
    cron_case '0 2 * * * ; reboot'  reject
    cron_case '0 2 * * $(id)'       reject
    cron_case '0 2 * * * && curl x' reject

    ds_case() {
        local n="$1" want="$2" got
        if zdc_valid_dataset "$n"; then got="valid"; else got="reject"; fi
        if [ "$got" = "$want" ]; then ok "dataset [$n] -> $want"
        else bad "dataset [$n] -> expected $want, got $got"; fi
    }
    ds_case 'cache/appdata'      valid
    ds_case 'tank/a/b/c'         valid
    ds_case 'cache'              valid
    ds_case 'cache/my data'      reject
    ds_case 'cache/appdata;rm'   reject
    ds_case '/cache/appdata'     reject
    ds_case 'cache/app@data'     reject

    b_case() {
        local spec="$1" total="$2" want="$3" got
        got=$(zdc_to_bytes "$spec" "$total" 2>/dev/null)
        if [ "${got:-}" = "$want" ]; then ok "to_bytes [$spec] -> $want"
        else bad "to_bytes [$spec] -> expected $want, got '${got:-}'"; fi
    }
    b_case '100G' 0    107374182400
    b_case '2T'   0    2199023255552
    b_case '512'  0    512
    b_case '10%'  1000 100
else
    bad "zdc_common.sh can be sourced"
fi

group "Behaviour: snapshot dataset JSON parser"
if command -v perl >/dev/null 2>&1; then
    tmp=$(mktemp -d)
    cat > "$tmp/ds.json" <<'JSON'
{
  "datasets": [
    {"name": "cache/appdata", "recursive": true, "use_template": true,
     "hourly": "", "daily": "", "weekly": "", "monthly": "", "yearly": ""},
    {"name": "tank\/media\/photos", "recursive": false, "use_template": false,
     "hourly": "6", "daily": "14", "weekly": "2", "monthly": "0", "yearly": "1"}
  ]
}
JSON
    out=$(python3 - "$tmp" <<'PY'
import re, subprocess, sys, pathlib
tmp = sys.argv[1]
src = pathlib.Path('src/scripts/snapshot_manager.sh').read_text()
m = re.search(r"perl -e '\n(.*?)\n' \"\$DATASETS_JSON\"", src, re.S)
if not m:
    print("EXTRACT_FAIL"); sys.exit(0)
pathlib.Path(tmp + '/p.pl').write_text(m.group(1))
r = subprocess.run(['perl', tmp + '/p.pl', tmp + '/ds.json'], capture_output=True, text=True)
sys.stdout.write(r.stdout)
PY
)
    if printf '%s' "$out" | grep -q '^cache/appdata'; then
        ok "parser: plain dataset name survives"
    else
        bad "parser: plain dataset name survives" "$out"
    fi
    if printf '%s' "$out" | grep -q '^tank/media/photos'; then
        ok "parser: escaped \\/ is unescaped, entry not dropped"
    else
        bad "parser: escaped \\/ is unescaped, entry not dropped" \
            "Expected 'tank/media/photos' in output, got: $out"
    fi
    if [ "$(printf '%s' "$out" | grep -c .)" = "2" ]; then
        ok "parser: both datasets returned"
    else
        bad "parser: both datasets returned" "$out"
    fi
    if printf '%s' "$out" | grep -qP '^tank/media/photos\t0\t6\t14\t2\t0\t1$' 2>/dev/null \
       || printf '%s' "$out" | grep -q "tank/media/photos.*6.*14.*2.*0.*1"; then
        ok "parser: per-dataset retention overrides preserved"
    else
        bad "parser: per-dataset retention overrides preserved" "$out"
    fi
    rm -rf "$tmp"
else
    skip "perl not installed"
fi

group "Behaviour: prune matcher"
snaps=$(cat <<'EOF'
cache/appdata@auto-hourly-2026-08-07_10-00	1	1	0
cache/appdata@auto-daily-2026-08-06	1	1	0
cache/appdata@autosnap_2026-08-07_09:00:00	1	1	0
cache/appdata@manual-before-upgrade	1	1	0
cache/appdata@sanoid-auto-hourly-2026-08-07	1	1	0
EOF
)
matched=$(printf '%s\n' "$snaps" | awk -F'\t' -v p="auto-hourly-" \
    '{ split($1, a, "@"); if (a[2] != "" && index(a[2], p) == 1) print $1 }')
if [ "$matched" = "cache/appdata@auto-hourly-2026-08-07_10-00" ]; then
    ok "prune matcher selects only our own snapshots of that type"
else
    bad "prune matcher selects only our own snapshots of that type" "$matched"
fi

group "Plugin manifest"
PLG="src/zfs.toolkit.plg"
if [ -f "$PLG" ]; then
    if python3 -c "import xml.dom.minidom,sys; xml.dom.minidom.parse('$PLG')" 2>/dev/null; then
        ok "plg is well-formed XML"
    else
        bad "plg is well-formed XML"
    fi

    ver=$(grep -oE '<!ENTITY version[[:space:]]+"[^"]+"' "$PLG" | grep -oE '"[^"]+"' | tr -d '"')
    md5=$(grep -oE '<!ENTITY md5[[:space:]]+"[^"]+"' "$PLG" | grep -oE '"[^"]+"' | tr -d '"')

    if [[ "$ver" =~ ^[0-9]{4}\.[0-9]{2}\.[0-9]{2}\.[0-9]+$ ]]; then
        ok "version entity looks like YYYY.MM.DD.NN ($ver)"
    else
        bad "version entity looks like YYYY.MM.DD.NN" "got '$ver'"
    fi

    if [[ "$md5" =~ ^[0-9a-f]{32}$ ]]; then
        ok "md5 entity is a hex digest"
    else
        bad "md5 entity is a hex digest" "got '$md5'"
    fi
else
    bad "$PLG exists"
fi

group "Unraid page rendering"

zdc_md_missing=""
for pg in src/*.page; do
    grep -q 'Markdown="false"' "$pg" || zdc_md_missing="$zdc_md_missing $pg"
done
if [ -z "$zdc_md_missing" ]; then
    ok 'every .page sets Markdown="false"'
else
    bad 'every .page sets Markdown="false"' \
        "missing in:$zdc_md_missing - Unraid runs .page bodies through Markdown otherwise, which eats */5 in a cron string and turns indented blocks into code, so the JS renders as text."
fi

page_body_lines=$(sed -n '/^---$/,$p' src/ZFSToolkit.page | grep -c .)
if [ "$page_body_lines" -le 6 ]; then
    ok ".page body stays minimal (${page_body_lines} lines, includes the real page)"
else
    bad ".page body stays minimal" "${page_body_lines} lines - keep markup in ZFSToolkitPage.php"
fi

if [ -f src/ZFSToolkitPage.php ]; then
    ok "ZFSToolkitPage.php exists"
else
    bad "ZFSToolkitPage.php exists"
fi

group "Uninstall removes what install creates"

zdc_missing_cron=""
for f in src/scripts/setup_*.sh; do
    base=$(grep -oE 'CRON_FILE_BASE="[^"]+"' "$f" | cut -d'"' -f2)
    [ -n "$base" ] || continue
    grep -q "/etc/cron.d/.*${base#zfs.toolkit}" src/zfs.toolkit.plg \
        || zdc_missing_cron="$zdc_missing_cron $base"
done
if [ -z "$zdc_missing_cron" ]; then
    ok "every cron file the setup scripts create is removed on uninstall"
else
    bad "every cron file the setup scripts create is removed on uninstall" \
        "not cleaned up:$zdc_missing_cron - cron would keep calling a deleted script"
fi

zdc_missing_kill=""
for pat in run_auto snapshot_manager zfs_send; do
    grep -q "$pat" <(sed -n '/Method="remove"/,/<\/FILE>/p' src/zfs.toolkit.plg) \
        || zdc_missing_kill="$zdc_missing_kill $pat"
done
if [ -z "$zdc_missing_kill" ]; then
    ok "uninstall strips every legacy crontab line"
else
    bad "uninstall strips every legacy crontab line" "missing:$zdc_missing_kill"
fi

group "GUI references resolve"

if out=$(python3 tests/js_checks.py src/ZFSToolkitPage.php 2>&1); then
    ok "JS calls, on* handlers and element ids all resolve"
    printf '       %s\n' "$(printf '%s' "$out" | sed 's/^ *//')"
else
    bad "JS calls, on* handlers and element ids all resolve" "$out"
fi
PAGE="src/ZFSToolkitPage.php"
if [ -f "$PAGE" ]; then
    missing=""
    for ep in $(grep -oE "scripts/[a-z_]+\.php" "$PAGE" | sort -u | sed 's|scripts/||'); do
        [ -f "src/scripts/$ep" ] || missing="$missing $ep"
    done
    if [ -z "$missing" ]; then
        ok "every PHP endpoint referenced by the page exists"
    else
        bad "every PHP endpoint referenced by the page exists" "missing:$missing"
    fi

    state_leak=""
    grep -qE "zdc-pill ' \+" "$PAGE" \
        && state_leak="$state_leak concatenates a bare state into the pill class;"
    grep -qE 'class="(ok|warn|err)"' "$PAGE" \
        && state_leak="$state_leak a bare ok/warn/err class literal;"
    grep -qE '^\.zdc-pill\.(ok|warn|err)\b' "$PAGE" \
        && state_leak="$state_leak CSS still targets the unprefixed state;"
    grep -q "' zdc-' + state" "$PAGE" \
        || state_leak="$state_leak the namespaced pill class is gone;"
    if [ -z "$state_leak" ]; then
        ok "state classes are namespaced (Unraid's theme defines .warn and .err itself)"
    else
        bad "state classes are namespaced" \
            "found:$state_leak - a bare .warn gets Unraid's yellow notice styling, which leaves our light value text unreadable on it"
    fi

    posts=$(grep -c "method: 'POST'" "$PAGE")
    tokens=$(grep -c "csrf_token" "$PAGE")
    if [ "$tokens" -ge "$posts" ]; then
        ok "CSRF token present for POST requests ($tokens references, $posts posts)"
    else
        bad "CSRF token present for POST requests" "$posts POSTs but only $tokens csrf_token references"
    fi
else
    bad "$PAGE exists"
fi

printf '\n\033[1mResult:\033[0m %d passed, %d failed, %d skipped\n' "$PASS" "$FAIL" "$SKIP"
[ "$FAIL" -eq 0 ]
