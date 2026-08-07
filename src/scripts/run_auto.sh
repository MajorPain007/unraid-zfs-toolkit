#!/bin/bash

PLUGIN_DIR="${ZDC_PLUGIN_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
. "${PLUGIN_DIR}/scripts/zdc_common.sh"

TEMPLATE="$PLUGIN_DIR/scripts/zfs_converter.sh"
STATUS_FILE="$ZDC_TMP_DIR/status.json"
CRON_LOG="$ZDC_TMP_DIR/cron_auto.log"

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"; }

zdc_trim_log "$CRON_LOG" 262144 500

if ! zdc_zfs_ready; then
    log "Skipped: ZFS not available or no pool imported (array not started?)."
    exit 0
fi

if [ -f "$STATUS_FILE" ]; then
    RUNNING_PID=$(grep -o '"pid":[0-9]*' "$STATUS_FILE" 2>/dev/null | grep -o '[0-9]*')
    if [ -n "$RUNNING_PID" ] && [ -d "/proc/$RUNNING_PID" ]; then
        log "Auto-conversion skipped: already running (PID $RUNNING_PID)"
        exit 0
    fi
fi

if [ ! -f "$ZDC_SETTINGS" ]; then
    log "Config not found: $ZDC_SETTINGS"
    exit 1
fi
if [ ! -f "$TEMPLATE" ]; then
    log "Template not found: $TEMPLATE"
    exit 1
fi

zdc_load_cfg "$ZDC_SETTINGS"

safe_val() {
    local key="$1" def="$2" val
    val=$(zdc_cfg "$key" "$def")
    case "$val" in
        *'|'*|*'"'*|*'`'*|*'$'*|*';'*|*'&'*|*$'\n'*|*$'\r'*|*'\'*)
            log "WARNING: ignoring unsafe value for ${key} ('${val}'), using default '${def}'."
            val="$def"
            ;;
    esac
    printf '%s' "$val"
}

safe_int() {
    local key="$1" def="$2" val
    val=$(zdc_cfg "$key" "$def")
    zdc_valid_int "$val" || val="$def"
    printf '%s' "$val"
}

EXTRA_DATASETS_STR=""
IFS=',' read -ra EXTRAS <<< "$(zdc_cfg extra_datasets '')"
for e in "${EXTRAS[@]}"; do
    e="${e#"${e%%[![:space:]]*}"}"; e="${e%"${e##*[![:space:]]}"}"
    [ -n "$e" ] || continue
    if zdc_valid_dataset "$e"; then
        EXTRA_DATASETS_STR+="\"$e\" "
    else
        log "WARNING: skipping invalid extra dataset '${e}'."
    fi
done

RUN_SCRIPT="$ZDC_TMP_DIR/zfs_converter_auto.sh"
LOG_FILE="$ZDC_TMP_DIR/auto_$(date '+%Y%m%d_%H%M%S').log"

sed \
    -e "s|__DRY_RUN__|$(safe_val dry_run yes)|g" \
    -e "s|__CLEANUP__|$(safe_val cleanup yes)|g" \
    -e "s|__REPLACE_SPACES__|$(safe_val replace_spaces no)|g" \
    -e "s|__PROCESS_CONTAINERS__|$(safe_val should_process_containers yes)|g" \
    -e "s|__APPDATA_POOL__|$(safe_val appdata_pool cache)|g" \
    -e "s|__APPDATA_DATASET__|$(safe_val appdata_dataset appdata)|g" \
    -e "s|__PROCESS_VMS__|$(safe_val should_process_vms no)|g" \
    -e "s|__VM_POOL__|$(safe_val vm_pool cache)|g" \
    -e "s|__VM_DATASET__|$(safe_val vm_dataset domains)|g" \
    -e "s|__VM_SHUTDOWN_WAIT__|$(safe_int vm_forceshutdown_wait 90)|g" \
    -e "s|__BUFFER_ZONE__|$(safe_int buffer_zone 11)|g" \
    -e "s|__SEND_NOTIFICATIONS__|$(safe_val send_notifications yes)|g" \
    -e "s|__VALIDATION_TOLERANCE__|$(safe_int validation_tolerance 5)|g" \
    -e "s|__EXTRA_DATASETS__|${EXTRA_DATASETS_STR}|g" \
    "$TEMPLATE" > "$RUN_SCRIPT"

if grep -q '__[A-Z_]*__' "$RUN_SCRIPT"; then
    log "ERROR: template substitution incomplete, aborting. Leftovers:"
    grep -o '__[A-Z_]*__' "$RUN_SCRIPT" | sort -u | while read -r ph; do log "  $ph"; done
    exit 1
fi

chmod +x "$RUN_SCRIPT"

zdc_prune_old_logs "$ZDC_TMP_DIR" 'auto_*.log' 10
zdc_prune_old_logs "$ZDC_TMP_DIR" 'conversion_*.log' 10

log "Starting auto-conversion (log: $LOG_FILE)"

nohup "$RUN_SCRIPT" > "$LOG_FILE" 2>&1 &
PID=$!

printf '{"pid":%d,"log_file":"%s","started":"%s","status":"running","source":"auto"}' \
    "$PID" "$LOG_FILE" "$(date '+%Y-%m-%d %H:%M:%S')" > "$STATUS_FILE"

log "Auto-conversion started (PID $PID)"
