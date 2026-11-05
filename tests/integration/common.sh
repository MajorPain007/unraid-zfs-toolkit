#!/usr/bin/env bash

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
AREA="$(basename "${BASH_SOURCE[1]:-x}" .sh)"

[ "$(id -u)" = 0 ] || { echo "run as root" >&2; exit 2; }
command -v zfs >/dev/null || { echo "needs zfs" >&2; exit 2; }

PASS=0; FAIL=0
ok()  { printf '  \033[32mPASS\033[0m %s\n' "$1"; PASS=$((PASS + 1)); }
bad() { printf '  \033[31mFAIL\033[0m %s\n' "$1"; [ -n "${2:-}" ] && printf '       %s\n' "$2"; FAIL=$((FAIL + 1)); }
group() { printf '\n\033[1m%s\033[0m\n' "$1"; }
check() { if eval "$2"; then ok "$1"; else bad "$1" "${3:-}"; fi; }

C="zdct_${AREA}_c"; B="zdct_${AREA}_b"
WORK=$(mktemp -d "/var/tmp/zdct-${AREA}.XXXXXX")
export ZDC_CONFIG_DIR="$WORK/config" ZDC_TMP_DIR="$WORK/tmp" ZDC_CRON_DIR="$WORK/cron"
export ZDC_CONVERSION_JOURNAL="$ZDC_CONFIG_DIR/conversion_in_progress"
export ZDC_NOTIFY=/bin/true ZDC_VAR_INI="$WORK/var.ini"
mkdir -p "$ZDC_CONFIG_DIR" "$ZDC_TMP_DIR" "$ZDC_CRON_DIR" "$WORK/bin"
printf 'NAME="tower"\nmdState="STARTED"\nfsState="Started"\n' > "$ZDC_VAR_INI"

GO=$(command -v go || echo /usr/local/go/bin/go)
export ZDC_BIN="$WORK/bin/zfs-toolkit"
( cd "$ROOT" && "$GO" build -o "$ZDC_BIN" ./cmd/zfs-toolkit ) || { echo "build failed" >&2; exit 2; }
ZT="$ZDC_BIN"

cleanup() {
    zpool destroy -f "$C" 2>/dev/null; zpool destroy -f "$B" 2>/dev/null
    rm -rf "$WORK"; rmdir "/mnt/$C" "/mnt/$B" 2>/dev/null
}
trap cleanup EXIT

zpool destroy -f "$C" 2>/dev/null; zpool destroy -f "$B" 2>/dev/null
truncate -s 2G "$WORK/c.img" "$WORK/b.img"
zpool create -f -m "/mnt/$C" "$C" "$WORK/c.img" && zpool create -f -m "/mnt/$B" "$B" "$WORK/b.img" \
    || { echo "cannot create the test pools" >&2; exit 2; }

mounted_at() { [ "$(zfs list -H -o mounted,mountpoint 2>/dev/null | awk -F'\t' -v p="$1" '$1=="yes" && $2==p' | wc -l)" = 1 ]; }
api() { local action="$1"; shift; "$ZT" api "$action" "${1:-{\}}"; }

finish() {
    printf '\n\033[1mResult:\033[0m %d passed, %d failed\n' "$PASS" "$FAIL"
    [ "$FAIL" -eq 0 ]
}
