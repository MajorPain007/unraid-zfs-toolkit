#!/bin/bash

PLUGIN_DIR="${ZDC_PLUGIN_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
. "${PLUGIN_DIR}/scripts/zdc_common.sh"

DATASETS_JSON="${ZDC_CONFIG_DIR}/snap_datasets.json"
LOG_FILE="${ZDC_TMP_DIR}/snapshots.log"
LAST_FILE="${ZDC_TMP_DIR}/snapshot_last.txt"
STATUS_FILE="${ZDC_TMP_DIR}/snapshot_status.json"

FORCE_NOW=0
FORCE_ALL=0
DRY_RUN=0
for arg in "$@"; do
    case "$arg" in
        --now)     FORCE_NOW=1 ;;
        --all)     FORCE_NOW=1; FORCE_ALL=1 ;;
        --dry-run) DRY_RUN=1 ;;
    esac
done

DRY_TAG=""
(( DRY_RUN )) && DRY_TAG="[dry-run] "

log()      { echo "[$(date '+%H:%M:%S')] ${DRY_TAG}$*"; }
log_ok()   { echo "[$(date '+%H:%M:%S')] ${DRY_TAG}OK: $*"; }
log_warn() { echo "[$(date '+%H:%M:%S')] ${DRY_TAG}WARNING: $*"; }
log_err()  { echo "[$(date '+%H:%M:%S')] ${DRY_TAG}ERROR: $*"; LAST_ERROR="$*"; (( ERRORS++ )); }

CREATED=0
PRUNED=0
FREED=0
ERRORS=0
LAST_ERROR=""
START_TS=$(date +%s)

if (( DRY_RUN )); then
    zdc_lock "snapshot_manager_dryrun" || { log_warn "Another dry run is active."; exit 0; }
else
    zdc_lock "snapshot_manager" || { log_warn "Another snapshot run is still active - skipping this cycle."; exit 0; }
fi

write_status() {
    local now
    now=$(date '+%Y-%m-%d %H:%M:%S')
    printf '{"last_run":"%s","duration":%s,"datasets":%s,"created":%s,"pruned":%s,"freed":%s,"errors":%s,"last_error":"%s","forced":%s,"dry_run":%s}\n' \
        "$now" "$(( $(date +%s) - START_TS ))" "${DS_COUNT:-0}" "$CREATED" "$PRUNED" "$FREED" "$ERRORS" \
        "$(printf '%s' "$LAST_ERROR" | sed 's/[\\"]/\\&/g' | tr -d '\n\r')" "$FORCE_NOW" "$DRY_RUN" \
        > "$STATUS_FILE"
    echo "$now" > "$LAST_FILE"
}

finish() {
    if (( ! DRY_RUN )); then
        write_status
        if (( ERRORS > 0 )); then
            zdc_notify "ZFS Snapshots - ${ERRORS} error(s)" \
                "Snapshot run finished with ${ERRORS} error(s). Last: ${LAST_ERROR}" "warning"
        fi
    fi
    log "Snapshot Manager finished (created=${CREATED}, pruned=${PRUNED}, freed=$(zdc_fmt_bytes "$FREED"), errors=${ERRORS})."
    zdc_trim_log "$LOG_FILE" 1048576 2000
}
trap finish EXIT

zdc_load_cfg "$ZDC_SETTINGS"

SNAP_FREQUENT=$(zdc_cfg snap_frequent 0)
SNAP_HOURLY=$(zdc_cfg snap_hourly 24)
SNAP_DAILY=$(zdc_cfg snap_daily 30)
SNAP_WEEKLY=$(zdc_cfg snap_weekly 4)
SNAP_MONTHLY=$(zdc_cfg snap_monthly 3)
SNAP_YEARLY=$(zdc_cfg snap_yearly 0)
SNAP_DAILY_HOUR=$(zdc_cfg snap_daily_hour 2)
SNAP_MIN_FREE_PCT=$(zdc_cfg snap_min_free_pct 5)
SNAP_FREE_TARGET=$(zdc_cfg snap_free_target '')

AGE_FREQUENT=$(zdc_cfg snap_age_frequent 0)
AGE_HOURLY=$(zdc_cfg snap_age_hourly 0)
AGE_DAILY=$(zdc_cfg snap_age_daily 0)
AGE_WEEKLY=$(zdc_cfg snap_age_weekly 0)
AGE_MONTHLY=$(zdc_cfg snap_age_monthly 0)
AGE_YEARLY=$(zdc_cfg snap_age_yearly 0)

for v in SNAP_FREQUENT SNAP_HOURLY SNAP_DAILY SNAP_WEEKLY SNAP_MONTHLY SNAP_YEARLY \
         AGE_FREQUENT AGE_HOURLY AGE_DAILY AGE_WEEKLY AGE_MONTHLY AGE_YEARLY; do
    zdc_valid_int "${!v}" || { log_warn "Invalid value for ${v}='${!v}', using 0."; printf -v "$v" '%s' 0; }
done
zdc_valid_int "$SNAP_DAILY_HOUR" && (( SNAP_DAILY_HOUR >= 0 && SNAP_DAILY_HOUR <= 23 )) || SNAP_DAILY_HOUR=2
zdc_valid_int "$SNAP_MIN_FREE_PCT" && (( SNAP_MIN_FREE_PCT >= 0 && SNAP_MIN_FREE_PCT <= 90 )) || SNAP_MIN_FREE_PCT=5

read_datasets() {
    perl -e '
use strict; use warnings;
local $/;
open(my $fh, "<", $ARGV[0]) or exit 0;
my $json = <$fh>;
close $fh;

sub unescape {
    my ($s) = @_;
    $s =~ s/\\u([0-9a-fA-F]{4})/chr(hex($1))/ge;
    $s =~ s/\\n/\n/g; $s =~ s/\\t/\t/g; $s =~ s/\\r//g;
    $s =~ s/\\(["\/\\])/$1/g;   # \" -> "   \/ -> /   \\ -> \
    return $s;
}

while ($json =~ /\{([^{}]*)\}/g) {
    my $block = $1;
    my %ds;
    while ($block =~ /"(\w+)"\s*:\s*(?:"((?:[^"\\]|\\.)*)"|(true|false|-?\d+))/g) {
        my $val = defined($2) ? unescape($2) : $3;
        $ds{$1} = $val;
    }
    next unless defined $ds{name} && length $ds{name};
    my $rec = (defined($ds{recursive}) && $ds{recursive} eq "true") ? "1" : "0";
    my $tpl = (!defined($ds{use_template}) || $ds{use_template} eq "true") ? "1" : "0";
    my @r = map { my $v = $ds{$_}; (defined($v) && $v ne "") ? $v : "t" }
            qw(hourly daily weekly monthly yearly);
    @r = ("t") x 5 if $tpl eq "1";
    print join("\t", $ds{name}, $rec, @r), "\n";
}
' "$DATASETS_JSON" 2>/dev/null
}

resolve() {
    local val="$1" global="$2"
    if [ "$val" = "t" ] || ! zdc_valid_int "$val"; then echo "$global"; else echo "$val"; fi
}

SNAP_ROWS=""

load_snaps() {
    SNAP_ROWS=$(zfs list -Hp -t snapshot -o name,creation,used,userrefs -s creation "$1" 2>/dev/null)
}

snap_exists() {
    [ -n "$SNAP_ROWS" ] && printf '%s\n' "$SNAP_ROWS" \
        | awk -F'\t' -v n="$1" '$1==n {found=1} END{exit !found}'
}

snap_rows_drop() {
    SNAP_ROWS=$(printf '%s\n' "$SNAP_ROWS" | awk -F'\t' -v n="$1" '$1!=n')
}

destroy_snapshot() {
    local snap="$1" recursive="$2" used="${3:-0}" derr
    local flags=()
    [ "$recursive" = "1" ] && flags+=("-r")

    if (( DRY_RUN )); then
        log "Would prune ${snap} ($(zdc_fmt_bytes "$used"))"
        (( PRUNED++ ))
        zdc_valid_int "$used" && (( FREED += used ))
        snap_rows_drop "$snap"
        return 0
    fi

    if derr=$(zfs destroy "${flags[@]}" "$snap" 2>&1); then
        log "Pruned ${snap} ($(zdc_fmt_bytes "$used"))"
        (( PRUNED++ ))
        zdc_valid_int "$used" && (( FREED += used ))
        snap_rows_drop "$snap"
        return 0
    fi
    log_warn "Could not prune ${snap}: ${derr}"
    return 1
}

create_snapshot() {
    local dataset="$1" type="$2" key="$3" recursive="$4"
    local snap_name="${dataset}@auto-${type}-${key}"

    snap_exists "$snap_name" && return 0   # already taken for this period

    if (( DRY_RUN )); then
        log "Would create ${snap_name}"
        (( CREATED++ ))
        SNAP_ROWS="${SNAP_ROWS}"$'\n'"${snap_name}"$'\t'"$(date +%s)"$'\t0\t0'
        return 0
    fi

    local flags=()
    [ "$recursive" = "1" ] && flags+=("-r")

    local out
    if out=$(zfs snapshot "${flags[@]}" "${snap_name}" 2>&1); then
        log_ok "Created ${snap_name}"
        SNAP_ROWS="${SNAP_ROWS}"$'\n'"${snap_name}"$'\t'"$(date +%s)"$'\t0\t0'
        (( CREATED++ ))
    else
        log_err "Failed to create ${snap_name}: ${out:-no error output from zfs}"
    fi
}

prune_snapshots() {
    local dataset="$1" type="$2" keep="$3" recursive="$4" max_age_days="${5:-0}"

    zdc_valid_int "$keep" || keep=0
    zdc_valid_int "$max_age_days" || max_age_days=0
    (( keep == 0 && max_age_days == 0 )) && return 0

    local rows=()
    mapfile -t rows < <(printf '%s\n' "$SNAP_ROWS" \
        | awk -F'\t' -v p="auto-${type}-" '
            { split($1, a, "@"); if (a[2] != "" && index(a[2], p) == 1) print }')

    local total="${#rows[@]}"
    (( total == 0 )) && return 0

    local now cutoff=0
    now=$(date +%s)
    (( max_age_days > 0 )) && cutoff=$(( now - max_age_days * 86400 ))

    local i sname screat sused srefs delete
    for (( i=0; i<total; i++ )); do
        [ -n "${rows[$i]}" ] || continue
        IFS=$'\t' read -r sname screat sused srefs <<< "${rows[$i]}"
        delete=0

        (( keep > 0 && i < total - keep )) && delete=1

        if (( cutoff > 0 )) && zdc_valid_int "$screat" && (( screat < cutoff )); then
            delete=1
        fi

        (( keep > 0 && i == total - 1 )) && delete=0

        (( delete )) || continue

        if [ -n "$srefs" ] && [ "$srefs" != "-" ] && zdc_valid_int "$srefs" && (( srefs > 0 )); then
            log_warn "Keeping held snapshot ${sname} (userrefs=${srefs}) - release the hold to allow pruning."
            continue
        fi

        destroy_snapshot "$sname" "$recursive" "$sused"
    done
}

free_space_prune() {
    local pool="$1"

    [ -n "$SNAP_FREE_TARGET" ] || return 0

    local size free target
    size=$(zdc_pool_size_bytes "$pool")
    free=$(zdc_pool_free_bytes "$pool")
    zdc_valid_int "$free" || { log_warn "Cannot read free space for pool '${pool}'."; return 0; }

    target=$(zdc_to_bytes "$SNAP_FREE_TARGET" "${size:-0}") || {
        log_warn "Cannot parse snap_free_target='${SNAP_FREE_TARGET}' (use e.g. 100G or 10%)."
        return 0
    }
    (( target > 0 )) || return 0

    if (( free >= target )); then
        return 0
    fi

    log_warn "Pool '${pool}': $(zdc_fmt_bytes "$free") free, target is $(zdc_fmt_bytes "$target") - pruning oldest auto snapshots."

    local candidates=()
    mapfile -t candidates < <(
        zfs list -Hp -t snapshot -o name,creation,used,userrefs -s creation -r "$pool" 2>/dev/null \
        | awk -F'\t' '
            {
                split($1, a, "@");
                ds = a[1]; sn = a[2];
                if (sn !~ /^auto-/) next;
                rest = substr(sn, 6);
                p = index(rest, "-");
                if (p == 0) next;
                key = ds "@" substr(rest, 1, p-1);
                n++;
                line[n] = $0;
                grp[n]  = key;
                held[n] = ($4 != "-" && $4 + 0 > 0);
                newest[key] = n;       # rows are creation-ascending
            }
            END {
                for (i = 1; i <= n; i++) {
                    if (newest[grp[i]] == i) continue;
                    if (held[i]) continue;
                    print line[i];
                }
            }')

    if (( ${#candidates[@]} == 0 )); then
        log_warn "Pool '${pool}': no prunable auto snapshots left, still below target."
        zdc_notify "ZFS pool ${pool} low on space" \
            "Free space is below the configured target and no further auto snapshots can be pruned." "alert"
        return 0
    fi

    local deleted=0 row sname screat sused srefs
    for row in "${candidates[@]}"; do
        [ -n "$row" ] || continue
        IFS=$'\t' read -r sname screat sused srefs <<< "$row"

        destroy_snapshot "$sname" "0" "$sused" || continue
        (( deleted++ ))

        if (( DRY_RUN )); then
            free=$(( free + sused ))
        else
            free=$(zdc_pool_free_bytes "$pool")
            zdc_valid_int "$free" || break
        fi
        (( free >= target )) && break

        if (( deleted >= 500 )); then
            log_warn "Pool '${pool}': stopped after 500 deletions in one run."
            break
        fi
    done

    if (( free < target )); then
        log_warn "Pool '${pool}': still $(zdc_fmt_bytes "$free") free after pruning ${deleted} snapshot(s), target $(zdc_fmt_bytes "$target")."
        (( DRY_RUN )) || zdc_notify "ZFS pool ${pool} still below free-space target" \
            "Pruned ${deleted} snapshot(s), free space is $(zdc_fmt_bytes "$free"), target $(zdc_fmt_bytes "$target")." "warning"
    else
        log_ok "Pool '${pool}': free space target reached after pruning ${deleted} snapshot(s)."
    fi
}

process_dataset() {
    local name="$1" recursive="$2"
    local h_keep="$3" d_keep="$4" w_keep="$5" m_keep="$6" y_keep="$7"

    local HOUR K_FREQ K_HOUR K_DAY K_WEEK K_MONTH K_YEAR
    HOUR=$(date '+%-H')
    K_FREQ=$(date '+%Y-%m-%d_%H-%M')
    K_HOUR=$(date '+%Y-%m-%d_%H-00')
    K_DAY=$(date '+%Y-%m-%d')
    K_WEEK=$(date '+%G-W%V')
    K_MONTH=$(date '+%Y-%m')
    K_YEAR=$(date '+%Y')

    load_snaps "$name"

    if (( SNAP_FREQUENT > 0 )) || (( FORCE_ALL )); then
        create_snapshot "$name" "frequent" "$K_FREQ" "$recursive"
    fi

    (( h_keep > 0 )) && create_snapshot "$name" "hourly" "$K_HOUR" "$recursive"

    if (( FORCE_NOW )) || (( HOUR >= SNAP_DAILY_HOUR )); then
        (( d_keep > 0 )) && create_snapshot "$name" "daily"   "$K_DAY"   "$recursive"
        (( w_keep > 0 )) && create_snapshot "$name" "weekly"  "$K_WEEK"  "$recursive"
        (( m_keep > 0 )) && create_snapshot "$name" "monthly" "$K_MONTH" "$recursive"
        (( y_keep > 0 )) && create_snapshot "$name" "yearly"  "$K_YEAR"  "$recursive"
    fi

    prune_snapshots "$name" "frequent" "$SNAP_FREQUENT" "$recursive" "$AGE_FREQUENT"
    prune_snapshots "$name" "hourly"   "$h_keep" "$recursive" "$AGE_HOURLY"
    prune_snapshots "$name" "daily"    "$d_keep" "$recursive" "$AGE_DAILY"
    prune_snapshots "$name" "weekly"   "$w_keep" "$recursive" "$AGE_WEEKLY"
    prune_snapshots "$name" "monthly"  "$m_keep" "$recursive" "$AGE_MONTHLY"
    prune_snapshots "$name" "yearly"   "$y_keep" "$recursive" "$AGE_YEARLY"
}

log "ZFS Snapshot Manager starting$( ((FORCE_NOW)) && echo ' (manual run)' )..."
(( DRY_RUN )) && log_warn "DRY RUN - no snapshots will be created or destroyed."

if ! zdc_zfs_ready; then
    log_warn "ZFS is not available or no pool is imported (array not started?). Nothing to do."
    exit 0
fi

if [ ! -f "$DATASETS_JSON" ]; then
    log_warn "No dataset config found (${DATASETS_JSON}). Add a dataset in the GUI."
    exit 0
fi

AVAILABLE=$(zfs list -H -o name 2>/dev/null)

DS_COUNT=0
POOLS=""

while IFS=$'\t' read -r name recursive h d w m y; do
    [ -n "$name" ] || continue

    if ! zdc_valid_dataset "$name"; then
        log_err "Skipping invalid dataset name from config: '${name}'"
        continue
    fi

    if ! printf '%s\n' "$AVAILABLE" | grep -Fxq "$name"; then
        log_err "Dataset does not exist: ${name}"
        log_err "Available ZFS datasets: $(printf '%s' "$AVAILABLE" | tr '\n' ' ')"
        continue
    fi

    pool="${name%%/*}"
    [[ "$POOLS" != *"|${pool}|"* ]] && POOLS="${POOLS}|${pool}|"

    h_keep=$(resolve "$h" "$SNAP_HOURLY")
    d_keep=$(resolve "$d" "$SNAP_DAILY")
    w_keep=$(resolve "$w" "$SNAP_WEEKLY")
    m_keep=$(resolve "$m" "$SNAP_MONTHLY")
    y_keep=$(resolve "$y" "$SNAP_YEARLY")

    log "Processing dataset: ${name} (recursive=${recursive})"
    process_dataset "$name" "$recursive" "$h_keep" "$d_keep" "$w_keep" "$m_keep" "$y_keep"
    (( DS_COUNT++ ))
done < <(read_datasets)

for pool in $(printf '%s' "$POOLS" | tr '|' '\n' | grep -v '^$' | sort -u); do
    free_space_prune "$pool"

    if (( SNAP_MIN_FREE_PCT > 0 )); then
        cap=$(zdc_pool_capacity "$pool")
        if zdc_valid_int "$cap" && (( cap > 100 - SNAP_MIN_FREE_PCT )); then
            log_warn "Pool '${pool}' is ${cap}% full (threshold ${SNAP_MIN_FREE_PCT}% free). Review your retention settings."
            (( DRY_RUN )) || zdc_notify "ZFS pool ${pool} is ${cap}% full" \
                "Snapshot retention may be keeping too much data. Threshold: ${SNAP_MIN_FREE_PCT}% free." "warning"
        fi
    fi
done

if (( DS_COUNT == 0 )); then
    log_warn "No datasets configured."
elif (( CREATED == 0 && PRUNED == 0 && ERRORS == 0 )); then
    log "Nothing due - all snapshots for the current period already exist."
fi
