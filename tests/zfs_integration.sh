#!/usr/bin/env bash
#
# Runs the plugin's scripts against real ZFS: conversion, its failure and
# recovery paths, free-space pruning and replication.
#
# Needs root, zfs and php on Linux - not the CI runners, which have no ZFS.
# Creates two small pools backed by files in /var/tmp, mounted at
# /mnt/zdctest_cache and /mnt/zdctest_backup, and destroys them again at the
# end. Pools of any other name are not touched.
#
#   sudo bash tests/zfs_integration.sh

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$ROOT/src/scripts"

[ "$(id -u)" = 0 ] || { echo "run as root" >&2; exit 2; }
command -v zfs >/dev/null && command -v php >/dev/null || { echo "needs zfs and php" >&2; exit 2; }

PASS=0; FAIL=0
ok()  { printf '  \033[32mPASS\033[0m %s\n' "$1"; PASS=$((PASS + 1)); }
bad() { printf '  \033[31mFAIL\033[0m %s\n' "$1"; [ -n "${2:-}" ] && printf '       %s\n' "$2"; FAIL=$((FAIL + 1)); }
group() { printf '\n\033[1m%s\033[0m\n' "$1"; }
check() { if eval "$2"; then ok "$1"; else bad "$1" "${3:-}"; fi; }

C=zdctest_cache; B=zdctest_backup
WORK=$(mktemp -d /var/tmp/zdctest.XXXXXX)
export ZDC_CONFIG_DIR="$WORK/config" ZDC_TMP_DIR="$WORK/tmp"
export ZDC_CONVERSION_JOURNAL="$ZDC_CONFIG_DIR/conversion_in_progress"
mkdir -p "$ZDC_CONFIG_DIR" "$ZDC_TMP_DIR" "$WORK/bin"

cleanup() {
    zpool destroy -f "$C" 2>/dev/null; zpool destroy -f "$B" 2>/dev/null
    rm -rf "$WORK"; rmdir "/mnt/$C" "/mnt/$B" 2>/dev/null
}
trap cleanup EXIT

truncate -s 2G "$WORK/c.img" "$WORK/b.img"
zpool create -f -m "/mnt/$C" "$C" "$WORK/c.img" && zpool create -f -m "/mnt/$B" "$B" "$WORK/b.img" \
    || { echo "cannot create the test pools" >&2; exit 2; }

# rsync as the converter sees it: the real one, or on request one that fails,
# drops a file, or takes its time.
cat > "$WORK/bin/rsync" <<'EOF'
#!/bin/bash
src="${@: -2:1}"; dst="${@: -1}"
case "$(cat "$(dirname "$0")/mode" 2>/dev/null)" in
  fail)    /usr/bin/rsync -a --exclude=b.txt "$src" "$dst"; exit 23 ;;
  partial) /usr/bin/rsync -a --exclude=b.txt "$src" "$dst"; exit 0 ;;
  slow)    sleep 60; exit 0 ;;
  *)       exec /usr/bin/rsync "$@" ;;
esac
EOF
chmod +x "$WORK/bin/rsync"
rsync_mode() { echo "$1" > "$WORK/bin/mode"; }

convert() {
    printf 'dry_run=no\ncleanup=yes\nreplace_spaces=%s\nshould_process_containers=yes\nappdata_pool=%s\nappdata_dataset=appdata\nshould_process_vms=no\n' \
        "${1:-no}" "$C" > "$ZDC_CONFIG_DIR/settings.cfg"
    PATH="$WORK/bin:$PATH" bash "$SRC/run_auto.sh" >/dev/null
    local pid; pid=$(grep -o '"pid":[0-9]*' "$ZDC_TMP_DIR/status.json" | grep -o '[0-9]*')
    [ "${2:-wait}" = "wait" ] && while kill -0 "$pid" 2>/dev/null; do sleep 0.2; done
    echo "$pid"
}
lastlog() { cat "$(ls -t "$ZDC_TMP_DIR"/auto_*.log | head -1)"; }
fresh_appdata() {
    # compression off: with it, ZFS would shrink the zeros of a sparse image
    # away by itself and the sparse check below would prove nothing.
    zfs destroy -r "$C/appdata" 2>/dev/null; rm -rf "/mnt/$C/appdata"; zfs create -o compression=off "$C/appdata"
    rm -f "$ZDC_CONVERSION_JOURNAL"
}
mounted_at() { [ "$(zfs list -H -o mounted,mountpoint 2>/dev/null | awk -F'\t' -v p="$1" '$1=="yes" && $2==p' | wc -l)" = 1 ]; }
A="/mnt/$C/appdata"

group "Conversion"
fresh_appdata
zfs create "$C/appdata/tdarr"; mkdir -p "$A/tdarr/configs" "$A/tdarr_temp"
echo keep > "$A/tdarr/configs/config.json"; echo cache > "$A/tdarr_temp/cache.tmp"
for d in plex "Home Assistant" "Büro (alt)" _backup; do mkdir -p "$A/$d"; echo "$d" > "$A/$d/file.txt"; done
truncate -s 1G "$A/plex/disk.img"
rsync_mode normal; convert >/dev/null
check "a plain folder becomes a dataset" 'mounted_at "$A/plex"'
check "a name with a space keeps it" 'zfs list "$C/appdata/Home Assistant" >/dev/null 2>&1 && mounted_at "$A/Home Assistant"'
check "a name ZFS refuses is converted under another and mounted at its own path" \
      'zfs list "$C/appdata/Buero _alt_" >/dev/null 2>&1 && mounted_at "$A/Büro (alt)"'
check "a leading underscore stays" 'mounted_at "$A/_backup"'
check "the contents arrive" '[ "$(cat "$A/Büro (alt)/file.txt")" = "Büro (alt)" ]'
check "a sparse image stays sparse" '[ "$(du -B1 "$A/plex/disk.img" | cut -f1)" -lt 1048576 ]' \
      "$(du -h "$A/plex/disk.img")"
check "<name>_temp next to the dataset <name> is left alone" \
      '[ -f "$A/tdarr_temp/cache.tmp" ] && [ -f "$A/tdarr/configs/config.json" ] && [ ! -f "$A/tdarr/cache.tmp" ]'
check "no journal is left behind" '[ ! -f "$ZDC_CONVERSION_JOURNAL" ]'

fresh_appdata; mkdir -p "$A/TV Shows"; echo x > "$A/TV Shows/f"
convert yes >/dev/null
check "Replace spaces renames the dataset, not the folder" \
      'zfs list "$C/appdata/TV_Shows" >/dev/null 2>&1 && mounted_at "$A/TV Shows"'

group "A conversion that fails is undone"
for mode in fail partial; do
    fresh_appdata; mkdir -p "$A/app"; echo a > "$A/app/a.txt"; echo b > "$A/app/b.txt"
    rsync_mode "$mode"; convert >/dev/null
    check "rsync $mode: the folder is back, whole, and no dataset is left" \
          '[ -f "$A/app/a.txt" ] && [ -f "$A/app/b.txt" ] && ! zfs list "$C/appdata/app" >/dev/null 2>&1 && [ ! -e "$A/app_temp" ]'
done

group "Stop puts the folder back at once"
fresh_appdata; mkdir -p "$A/app"; echo a > "$A/app/a.txt"; echo b > "$A/app/b.txt"
rsync_mode slow; pid=$(convert no nowait); sleep 2
# shellcheck disable=SC2034  # read inside the check below, through eval
t0=$SECONDS; kill "$pid"; pkill -TERM -P "$pid"; while kill -0 "$pid" 2>/dev/null; do sleep 0.2; done
check "the converter ends within seconds" '[ $((SECONDS - t0)) -le 5 ]'
check "the folder is back and the dataset gone" \
      '[ -f "$A/app/b.txt" ] && ! zfs list "$C/appdata/app" >/dev/null 2>&1 && [ ! -f "$ZDC_CONVERSION_JOURNAL" ]'

group "A run that was cut off hard is settled by the next"
journal() { printf '%s\n' "$C/appdata" app app "$A/app_temp" > "$ZDC_CONVERSION_JOURNAL"; }
rsync_mode normal

fresh_appdata; mkdir -p "$A/app_temp" "$A/app"; echo a > "$A/app_temp/a.txt"; journal
convert >/dev/null
check "renamed but not copied: the original goes back (over Docker's empty stand-in)" \
      '[ -f "$A/app/a.txt" ] && mounted_at "$A/app" && [ ! -e "$A/app_temp" ]'

fresh_appdata; mkdir -p "$A/app_temp"; echo a > "$A/app_temp/a.txt"; journal
zfs create "$C/appdata/app"; /usr/bin/rsync -a "$A/app_temp/" "$A/app/"
convert >/dev/null
check "copied but not cleaned up: finished" '[ ! -e "$A/app_temp" ] && [ -f "$A/app/a.txt" ] && [ ! -f "$ZDC_CONVERSION_JOURNAL" ]'

fresh_appdata; mkdir -p "$A/app_temp"; echo a > "$A/app_temp/a.txt"; echo b > "$A/app_temp/b.txt"; journal
zfs create "$C/appdata/app"; echo other > "$A/app/a.txt"
mkdir -p "$A/plex"; echo p > "$A/plex/f"
convert >/dev/null
check "copy and original differ: nothing is touched and nothing else converted" \
      '[ -f "$A/app_temp/b.txt" ] && [ "$(cat "$A/app/a.txt")" = other ] && [ -f "$ZDC_CONVERSION_JOURNAL" ] && ! mounted_at "$A/plex"'
check "and the log says what to do" 'lastlog | grep -q "Keep one of them"'

group "Free-space pruning"
zfs create "$C/fill"
for i in 01 02 03 04 05 06; do
    dd if=/dev/urandom of="/mnt/$C/fill/data" bs=1M count=100 status=none; sync
    zfs snapshot "$C/fill@auto-hourly-2026-01-01_$i-00"
done
rm "/mnt/$C/fill/data"; sync; sleep 6
free=$(zpool list -Hp -o free "$C")
printf 'snapshots_enabled=yes\nsnap_hourly=50\nsnap_daily=0\nsnap_weekly=0\nsnap_monthly=0\nsnap_free_target=%sM\n' \
    $(( (free + 150 * 1048576) / 1048576 )) > "$ZDC_CONFIG_DIR/settings.cfg"
echo "{\"datasets\":[{\"name\":\"$C/fill\",\"recursive\":false,\"use_template\":true}]}" > "$ZDC_CONFIG_DIR/snap_datasets.json"
bash "$SRC/snapshot_manager.sh" >/dev/null 2>&1
pruned=$(grep -o '"pruned":[0-9]*' "$ZDC_TMP_DIR/snapshot_status.json" | cut -d: -f2)
check "exactly as many snapshots go as the target needs (2 of 100 MB for 150 MB)" '[ "$pruned" = 2 ]' "pruned $pruned"

group "Replication"
zfs create "$C/appdata/plex" 2>/dev/null; zfs create "$C/appdata/Home Assistant" 2>/dev/null
cat > "$ZDC_CONFIG_DIR/send_jobs.json" <<EOF
{"jobs":[{"id":"t1","enabled":true,"name":"t","source":"$C/appdata","dest":"$B/replica/appdata","recursive":true,"transport":"local","ssh_host":"","ssh_port":22,"ssh_key":"","raw":false,"compressed":true,"allow_rollback":true,"keep_dest":2}]}
EOF
zfs create "$B/replica"
for _ in 1 2 3 4; do bash "$SRC/zfs_send.sh" >"$WORK/send.log" 2>&1; sleep 1.1; done
check "four runs replicate" 'grep -q "replicated $C/appdata" "$WORK/send.log"' "$(tail -3 "$WORK/send.log")"
for side in "$C/appdata" "$B/replica/appdata"; do
    counts=$(zfs list -H -t snapshot -o name -r "$side" | grep zdc-send | sed 's/@.*//' | sort | uniq -c | awk '{print $1}' | sort -u)
    check "two checkpoints on every dataset under $side" '[ "$counts" = 2 ]' "counts: $(echo $counts)"
done
bash "$SRC/zfs_send.sh" --dry-run >"$WORK/send.log" 2>&1
check "Force reaches zfs recv, raw stays off" 'grep -q "zfs recv -u -x mountpoint -F" "$WORK/send.log" && ! grep -q "send -R -w" "$WORK/send.log"'

zfs snapshot -r "$C@root1"
cat > "$ZDC_CONFIG_DIR/send_jobs.json" <<EOF
{"jobs":[{"id":"t2","enabled":true,"name":"pool","source":"$C","dest":"$B/replica/pool","recursive":true,"transport":"local"}]}
EOF
bash "$SRC/zfs_send.sh" >"$WORK/send.log" 2>&1
check "a whole pool's copy does not take the pool's mountpoint" \
      '[ "$(zfs get -H -o value mountpoint "$B/replica/pool")" = "/mnt/$B/replica/pool" ]' \
      "$(zfs get -H -o value,source mountpoint "$B/replica/pool" 2>&1)"

printf '\n\033[1mResult:\033[0m %d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]
