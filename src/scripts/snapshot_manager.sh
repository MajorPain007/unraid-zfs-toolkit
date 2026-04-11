#!/bin/bash
# snapshot_manager.sh - Create and prune ZFS snapshots using native zfs commands
# Called by cron every N minutes. Decides which snapshot types to create based on current time.
#
# Snapshot naming: DATASET@auto-TYPE-TIMESTAMP
#   Types: frequent, hourly, daily, weekly, monthly, yearly
#
# Retention: keeps latest N snapshots of each type, prunes older ones.

CONFIG_DIR="/boot/config/plugins/zfs.dataset.converter"
SETTINGS="${CONFIG_DIR}/settings.cfg"
DATASETS_JSON="${CONFIG_DIR}/snap_datasets.json"
LOG_DIR="/tmp/zfs.dataset.converter"
LAST_FILE="${LOG_DIR}/snapshot_last.txt"

mkdir -p "${LOG_DIR}"

log()      { echo "[$(date '+%H:%M:%S')] $*"; }
log_ok()   { echo "[$(date '+%H:%M:%S')] OK: $*"; }
log_warn() { echo "[$(date '+%H:%M:%S')] WARNING: $*"; }
log_err()  { echo "[$(date '+%H:%M:%S')] ERROR: $*"; }

# -----------------------------------------------------------------------
# Read settings.cfg
# -----------------------------------------------------------------------
declare -A cfg
if [ -f "$SETTINGS" ]; then
    while IFS='=' read -r key val; do
        [[ -z "$key" || "$key" =~ ^# ]] && continue
        cfg["${key}"]="${val}"
    done < "$SETTINGS"
fi

SNAP_FREQUENT="${cfg[snap_frequent]:-0}"
SNAP_HOURLY="${cfg[snap_hourly]:-24}"
SNAP_DAILY="${cfg[snap_daily]:-30}"
SNAP_WEEKLY="${cfg[snap_weekly]:-4}"
SNAP_MONTHLY="${cfg[snap_monthly]:-3}"
SNAP_YEARLY="${cfg[snap_yearly]:-0}"
SNAP_DAILY_HOUR="${cfg[snap_daily_hour]:-2}"

# -----------------------------------------------------------------------
# Parse dataset list from JSON (perl for portability, no jq needed)
# Outputs: one line per dataset: NAME RECURSIVE HOURLY DAILY WEEKLY MONTHLY YEARLY
# where values are "t" (use template) or a number
# -----------------------------------------------------------------------
read_datasets() {
    perl -e '
use strict;
local $/;
open(my $fh, "<", $ARGV[0]) or exit 1;
my $json = <$fh>;
close $fh;

while ($json =~ /\{([^}]+)\}/g) {
    my $block = $1;
    my %ds;
    while ($block =~ /"(\w+)"\s*:\s*(?:"([^"]*)"|(true|false|-?\d+))/g) {
        $ds{$1} = defined($2) ? $2 : $3;
    }
    next unless $ds{name};
    my $rec = ($ds{recursive} && $ds{recursive} eq "true") ? "1" : "0";
    my $tpl = (!defined($ds{use_template}) || $ds{use_template} eq "true") ? "1" : "0";
    my $h = $ds{hourly}  // "t";
    my $d = $ds{daily}   // "t";
    my $w = $ds{weekly}  // "t";
    my $m = $ds{monthly} // "t";
    my $y = $ds{yearly}  // "t";
    if ($tpl eq "1") { $h="t"; $d="t"; $w="t"; $m="t"; $y="t"; }
    print "$ds{name} $rec $h $d $w $m $y\n";
}
' "$DATASETS_JSON" 2>/dev/null
}

# -----------------------------------------------------------------------
# Resolve retention value: "t" uses global template, number is override
# -----------------------------------------------------------------------
resolve() {
    local val="$1" global="$2"
    [ "$val" = "t" ] && echo "$global" || echo "$val"
}

# -----------------------------------------------------------------------
# Create a snapshot, skipping if it already exists
# -----------------------------------------------------------------------
create_snapshot() {
    local dataset="$1" type="$2" label="$3" recursive="$4"

    local snap_name="${dataset}@auto-${type}-${label}"

    # Skip if already exists (idempotent runs)
    if zfs list -H -t snapshot -o name "$snap_name" &>/dev/null 2>&1; then
        return 0
    fi

    local flags=""
    [ "$recursive" = "1" ] && flags="-r"

    if zfs snapshot $flags "${snap_name}" 2>/dev/null; then
        log_ok "Created ${snap_name}"
    else
        log_err "Failed to create ${snap_name}"
    fi
}

# -----------------------------------------------------------------------
# Prune snapshots of a given type, keeping the latest N
# -----------------------------------------------------------------------
prune_snapshots() {
    local dataset="$1" type="$2" keep="$3" recursive="$4"

    [ "$keep" -le 0 ] 2>/dev/null && return

    # List snapshots of this type, sorted oldest-first
    local snaps
    mapfile -t snaps < <(
        zfs list -H -t snapshot -o name -s creation "$dataset" 2>/dev/null \
        | grep "@auto-${type}-"
    )

    local total="${#snaps[@]}"
    if (( total <= keep )); then
        return  # nothing to prune
    fi

    local to_delete=$(( total - keep ))
    for (( i=0; i<to_delete; i++ )); do
        local snap="${snaps[$i]}"
        local flags=""
        [ "$recursive" = "1" ] && flags="-r"
        if zfs destroy $flags "$snap" 2>/dev/null; then
            log "Pruned ${snap}"
        else
            log_warn "Could not prune ${snap}"
        fi
    done
}

# -----------------------------------------------------------------------
# Process a single dataset
# -----------------------------------------------------------------------
process_dataset() {
    local name="$1" recursive="$2"
    local h_keep="$3" d_keep="$4" w_keep="$5" m_keep="$6" y_keep="$7"

    local NOW HOUR MINUTE DOM MONTH DOW
    NOW=$(date '+%Y-%m-%d_%H-%M')
    HOUR=$(date '+%-H')       # no leading zero
    MINUTE=$(date '+%M')
    DOM=$(date '+%-d')        # day of month, no leading zero
    MONTH=$(date '+%-m')
    DOW=$(date '+%u')         # 1=Mon .. 7=Sun

    # Frequent (every cron run)
    if (( SNAP_FREQUENT > 0 )); then
        create_snapshot "$name" "frequent" "$NOW" "$recursive"
        prune_snapshots "$name" "frequent" "$SNAP_FREQUENT" "$recursive"
    fi

    # Hourly (on the hour)
    if [[ "$MINUTE" == "00" ]] && (( h_keep > 0 )); then
        create_snapshot "$name" "hourly" "$(date '+%Y-%m-%d_%H-00')" "$recursive"
        prune_snapshots "$name" "hourly" "$h_keep" "$recursive"
    fi

    # Daily (at configured daily hour, on the hour)
    if [[ "$MINUTE" == "00" && "$HOUR" == "$SNAP_DAILY_HOUR" ]] && (( d_keep > 0 )); then
        create_snapshot "$name" "daily" "$(date '+%Y-%m-%d')" "$recursive"
        prune_snapshots "$name" "daily" "$d_keep" "$recursive"
    fi

    # Weekly (Sunday at daily hour)
    if [[ "$MINUTE" == "00" && "$HOUR" == "$SNAP_DAILY_HOUR" && "$DOW" == "7" ]] && (( w_keep > 0 )); then
        create_snapshot "$name" "weekly" "$(date '+%G-W%V')" "$recursive"
        prune_snapshots "$name" "weekly" "$w_keep" "$recursive"
    fi

    # Monthly (1st of month at daily hour)
    if [[ "$MINUTE" == "00" && "$HOUR" == "$SNAP_DAILY_HOUR" && "$DOM" == "1" ]] && (( m_keep > 0 )); then
        create_snapshot "$name" "monthly" "$(date '+%Y-%m')" "$recursive"
        prune_snapshots "$name" "monthly" "$m_keep" "$recursive"
    fi

    # Yearly (Jan 1 at daily hour)
    if [[ "$MINUTE" == "00" && "$HOUR" == "$SNAP_DAILY_HOUR" && "$DOM" == "1" && "$MONTH" == "1" ]] && (( y_keep > 0 )); then
        create_snapshot "$name" "yearly" "$(date '+%Y')" "$recursive"
        prune_snapshots "$name" "yearly" "$y_keep" "$recursive"
    fi
}

# -----------------------------------------------------------------------
# Main
# -----------------------------------------------------------------------
log "ZFS Snapshot Manager starting..."

if [ ! -f "$DATASETS_JSON" ]; then
    log_warn "No dataset config found (${DATASETS_JSON}). Nothing to do."
    exit 0
fi

count=0
while read -r name recursive h d w m y; do
    [[ -z "$name" ]] && continue

    h_keep=$(resolve "$h" "$SNAP_HOURLY")
    d_keep=$(resolve "$d" "$SNAP_DAILY")
    w_keep=$(resolve "$w" "$SNAP_WEEKLY")
    m_keep=$(resolve "$m" "$SNAP_MONTHLY")
    y_keep=$(resolve "$y" "$SNAP_YEARLY")

    log "Processing dataset: ${name} (recursive=${recursive})"
    process_dataset "$name" "$recursive" "$h_keep" "$d_keep" "$w_keep" "$m_keep" "$y_keep"
    (( count++ ))
done < <(read_datasets)

if (( count == 0 )); then
    log_warn "No datasets configured."
fi

date '+%Y-%m-%d %H:%M:%S' > "${LAST_FILE}"
log "Snapshot Manager finished."
