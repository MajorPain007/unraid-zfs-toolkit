#!/bin/bash

PLUGIN_DIR="${ZDC_PLUGIN_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
. "${PLUGIN_DIR}/scripts/zdc_common.sh"

JOBS_JSON="${ZDC_CONFIG_DIR}/send_jobs.json"
LOG_FILE="${ZDC_TMP_DIR}/send.log"
STATUS_FILE="${ZDC_TMP_DIR}/send_status.json"

ONLY_JOB=""
DRY_RUN=0
TEST_ONLY=0

while [ $# -gt 0 ]; do
    case "$1" in
        --job)     ONLY_JOB="$2"; shift 2 ;;
        --test)    TEST_ONLY=1; ONLY_JOB="$2"; shift 2 ;;
        --dry-run) DRY_RUN=1; shift ;;
        *)         shift ;;
    esac
done

DRY_TAG=""
(( DRY_RUN )) && DRY_TAG="[dry-run] "

log()      { echo "[$(date '+%H:%M:%S')] ${DRY_TAG}$*"; }
log_ok()   { echo "[$(date '+%H:%M:%S')] ${DRY_TAG}OK: $*"; }
log_warn() { echo "[$(date '+%H:%M:%S')] ${DRY_TAG}WARNING: $*"; }
log_err()  { echo "[$(date '+%H:%M:%S')] ${DRY_TAG}ERROR: $*"; LAST_ERROR="$*"; (( ERRORS++ )); }

JOBS_RUN=0
JOBS_OK=0
ERRORS=0
LAST_ERROR=""
CURRENT_JOB=""
START_TS=$(date +%s)

zdc_lock "zfs_send" || { log_warn "Another replication run is still active - skipping."; exit 0; }

write_status() {
    printf '{"last_run":"%s","duration":%s,"jobs_run":%s,"jobs_ok":%s,"errors":%s,"current":"%s","last_error":"%s","dry_run":%s}\n' \
        "$(date '+%Y-%m-%d %H:%M:%S')" "$(( $(date +%s) - START_TS ))" \
        "$JOBS_RUN" "$JOBS_OK" "$ERRORS" \
        "$(printf '%s' "$CURRENT_JOB" | sed 's/[\\"]/\\&/g')" \
        "$(printf '%s' "$LAST_ERROR" | sed 's/[\\"]/\\&/g' | tr -d '\n\r')" \
        "$DRY_RUN" > "$STATUS_FILE"
}

finish() {
    CURRENT_JOB=""
    (( TEST_ONLY )) || write_status
    if (( ERRORS > 0 )) && (( ! DRY_RUN )) && (( ! TEST_ONLY )); then
        zdc_notify "ZFS replication - ${ERRORS} error(s)" \
            "Replication finished with ${ERRORS} error(s). Last: ${LAST_ERROR}" "warning"
    fi
    log "Replication finished (jobs=${JOBS_RUN}, ok=${JOBS_OK}, errors=${ERRORS})."
    zdc_trim_log "$LOG_FILE" 2097152 4000
}
trap finish EXIT

SSH_CMD=()

build_ssh() {
    local host="$1" port="$2" key="$3"
    SSH_CMD=(ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new
             -o ConnectTimeout=15 -o ServerAliveInterval=30 -p "$port")
    [ -n "$key" ] && SSH_CMD+=(-i "$key")
    SSH_CMD+=("$host")
}

# ssh hands the remote shell one command line, joined with spaces, and that
# shell splits it again - "tank/TV Shows" would arrive as two arguments. Quote
# each word for it. The names are validated to letters, digits, space and
# _ . : - / @, for which %q produces plain backslash escapes any sh accepts.
remote_cmd() {
    printf '%q ' "$@"
}

remote_zfs() {
    if [ "${#SSH_CMD[@]}" -gt 0 ]; then
        "${SSH_CMD[@]}" "$(remote_cmd zfs "$@")"
    else
        zfs "$@"
    fi
}

# Keep the newest $keep checkpoints of a job and destroy the rest, on one side.
# By name across the whole tree, not by destroying the parent's snapshot: a
# recursive job snapshots every child too, and destroying only the parent's
# left the children's copies behind - one more on each child with every run.
# The stamp in the name sorts in time order.
prune_checkpoints() {
    local side="$1" ds="$2" id="$3" keep="$4" recursive="$5"
    local list_args=(list -H -t snapshot -o name)
    [ "$recursive" = "1" ] && list_args+=(-r)

    local rows names old
    if [ "$side" = "remote" ]; then
        rows=$(remote_zfs "${list_args[@]}" "$ds" 2>/dev/null)
    else
        rows=$(zfs "${list_args[@]}" "$ds" 2>/dev/null)
    fi
    rows=$(printf '%s\n' "$rows" | grep -F "@zdc-send-${id}-")
    [ -n "$rows" ] || return 0

    names=$(printf '%s\n' "$rows" | sed 's/.*@//' | sort -u)
    old=$(printf '%s\n' "$names" | head -n "-${keep}")
    [ -n "$old" ] || return 0

    local s
    while IFS= read -r s; do
        [ -n "$s" ] || continue
        printf '%s\n' "$old" | grep -Fxq -- "${s#*@}" || continue
        if [ "$side" = "remote" ]; then
            remote_zfs destroy "$s" 2>/dev/null \
                && log "Pruned destination checkpoint ${s}" \
                || log_warn "Could not prune destination checkpoint ${s}"
        else
            zfs destroy "$s" 2>/dev/null \
                && log "Pruned old checkpoint ${s}" \
                || log_warn "Could not prune checkpoint ${s}"
        fi
    done <<< "$rows"
}

local_snap_names() {
    zfs list -H -t snapshot -o name -s creation "$1" 2>/dev/null | awk -F'@' 'NF==2 {print $2}'
}

remote_snap_names() {
    remote_zfs list -H -t snapshot -o name -s creation "$1" 2>/dev/null | awk -F'@' 'NF==2 {print $2}'
}

run_job() {
    local id="$1" enabled="$2" name="$3" source="$4" dest="$5" recursive="$6" \
          transport="$7" ssh_host="$8" ssh_port="$9" ssh_key="${10}" \
          raw="${11}" compressed="${12}" allow_rollback="${13}" keep_dest="${14}"

    CURRENT_JOB="$name"
    log "──────── Job '${name}' (${id}) ────────"

    if ! zdc_valid_dataset "$source"; then log_err "Job '${name}': invalid source '${source}'"; return 1; fi
    if ! zdc_valid_dataset "$dest";   then log_err "Job '${name}': invalid destination '${dest}'"; return 1; fi

    if [ "$transport" = "local" ]; then
        if [ "$source" = "$dest" ]; then
            log_err "Job '${name}': source and destination are the same dataset."; return 1
        fi
        case "$dest/" in
            "$source"/*) log_err "Job '${name}': destination '${dest}' is inside source '${source}'."; return 1 ;;
        esac
    fi

    if ! zfs list -H -o name "$source" >/dev/null 2>&1; then
        log_err "Job '${name}': source dataset '${source}' does not exist."; return 1
    fi

    SSH_CMD=()
    if [ "$transport" = "ssh" ]; then
        if [ -z "$ssh_host" ]; then log_err "Job '${name}': transport is ssh but no host configured."; return 1; fi
        if [ -n "$ssh_key" ] && [ ! -r "$ssh_key" ]; then
            log_err "Job '${name}': ssh key '${ssh_key}' is not readable."; return 1
        fi
        build_ssh "$ssh_host" "$ssh_port" "$ssh_key"
        if ! "${SSH_CMD[@]}" true 2>/dev/null; then
            log_err "Job '${name}': cannot reach ${ssh_host}:${ssh_port} with key based auth. Set up an SSH key first."
            return 1
        fi
        log "SSH to ${ssh_host}:${ssh_port} OK"
    fi

    local dest_exists=0
    if remote_zfs list -H -o name "$dest" >/dev/null 2>&1; then dest_exists=1; fi
    log "Destination '${dest}' $( ((dest_exists)) && echo exists || echo 'does not exist yet (full send)')"

    if (( TEST_ONLY )); then
        log_ok "Job '${name}': preconditions OK."
        return 0
    fi

    local base=""
    if (( dest_exists )); then
        local dest_snaps
        dest_snaps=$(remote_snap_names "$dest")
        if [ -n "$dest_snaps" ]; then
            base=$(local_snap_names "$source" | tac | while read -r s; do
                       if printf '%s\n' "$dest_snaps" | grep -Fxq "$s"; then echo "$s"; break; fi
                   done)
        fi
        if [ -z "$base" ]; then
            log_err "Job '${name}': destination exists but shares no snapshot with the source. Destroy '${dest}' for a fresh full send, or restore a common snapshot."
            return 1
        fi
        log "Incremental base: ${base}"
    fi

    local stamp checkpoint
    stamp=$(date '+%Y%m%d-%H%M%S')
    checkpoint="zdc-send-${id}-${stamp}"

    if (( DRY_RUN )); then
        log "Would create checkpoint ${source}@${checkpoint}"
    else
        local snapflags=()
        [ "$recursive" = "1" ] && snapflags+=("-r")
        local serr
        if ! serr=$(zfs snapshot "${snapflags[@]}" "${source}@${checkpoint}" 2>&1); then
            log_err "Job '${name}': cannot create checkpoint: ${serr}"
            return 1
        fi
        log_ok "Created checkpoint ${source}@${checkpoint}"
    fi

    local send_args=()
    [ "$recursive" = "1" ]  && send_args+=(-R)
    [ "$raw" = "1" ]        && send_args+=(-w)
    [ "$compressed" = "1" ] && send_args+=(-L -e -c)
    if [ -n "$base" ]; then
        send_args+=(-I "@${base}")
    fi
    send_args+=("${source}@${checkpoint}")

    # -u: do not mount what arrives. -x mountpoint: a source with its own
    # mountpoint - a pool's root dataset has one - would otherwise hand it to
    # the copy, and at the next import the copy mounts over the original.
    local recv_args=(-u -x mountpoint)
    [ "$allow_rollback" = "1" ] && recv_args+=(-F)
    recv_args+=("$dest")

    local est
    est=$(zfs send -nP "${send_args[@]}" 2>/dev/null | awk '/^size/{print $2}')
    [ -n "$est" ] && log "Estimated stream size: $(zdc_fmt_bytes "$est")"

    if (( DRY_RUN )); then
        log "Would run: zfs send ${send_args[*]} | zfs recv ${recv_args[*]}"
        log_ok "Job '${name}': dry run complete."
        return 0
    fi

    log "Sending..."
    local rc=0
    if [ "${#SSH_CMD[@]}" -gt 0 ]; then
        local remote_recv
        remote_recv=$(remote_cmd zfs recv "${recv_args[@]}")
        if command -v pv >/dev/null 2>&1 && [ -n "$est" ]; then
            zfs send "${send_args[@]}" \
                | pv -f -i 10 -b -t -r -e -s "$est" \
                | "${SSH_CMD[@]}" "$remote_recv"
        else
            zfs send -v "${send_args[@]}" | "${SSH_CMD[@]}" "$remote_recv"
        fi
    else
        if command -v pv >/dev/null 2>&1 && [ -n "$est" ]; then
            zfs send "${send_args[@]}" \
                | pv -f -i 10 -b -t -r -e -s "$est" \
                | zfs recv "${recv_args[@]}"
        else
            zfs send -v "${send_args[@]}" | zfs recv "${recv_args[@]}"
        fi
    fi
    local statuses=("${PIPESTATUS[@]}")
    for s in "${statuses[@]}"; do [ "$s" != "0" ] && rc=1; done

    if (( rc != 0 )); then
        log_err "Job '${name}': transfer failed (exit codes: ${statuses[*]})."
        log_err "The checkpoint ${source}@${checkpoint} was kept so the next run can retry from the same base."
        return 1
    fi

    log_ok "Job '${name}': replicated ${source} -> ${dest}"

    # The newest two stay: the one just sent is the base of the next run, the
    # one before it covers a run that fails halfway.
    prune_checkpoints local "$source" "$id" 2 "$recursive"
    if zdc_valid_int "$keep_dest" && (( keep_dest > 0 )); then
        prune_checkpoints remote "$dest" "$id" "$keep_dest" "$recursive"
    fi

    return 0
}

log "ZFS replication starting..."

if ! zdc_zfs_ready; then
    log_warn "ZFS is not available or no pool is imported (array not started?). Nothing to do."
    exit 0
fi

if [ ! -f "$JOBS_JSON" ]; then
    log_warn "No replication jobs configured (${JOBS_JSON})."
    exit 0
fi

if ! command -v php >/dev/null 2>&1; then
    log_err "php is required to read the job configuration but was not found."
    exit 1
fi

# \x1f, not a tab - see send_jobs_tsv.php for why.
while IFS=$'\x1f' read -r id enabled name source dest recursive transport \
                          ssh_host ssh_port ssh_key raw compressed allow_rollback keep_dest; do
    [ -n "$id" ] || continue

    if [ -n "$ONLY_JOB" ]; then
        [ "$id" = "$ONLY_JOB" ] || continue
    elif [ "$enabled" != "1" ]; then
        log "Skipping disabled job '${name}' (${id})."
        continue
    fi

    (( JOBS_RUN++ ))
    write_status
    if run_job "$id" "$enabled" "$name" "$source" "$dest" "$recursive" "$transport" \
               "$ssh_host" "$ssh_port" "$ssh_key" "$raw" "$compressed" "$allow_rollback" "$keep_dest"; then
        (( JOBS_OK++ ))
    fi
    write_status
done < <(php "${PLUGIN_DIR}/scripts/send_jobs_tsv.php" 2>/dev/null)

if (( JOBS_RUN == 0 )); then
    if [ -n "$ONLY_JOB" ]; then
        log_err "No job with id '${ONLY_JOB}' found."
    else
        log_warn "No enabled replication jobs."
    fi
fi
