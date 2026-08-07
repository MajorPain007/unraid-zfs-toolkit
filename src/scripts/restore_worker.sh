#!/bin/bash

PLUGIN_DIR="${ZDC_PLUGIN_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
. "${PLUGIN_DIR}/scripts/zdc_common.sh"

JOB="$1"
[ -n "$JOB" ] && [ -f "$JOB" ] || { echo "usage: restore_worker.sh <jobfile>" >&2; exit 2; }

STATUS="${JOB%.job}.status"
LOG="${JOB%.job}.log"

RSYNC_OPTS=(-a -H -A -X --numeric-ids --info=progress2 --no-inc-recursive)

TOTAL=$(grep -c . "$JOB")
DONE=0
FAILED=0
CURRENT=""
STATE="running"
MESSAGE=""

json_escape() { printf '%s' "$1" | sed 's/[\\"]/\\&/g' | tr -d '\n\r\t'; }

write_status() {
    printf '{"state":"%s","total":%s,"done":%s,"failed":%s,"current":"%s","message":"%s","updated":"%s"}\n' \
        "$STATE" "$TOTAL" "$DONE" "$FAILED" \
        "$(json_escape "$CURRENT")" "$(json_escape "$MESSAGE")" \
        "$(date '+%Y-%m-%d %H:%M:%S')" > "$STATUS"
}

finish() {
    if [ "$STATE" = "running" ]; then
        if (( FAILED > 0 )); then
            STATE="error"
            MESSAGE="${DONE} restored, ${FAILED} failed - see the log"
        else
            STATE="done"
            MESSAGE="${DONE} item(s) restored"
        fi
    fi
    write_status
    echo "[$(date '+%H:%M:%S')] Finished: ${STATE} (${DONE} ok, ${FAILED} failed)" >> "$LOG"
    zdc_prune_old_logs "$ZDC_TMP_DIR" 'restore_*.log'    10
    zdc_prune_old_logs "$ZDC_TMP_DIR" 'restore_*.status' 10
    zdc_prune_old_logs "$ZDC_TMP_DIR" 'restore_*.job'    10
}
trap finish EXIT
trap 'STATE="cancelled"; MESSAGE="Cancelled"; exit 130' INT TERM HUP

: > "$LOG"
write_status

while IFS=$'\t' read -r src dst mode; do
    [ -n "$src" ] || continue
    CURRENT="$(basename "$src")"
    write_status

    echo "[$(date '+%H:%M:%S')] ${mode}: ${src} -> ${dst}" >> "$LOG"

    if [ ! -e "$src" ]; then
        echo "  ERROR: source does not exist" >> "$LOG"
        (( FAILED++ )); write_status; continue
    fi

    if ! mkdir -p "$dst" 2>>"$LOG"; then
        echo "  ERROR: cannot create destination ${dst}" >> "$LOG"
        (( FAILED++ )); write_status; continue
    fi

    if [ "$mode" = "dir" ]; then
        target="${dst%/}/$(basename "$src")"
        if ! mkdir -p "$target" 2>>"$LOG"; then
            echo "  ERROR: cannot create ${target}" >> "$LOG"
            (( FAILED++ )); write_status; continue
        fi
        if rsync "${RSYNC_OPTS[@]}" "${src}/" "${target}/" >> "$LOG" 2>&1; then
            (( DONE++ ))
        else
            echo "  ERROR: rsync exited $?" >> "$LOG"
            (( FAILED++ ))
        fi
    else
        if rsync "${RSYNC_OPTS[@]}" "$src" "${dst%/}/" >> "$LOG" 2>&1; then
            (( DONE++ ))
        else
            echo "  ERROR: rsync exited $?" >> "$LOG"
            (( FAILED++ ))
        fi
    fi

    write_status
done < "$JOB"
