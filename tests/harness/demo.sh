#!/bin/bash
set -u
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
P=${ZT_DEMO_PREFIX:-}
PC=${P}cache
PB=${P}backup
T=/var/tmp/ztdemo${P:+-$P}
PORT=${ZT_DEMO_PORT:-8090}
export ZDC_CONFIG_DIR=$T/config ZDC_TMP_DIR=$T/tmp ZDC_CRON_DIR=$T/config ZDC_VAR_INI=$T/var.ini ZDC_NOTIFY=/bin/true
export ZDC_CONVERSION_JOURNAL=$T/config/conversion_in_progress
GO=$(command -v go || echo /usr/local/go/bin/go)
export ZDC_BIN=$T-bin/zfs-toolkit
mkdir -p "$(dirname "$ZDC_BIN")"
( cd "$ROOT" && "$GO" build -o "$ZDC_BIN" ./cmd/zfs-toolkit ) || exit 1

pkill -f "php -S 127.0.0.1:$PORT" 2>/dev/null
if [ "${1:-}" = stop ]; then
    for p in $PC $PB; do zpool destroy -f "$p" 2>/dev/null; done
    rm -rf "$T"
    exit 0
fi

if [ "${1:-}" != keep ]; then
    for p in $PC $PB; do zpool destroy -f "$p" 2>/dev/null; done
    rm -rf "$T"
    mkdir -p "$T/config" "$T/tmp"
    truncate -s 3G "$T/cache.img" "$T/backup.img"
    zpool create -f -m /mnt/$PC $PC "$T/cache.img"
    zpool create -f -m /mnt/$PB $PB "$T/backup.img"
    printf 'NAME="tower"\nmdState="STARTED"\nfsState="Started"\n' > "$T/var.ini"

    zfs create $PC/appdata
    zfs create $PC/domains
    zfs create "$PC/TV Shows"
    zfs create $PC/system
    for d in plex nextcloud "Home Assistant"; do
        zfs create "$PC/appdata/$d"
        mkdir -p "/mnt/$PC/appdata/$d/config"
        echo "settings for $d" > "/mnt/$PC/appdata/$d/config/settings.xml"
        head -c 2000000 /dev/urandom > "/mnt/$PC/appdata/$d/database.db"
    done
    for d in sonarr radarr "Büro (alt)"; do
        mkdir -p "/mnt/$PC/appdata/$d/logs"
        echo "$d" > "/mnt/$PC/appdata/$d/config.xml"
        head -c 500000 /dev/urandom > "/mnt/$PC/appdata/$d/logs/app.log"
    done
    mkdir -p /mnt/$PC/domains/Windows11 /mnt/$PC/domains/HomeAssistantOS
    head -c 3000000 /dev/urandom > /mnt/$PC/domains/Windows11/vdisk1.img
    mkdir -p "/mnt/$PC/TV Shows/Show A/Season 1"
    echo episode > "/mnt/$PC/TV Shows/Show A/Season 1/e01.mkv"
    zfs create $PB/replica

    stamp() { date -d "$1" '+%Y-%m-%d_%H-00'; }
    for h in 5 4 3 2 1; do
        echo "change $h" >> /mnt/$PC/appdata/plex/config/settings.xml
        zfs snapshot -r "$PC/appdata@auto-hourly-$(stamp "-$h hours")"
    done
    for d in 6 5 4 3 2 1; do
        echo "day $d" >> /mnt/$PC/appdata/nextcloud/config/settings.xml
        zfs snapshot -r "$PC/appdata@auto-daily-$(date -d "-$d days" '+%Y-%m-%d')"
    done
    zfs snapshot -r "$PC/appdata@auto-weekly-$(date -d '-8 days' '+%G-W%V')"
    zfs snapshot "$PC/appdata@before-upgrade"
    zfs hold keep "$PC/appdata@before-upgrade"
    zfs snapshot "$PC/TV Shows@auto-daily-$(date -d '-1 days' '+%Y-%m-%d')"

    cat > "$T/config/settings.cfg" <<CFG
dry_run=yes
cleanup=yes
replace_spaces=no
send_notifications=yes
show_in_menu=yes
should_process_containers=yes
appdata_pool=$PC
appdata_dataset=appdata
should_process_vms=yes
vm_pool=$PC
vm_dataset=domains
vm_forceshutdown_wait=90
buffer_zone=11
validation_tolerance=5
extra_datasets=
cron_enabled=no
cron_preset=daily
cron_hour=2
cron_minute=0
cron_weekday=0
cron_custom=0 2 * * *
snapshots_enabled=yes
snap_frequent=0
snap_hourly=24
snap_daily=30
snap_weekly=4
snap_monthly=3
snap_yearly=0
snap_daily_hour=2
snap_min_free_pct=5
snap_free_target=
snap_schedule_preset=15min
snap_schedule_custom=*/15 * * * *
send_enabled=yes
send_schedule_preset=daily
send_schedule_hour=4
send_schedule_custom=0 4 * * *
CFG
    cat > "$T/config/snap_datasets.json" <<JSON
{"datasets":[{"name":"$PC/appdata","recursive":true,"use_template":true,"hourly":"","daily":"","weekly":"","monthly":"","yearly":""},{"name":"$PC/TV Shows","recursive":false,"use_template":false,"hourly":"0","daily":"14","weekly":"4","monthly":"0","yearly":"0"}]}
JSON
    cat > "$T/config/send_jobs.json" <<JSON
{"jobs":[{"id":"demo1","enabled":true,"name":"Appdata to backup pool","source":"$PC/appdata","dest":"backup/replica/appdata","recursive":true,"transport":"local","ssh_host":"","ssh_port":22,"ssh_key":"","raw":false,"compressed":true,"allow_rollback":false,"keep_dest":0}]}
JSON
fi

cd "$ROOT"
nohup env ZDC_BIN=$ZDC_BIN ZDC_CONFIG_DIR=$ZDC_CONFIG_DIR ZDC_TMP_DIR=$ZDC_TMP_DIR ZDC_CRON_DIR=$ZDC_CRON_DIR \
    ZDC_VAR_INI=$ZDC_VAR_INI ZDC_NOTIFY=/bin/true ZDC_CONVERSION_JOURNAL=$ZDC_CONVERSION_JOURNAL \
    php -S 127.0.0.1:$PORT tests/harness/router.php > "$T/server.log" 2>&1 &
/bin/sleep 0.5
echo "harness on http://127.0.0.1:$PORT"
