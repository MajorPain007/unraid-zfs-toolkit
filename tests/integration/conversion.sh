#!/usr/bin/env bash

. "$(dirname "${BASH_SOURCE[0]}")/common.sh"

CALLS="$WORK/calls.log"
DK="$WORK/docker"
VM="$WORK/virsh"
mkdir -p "$DK" "$VM"
export ZDC_DOCKER="$WORK/bin/docker" ZDC_VIRSH="$WORK/bin/virsh" ZDC_RSYNC="$WORK/bin/rsync"

cat > "$WORK/bin/rsync" <<EOF
#!/bin/bash
src="\${@: -2:1}"; dst="\${@: -1}"
echo "rsync \$dst" >> "$CALLS"
case "\$(cat "\$(dirname "\$0")/mode" 2>/dev/null)" in
  fail)    /usr/bin/rsync -a --exclude=b.txt "\$src" "\$dst"; exit 23 ;;
  partial) /usr/bin/rsync -a --exclude=b.txt "\$src" "\$dst"; exit 0 ;;
  slow)    sleep 60; exit 0 ;;
  throttle) exec /usr/bin/rsync --bwlimit=400 "\$@" ;;
  *)       exec /usr/bin/rsync "\$@" ;;
esac
EOF

cat > "$WORK/bin/docker" <<EOF
#!/bin/bash
D="$DK"; C="$CALLS"
EOF
cat >> "$WORK/bin/docker" <<'EOF'
case "$1" in
  ps)
    all=no; for a in "$@"; do [ "$a" = -a ] && all=yes; done
    for f in "$D"/*.state; do [ -e "$f" ] || continue; n=$(basename "$f" .state)
      { [ "$all" = yes ] || [ "$(cat "$f")" = running ]; } && echo "$n"; done ;;
  inspect)
    shift
    if [ "$1" = --format ]; then shift 2; [ "$(cat "$D/$1.state")" = running ] && echo true || echo false; exit 0; fi
    printf '['; sep=''
    for n in "$@"; do
      printf '%s{"Id":"%s","Name":"/%s","State":{"Running":%s},"Mounts":[' "$sep" "$n" "$n" "$([ "$(cat "$D/$n.state")" = running ] && echo true || echo false)"
      s2=''; while IFS= read -r m; do [ -n "$m" ] || continue; printf '%s{"Type":"bind","Source":"%s"}' "$s2" "$m"; s2=','; done < "$D/$n.mounts"
      printf ']}'; sep=','
    done
    printf ']\n' ;;
  stop)
    echo "docker stop $2" >> "$C"
    [ -e "$D/$2.unstoppable" ] && { echo "cannot stop" >&2; exit 1; }
    echo stopped > "$D/$2.state" ;;
  start)
    echo "docker start $2" >> "$C"; echo running > "$D/$2.state" ;;
esac
EOF

cat > "$WORK/bin/virsh" <<EOF
#!/bin/bash
V="$VM"; C="$CALLS"
EOF
cat >> "$WORK/bin/virsh" <<'EOF'
case "$1" in
  list)
    all=no; for a in "$@"; do [ "$a" = --all ] && all=yes; done
    for f in "$V"/*.state; do [ -e "$f" ] || continue; n=$(basename "$f" .state)
      { [ "$all" = yes ] || [ "$(cat "$f")" = running ]; } && echo "$n"; done; echo ;;
  domblklist)
    echo " Type   Device   Target   Source"; echo "------------------------------------------------"
    i=0; while IFS= read -r d; do [ -n "$d" ] && echo " file   disk     hd$i      $d"; i=$((i+1)); done < "$V/$2.disks"
    echo " file   cdrom    hdz      -"; echo ;;
  domstate) [ "$(cat "$V/$2.state")" = running ] && echo running || echo "shut off" ;;
  shutdown) echo "virsh shutdown $2" >> "$C"; [ -e "$V/$2.stubborn" ] || echo off > "$V/$2.state" ;;
  destroy) echo "virsh destroy $2" >> "$C"; echo off > "$V/$2.state" ;;
  start) echo "virsh start $2" >> "$C"; echo running > "$V/$2.state" ;;
esac
EOF
chmod +x "$WORK/bin/rsync" "$WORK/bin/docker" "$WORK/bin/virsh"

rsync_mode() { echo "$1" > "$WORK/bin/mode"; }
container() { local n="$1"; shift; echo running > "$DK/$n.state"; printf '%s\n' "$@" > "$DK/$n.mounts"; }
vm() { local n="$1"; shift; echo running > "$VM/$n.state"; printf '%s\n' "$@" > "$VM/$n.disks"; }
reset_services() { rm -f "$DK"/* "$VM"/* "$CALLS"; touch "$CALLS"; }
called() { grep -qxF -- "$1" "$CALLS"; }
order() { local a b; a=$(grep -nxF -- "$1" "$CALLS" | head -1 | cut -d: -f1); b=$(grep -nxF -- "$2" "$CALLS" | head -1 | cut -d: -f1); [ -n "$a" ] && [ -n "$b" ] && [ "$a" -lt "$b" ]; }

settings() {
    printf 'dry_run=%s\ncleanup=yes\nreplace_spaces=%s\nshould_process_containers=yes\nappdata_pool=%s\nappdata_dataset=appdata\nshould_process_vms=%s\nvm_pool=%s\nvm_dataset=domains\nvm_forceshutdown_wait=10\nbuffer_zone=%s\nvalidation_tolerance=5\nextra_datasets=%s\nsend_notifications=no\n' \
        "${DRY:-no}" "${1:-no}" "$C" "${VMS:-no}" "$C" "${BUFFER:-11}" "${EXTRA:-}" > "$ZDC_CONFIG_DIR/settings.cfg"
}
status_pid() { jq -r .pid "$ZDC_TMP_DIR/status.json"; }
wait_pid() { while kill -0 "$1" 2>/dev/null; do sleep 0.2; done; }
convert() {
    settings "${1:-no}"
    PATH="$WORK/bin:$PATH" "$ZT" convert --auto >/dev/null
    local pid; pid=$(status_pid)
    [ "${2:-wait}" = "wait" ] && wait_pid "$pid"
    echo "$pid"
}
lastlog() { cat "$(ls -t "$ZDC_TMP_DIR"/auto_*.log "$ZDC_TMP_DIR"/conversion_*.log 2>/dev/null | head -1)"; }
status_field() { jq -r "$1" "$ZDC_TMP_DIR/status.json"; }
fresh_appdata() {
    zfs destroy -r "$C/appdata" 2>/dev/null; rm -rf "/mnt/$C/appdata"; zfs create -o compression=off "$C/appdata"
    rm -f "$ZDC_CONVERSION_JOURNAL"; reset_services
}
A="/mnt/$C/appdata"

group "Conversion"
fresh_appdata
zfs create "$C/appdata/tdarr"; mkdir -p "$A/tdarr/configs" "$A/tdarr_temp"
echo keep > "$A/tdarr/configs/config.json"; echo cache > "$A/tdarr_temp/cache.tmp"
for d in plex "Home Assistant" "Büro (alt)" _backup; do mkdir -p "$A/$d"; echo "$d" > "$A/$d/file.txt"; done
truncate -s 1G "$A/plex/disk.img"
ln "$A/plex/file.txt" "$A/plex/hardlink.txt"
rsync_mode normal; convert >/dev/null
check "a plain folder becomes a dataset" 'mounted_at "$A/plex"'
check "a name with a space keeps it" 'zfs list "$C/appdata/Home Assistant" >/dev/null 2>&1 && mounted_at "$A/Home Assistant"'
check "a name ZFS refuses is converted under another and mounted at its own path" \
      'zfs list "$C/appdata/Buero _alt_" >/dev/null 2>&1 && mounted_at "$A/Büro (alt)"'
check "a leading underscore stays" 'mounted_at "$A/_backup"'
check "the contents arrive" '[ "$(cat "$A/Büro (alt)/file.txt")" = "Büro (alt)" ]'
check "a sparse image stays sparse" '[ "$(du -B1 "$A/plex/disk.img" | cut -f1)" -lt 1048576 ]' \
      "$(du -h "$A/plex/disk.img")"
check "hard links stay hard links" '[ "$(stat -c %i "$A/plex/file.txt")" = "$(stat -c %i "$A/plex/hardlink.txt")" ]'
check "<name>_temp next to the dataset <name> is left alone" \
      '[ -f "$A/tdarr_temp/cache.tmp" ] && [ -f "$A/tdarr/configs/config.json" ] && [ ! -f "$A/tdarr/cache.tmp" ]'
check "no journal is left behind" '[ ! -f "$ZDC_CONVERSION_JOURNAL" ]'
check "no other _temp folder is left behind" '[ -z "$(ls -d "$A"/*_temp 2>/dev/null | grep -v tdarr_temp)" ]'
check "the status file reports the run as completed" '[ "$(status_field .status)" = completed ] && [ "$(status_field ".result.converted | length")" = 4 ]'
check "the run is recorded in the history" '[ "$(jq ".runs[0].converted | length" "$ZDC_CONFIG_DIR/conversion_history.json")" = 4 ]'
check "the log ends like the old one" 'lastlog | grep -q "Script execution completed successfully"'

fresh_appdata; mkdir -p "$A/TV Shows"; echo x > "$A/TV Shows/f"
convert yes >/dev/null
check "Replace spaces renames the dataset, not the folder" \
      'zfs list "$C/appdata/TV_Shows" >/dev/null 2>&1 && mounted_at "$A/TV Shows"'

group "A conversion that fails is undone"
for mode in fail partial; do
    fresh_appdata; mkdir -p "$A/app"; echo a > "$A/app/a.txt"; echo b > "$A/app/b.txt"
    rsync_mode "$mode"; convert >/dev/null
    check "rsync $mode: the folder is back, whole, and no dataset is left" \
          '[ -f "$A/app/a.txt" ] && [ -f "$A/app/b.txt" ] && ! zfs list "$C/appdata/app" >/dev/null 2>&1 && [ ! -e "$A/app_temp" ]'
    check "rsync $mode: the run is reported as failed" '[ "$(status_field .status)" = error ] && [ "$(status_field ".result.failed[0]")" = app ]'
done
rsync_mode normal

group "Stop puts the folder back at once"
fresh_appdata; mkdir -p "$A/app"; echo a > "$A/app/a.txt"; echo b > "$A/app/b.txt"
container appc "$A/app/config"
rsync_mode slow; pid=$(convert no nowait); sleep 2
t0=$SECONDS; kill "$pid"; wait_pid "$pid"
check "the converter ends within seconds" '[ $((SECONDS - t0)) -le 5 ]'
check "the folder is back and the dataset gone" \
      '[ -f "$A/app/b.txt" ] && ! zfs list "$C/appdata/app" >/dev/null 2>&1 && [ ! -f "$ZDC_CONVERSION_JOURNAL" ] && [ ! -e "$A/app_temp" ]'
check "the container it stopped runs again" 'called "docker stop appc" && called "docker start appc"'
check "the status says stopped" '[ "$(status_field .status)" = stopped ]'

fresh_appdata; mkdir -p "$A/app"; echo a > "$A/app/a.txt"
container appc "$A/app"
rsync_mode slow; settings; out=$(api convert_start '{"dry_run":"no"}')
pid=$(echo "$out" | jq -r .pid)
check "convert_start starts a detached run" '[ "$(echo "$out" | jq -r .ok)" = true ] && kill -0 "$pid" 2>/dev/null'
sleep 2
check "convert_status shows the folder being copied" '[ "$(api convert_status | jq -r ".items[0].phase")" = copying ]'
check "a second start is refused while one runs" '[ "$(api convert_start | jq -r .ok)" = false ]'
t0=$SECONDS; api convert_stop >/dev/null; wait_pid "$pid"
check "convert_stop ends it within seconds" '[ $((SECONDS - t0)) -le 5 ]'
check "after convert_stop the folder is back and the container runs" \
      '[ -f "$A/app/a.txt" ] && ! zfs list "$C/appdata/app" >/dev/null 2>&1 && [ ! -e "$A/app_temp" ] && [ "$(cat "$DK/appc.state")" = running ]'
rsync_mode normal

group "Docker and VMs"
fresh_appdata
zfs create "$C/appdata/done"; mkdir -p "$A/plex/config" "$A/sonarr"; echo p > "$A/plex/config/x"; echo s > "$A/sonarr/x"
container plexc "$A/plex/config"
container donec "$A/done"
container dockge "$A"
container unrelated "/mnt/$B/elsewhere"
convert >/dev/null
check "a container whose folder is converted is stopped before the copy and restarted after" \
      'order "docker stop plexc" "rsync $A/plex/" && order "rsync $A/plex/" "docker start plexc"'
check "a container whose folder is already a dataset keeps running" '! called "docker stop donec"'
check "a container with a bind mount on the appdata root keeps running" '! called "docker stop dockge"'
check "an unrelated container keeps running" '! called "docker stop unrelated"'
check "everything ends up running" '[ "$(cat "$DK"/*.state | sort -u)" = running ]'

fresh_appdata; mkdir -p "$A/stuck" "$A/free"; echo a > "$A/stuck/a"; echo b > "$A/free/b"
container stuckc "$A/stuck"; touch "$DK/stuckc.unstoppable"
convert >/dev/null
check "a container that cannot be stopped keeps its folder a plain folder" '! mounted_at "$A/stuck" && [ -f "$A/stuck/a" ] && [ ! -e "$A/stuck_temp" ]'
check "the other folders are still converted" 'mounted_at "$A/free"'
check "the run reports the blocked folder as failed" 'lastlog | grep -q "Not converting .stuck.: container stuckc could not be stopped"'

fresh_appdata
zfs destroy -r "$C/domains" 2>/dev/null; rm -rf "/mnt/$C/domains"; zfs create "$C/domains"
D="/mnt/$C/domains"; mkdir -p "$D/Windows 11" "$D/HAOS" "$D/Other"
truncate -s 200M "$D/Windows 11/vdisk1.img"; echo h > "$D/HAOS/haos.qcow2"; echo o > "$D/Other/o"
vm win "$D/Windows 11/vdisk1.img"
vm haos "$D/HAOS/haos.qcow2"; touch "$VM/haos.stubborn"
vm elsewhere "/mnt/$B/vm.img"
t0=$SECONDS; VMS=yes convert >/dev/null
check "the VM domains folder is converted" 'mounted_at "$D/Windows 11" && mounted_at "$D/HAOS" && mounted_at "$D/Other"'
check "a VM whose disk is converted is shut down and started again" \
      'order "virsh shutdown win" "rsync $D/Windows 11/" && order "rsync $D/Windows 11/" "virsh start win"'
check "a VM that does not shut down is forced off after the wait" \
      'called "virsh destroy haos" && called "virsh start haos" && [ $((SECONDS - t0)) -ge 10 ]'
check "a VM on another pool keeps running" '! called "virsh shutdown elsewhere"'
check "the vdisk stays sparse" '[ "$(du -B1 "$D/Windows 11/vdisk1.img" | cut -f1)" -lt 1048576 ]'
zfs destroy -r "$C/domains"

group "Dry run, space, extra datasets, nested mounts"
fresh_appdata; mkdir -p "$A/plex"; echo p > "$A/plex/x"; container plexc "$A/plex"
DRY=yes convert >/dev/null
check "a dry run converts nothing" '! mounted_at "$A/plex" && [ -f "$A/plex/x" ] && [ ! -e "$A/plex_temp" ] && ! zfs list "$C/appdata/plex" >/dev/null 2>&1'
check "a dry run stops no container" '! called "docker stop plexc" && lastlog | grep -q "Dry run: would stop container plexc"'
check "a dry run says what it would do" 'lastlog | grep -q "Dry run: would create dataset .$C/appdata/plex."'
check "a dry run is not recorded in the history" '[ "$(jq "[.runs[] | select(.dry_run)] | length" "$ZDC_CONFIG_DIR/conversion_history.json")" = 0 ]'
settings; out=$(api convert_start '{"dry_run":"yes"}'); wait_pid "$(echo "$out" | jq -r .pid)"
check "convert_start with dry_run=yes runs dry even when the setting is off" '! mounted_at "$A/plex" && [ "$(status_field .dry_run)" = true ]'

fresh_appdata; mkdir -p "$A/huge" "$A/small"; head -c 900M /dev/urandom > "$A/huge/blob"; echo s > "$A/small/s"
BUFFER=100 convert >/dev/null
check "a folder that does not fit with the buffer zone is refused" '! mounted_at "$A/huge" && [ -f "$A/huge/blob" ] && [ ! -e "$A/huge_temp" ]'
check "the refusal is logged" 'lastlog | grep -q "Not enough space on $C/appdata"'
check "a folder that fits is still converted" 'mounted_at "$A/small"'
rm -rf "$A/huge"

fresh_appdata
zfs destroy -r "$C/media" 2>/dev/null; rm -rf "/mnt/$C/media"; zfs create "$C/media"
mkdir -p "/mnt/$C/media/photos" "/mnt/$C/media/My Music"; echo p > "/mnt/$C/media/photos/p.jpg"
EXTRA="$C/media, bad;name" convert >/dev/null
check "folders in an extra dataset are converted" 'mounted_at "/mnt/$C/media/photos" && mounted_at "/mnt/$C/media/My Music"'
check "an invalid extra dataset is skipped with a warning" 'lastlog | grep -q "bad;name. is not a valid dataset name"'
zfs destroy -r "$C/media"

fresh_appdata; mkdir -p "$A/outer/inner" "$A/plain"; echo x > "$A/plain/x"
mount -t tmpfs tmpfs "$A/outer/inner"; echo secret > "$A/outer/inner/s"
convert >/dev/null
check "a folder with a mount inside is left alone" '! mounted_at "$A/outer" && [ -f "$A/outer/inner/s" ] && [ ! -e "$A/outer_temp" ]'
umount "$A/outer/inner"
check "the rest is converted" 'mounted_at "$A/plain"'

group "Scan"
fresh_appdata
zfs create "$C/appdata/plex"; mkdir -p "$A/sonarr" "$A/plex_temp" "$A/Büro (alt)" "$A/TV Shows" "$A/a+b" "$A/a,b"
zfs create -o canmount=noauto "$C/appdata/ghost"; mkdir -p "$A/ghost"
ln -s /tmp "$A/link"
container sonarrc "$A/sonarr"
settings; scan=$(api scan_folders)
st() { echo "$scan" | jq -r --arg n "$1" '.sources[0].folders[] | select(.name == $n) | "\(.state)/\(.reason)/\(.as)"'; }
check "scan: plain folder" '[ "$(st sonarr)" = "convert//" ]'
check "scan: dataset" '[ "$(st plex)" = "dataset//" ]'
check "scan: _temp next to a dataset" '[ "$(st plex_temp)" = "kept/kept_copy/" ]'
check "scan: a name ZFS refuses gets another dataset name" '[ "$(st "Büro (alt)")" = "convert//Buero _alt_" ]'
check "scan: a space is fine" '[ "$(st "TV Shows")" = "convert//" ]'
check "scan: x+y becomes x_y" '[ "$(st a+b)" = "convert//a_b" ]'
check "scan: two folders with the same dataset name" '[ "$(st a,b)" = "blocked/collision/a_b" ]'
check "scan: a dataset that exists but is not mounted there" '[ "$(st ghost)" = "kept/unmounted_dataset/" ]'
check "scan: symlinks are not listed" '[ -z "$(st link)" ]'
check "scan: the container using a folder is shown" '[ "$(echo "$scan" | jq -r ".sources[0].folders[] | select(.name == \"sonarr\") | .users[0].name")" = sonarrc ]'
check "scan: sizes are measured" '[ "$(echo "$scan" | jq -r ".sources[0].folders[] | select(.name == \"sonarr\") | .size")" -ge 0 ]'
scan=$(settings yes; api scan_folders)
check "scan with replace_spaces" '[ "$(st "TV Shows")" = "convert//TV_Shows" ] && [ "$(st "Büro (alt)")" = "convert//Buero__alt_" ]'
settings; ov=$(api overview)
check "overview counts the waiting folders" '[ "$(echo "$ov" | jq -r .conversion.count)" = 4 ] && [ "$(echo "$ov" | jq -r .conversion.status.level)" = warn ]'
out=$(api convert_start "{\"only\":[\"$A/sonarr\"]}"); wait_pid "$(echo "$out" | jq -r .pid)"
check "convert_start with a selection converts only that" 'mounted_at "$A/sonarr" && ! mounted_at "$A/TV Shows"'
zfs destroy "$C/appdata/ghost"

group "A run that was cut off hard is settled by the next"
journal() { printf '%s\n' "$C/appdata" app app "$A/app_temp" > "$ZDC_CONVERSION_JOURNAL"; }

fresh_appdata; mkdir -p "$A/app_temp" "$A/app"; echo a > "$A/app_temp/a.txt"; journal
convert >/dev/null
check "renamed but not copied: the original goes back (over Docker's empty stand-in)" \
      '[ -f "$A/app/a.txt" ] && mounted_at "$A/app" && [ ! -e "$A/app_temp" ]'

fresh_appdata; mkdir -p "$A/app_temp"; echo a > "$A/app_temp/a.txt"; journal
zfs create "$C/appdata/app"; /usr/bin/rsync -a "$A/app_temp/" "$A/app/"
convert >/dev/null
check "copied but not cleaned up: finished" '[ ! -e "$A/app_temp" ] && [ -f "$A/app/a.txt" ] && [ ! -f "$ZDC_CONVERSION_JOURNAL" ]'

fresh_appdata; mkdir -p "$A/app_temp"; echo a > "$A/app_temp/a.txt"; echo half > "$A/app_temp/b.txt"
zfs create "$C/appdata/app"; echo a > "$A/app/a.txt"; echo b > "$A/app/b.txt"; echo c > "$A/app/c.txt"
{ journal; echo committed >> "$ZDC_CONVERSION_JOURNAL"; }
convert >/dev/null
check "validated and cut off while cleaning up: the rest of the original is deleted" \
      '[ ! -e "$A/app_temp" ] && [ -f "$A/app/c.txt" ] && [ ! -f "$ZDC_CONVERSION_JOURNAL" ]'

fresh_appdata; mkdir -p "$A/vm"; echo c > "$A/vm/a.conf"; head -c 10M /dev/urandom > "$A/vm/vdisk1.img"
sum=$(md5sum < "$A/vm/vdisk1.img")
part() { stat -c %s "$A"/vm/.vdisk1.img.* 2>/dev/null || echo 0; }
rsync_mode throttle; pid=$(convert no nowait)
for _ in $(seq 1 600); do [ "$(part)" -ge 10066330 ] && break; sleep 0.1; done
kill -9 "$pid"; pkill -9 -x rsync; wait_pid "$pid"; rsync_mode normal
check "killed hard during rsync: the copy ends in rsync's partial file, the journal is not committed" \
      '[ -f "$A/vm/a.conf" ] && [ ! -e "$A/vm/vdisk1.img" ] && [ "$(part)" -ge 10066330 ] && [ -f "$ZDC_CONVERSION_JOURNAL" ] && ! grep -q committed "$ZDC_CONVERSION_JOURNAL"' \
      "partial file: $(part) bytes"
convert >/dev/null
check "the next run keeps the original and does not take the cut copy as complete" \
      '[ "$(md5sum < "$A/vm_temp/vdisk1.img")" = "$sum" ] && [ -f "$ZDC_CONVERSION_JOURNAL" ] && lastlog | grep -q "Keep one of them"'

fresh_appdata; mkdir -p "$A/app_temp"; echo a > "$A/app_temp/a.txt"; journal
DRY=yes convert >/dev/null
check "a dry run leaves a pending journal alone and says so" '[ -f "$ZDC_CONVERSION_JOURNAL" ] && [ -d "$A/app_temp" ] && lastlog | grep -q "A real run settles that first"'
check "the overview reports the pending journal as an error" '[ "$(api overview | jq -r .conversion.status.level)" = err ]'

fresh_appdata; mkdir -p "$A/app_temp"; echo a > "$A/app_temp/a.txt"; echo b > "$A/app_temp/b.txt"; journal
zfs create "$C/appdata/app"; echo other > "$A/app/a.txt"
mkdir -p "$A/plex"; echo p > "$A/plex/f"
convert >/dev/null
check "copy and original differ: nothing is touched and nothing else converted" \
      '[ -f "$A/app_temp/b.txt" ] && [ "$(cat "$A/app/a.txt")" = other ] && [ -f "$ZDC_CONVERSION_JOURNAL" ] && ! mounted_at "$A/plex"'
check "and the log says what to do" 'lastlog | grep -q "Keep one of them"'

group "Logs"
log=$(api convert_log '{"offset":0}')
check "convert_log returns the newest log" 'echo "$log" | jq -r .text | grep -q "Keep one of them"'
check "convert_log refuses other files" '[ "$(api convert_log "{\"file\":\"/etc/passwd\"}" | jq -r .ok)" = false ]'
for i in $(seq 1 12); do touch -d "-$i hours" "$ZDC_TMP_DIR/auto_2020$(printf %02d "$i")01_000000.log"; done
rm -f "$ZDC_CONVERSION_JOURNAL"; convert >/dev/null
check "at most ten auto logs are kept" '[ "$(ls "$ZDC_TMP_DIR"/auto_*.log | wc -l)" -le 10 ]'
check "the newest one is the run that just happened" 'lastlog | grep -q "ZFS Toolkit starting"'

finish
