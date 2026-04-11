#!/bin/bash
# setup_cron.sh - Install or remove the auto-conversion cron job
# Uses crontab directly (works with Unraid's busybox crond)

CONFIG="/boot/config/plugins/zfs.dataset.converter/settings.cfg"
RUNNER="/usr/local/emhttp/plugins/zfs.dataset.converter/scripts/run_auto.sh"
MARKER="# zfs-dataset-converter-auto"

mkdir -p "/tmp/zfs.dataset.converter"

# Read config
declare -A cfg
if [ -f "$CONFIG" ]; then
    while IFS='=' read -r key val; do
        [[ -z "$key" || "$key" =~ ^# ]] && continue
        cfg["${key}"]="${val}"
    done < "$CONFIG"
fi

ENABLED="${cfg[cron_enabled]:-no}"

# Always remove existing entry first (clean slate)
( crontab -l 2>/dev/null | grep -v "$MARKER" ) | crontab - 2>/dev/null

if [[ ! "$ENABLED" =~ ^[Yy]es$ ]]; then
    echo "Cron job removed."
    exit 0
fi

# Build cron expression
PRESET="${cfg[cron_preset]:-daily}"
HOUR="${cfg[cron_hour]:-2}"
MINUTE="${cfg[cron_minute]:-0}"
WEEKDAY="${cfg[cron_weekday]:-0}"
CUSTOM="${cfg[cron_custom]:-0 2 * * *}"

case "$PRESET" in
    hourly)   EXPR="${MINUTE} * * * *" ;;
    6hourly)  EXPR="${MINUTE} */6 * * *" ;;
    daily)    EXPR="${MINUTE} ${HOUR} * * *" ;;
    weekly)   EXPR="${MINUTE} ${HOUR} * * ${WEEKDAY}" ;;
    custom)   EXPR="${CUSTOM}" ;;
    *)        EXPR="0 2 * * *" ;;
esac

# Add to root crontab
(
    crontab -l 2>/dev/null
    echo "${MARKER}"
    echo "${EXPR} ${RUNNER}"
) | crontab -

echo "Cron job installed: ${EXPR}"
echo "Runner: ${RUNNER}"
echo "System time: $(date)"
