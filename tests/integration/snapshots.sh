#!/usr/bin/env bash

. "$(dirname "${BASH_SOURCE[0]}")/common.sh"

STATUS="$ZDC_TMP_DIR/snapshot_status.json"
LOG="$ZDC_TMP_DIR/snapshots.log"
DSJ="$ZDC_CONFIG_DIR/snap_datasets.json"

settings() {
    {
        echo "snapshots_enabled=yes"
        for t in frequent hourly daily weekly monthly yearly; do echo "snap_$t=0"; done
        echo "snap_daily_hour=0"
        echo "snap_min_free_pct=0"
        for kv in "$@"; do echo "$kv"; done
    } > "$ZDC_CONFIG_DIR/settings.cfg"
}
datasets() { printf '{"datasets":[%s]}\n' "$1" > "$DSJ"; }
snaps() { zfs list -H -t snapshot -o name -s creation -d 1 "$1" 2>/dev/null | sed 's/.*@//'; }
nsnaps() { snaps "$1" | grep -c "${2:-.}"; }
jget() { grep -o "\"$1\": *[0-9]*" "$STATUS" | grep -o '[0-9]*$'; }
tick() { "$ZT" snapshot-tick "$@" >> "$LOG" 2>&1; }
apij() { "$ZT" api "$1" "${2:-{\}}"; }
okj() { printf '%s' "$1" | grep -q '"ok": true'; }

H=$(date '+%Y-%m-%d_%H-00'); D=$(date '+%Y-%m-%d'); W=$(date '+%G-W%V'); MO=$(date '+%Y-%m'); Y=$(date '+%Y')

group "Due-based creation"
zfs create "$C/app"
zfs create "$C/app/plex"
zfs create "$C/TV Shows"
settings snap_hourly=24 snap_daily=7 snap_weekly=4 snap_monthly=3 snap_yearly=1
datasets "{\"name\":\"$C\\/app\",\"recursive\":true,\"use_template\":true,\"hourly\":\"\",\"daily\":\"\",\"weekly\":\"\",\"monthly\":\"\",\"yearly\":\"\"},{\"name\":\"$C/TV Shows\",\"recursive\":false,\"use_template\":false,\"hourly\":\"\",\"daily\":\"5\",\"weekly\":\"\",\"monthly\":\"\",\"yearly\":\"\"}"
tick
check "one snapshot per active type and period" '[ "$(nsnaps "$C/app" ^auto-)" = 5 ]' "$(snaps "$C/app" | tr '\n' ' ')"
check "period keys in the names" 'snaps "$C/app" | grep -qx "auto-hourly-$H" && snaps "$C/app" | grep -qx "auto-weekly-$W" && snaps "$C/app" | grep -qx "auto-monthly-$MO" && snaps "$C/app" | grep -qx "auto-yearly-$Y"'
check "escaped slash in snap_datasets.json is read" 'grep -q "\\\\/" "$DSJ" && snaps "$C/app" | grep -qx "auto-daily-$D"'
check "recursive reaches the child" '[ "$(nsnaps "$C/app/plex" ^auto-)" = 5 ]'
check "dataset name with a space, own retention (daily only)" '[ "$(snaps "$C/TV Shows" | tr "\n" " ")" = "auto-daily-$D " ]'
check "status file written" '[ "$(jget created)" = 6 ] && [ "$(jget errors)" = 0 ] && [ -s "$ZDC_TMP_DIR/snapshot_last.txt" ]'
tick
check "a second run in the same period creates nothing" '[ "$(nsnaps "$C/app" ^auto-)" = 5 ] && [ "$(jget created)" = 0 ]'
check "log says nothing is due" 'grep -q "Nothing due" "$LOG"'

group "Daily hour and catch-up"
zfs destroy -r "$C/app@auto-daily-$D"
settings snap_hourly=24 snap_daily=7 snap_weekly=4 snap_monthly=3 snap_yearly=1 snap_daily_hour=23
if [ "$(date +%-H)" -lt 23 ]; then
    tick
    check "daily waits for the daily hour" '! snaps "$C/app" | grep -qx "auto-daily-$D"'
    tick --now
    check "--now ignores the daily hour" 'snaps "$C/app" | grep -qx "auto-daily-$D"'
else
    ok "skipped (it is already 23:xx)"
fi
zfs destroy -r "$C/app@auto-daily-$D" 2>/dev/null
settings snap_hourly=24 snap_daily=7 snap_weekly=4 snap_monthly=3 snap_yearly=1
tick
check "a missed daily is caught up on the next run" 'snaps "$C/app" | grep -qx "auto-daily-$D"'
settings snap_hourly=24 snap_frequent=0
tick --all
check "--all also takes a frequent snapshot" '[ "$(nsnaps "$C/app" ^auto-frequent-)" = 1 ]'

group "Count and age retention"
zfs create "$C/ret"
for i in 01 02 03 04 05 06; do zfs snapshot "$C/ret@auto-hourly-2026-01-01_$i-00"; sleep 1; done
zfs snapshot "$C/ret@before-upgrade"
zfs snapshot "$C/ret@autosnap_2026-01-01_00:00:00_hourly"
zfs snapshot "$C/ret@zdc-send-job1-20260101"
zfs hold keep "$C/ret@auto-hourly-2026-01-01_01-00"
settings snap_hourly=3
datasets "{\"name\":\"$C/ret\",\"recursive\":false,\"use_template\":true}"
tick
left=$(snaps "$C/ret" | tr '\n' ' ')
check "newest 3 hourly kept (incl. the new one), held one survives" '[ "$left" = "auto-hourly-2026-01-01_01-00 auto-hourly-2026-01-01_05-00 auto-hourly-2026-01-01_06-00 before-upgrade autosnap_2026-01-01_00:00:00_hourly zdc-send-job1-20260101 auto-hourly-$H " ]' "$left"
check "held snapshot logged" 'grep -q "Keeping held snapshot $C/ret@auto-hourly-2026-01-01_01-00" "$LOG"'
zfs release keep "$C/ret@auto-hourly-2026-01-01_01-00"
settings snap_hourly=50 snap_age_hourly=0
tick
check "raising the count prunes nothing" '[ "$(nsnaps "$C/ret" ^auto-hourly-)" = 4 ]'
zfs snapshot "$C/ret@auto-hourly-2025-01-01_00-00"
settings snap_hourly=0 snap_age_hourly=0
tick
check "a type without retention is left alone" 'snaps "$C/ret" | grep -qx auto-hourly-2025-01-01_00-00'
zfs destroy "$C/ret@auto-hourly-2025-01-01_00-00"
settings snap_hourly=1
zfs hold keep "$C/ret@auto-hourly-$H"
tick
check "the newest of an active type stays, older go" '[ "$(snaps "$C/ret" | grep ^auto-hourly- | tr "\n" " ")" = "auto-hourly-$H " ]'
zfs release keep "$C/ret@auto-hourly-$H"
check "foreign snapshots are never pruned" 'snaps "$C/ret" | grep -qx before-upgrade && snaps "$C/ret" | grep -qx "autosnap_2026-01-01_00:00:00_hourly" && snaps "$C/ret" | grep -qx zdc-send-job1-20260101'

group "Dry run and preview"
for i in 01 02 03; do zfs snapshot "$C/ret@auto-hourly-2026-02-01_$i-00"; done
before=$(nsnaps "$C/ret")
"$ZT" snapshot-tick --dry-run >> "$LOG" 2>&1
check "--dry-run destroys nothing" '[ "$(nsnaps "$C/ret")" = "$before" ]'
out=$(apij snap_preview)
check "preview lists the planned prunes" 'okj "$out" && [ "$(printf "%s" "$out" | grep -o "\"op\": \"prune\"" | wc -l)" -ge 3 ]' "$out"
out=$(apij snap_preview "{\"settings\":{\"snap_hourly\":\"10\"}}")
check "preview with unsaved settings" 'okj "$out" && ! printf "%s" "$out" | grep -q "\"op\": \"prune\""' "$out"
check "preview changes nothing" '[ "$(nsnaps "$C/ret")" = "$before" ]'

group "Age retention"
settings snap_hourly=50 snap_age_hourly=1
tick
check "age goes by creation time, not by the date in the name" '[ "$(nsnaps "$C/ret" ^auto-hourly-2026-02-01)" = 3 ]' "$(snaps "$C/ret" | tr '\n' ' ')"

group "Manager actions"
zfs snapshot "$C/ret@manual-1"
out=$(apij snap_hold "{\"snapshots\":[\"$C/ret@manual-1\"]}")
check "hold" 'okj "$out" && [ "$(zfs get -H -o value userrefs "$C/ret@manual-1")" = 1 ]' "$out"
out=$(apij snap_hold "{\"snapshots\":[\"$C/ret@manual-1\"]}")
check "hold twice is fine" 'printf "%s" "$out" | grep -q "already held"'
out=$(apij snap_destroy "{\"snapshots\":[\"$C/ret@manual-1\"]}")
check "held snapshot is not destroyed" 'printf "%s" "$out" | grep -q "\"deleted\": 0" && snaps "$C/ret" | grep -qx manual-1' "$out"
out=$(apij snap_release "{\"snapshots\":[\"$C/ret@manual-1\"]}")
check "release" 'okj "$out" && [ "$(zfs get -H -o value userrefs "$C/ret@manual-1")" = 0 ]'
out=$(apij snap_destroy "{\"snapshots\":[\"$C/ret@manual-1\",\"$C/ret@zdc-send-job1-20260101\",\"bad;name@x\"]}")
check "destroy, but not the replication checkpoint" 'printf "%s" "$out" | grep -q "\"deleted\": 1" && ! snaps "$C/ret" | grep -qx manual-1 && snaps "$C/ret" | grep -qx zdc-send-job1-20260101' "$out"
check "invalid name refused" 'printf "%s" "$out" | grep -q "Invalid snapshot name"'
out=$(apij snap_destroy "{\"snapshots\":[\"$C/ret@zdc-send-job1-20260101\"],\"force_checkpoint\":true}")
check "checkpoint only with force" '! snaps "$C/ret" | grep -qx zdc-send-job1-20260101' "$out"
out=$(apij snap_create "{\"dataset\":\"$C/TV Shows\",\"name\":\"before move\"}")
check "manual snapshot on a dataset with a space" 'okj "$out" && snaps "$C/TV Shows" | grep -qx "before move"' "$out"
out=$(apij snap_create "{\"dataset\":\"$C/ret\",\"name\":\"auto-hourly-x\"}")
check "auto- names are reserved" '! okj "$out"'
out=$(apij snap_list "{\"dataset\":\"$C/TV Shows\"}")
check "list a dataset with a space" 'okj "$out" && printf "%s" "$out" | grep -q "\"snapshot\": \"before move\""' "$out"

zfs create "$C/rb"
echo one > "/mnt/$C/rb/file"; zfs snapshot "$C/rb@one"; sleep 1
echo two > "/mnt/$C/rb/file"; zfs snapshot "$C/rb@two"; sleep 1
zfs snapshot "$C/rb@zdc-send-j-1"
echo three > "/mnt/$C/rb/file"
out=$(apij snap_rollback_check "{\"snapshot\":\"$C/rb@one\"}")
check "rollback check names the newer snapshots" 'printf "%s" "$out" | grep -q "\"checkpoints\": 1" && printf "%s" "$out" | grep -q "$C/rb@two"' "$out"
out=$(apij snap_rollback "{\"snapshot\":\"$C/rb@one\",\"confirm\":\"$C/rb@two\"}")
check "rollback needs the matching confirmation" '! okj "$out" && [ "$(cat /mnt/$C/rb/file)" = three ]'
zfs hold keep "$C/rb@two"
out=$(apij snap_rollback "{\"snapshot\":\"$C/rb@one\",\"confirm\":\"$C/rb@one\"}")
check "a held newer snapshot blocks the rollback before anything is destroyed" '! okj "$out" && [ "$(cat /mnt/$C/rb/file)" = three ] && [ "$(nsnaps "$C/rb")" = 3 ]' "$out"
zfs release keep "$C/rb@two"
out=$(apij snap_rollback "{\"snapshot\":\"$C/rb@one\",\"confirm\":\"$C/rb@one\"}")
check "rollback restores the data and reports lost checkpoints" 'okj "$out" && [ "$(cat /mnt/$C/rb/file)" = one ] && printf "%s" "$out" | grep -q "full send"' "$out"

group "Dataset list API"
out=$(apij snap_datasets_save "{\"datasets\":[{\"name\":\"$C/app\",\"recursive\":true,\"use_template\":true},{\"name\":\"$C/TV Shows\",\"use_template\":false,\"daily\":\"5\",\"hourly\":\"\"}]}")
check "save" 'okj "$out" && grep -q "\"name\": \"$C/app\"" "$DSJ" && ! grep -q "\\\\/" "$DSJ"' "$(cat "$DSJ")"
out=$(apij snap_datasets_save "{\"datasets\":[{\"name\":\"$C/Büro\"}]}")
check "invalid names are refused and nothing is written" '! okj "$out" && grep -q "TV Shows" "$DSJ"'
out=$(apij snap_datasets_save "{\"datasets\":[{\"name\":\"$C/app\",\"use_template\":false,\"daily\":\"x\"}]}")
check "invalid retention refused" '! okj "$out"'
out=$(apij snap_datasets)
check "read back" 'okj "$out" && printf "%s" "$out" | grep -q "\"daily\": \"5\""'
out=$(apij overview)
check "overview part" 'printf "%s" "$out" | grep -q "\"snapshots\": {" && printf "%s" "$out" | grep -q "\"by_type\":"' "$(printf '%s' "$out" | head -c 400)"
out=$(apij snap_schedule "{\"expr\":\"*/15 * * * *\"}")
check "schedule preview" 'okj "$out" && printf "%s" "$out" | grep -q "\"problem\": \"\""'
out=$(apij snap_schedule "{\"expr\":\"*/15 * * *\"}")
check "schedule problem named" 'printf "%s" "$out" | grep -q "5 fields"'

group "Run now"
settings snap_hourly=24 snap_daily=5
zfs destroy "$C/TV Shows@auto-daily-$D"
out=$(apij snap_run)
for _ in $(seq 1 50); do sleep 0.2; [ -e "$ZDC_TMP_DIR/snapshot_running.json" ] || break; done
sleep 0.5
check "run now starts a detached manual run" 'okj "$out" && snaps "$C/TV Shows" | grep -qx "auto-daily-$D" && grep -q "(manual run)" "$LOG"' "$out"
check "status says forced" '[ "$(jget forced)" = 1 ]'

group "Locking"
exec 8>"$ZDC_TMP_DIR/snapshot_manager.lock"; flock -n 8
out=$("$ZT" snapshot-tick 2>&1)
check "a second run skips while one is active" 'printf "%s" "$out" | grep -q "still active"'
exec 8>&-

group "Errors"
datasets "{\"name\":\"$C/gone\",\"use_template\":true}"
tick
check "missing dataset is an error with the available list" '[ "$(jget errors)" = 2 ] && grep -q "Available ZFS datasets: .*$C/app" "$LOG"'
printf '{"datasets":[' > "$DSJ"
tick
check "broken dataset list is reported" '[ "$(jget errors)" = 1 ] && grep -q "not valid JSON" "$LOG"'

group "Free-space pruning"
zfs create "$B/fill"
for i in 01 02 03 04 05 06; do
    dd if=/dev/urandom of="/mnt/$B/fill/data" bs=1M count=100 status=none; sync
    zfs snapshot "$B/fill@auto-hourly-2026-01-01_$i-00"
done
rm "/mnt/$B/fill/data"; sync; sleep 6
free=$(zpool list -Hp -o free "$B")
settings snap_hourly=50 "snap_free_target=$(( (free + 150 * 1048576) / 1048576 ))M"
datasets "{\"name\":\"$B/fill\",\"recursive\":false,\"use_template\":true}"
tick
check "exactly as many snapshots go as the target needs (2 of 100 MB for 150 MB)" '[ "$(jget pruned)" = 2 ]' "pruned $(jget pruned)"
check "oldest first" '! snaps "$B/fill" | grep -q "_0[12]-00" && snaps "$B/fill" | grep -q "_03-00"'

group "Log stays bounded"
head -c 1200000 /dev/zero | tr '\0' 'x' | fold -w 100 >> "$LOG"
tick
check "log trimmed below 1 MB" '[ "$(stat -c %s "$LOG")" -lt 1048576 ]'

finish
