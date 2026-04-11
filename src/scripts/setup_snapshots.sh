#!/bin/bash
# setup_snapshots.sh - Install or remove the snapshot cron job
# Called: after saving snapshot settings, and on every boot via rc.d

CONFIG="/boot/config/plugins/zfs.dataset.converter/settings.cfg"
RUNNER="/usr/local/emhttp/plugins/zfs.dataset.converter/scripts/snapshot_manager.sh"
MARKER="# zfs-dataset-converter-snapshots"

mkdir -p "/tmp/zfs.dataset.converter"

# Read config
declare -A cfg
if [ -f "$CONFIG" ]; then
    while IFS='=' read -r key val; do
        [[ -z "$key" || "$key" =~ ^# ]] && continue
        cfg["${key}"]="${val}"
    done < "$CONFIG"
fi

ENABLED="${cfg[snapshots_enabled]:-no}"

# Always remove existing entries first (comment + runner line)
( crontab -l 2>/dev/null \
    | grep -v "$MARKER" \
    | grep -v "snapshot_manager\.sh" \
) | crontab - 2>/dev/null

if [[ ! "$ENABLED" =~ ^[Yy]es$ ]]; then
    echo "Snapshot cron job removed."
    exit 0
fi

# Build cron expression
PRESET="${cfg[snap_schedule_preset]:-15min}"
CUSTOM="${cfg[snap_schedule_custom]:-*/15 * * * *}"

case "$PRESET" in
    15min)  EXPR="*/15 * * * *" ;;
    30min)  EXPR="*/30 * * * *" ;;
    hourly) EXPR="0 * * * *"    ;;
    custom) EXPR="${CUSTOM}"     ;;
    *)      EXPR="*/15 * * * *" ;;
esac

# Add to crontab
(
    crontab -l 2>/dev/null
    echo "${MARKER}"
    echo "${EXPR} /bin/bash ${RUNNER} >> /tmp/zfs.dataset.converter/snapshots.log 2>&1"
) | crontab -

echo "Snapshot cron installed: ${EXPR}"
echo "Runner: ${RUNNER}"
echo "System time: $(date)"
