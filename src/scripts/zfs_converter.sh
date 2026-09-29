#!/bin/bash

dry_run="__DRY_RUN__"
cleanup="__CLEANUP__"
replace_spaces="__REPLACE_SPACES__"
should_process_containers="__PROCESS_CONTAINERS__"
source_pool_where_appdata_is="__APPDATA_POOL__"
source_dataset_where_appdata_is="__APPDATA_DATASET__"
should_process_vms="__PROCESS_VMS__"
source_pool_where_vm_domains_are="__VM_POOL__"
source_dataset_where_vm_domains_are="__VM_DATASET__"
vm_forceshutdown_wait="__VM_SHUTDOWN_WAIT__"
buffer_zone="__BUFFER_ZONE__"
send_notifications="__SEND_NOTIFICATIONS__"
validation_tolerance="__VALIDATION_TOLERANCE__"

source_datasets_array=(__EXTRA_DATASETS__)

mount_point="/mnt"

# Which folder is being converted and where its original went. On the flash
# drive on purpose: after a power cut this is what tells the next run that a
# folder was half converted - a leftover <name>_temp by itself says nothing,
# since a folder of that name can just as well be the user's own.
journal_file="${ZDC_CONVERSION_JOURNAL:-/boot/config/plugins/zfs.toolkit/conversion_in_progress}"

stopped_containers=()
stopped_vms=()
converted_folders=()
failed_folders=()
skipped_folders=()
services_restarted=0

# Which folder each stopped container or VM was stopped for, and the folders
# that could not be put back after a failed conversion. Whatever uses one of
# those stays stopped: started again, it would run on a half-made copy.
declare -A stopped_for=()
declare -A unsafe_paths=()

# The conversion in progress, so an error or a Stop can put it back.
cur_source=""
cur_entry=""
cur_name=""
cur_temp=""
cur_created=0

# What this run converts, worked out before anything is stopped.
plan_source=()
plan_entry=()
plan_name=()

shopt -s nullglob

# --sparse: a VM image is mostly holes. Without it the copy writes every one of
# them out, and on a dataset without compression the image then takes up its
# full nominal size.
RSYNC_OPTS=(-a -H -A -X --numeric-ids --sparse)

_zfs_datasets_cache=""
_zfs_mounts_cache=""

log()      { echo "[$(date '+%H:%M:%S')] $*"; }
log_ok()   { echo "[$(date '+%H:%M:%S')] OK: $*"; }
log_warn() { echo "[$(date '+%H:%M:%S')] WARNING: $*"; }
log_err()  { echo "[$(date '+%H:%M:%S')] ERROR: $*"; }

step() { echo ""; echo "=== Step $* ==="; }

refresh_zfs_cache() {
    _zfs_datasets_cache=$(zfs list -H -o name 2>/dev/null)
    _zfs_mounts_cache=$(zfs list -H -o mounted,mountpoint 2>/dev/null | awk -F'\t' '$1 == "yes" {print $2}')
}

dataset_exists() {
    printf '%s\n' "$_zfs_datasets_cache" | grep -qxF -- "$1"
}

# A path a dataset is mounted at is converted, whatever the dataset is called:
# a folder whose name ZFS cannot use is converted under another name and
# mounted where it was.
is_zfs_dataset() {
    printf '%s\n' "$_zfs_mounts_cache" | grep -qxF -- "$1"
}

find_real_location() {
    local path="$1"

    if [[ ! -e "$path" ]]; then
        log_err "Path not found: $path"
        return 1
    fi

    if [[ "$path" != /mnt/user/* ]]; then
        echo "$path"
        return 0
    fi

    local relative="${path#/mnt/user}"
    for disk_path in /mnt/*/; do
        [[ "$disk_path" == "/mnt/user/" ]] && continue
        local candidate="${disk_path%/}${relative}"
        if [[ -e "$candidate" ]]; then
            echo "$candidate"
            return 0
        fi
    done

    log_err "Real location not found for: $path"
    return 2
}

# What ZFS accepts in a dataset name, checked against a real pool: letters,
# digits, space and _ . : -. Umlauts, brackets, commas, plus signs and the like
# it refuses.
valid_zfs_name() {
    local name="$1"
    [ -n "$name" ] || return 1
    [ "$name" != "." ] && [ "$name" != ".." ] || return 1
    (( ${#name} <= 200 )) || return 1
    [[ "$name" =~ ^[A-Za-z0-9_.:\ -]+$ ]]
}

# The dataset name for a folder: its own name when ZFS accepts that, otherwise
# the nearest thing it does - umlauts spelled out, every other refused character
# turned into "_". Spaces become "_" only with Replace spaces on.
#
# Only the dataset is named this way. The folder keeps its path either way
# (convert_one mounts the dataset where the folder was), so a container or VM
# pointing at it keeps working. Renaming the folder instead, as this used to,
# left the container with an empty directory and the VM without its disk.
dataset_name_for() {
    local name="$1"
    if [[ "$replace_spaces" =~ ^[Yy]es$ ]]; then
        name="${name// /_}"
    fi
    if valid_zfs_name "$name"; then
        printf '%s' "$name"
        return
    fi
    name=$(printf '%s' "$name" | sed 's/ä/ae/g; s/ö/oe/g; s/ü/ue/g; s/Ä/Ae/g; s/Ö/Oe/g; s/Ü/Ue/g; s/ß/ss/g')
    printf '%s' "$name" | LC_ALL=C sed 's/[^A-Za-z0-9_.: -]/_/g'
}

send_notification() {
    local subject="$1"
    local message="$2"
    local importance="${3:-normal}"   # normal | warning | alert

    if [[ "$send_notifications" =~ ^[Yy]es$ ]]; then
        /usr/local/emhttp/webGui/scripts/notify \
            -e "ZFS Toolkit" \
            -s "$subject" \
            -d "$message" \
            -i "$importance" 2>/dev/null || true
    fi
}

# Whether a path is one of the folders this run converts. Containers and VMs
# are stopped for those only: a folder that is skipped - a name clash, too
# little room - used to have its container stopped and started again on every
# run, for nothing.
planned_path() {
    local p="$1" i
    for i in "${!plan_entry[@]}"; do
        [ "$p" = "${mount_point}/${plan_source[$i]}/${plan_entry[$i]}" ] && return 0
    done
    return 1
}

# A container or VM whose folder could not be put back after a failed
# conversion. $1 is its key in stopped_for.
held_back() {
    local p="${stopped_for[$1]:-}"
    [ -n "$p" ] && [ -n "${unsafe_paths[$p]:-}" ]
}

stop_docker_containers() {
    [[ ! "$should_process_containers" =~ ^[Yy]es$ ]] && return

    step "Stop Docker containers (if needed)"

    if ! command -v docker &>/dev/null; then
        log_warn "docker command not found, skipping container handling."
        return
    fi

    for container_id in $(docker ps -q 2>/dev/null); do
        local container_name
        container_name=$(docker container inspect --format '{{.Name}}' "$container_id" | cut -c 2-)

        local bindmounts
        bindmounts=$(docker inspect --format \
            '{{ range .Mounts }}{{ if eq .Type "bind" }}{{ .Source }}{{printf "\n"}}{{ end }}{{ end }}' \
            "$container_id" 2>/dev/null)

        if [[ -z "$bindmounts" ]]; then
            log "Container ${container_name}: no bind mounts, skipping."
            continue
        fi

        local stop_container=false
        local stop_path=""

        while IFS= read -r bindmount; do
            [[ -z "$bindmount" ]] && continue

            if [[ "$bindmount" == /mnt/user/* ]]; then
                bindmount=$(find_real_location "$bindmount") || continue
            fi

            [[ "$bindmount" != "/mnt/$source_path_appdata/"* ]] && continue

            local immediate_child
            immediate_child=$(echo "$bindmount" | sed -n "s|^/mnt/$source_path_appdata/||p" | cut -d "/" -f 1)
            [[ -z "$immediate_child" ]] && continue

            local combined_path="/mnt/$source_path_appdata/$immediate_child"

            if planned_path "$combined_path"; then
                log "Container ${container_name}: appdata '${combined_path}' is about to be converted → will stop container."
                stop_container=true
                stop_path="$combined_path"
                break
            fi
        done <<< "$bindmounts"

        if [[ "$stop_container" == true ]]; then
            if [[ "$dry_run" =~ ^[Yy]es$ ]]; then
                log "Dry run: would stop container ${container_name}"
            else
                log "Stopping container ${container_name}..."
                docker stop "$container_id"
            fi
            stopped_containers+=("$container_name")
            stopped_for["c:${container_name}"]="$stop_path"
        else
            log "Container ${container_name}: nothing of it is converted in this run, no action needed."
        fi
    done

    if [[ "${#stopped_containers[@]}" -gt 0 ]]; then
        log_ok "Stopped containers: ${stopped_containers[*]}"
    fi
}

start_docker_containers() {
    [[ ! "$should_process_containers" =~ ^[Yy]es$ ]] && return
    [[ "${#stopped_containers[@]}" -eq 0 ]] && return

    step "Restart Docker containers"
    for container_name in "${stopped_containers[@]}"; do
        if held_back "c:${container_name}"; then
            log_err "Container ${container_name} stays stopped: its folder ${stopped_for[c:${container_name}]} could not be put back."
            continue
        fi
        if [[ "$dry_run" =~ ^[Yy]es$ ]]; then
            log "Dry run: would restart container ${container_name}"
        else
            log "Restarting container ${container_name}..."
            docker start "$container_name"
        fi
    done
}

get_vm_disk() {
    local vm_name="$1"
    local vm_target
    vm_target=$(virsh domblklist "$vm_name" --details 2>/dev/null | awk '/disk/{print $3; exit}')

    if [[ -z "$vm_target" ]]; then
        log_err "No disk target found for VM: $vm_name"
        return 1
    fi

    local vm_disk
    vm_disk=$(virsh domblklist "$vm_name" 2>/dev/null | awk -v tgt="$vm_target" '$1==tgt{$1=""; sub(/^ /,""); print; exit}')

    if [[ -z "$vm_disk" ]]; then
        log_err "No disk path found for VM: $vm_name"
        return 1
    fi

    echo "$vm_disk"
}

stop_virtual_machines() {
    [[ ! "$should_process_vms" =~ ^[Yy]es$ ]] && return

    step "Stop Virtual Machines (if needed)"

    if ! command -v virsh &>/dev/null; then
        log_warn "virsh command not found, skipping VM handling."
        return
    fi

    while IFS= read -r vm; do
        [[ -z "$vm" ]] && continue

        local vm_disk
        vm_disk=$(get_vm_disk "$vm") || { log "No disk found for VM $vm. Skipping."; continue; }

        if [[ "$vm_disk" == /mnt/user/* ]]; then
            vm_disk=$(find_real_location "$vm_disk") || continue
        fi

        [[ "$vm_disk" != "/mnt/$source_path_vms/"* ]] && continue

        local immediate_child
        immediate_child=$(echo "$vm_disk" | sed -n "s|^/mnt/$source_path_vms/||p" | cut -d "/" -f 1)
        [[ -z "$immediate_child" ]] && continue

        local combined_path="/mnt/$source_path_vms/$immediate_child"

        if planned_path "$combined_path"; then
            log "VM ${vm}: vdisk folder '${combined_path}' is about to be converted → will stop VM."

            if [[ "$dry_run" =~ ^[Yy]es$ ]]; then
                log "Dry run: would stop VM ${vm}"
            else
                virsh shutdown "$vm" 2>/dev/null

                local start_time
                start_time=$(date +%s)
                while virsh dominfo "$vm" 2>/dev/null | grep -q 'running'; do
                    sleep 5
                    local elapsed=$(( $(date +%s) - start_time ))
                    if (( elapsed >= vm_forceshutdown_wait )); then
                        log_warn "VM $vm did not shut down after ${vm_forceshutdown_wait}s. Forcing shutdown."
                        virsh destroy "$vm" 2>/dev/null
                        break
                    fi
                done
            fi
            stopped_vms+=("$vm")
            stopped_for["v:${vm}"]="$combined_path"
        else
            log "VM ${vm}: nothing of it is converted in this run, no action needed."
        fi
    done < <(virsh list --name 2>/dev/null | grep -v '^$')

    if [[ "${#stopped_vms[@]}" -gt 0 ]]; then
        log_ok "Stopped VMs: ${stopped_vms[*]}"
    fi
}

start_virtual_machines() {
    [[ ! "$should_process_vms" =~ ^[Yy]es$ ]] && return
    [[ "${#stopped_vms[@]}" -eq 0 ]] && return

    step "Restart Virtual Machines"
    for vm in "${stopped_vms[@]}"; do
        if held_back "v:${vm}"; then
            log_err "VM ${vm} stays stopped: its folder ${stopped_for[v:${vm}]} could not be put back."
            continue
        fi
        if [[ "$dry_run" =~ ^[Yy]es$ ]]; then
            log "Dry run: would restart VM ${vm}"
        else
            log "Restarting VM ${vm}..."
            virsh start "$vm" 2>/dev/null
        fi
    done
}

restore_services() {
    (( services_restarted )) && return 0
    services_restarted=1
    start_docker_containers
    start_virtual_machines
    return 0
}

# One field per line: folder names are free text, and a tab in one would shift
# the fields of a tab-separated line.
journal_write() {
    mkdir -p "$(dirname "$journal_file")" 2>/dev/null
    printf '%s\n' "$cur_source" "$cur_entry" "$cur_name" "$cur_temp" > "$journal_file"
}

journal_clear() {
    rm -f "$journal_file"
}

# Put the folder being converted back as it was: the copy goes, the original
# returns under its own name. Without this an error left the original renamed
# to <name>_temp and the half-filled dataset mounted in its place - and the
# container was then started again on the half copy.
rollback_current() {
    [ -n "$cur_temp" ] || return 0
    local path="${mount_point}/${cur_source}/${cur_entry}"
    local dataset="${cur_source}/${cur_name}"
    local err

    # Stopped before the rename happened: there is nothing to put back.
    if (( ! cur_created )) && [ ! -e "$cur_temp" ] && [ -e "$path" ]; then
        journal_clear
        cur_temp=""
        return 0
    fi

    if (( cur_created )); then
        if ! err=$(zfs destroy "$dataset" 2>&1); then
            log_err "Cannot remove the incomplete dataset '${dataset}': ${err}"
            log_err "The original is untouched in ${cur_temp}."
            return 1
        fi
        cur_created=0
        rmdir "$path" 2>/dev/null   # the mountpoint directory outlives the dataset
    fi

    if [ -e "$path" ]; then
        log_err "Cannot move ${cur_temp} back: ${path} exists. The original is untouched in ${cur_temp}."
        return 1
    fi
    if ! mv "$cur_temp" "$path"; then
        log_err "Cannot move ${cur_temp} back to ${path}. The original is untouched in ${cur_temp}."
        return 1
    fi

    journal_clear
    cur_temp=""
    return 0
}

abandon() {
    local entry="$1"
    local path="${mount_point}/${cur_source}/${cur_entry}"
    if rollback_current; then
        log_warn "Put '${entry}' back the way it was."
    else
        unsafe_paths["$path"]=1
        cur_temp=""   # the journal stays: the next run takes it from there
        send_notification "ZFS conversion needs attention" \
            "The conversion of '${entry}' failed and could not be undone. See the conversion log." "alert"
    fi
    failed_folders+=("$entry")
}

on_interrupt() {
    echo ""
    log_warn "Interrupted."
    exit 130
}

on_exit() {
    local rc=$?
    if [ -n "$cur_temp" ]; then
        log_warn "Stopped while converting '${cur_entry}' - putting it back the way it was."
        if ! rollback_current; then
            unsafe_paths["${mount_point}/${cur_source}/${cur_entry}"]=1
            send_notification "ZFS conversion needs attention" \
                "A stopped conversion of '${cur_entry}' could not be undone. See the conversion log." "alert"
        fi
    fi
    restore_services
    return "$rc"
}

trap on_exit EXIT
trap on_interrupt INT TERM HUP

check_space() {
    local parent_dataset="$1"
    local folder_size_bytes="$2"

    local available
    available=$(zfs list -o avail -p -H "$parent_dataset" 2>/dev/null) || {
        log_err "Cannot query available space on ${parent_dataset}"
        return 1
    }

    local required=$(( folder_size_bytes + folder_size_bytes * buffer_zone / 100 ))

    if (( available < required )); then
        local available_hr required_hr
        available_hr=$(numfmt --to=iec-i --suffix=B "$available" 2>/dev/null || echo "${available} bytes")
        required_hr=$(numfmt --to=iec-i --suffix=B "$required" 2>/dev/null || echo "${required} bytes")
        log_err "Insufficient space on ${parent_dataset}: available ${available_hr}, required ${required_hr} (folder + ${buffer_zone}% buffer)."
        return 1
    fi

    return 0
}

# Space the folder takes on disk, not its apparent size. A sparse VM image
# can be a few gigabytes of data in a file that claims a hundred, and the copy
# (rsync --sparse) takes the few.
folder_bytes() {
    du -sB1 "$1" 2>/dev/null | cut -f1
}

perform_validation() {
    local src="$1"
    local dst="$2"

    log "Validating copy..."
    local src_count dst_count src_size dst_size
    src_count=$(find "$src" -mindepth 1 2>/dev/null | wc -l)
    dst_count=$(find "$dst" -mindepth 1 2>/dev/null | wc -l)
    src_size=$(du -sb "$src" 2>/dev/null | cut -f1)
    dst_size=$(du -sb "$dst" 2>/dev/null | cut -f1)
    src_size="${src_size:-0}"; dst_size="${dst_size:-0}"

    log "  Source : ${src_count} entries, ${src_size} bytes"
    log "  Dest   : ${dst_count} entries, ${dst_size} bytes"

    local tol="${validation_tolerance:-5}"

    if (( src_count != dst_count )); then
        log_err "VALIDATION FAILED: entry count mismatch (src=${src_count}, dst=${dst_count})"
        return 1
    fi

    if (( src_size > 0 )); then
        local diff=$(( src_size - dst_size ))
        [[ $diff -lt 0 ]] && diff=$(( -diff ))
        local pct=$(( diff * 100 / src_size ))
        if (( pct > tol )); then
            log_err "VALIDATION FAILED: size difference ${pct}% exceeds tolerance ${tol}% (src=${src_size}, dst=${dst_size})"
            return 1
        fi
    fi

    log_ok "Validation passed."
    return 0
}

# A run that was cut off hard - power cut, kill -9 - leaves the journal behind.
# Settle that conversion first: undo it if the copy never started, finish it if
# the copy is complete, and otherwise stop and say so. With both an original and
# a copy that do not match, only a person can tell which one is right.
recover_interrupted() {
    [ -f "$journal_file" ] || return 0

    local j_source="" j_entry="" j_name="" j_temp=""
    { IFS= read -r j_source; IFS= read -r j_entry; IFS= read -r j_name; IFS= read -r j_temp; } < "$journal_file"
    local path="${mount_point}/${j_source}/${j_entry}"
    local dataset="${j_source}/${j_name}"

    step "Earlier conversion of '${j_entry}' was cut off"
    refresh_zfs_cache

    if [ -z "$j_temp" ] || [ ! -d "$j_temp" ]; then
        log "Nothing of it is left to finish or undo."
        journal_clear
        return 0
    fi

    if ! dataset_exists "$dataset"; then
        # Renamed, never copied: the original just goes back. Docker creates an
        # empty directory for a bind mount whose source is missing, so an empty
        # one in the way is not data.
        if [ -d "$path" ] && [ -z "$(ls -A "$path" 2>/dev/null)" ]; then
            rmdir "$path" 2>/dev/null
        fi
        if [ ! -e "$path" ] && mv "$j_temp" "$path"; then
            log_ok "Put '${j_entry}' back where it was. Restart whatever uses it if it is running."
            journal_clear
            return 0
        fi
    elif is_zfs_dataset "$path" && perform_validation "$j_temp" "$path"; then
        if [[ "$cleanup" =~ ^[Yy]es$ ]]; then
            rm -rf "$j_temp"
        fi
        log_ok "Finished the conversion of '${j_entry}': the copy in ${dataset} is complete."
        converted_folders+=("${j_entry} (finished)")
        journal_clear
        return 0
    fi

    log_err "The conversion of '${j_entry}' was cut off and cannot be finished or undone by itself."
    log_err "The original is in ${j_temp}. ${dataset} holds a copy that does not match it, or is missing."
    log_err "Keep one of them: delete ${j_temp} to keep the dataset, or destroy ${dataset} and rename ${j_temp} back to ${path}. The next run carries on after that."
    send_notification "ZFS conversion needs attention" \
        "An earlier conversion of '${j_entry}' was cut off and has to be settled by hand. See the conversion log." "alert"
    return 1
}

# Everything this run is going to convert, decided before anything is stopped.
plan_conversions() {
    step "Planning"
    refresh_zfs_cache

    local source_path path base name
    for source_path in "${source_datasets_array[@]}"; do
        for path in "${mount_point}/${source_path}"/*; do
            [ -d "$path" ] || continue
            [ -L "$path" ] && continue          # points somewhere else - not ours to move
            is_zfs_dataset "$path" && continue  # converted already
            base=$(basename "$path")

            if [[ "$base" == *$'\n'* ]]; then
                log_warn "Skipping a folder with a line break in its name in ${source_path}."
                skipped_folders+=("(line break in name)")
                continue
            fi

            # Next to a converted folder of the same name without _temp this is
            # a copy kept with Cleanup off, or an original that an older version
            # left behind. Either way it is left alone. Older versions resumed
            # into it with rsync --delete, which also wiped a converted folder
            # whenever the user happened to have one called <name>_temp.
            if [[ "$base" == *_temp ]] && is_zfs_dataset "${path%_temp}"; then
                log_warn "Leaving '${base}' alone: next to the dataset at ${path%_temp} it is a copy kept with Cleanup off, or an original an older version left behind. Delete it once you are sure it is not needed."
                skipped_folders+=("$base")
                continue
            fi

            name=$(dataset_name_for "$base")
            if dataset_exists "${source_path}/${name}"; then
                log_warn "Skipping '${base}': the dataset '${source_path}/${name}' exists but is not mounted at ${path}."
                skipped_folders+=("$base")
                continue
            fi

            if ! check_space "$source_path" "$(folder_bytes "$path")"; then
                log_warn "Skipping '${base}' due to insufficient space."
                skipped_folders+=("$base")
                continue
            fi

            plan_source+=("$source_path")
            plan_entry+=("$base")
            plan_name+=("$name")
            if [ "$name" = "$base" ]; then
                log "Will convert '${source_path}/${base}'."
            else
                log "Will convert '${source_path}/${base}' as dataset '${name}', mounted at the same path."
            fi
        done
    done
}

convert_one() {
    local source_path="$1" entry="$2" name="$3"
    local path="${mount_point}/${source_path}/${entry}"
    local temp_path="${path}_temp"
    local dataset="${source_path}/${name}"

    refresh_zfs_cache
    if [ ! -d "$path" ] || is_zfs_dataset "$path"; then
        log "Skipping '${entry}': no longer a plain folder."
        return
    fi

    local folder_size folder_size_hr
    folder_size=$(folder_bytes "$path")
    folder_size_hr=$(du -sh "$path" 2>/dev/null | cut -f1)
    log "Processing '${entry}' (${folder_size_hr})..."

    if ! check_space "$source_path" "${folder_size:-0}"; then
        log_warn "Skipping '${entry}' due to insufficient space."
        failed_folders+=("$entry")
        return
    fi

    if [[ "$dry_run" =~ ^[Yy]es$ ]]; then
        log "Dry run: would create dataset '${dataset}' from folder '${entry}'"
        return
    fi

    if [ -e "$temp_path" ]; then
        log_err "Temp path already exists: ${temp_path}. Resolve it manually before retrying '${entry}'."
        failed_folders+=("$entry")
        return
    fi
    # Journal first, rename second: a run cut off between the two leaves an
    # entry for a rename that did not happen, which the next run recognises,
    # rather than a renamed folder nothing knows about.
    cur_source="$source_path"; cur_entry="$entry"; cur_name="$name"
    cur_temp="$temp_path"; cur_created=0
    journal_write
    if ! mv "$path" "$temp_path"; then
        log_err "Failed to rename '${entry}' to temp. Skipping."
        journal_clear
        cur_temp=""
        failed_folders+=("$entry")
        return
    fi

    # A dataset named differently from its folder is mounted where the folder
    # was, so no path that points at it changes.
    local create_opts=() err
    [ "$name" != "$entry" ] && create_opts=(-o "mountpoint=${path}")
    if ! err=$(zfs create "${create_opts[@]}" "$dataset" 2>&1); then
        log_err "Failed to create dataset '${dataset}': ${err}"
        abandon "$entry"
        return
    fi
    cur_created=1

    refresh_zfs_cache
    if ! is_zfs_dataset "$path"; then
        log_err "Dataset '${dataset}' was created but is not mounted at ${path}."
        abandon "$entry"
        return
    fi

    log "Copying data with rsync..."
    if ! rsync "${RSYNC_OPTS[@]}" "${temp_path}/" "${path}/"; then
        log_err "rsync failed for '${entry}'."
        abandon "$entry"
        return
    fi

    if ! perform_validation "$temp_path" "$path"; then
        abandon "$entry"
        return
    fi

    if [[ "$cleanup" =~ ^[Yy]es$ ]]; then
        rm -rf "$temp_path"
        log_ok "Cleaned up temp dir."
    else
        log "Cleanup disabled. The original is kept as ${temp_path}."
    fi
    journal_clear
    cur_temp=""

    converted_folders+=("$entry")
    log_ok "Successfully converted '${entry}' → dataset '${dataset}'"
}

can_i_go_to_work() {
    step "Pre-flight checks"
    refresh_zfs_cache

    if [[ "${#source_datasets_array[@]}" -eq 0 ]]; then
        log_err "No source datasets defined. Check your configuration."
        exit 1
    fi

    for source_path in "${source_datasets_array[@]}"; do
        local full_path="${mount_point}/${source_path}"

        if [[ ! -e "$full_path" ]]; then
            log_err "Source path does not exist: ${full_path}"
            exit 1
        fi

        if ! dataset_exists "$source_path"; then
            log_err "Source '${source_path}' is not a ZFS dataset. Sources must be datasets to host child datasets."
            exit 1
        fi

        log_ok "Source '${source_path}' is valid."
    done
}

print_summary() {
    step "Summary"

    if [[ "${#converted_folders[@]}" -gt 0 ]]; then
        log_ok "Successfully converted ${#converted_folders[@]} folder(s):"
        for f in "${converted_folders[@]}"; do
            echo "  + $f"
        done
    fi

    if [[ "${#skipped_folders[@]}" -gt 0 ]]; then
        log_warn "${#skipped_folders[@]} folder(s) left as they are:"
        for f in "${skipped_folders[@]}"; do
            echo "  = $f"
        done
    fi

    if [[ "${#failed_folders[@]}" -gt 0 ]]; then
        log_err "${#failed_folders[@]} folder(s) failed:"
        for f in "${failed_folders[@]}"; do
            echo "  - $f"
        done
    fi

    if [[ "${#converted_folders[@]}" -gt 0 && "${#failed_folders[@]}" -eq 0 ]]; then
        send_notification \
            "ZFS Conversion Complete" \
            "Successfully converted ${#converted_folders[@]} folder(s) to ZFS datasets." \
            "normal"
    elif [[ "${#failed_folders[@]}" -gt 0 ]]; then
        send_notification \
            "ZFS Conversion - Issues Detected" \
            "Converted: ${#converted_folders[@]}, Failed: ${#failed_folders[@]}. Check the logs." \
            "warning"
    fi
}

if [[ "$should_process_containers" =~ ^[Yy]es$ ]]; then
    source_datasets_array+=("${source_pool_where_appdata_is}/${source_dataset_where_appdata_is}")
    source_path_appdata="${source_pool_where_appdata_is}/${source_dataset_where_appdata_is}"
fi

if [[ "$should_process_vms" =~ ^[Yy]es$ ]]; then
    source_datasets_array+=("${source_pool_where_vm_domains_are}/${source_dataset_where_vm_domains_are}")
    source_path_vms="${source_pool_where_vm_domains_are}/${source_dataset_where_vm_domains_are}"
fi

log "ZFS Toolkit starting..."
[[ "$dry_run" =~ ^[Yy]es$ ]] && log_warn "DRY RUN MODE - no changes will be made."

can_i_go_to_work
if [[ ! "$dry_run" =~ ^[Yy]es$ ]]; then
    recover_interrupted || exit 1
elif [ -f "$journal_file" ]; then
    log_warn "An earlier conversion of '$(sed -n 2p "$journal_file")' was cut off. A real run settles that first."
fi
plan_conversions

if [[ "${#plan_entry[@]}" -eq 0 ]]; then
    if [[ "${#skipped_folders[@]}" -gt 0 ]]; then
        log_ok "Nothing to convert - ${#skipped_folders[@]} folder(s) left as they are, see above."
    else
        log_ok "Nothing to convert. All folders are already datasets."
    fi
    log "Script execution completed successfully."
    exit 0
fi

stop_docker_containers
stop_virtual_machines

step "Convert"
for i in "${!plan_entry[@]}"; do
    convert_one "${plan_source[$i]}" "${plan_entry[$i]}" "${plan_name[$i]}"
done

restore_services
print_summary

log "Script execution completed successfully."
