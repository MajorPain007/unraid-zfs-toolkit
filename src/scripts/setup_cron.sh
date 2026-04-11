#!/bin/bash
# setup_cron.sh - Write or remove the cron job based on saved settings
# Called from: save_settings.php, plugin install, rc.d on boot

CONFIG="/boot/config/plugins/zfs.dataset.converter/settings.cfg"
CRON_FILE="/etc/cron.d/zfs.dataset.converter"
RUNNER="/usr/local/emhttp/plugins/zfs.dataset.converter/scripts/run_auto.sh"
LOG_DIR="/tmp/zfs.dataset.converter"

mkdir -p "$LOG_DIR"

# Read config
declare -A cfg
if [ -f "$CONFIG" ]; then
    while IFS='=' read -r key val; do
        [[ -z "$key" || "$key" =~ ^# ]] && continue
        cfg["$key"]="$val"
    done < "$CONFIG"
fi

ENABLED="${cfg[cron_enabled]:-no}"

if [[ ! "$ENABLED" =~ ^[Yy]es$ ]]; then
    if [ -f "$CRON_FILE" ]; then
        rm -f "$CRON_FILE"
        echo "Cron job removed."
    else
        echo "Cron job not active."
    fi
    exit 0
fi

# Build cron expression
PRESET="${cfg[cron_preset]:-daily}"
HOUR="${cfg[cron_hour]:-2}"
MINUTE="${cfg[cron_minute]:-0}"
WEEKDAY="${cfg[cron_weekday]:-0}"
CUSTOM="${cfg[cron_custom]:-0 2 * * *}"

case "$PRESET" in
    hourly)   EXPR="$MINUTE * * * *" ;;
    6hourly)  EXPR="$MINUTE */6 * * *" ;;
    daily)    EXPR="$MINUTE $HOUR * * *" ;;
    weekly)   EXPR="$MINUTE $HOUR * * $WEEKDAY" ;;
    custom)   EXPR="$CUSTOM" ;;
    *)        EXPR="0 2 * * *" ;;
esac

# Write cron file (cron.d format: expression user command)
cat > "$CRON_FILE" << EOF
# ZFS Dataset Converter - auto conversion (managed by plugin)
$EXPR root $RUNNER >> $LOG_DIR/auto.log 2>&1
EOF

# Reload crond
if [ -f /var/run/crond.pid ]; then
    kill -HUP "$(cat /var/run/crond.pid)" 2>/dev/null
fi

echo "Cron job set: $EXPR"
