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
stopped_containers=()
stopped_vms=()
converted_folders=()
failed_folders=()
services_restarted=0

shopt -s nullglob

RSYNC_OPTS=(-a -H -A -X --numeric-ids)

_zfs_datasets_cache=""

log()      { echo "[$(date '+%H:%M:%S')] $*"; }
log_ok()   { echo "[$(date '+%H:%M:%S')] OK: $*"; }
log_warn() { echo "[$(date '+%H:%M:%S')] WARNING: $*"; }
log_err()  { echo "[$(date '+%H:%M:%S')] ERROR: $*"; }

step() { echo ""; echo "=== Step $* ==="; }

refresh_zfs_cache() {
    _zfs_datasets_cache=$(zfs list -H -o name 2>/dev/null)
}

dataset_exists() {
    local name="$1"
    printf '%s\n' "$_zfs_datasets_cache" | grep -qxF "$name"
}

is_zfs_dataset() {
    local location="$1"
    zfs list -H -o mounted,mountpoint 2>/dev/null \
        | awk -F'\t' -v loc="$location" '$1=="yes" && $2==loc {found=1} END{exit !found}'
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

normalize_name() {
    local name="$1"
    name=$(echo "$name" | sed \
        's/ä/ae/g; s/ö/oe/g; s/ü/ue/g;
         s/Ä/Ae/g; s/Ö/Oe/g; s/Ü/Ue/g;
         s/ß/ss/g')

    if [[ "$replace_spaces" =~ ^[Yy]es$ ]]; then
        name="${name// /_}"
    fi

    name=$(echo "$name" | sed 's/[^a-zA-Z0-9._: -]/_/g')

    name=$(echo "$name" | sed 's/^[._]*//; s/[._]*$//')

    echo "$name"
}

validate_dataset_name() {
    local name="$1"

    if [[ -z "$name" ]]; then
        log_err "Dataset name is empty after normalization."
        return 1
    fi

    if [[ ${#name} -gt 200 ]]; then
        log_err "Dataset name too long (${#name} chars, max 200): $name"
        return 1
    fi

    if [[ "$name" =~ [@#] ]]; then
        log_err "Dataset name contains invalid characters (@ or #): $name"
        return 1
    fi

    if [[ "$name" == *" "* ]]; then
        log_err "Dataset name contains a space: '$name'. Enable 'Replace spaces with underscores' in the settings."
        return 1
    fi

    if [[ ! "$name" =~ ^[a-zA-Z0-9][a-zA-Z0-9_.:-]*$ ]]; then
        log_err "Dataset name is not valid for ZFS: '$name'"
        return 1
    fi

    return 0
}

send_notification() {
    local subject="$1"
    local message="$2"
    local importance="${3:-normal}"   # normal | warning | alert

    if [[ "$send_notifications" =~ ^[Yy]es$ ]]; then
        /usr/local/emhttp/webGui/scripts/notify \
            -e "ZFS Dataset Converter" \
            -s "$subject" \
            -d "$message" \
            -i "$importance" 2>/dev/null || true
    fi
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

        while IFS= read -r bindmount; do
            [[ -z "$bindmount" ]] && continue

            if [[ "$bindmount" == /mnt/user/* ]]; then
                bindmount=$(find_real_location "$bindmount") || continue
            fi

            [[ "$bindmount" != "/mnt/$source_path_appdata"* ]] && continue

            local immediate_child
            immediate_child=$(echo "$bindmount" | sed -n "s|^/mnt/$source_path_appdata/||p" | cut -d "/" -f 1)

            if [[ -z "$immediate_child" ]]; then
                log "Container ${container_name}: bind mount '${bindmount}' is to appdata root, skipping."
                continue
            fi

            local combined_path="/mnt/$source_path_appdata/$immediate_child"

            if ! is_zfs_dataset "$combined_path"; then
                log "Container ${container_name}: appdata '${combined_path}' is a plain folder → will stop container."
                stop_container=true
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
        else
            log "Container ${container_name}: already on a dataset, no action needed."
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

        [[ "$vm_disk" != "/mnt/$source_path_vms"* ]] && continue

        local immediate_child
        immediate_child=$(echo "$vm_disk" | sed -n "s|^/mnt/$source_path_vms/||p" | cut -d "/" -f 1)

        if [[ -z "$immediate_child" ]]; then
            log "VM ${vm}: vdisk '${vm_disk}' is at VM root, skipping."
            continue
        fi

        local combined_path="/mnt/$source_path_vms/$immediate_child"

        if ! is_zfs_dataset "$combined_path"; then
            log "VM ${vm}: vdisk '${combined_path}' is a plain folder → will stop VM."

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
        else
            log "VM ${vm}: vdisk already in a dataset, no action needed."
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
        if [[ "$dry_run" =~ ^[Yy]es$ ]]; then
            log "Dry run: would restart VM ${vm}"
        else
            log "Restarting VM ${vm}..."
            virsh start "$vm" 2>/dev/null
        fi
    done
}

restore_services() {
    local rc=$?
    (( services_restarted )) && return "$rc"
    services_restarted=1
    start_docker_containers
    start_virtual_machines
    return "$rc"
}

on_interrupt() {
    echo ""
    log_warn "Interrupted - restarting stopped containers/VMs before exiting."
    exit 130
}

trap restore_services EXIT
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
        local available_hr folder_hr required_hr
        available_hr=$(numfmt --to=iec-i --suffix=B "$available" 2>/dev/null || echo "${available} bytes")
        required_hr=$(numfmt --to=iec-i --suffix=B "$required" 2>/dev/null || echo "${required} bytes")
        log_err "Insufficient space on ${parent_dataset}: available ${available_hr}, required ${required_hr} (folder + ${buffer_zone}% buffer)."
        return 1
    fi

    return 0
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

create_datasets() {
    local source_path="$1"

    step "Processing source: ${mount_point}/${source_path}"
    refresh_zfs_cache

    for entry in "${mount_point}/${source_path}"/*; do
        local base_entry
        base_entry=$(basename "$entry")

        if [[ "$base_entry" == *_temp ]]; then
            local original_name="${base_entry%_temp}"
            local normalized_name
            normalized_name=$(normalize_name "$original_name")

            if dataset_exists "${source_path}/${normalized_name}"; then
                log "Found leftover temp dir '${base_entry}' and dataset already exists → resuming rsync + cleanup."
                if [[ "$dry_run" =~ ^[Yy]es$ ]]; then
                    log "Dry run: would resume rsync for '${original_name}'"
                    continue
                fi

                rsync "${RSYNC_OPTS[@]}" --delete \
                    "${mount_point}/${source_path}/${base_entry}/" \
                    "${mount_point}/${source_path}/${normalized_name}/" \
                    && log_ok "Resume rsync finished." \
                    || { log_err "Resume rsync failed for '${base_entry}'"; failed_folders+=("$original_name"); continue; }

                if perform_validation \
                    "${mount_point}/${source_path}/${base_entry}" \
                    "${mount_point}/${source_path}/${normalized_name}"; then
                    rm -rf "${mount_point}/${source_path}/${base_entry}"
                    log_ok "Cleaned up temp dir: ${base_entry}"
                    converted_folders+=("$original_name (resumed)")
                else
                    failed_folders+=("$original_name")
                fi
            else
                log_warn "Found leftover temp dir '${base_entry}' but no matching dataset — will treat as new folder on next run."
            fi
            continue
        fi

        local normalized_name
        normalized_name=$(normalize_name "$base_entry")

        if ! validate_dataset_name "$normalized_name"; then
            log_warn "Skipping '${base_entry}': name invalid after normalization → '${normalized_name}'"
            failed_folders+=("$base_entry")
            continue
        fi

        if dataset_exists "${source_path}/${normalized_name}"; then
            log "Skipping '${base_entry}': already a dataset."
            continue
        fi

        if [[ ! -d "$entry" ]]; then
            continue
        fi

        local folder_size
        folder_size=$(du -sb "$entry" | cut -f1)
        local folder_size_hr
        folder_size_hr=$(du -sh "$entry" | cut -f1)
        log "Processing '${base_entry}' (${folder_size_hr})..."

        if ! check_space "$source_path" "$folder_size"; then
            log_warn "Skipping '${base_entry}' due to insufficient space."
            failed_folders+=("$base_entry")
            continue
        fi

        if [[ "$dry_run" =~ ^[Yy]es$ ]]; then
            log "Dry run: would create dataset '${source_path}/${normalized_name}' from folder '${base_entry}'"
            continue
        fi

        local temp_path="${mount_point}/${source_path}/${normalized_name}_temp"
        if [[ -e "$temp_path" ]]; then
            log_err "Temp path already exists: ${temp_path}. Resolve it manually before retrying '${base_entry}'."
            failed_folders+=("$base_entry")
            continue
        fi
        mv "$entry" "$temp_path" || {
            log_err "Failed to rename '${base_entry}' to temp. Skipping."
            failed_folders+=("$base_entry")
            continue
        }

        if ! zfs create "${source_path}/${normalized_name}"; then
            log_err "Failed to create dataset '${source_path}/${normalized_name}'. Restoring original folder."
            mv "$temp_path" "$entry"
            failed_folders+=("$base_entry")
            continue
        fi

        local target="${mount_point}/${source_path}/${normalized_name}"
        if ! is_zfs_dataset "$target"; then
            log_err "Dataset '${source_path}/${normalized_name}' was created but is not mounted at ${target}. Restoring original folder."
            zfs destroy "${source_path}/${normalized_name}" 2>/dev/null
            rmdir "$target" 2>/dev/null
            mv "$temp_path" "$entry"
            failed_folders+=("$base_entry")
            continue
        fi

        log "Copying data with rsync..."
        rsync "${RSYNC_OPTS[@]}" \
            "${temp_path}/" \
            "${target}/"
        local rsync_rc=$?

        if (( rsync_rc != 0 )); then
            log_err "rsync failed (exit ${rsync_rc}) for '${base_entry}'. Temp dir preserved for manual recovery."
            failed_folders+=("$base_entry")
            continue
        fi

        if ! perform_validation "$temp_path" "${mount_point}/${source_path}/${normalized_name}"; then
            log_err "Keeping temp dir for manual inspection: ${temp_path}"
            failed_folders+=("$base_entry")
            continue
        fi

        if [[ "$cleanup" =~ ^[Yy]es$ ]]; then
            rm -rf "$temp_path"
            log_ok "Cleaned up temp dir."
        else
            log "Cleanup disabled. Temp dir kept: ${temp_path}"
        fi

        converted_folders+=("$base_entry")
        log_ok "Successfully converted '${base_entry}' → dataset '${source_path}/${normalized_name}'"

        refresh_zfs_cache
    done
}

can_i_go_to_work() {
    step "Pre-flight checks"
    refresh_zfs_cache

    if [[ "${#source_datasets_array[@]}" -eq 0 ]]; then
        log_err "No source datasets defined. Check your configuration."
        exit 1
    fi

    local total_folders=0

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

        local count=0
        for entry in "${full_path}"/*; do
            [[ -d "$entry" ]] || continue
            local base
            base=$(basename "$entry")
            [[ "$base" == *_temp ]] && continue
            dataset_exists "${source_path}/${base}" || (( count++ ))
        done

        if (( count == 0 )); then
            log "All children in '${source_path}' are already datasets."
        else
            log "Found ${count} folder(s) to convert in '${source_path}'."
        fi

        (( total_folders += count ))
    done

    if (( total_folders == 0 )); then
        log_ok "Nothing to convert. All folders are already datasets. Exiting."
        exit 0
    fi
}

print_summary() {
    step "Summary"

    if [[ "${#converted_folders[@]}" -gt 0 ]]; then
        log_ok "Successfully converted ${#converted_folders[@]} folder(s):"
        for f in "${converted_folders[@]}"; do
            echo "  + $f"
        done
    fi

    if [[ "${#failed_folders[@]}" -gt 0 ]]; then
        log_err "${#failed_folders[@]} folder(s) failed or were skipped:"
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

convert_all() {
    for dataset in "${source_datasets_array[@]}"; do
        create_datasets "$dataset"
    done
}

if [[ "$should_process_containers" =~ ^[Yy]es$ ]]; then
    source_datasets_array+=("${source_pool_where_appdata_is}/${source_dataset_where_appdata_is}")
    source_path_appdata="${source_pool_where_appdata_is}/${source_dataset_where_appdata_is}"
fi

if [[ "$should_process_vms" =~ ^[Yy]es$ ]]; then
    source_datasets_array+=("${source_pool_where_vm_domains_are}/${source_dataset_where_vm_domains_are}")
    source_path_vms="${source_pool_where_vm_domains_are}/${source_dataset_where_vm_domains_are}"
fi

log "ZFS Dataset Converter starting..."
[[ "$dry_run" =~ ^[Yy]es$ ]] && log_warn "DRY RUN MODE - no changes will be made."

can_i_go_to_work
stop_docker_containers
stop_virtual_machines
convert_all
restore_services
print_summary

log "Script execution completed successfully."
