#!/bin/bash

PLUGIN_DIR="${ZDC_PLUGIN_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
. "${PLUGIN_DIR}/scripts/zdc_common.sh"

RUNNER="${PLUGIN_DIR}/scripts/snapshot_manager.sh"
CRON_FILE_BASE="zfs.dataset.converter-snapshots"
PATTERN="snapshot_manager\.sh"

zdc_load_cfg "$ZDC_SETTINGS"

ENABLED=$(zdc_cfg snapshots_enabled no)

if [[ ! "$ENABLED" =~ ^[Yy]es$ ]]; then
    zdc_remove_cron "$CRON_FILE_BASE" "$PATTERN"
    echo "Snapshot cron job removed."
    exit 0
fi

PRESET=$(zdc_cfg snap_schedule_preset 15min)
CUSTOM=$(zdc_cfg snap_schedule_custom "*/15 * * * *")

case "$PRESET" in
    5min)   EXPR="*/5 * * * *"  ;;
    15min)  EXPR="*/15 * * * *" ;;
    30min)  EXPR="*/30 * * * *" ;;
    hourly) EXPR="0 * * * *"    ;;
    custom) EXPR="${CUSTOM}"    ;;
    *)      EXPR="*/15 * * * *" ;;
esac

if ! zdc_valid_cron "$EXPR"; then
    echo "ERROR: invalid cron expression '${EXPR}' (preset=${PRESET})." >&2
    echo "Snapshot cron NOT installed - the previous schedule was left untouched." >&2
    exit 1
fi

if ! zdc_install_cron "$CRON_FILE_BASE" "$EXPR" \
        "/bin/bash ${RUNNER} >> ${ZDC_TMP_DIR}/snapshots.log 2>&1" "$PATTERN"; then
    exit 1
fi

echo "Runner: ${RUNNER}"
echo "System time: $(date)"
