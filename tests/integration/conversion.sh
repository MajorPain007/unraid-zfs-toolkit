#!/usr/bin/env bash

. "$(dirname "${BASH_SOURCE[0]}")/common.sh"
SRC="$ROOT/src/scripts"

cat > "$WORK/bin/rsync" <<'EOF'
#!/bin/bash
src="${@: -2:1}"; dst="${@: -1}"
case "$(cat "$(dirname "$0")/mode" 2>/dev/null)" in
  fail)    /usr/bin/rsync -a --exclude=b.txt "$src" "$dst"; exit 23 ;;
  partial) /usr/bin/rsync -a --exclude=b.txt "$src" "$dst"; exit 0 ;;
  slow)    sleep 60; exit 0 ;;
  *)       exec /usr/bin/rsync "$@" ;;
esac
EOF
chmod +x "$WORK/bin/rsync"
rsync_mode() { echo "$1" > "$WORK/bin/mode"; }

convert() {
    printf 'dry_run=no\ncleanup=yes\nreplace_spaces=%s\nshould_process_containers=yes\nappdata_pool=%s\nappdata_dataset=appdata\nshould_process_vms=no\n' \
        "${1:-no}" "$C" > "$ZDC_CONFIG_DIR/settings.cfg"
    PATH="$WORK/bin:$PATH" bash "$SRC/run_auto.sh" >/dev/null
    local pid; pid=$(grep -o '"pid":[0-9]*' "$ZDC_TMP_DIR/status.json" | grep -o '[0-9]*')
    [ "${2:-wait}" = "wait" ] && while kill -0 "$pid" 2>/dev/null; do sleep 0.2; done
    echo "$pid"
}
lastlog() { cat "$(ls -t "$ZDC_TMP_DIR"/auto_*.log | head -1)"; }
fresh_appdata() {
    zfs destroy -r "$C/appdata" 2>/dev/null; rm -rf "/mnt/$C/appdata"; zfs create -o compression=off "$C/appdata"
    rm -f "$ZDC_CONVERSION_JOURNAL"
}
mounted_at() { [ "$(zfs list -H -o mounted,mountpoint 2>/dev/null | awk -F'\t' -v p="$1" '$1=="yes" && $2==p' | wc -l)" = 1 ]; }
A="/mnt/$C/appdata"

group "Conversion"
fresh_appdata
zfs create "$C/appdata/tdarr"; mkdir -p "$A/tdarr/configs" "$A/tdarr_temp"
echo keep > "$A/tdarr/configs/config.json"; echo cache > "$A/tdarr_temp/cache.tmp"
for d in plex "Home Assistant" "Büro (alt)" _backup; do mkdir -p "$A/$d"; echo "$d" > "$A/$d/file.txt"; done
truncate -s 1G "$A/plex/disk.img"
rsync_mode normal; convert >/dev/null
check "a plain folder becomes a dataset" 'mounted_at "$A/plex"'
check "a name with a space keeps it" 'zfs list "$C/appdata/Home Assistant" >/dev/null 2>&1 && mounted_at "$A/Home Assistant"'
check "a name ZFS refuses is converted under another and mounted at its own path" \
      'zfs list "$C/appdata/Buero _alt_" >/dev/null 2>&1 && mounted_at "$A/Büro (alt)"'
check "a leading underscore stays" 'mounted_at "$A/_backup"'
check "the contents arrive" '[ "$(cat "$A/Büro (alt)/file.txt")" = "Büro (alt)" ]'
check "a sparse image stays sparse" '[ "$(du -B1 "$A/plex/disk.img" | cut -f1)" -lt 1048576 ]' \
      "$(du -h "$A/plex/disk.img")"
check "<name>_temp next to the dataset <name> is left alone" \
      '[ -f "$A/tdarr_temp/cache.tmp" ] && [ -f "$A/tdarr/configs/config.json" ] && [ ! -f "$A/tdarr/cache.tmp" ]'
check "no journal is left behind" '[ ! -f "$ZDC_CONVERSION_JOURNAL" ]'

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
done

group "Stop puts the folder back at once"
fresh_appdata; mkdir -p "$A/app"; echo a > "$A/app/a.txt"; echo b > "$A/app/b.txt"
rsync_mode slow; pid=$(convert no nowait); sleep 2
t0=$SECONDS; kill "$pid"; pkill -TERM -P "$pid"; while kill -0 "$pid" 2>/dev/null; do sleep 0.2; done
check "the converter ends within seconds" '[ $((SECONDS - t0)) -le 5 ]'
check "the folder is back and the dataset gone" \
      '[ -f "$A/app/b.txt" ] && ! zfs list "$C/appdata/app" >/dev/null 2>&1 && [ ! -f "$ZDC_CONVERSION_JOURNAL" ]'

group "A run that was cut off hard is settled by the next"
journal() { printf '%s\n' "$C/appdata" app app "$A/app_temp" > "$ZDC_CONVERSION_JOURNAL"; }
rsync_mode normal

fresh_appdata; mkdir -p "$A/app_temp" "$A/app"; echo a > "$A/app_temp/a.txt"; journal
convert >/dev/null
check "renamed but not copied: the original goes back (over Docker's empty stand-in)" \
      '[ -f "$A/app/a.txt" ] && mounted_at "$A/app" && [ ! -e "$A/app_temp" ]'

fresh_appdata; mkdir -p "$A/app_temp"; echo a > "$A/app_temp/a.txt"; journal
zfs create "$C/appdata/app"; /usr/bin/rsync -a "$A/app_temp/" "$A/app/"
convert >/dev/null
check "copied but not cleaned up: finished" '[ ! -e "$A/app_temp" ] && [ -f "$A/app/a.txt" ] && [ ! -f "$ZDC_CONVERSION_JOURNAL" ]'

fresh_appdata; mkdir -p "$A/app_temp"; echo a > "$A/app_temp/a.txt"; echo b > "$A/app_temp/b.txt"; journal
zfs create "$C/appdata/app"; echo other > "$A/app/a.txt"
mkdir -p "$A/plex"; echo p > "$A/plex/f"
convert >/dev/null
check "copy and original differ: nothing is touched and nothing else converted" \
      '[ -f "$A/app_temp/b.txt" ] && [ "$(cat "$A/app/a.txt")" = other ] && [ -f "$ZDC_CONVERSION_JOURNAL" ] && ! mounted_at "$A/plex"'
check "and the log says what to do" 'lastlog | grep -q "Keep one of them"'

finish
