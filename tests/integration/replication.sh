#!/usr/bin/env bash

. "$(dirname "${BASH_SOURCE[0]}")/common.sh"
SRC="$ROOT/src/scripts"

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

finish
