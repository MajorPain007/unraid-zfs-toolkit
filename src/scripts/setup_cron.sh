#!/bin/bash

PLUGIN_DIR="${ZDC_PLUGIN_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
. "${PLUGIN_DIR}/scripts/zdc_common.sh"

RUNNER="${PLUGIN_DIR}/scripts/run_auto.sh"
CRON_FILE_BASE="zfs.dataset.converter"
PATTERN="run_auto\.sh"

zdc_load_cfg "$ZDC_SETTINGS"

ENABLED=$(zdc_cfg cron_enabled no)

if [[ ! "$ENABLED" =~ ^[Yy]es$ ]]; then
    zdc_remove_cron "$CRON_FILE_BASE" "$PATTERN"
    echo "Conversion cron job removed."
    exit 0
fi

PRESET=$(zdc_cfg cron_preset daily)
HOUR=$(zdc_cfg cron_hour 2)
MINUTE=$(zdc_cfg cron_minute 0)
WEEKDAY=$(zdc_cfg cron_weekday 0)
CUSTOM=$(zdc_cfg cron_custom "0 2 * * *")

zdc_valid_int "$HOUR"    && (( HOUR    >= 0 && HOUR    <= 23 )) || HOUR=2
zdc_valid_int "$MINUTE"  && (( MINUTE  >= 0 && MINUTE  <= 59 )) || MINUTE=0
zdc_valid_int "$WEEKDAY" && (( WEEKDAY >= 0 && WEEKDAY <= 7  )) || WEEKDAY=0

case "$PRESET" in
    hourly)   EXPR="${MINUTE} * * * *" ;;
    6hourly)  EXPR="${MINUTE} */6 * * *" ;;
    daily)    EXPR="${MINUTE} ${HOUR} * * *" ;;
    weekly)   EXPR="${MINUTE} ${HOUR} * * ${WEEKDAY}" ;;
    custom)   EXPR="${CUSTOM}" ;;
    *)        EXPR="0 2 * * *" ;;
esac

if ! zdc_valid_cron "$EXPR"; then
    echo "ERROR: invalid cron expression '${EXPR}' (preset=${PRESET})." >&2
    echo "Cron job NOT installed - the previous schedule was left untouched." >&2
    exit 1
fi

if ! zdc_install_cron "$CRON_FILE_BASE" "$EXPR" \
        "/bin/bash ${RUNNER} >> ${ZDC_TMP_DIR}/cron_auto.log 2>&1" "$PATTERN"; then
    exit 1
fi

echo "Runner: ${RUNNER}"
echo "System time: $(date)"
