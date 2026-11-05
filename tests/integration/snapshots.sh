#!/usr/bin/env bash

. "$(dirname "${BASH_SOURCE[0]}")/common.sh"
SRC="$ROOT/src/scripts"

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


finish
