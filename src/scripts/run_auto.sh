#!/bin/bash
# run_auto.sh - Called by cron to auto-convert folders to ZFS datasets
# Reads settings from /boot/config, skips if conversion already running

PLUGIN_DIR="/usr/local/emhttp/plugins/zfs.dataset.converter"
CONFIG="/boot/config/plugins/zfs.dataset.converter/settings.cfg"
TMPDIR="/tmp/zfs.dataset.converter"
TEMPLATE="$PLUGIN_DIR/scripts/zfs_converter.sh"
STATUS_FILE="$TMPDIR/status.json"

mkdir -p "$TMPDIR"

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"; }

# Check if a conversion is already running
if [ -f "$STATUS_FILE" ]; then
    RUNNING_PID=$(grep -o '"pid":[0-9]*' "$STATUS_FILE" 2>/dev/null | grep -o '[0-9]*')
    if [ -n "$RUNNING_PID" ] && [ -d "/proc/$RUNNING_PID" ]; then
        log "Auto-conversion skipped: already running (PID $RUNNING_PID)"
        exit 0
    fi
fi

# Read settings
declare -A cfg
if [ ! -f "$CONFIG" ]; then
    log "Config not found: $CONFIG"
    exit 1
fi
while IFS='=' read -r key val; do
    [[ -z "$key" || "$key" =~ ^# ]] && continue
    cfg["$key"]="$val"
done < "$CONFIG"

if [ ! -f "$TEMPLATE" ]; then
    log "Template not found: $TEMPLATE"
    exit 1
fi

# Build extra datasets string
EXTRA_DATASETS_STR=""
IFS=',' read -ra EXTRAS <<< "${cfg[extra_datasets]:-}"
for e in "${EXTRAS[@]}"; do
    e=$(echo "$e" | xargs 2>/dev/null)
    [ -n "$e" ] && EXTRA_DATASETS_STR+="\"$e\" "
done

# Generate run script from template
RUN_SCRIPT="$TMPDIR/zfs_converter_auto.sh"
LOG_FILE="$TMPDIR/auto_$(date '+%Y%m%d_%H%M%S').log"

sed \
    -e "s|__DRY_RUN__|${cfg[dry_run]:-yes}|g" \
    -e "s|__CLEANUP__|${cfg[cleanup]:-yes}|g" \
    -e "s|__REPLACE_SPACES__|${cfg[replace_spaces]:-no}|g" \
    -e "s|__PROCESS_CONTAINERS__|${cfg[should_process_containers]:-yes}|g" \
    -e "s|__APPDATA_POOL__|${cfg[appdata_pool]:-cache}|g" \
    -e "s|__APPDATA_DATASET__|${cfg[appdata_dataset]:-appdata}|g" \
    -e "s|__PROCESS_VMS__|${cfg[should_process_vms]:-no}|g" \
    -e "s|__VM_POOL__|${cfg[vm_pool]:-cache}|g" \
    -e "s|__VM_DATASET__|${cfg[vm_dataset]:-domains}|g" \
    -e "s|__VM_SHUTDOWN_WAIT__|${cfg[vm_forceshutdown_wait]:-90}|g" \
    -e "s|__BUFFER_ZONE__|${cfg[buffer_zone]:-11}|g" \
    -e "s|__SEND_NOTIFICATIONS__|${cfg[send_notifications]:-yes}|g" \
    -e "s|__VALIDATION_TOLERANCE__|${cfg[validation_tolerance]:-5}|g" \
    -e "s|__EXTRA_DATASETS__|${EXTRA_DATASETS_STR}|g" \
    "$TEMPLATE" > "$RUN_SCRIPT"

chmod +x "$RUN_SCRIPT"

log "Starting auto-conversion (log: $LOG_FILE)"

# Launch in background, store PID
nohup "$RUN_SCRIPT" > "$LOG_FILE" 2>&1 &
PID=$!

# Write status so GUI can track it
printf '{"pid":%d,"log_file":"%s","started":"%s","status":"running","source":"auto"}' \
    "$PID" "$LOG_FILE" "$(date '+%Y-%m-%d %H:%M:%S')" > "$STATUS_FILE"

log "Auto-conversion started (PID $PID)"
