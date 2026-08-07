#!/bin/bash

PLUGIN_DIR="${ZDC_PLUGIN_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
. "${PLUGIN_DIR}/scripts/zdc_common.sh"

RUNNER="${PLUGIN_DIR}/scripts/zfs_send.sh"
CRON_FILE_BASE="zfs.dataset.converter-send"
PATTERN="zfs_send\.sh"

zdc_load_cfg "$ZDC_SETTINGS"

ENABLED=$(zdc_cfg send_enabled no)

if [[ ! "$ENABLED" =~ ^[Yy]es$ ]]; then
    zdc_remove_cron "$CRON_FILE_BASE" "$PATTERN"
    echo "Replication cron job removed."
    exit 0
fi

PRESET=$(zdc_cfg send_schedule_preset daily)
HOUR=$(zdc_cfg send_schedule_hour 4)
CUSTOM=$(zdc_cfg send_schedule_custom "0 4 * * *")

zdc_valid_int "$HOUR" && (( HOUR >= 0 && HOUR <= 23 )) || HOUR=4

case "$PRESET" in
    hourly)  EXPR="15 * * * *" ;;
    6hourly) EXPR="15 */6 * * *" ;;
    daily)   EXPR="0 ${HOUR} * * *" ;;
    weekly)  EXPR="0 ${HOUR} * * 0" ;;
    custom)  EXPR="${CUSTOM}" ;;
    *)       EXPR="0 4 * * *" ;;
esac

if ! zdc_valid_cron "$EXPR"; then
    echo "ERROR: invalid cron expression '${EXPR}' (preset=${PRESET})." >&2
    echo "Replication cron NOT installed - the previous schedule was left untouched." >&2
    exit 1
fi

if ! zdc_install_cron "$CRON_FILE_BASE" "$EXPR" \
        "/bin/bash ${RUNNER} >> ${ZDC_TMP_DIR}/send.log 2>&1" "$PATTERN"; then
    exit 1
fi

echo "Runner: ${RUNNER}"
echo "System time: $(date)"
