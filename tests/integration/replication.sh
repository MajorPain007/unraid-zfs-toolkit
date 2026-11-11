#!/usr/bin/env bash

. "$(dirname "${BASH_SOURCE[0]}")/common.sh"

LOG="$ZDC_TMP_DIR/send.log"
jobs_json() { printf '{"jobs":[%s]}\n' "$1" > "$ZDC_CONFIG_DIR/send_jobs.json"; }
job() {
    local id="$1" src="$2" dst="$3" extra="${4:-}"
    printf '{"id":"%s","enabled":true,"name":"%s","source":"%s","dest":"%s","transport":"local"%s}' "$id" "$id" "$src" "$dst" "$extra"
}
run() { : > "$LOG"; "$ZT" send-run "$@" >"$WORK/out.txt" 2>&1; }
ncp() { zfs list -H -t snapshot -o name -d 1 "$1" 2>/dev/null | grep -c "@zdc-send-$2-"; }
jget() { python3 -c 'import json,sys; d=json.load(sys.stdin); print(eval(sys.argv[1]))' "$1"; }
seen() { grep -q -- "$1" "$LOG"; }

zfs create "$C/appdata"
zfs create "$C/appdata/plex"
zfs create "$C/appdata/Home Assistant"
zfs create "$C/TV Shows"
echo one > "/mnt/$C/appdata/plex/prefs.xml"
echo ha > "/mnt/$C/appdata/Home Assistant/config.yaml"
echo show > "/mnt/$C/TV Shows/episode.mkv"
zfs snapshot -r "$C/appdata@auto-daily-2026-11-01"

group "Save semantics"
out=$(api send_save_job "{\"job\":{\"name\":\"draft\",\"source\":\"$C/appdata\",\"dest\":\"\",\"enabled\":true}}")
check "an incomplete job is kept, disabled" '[ "$(echo "$out" | jget "len(d[\"jobs\"])")" = 1 ] && [ "$(echo "$out" | jget "d[\"job\"][\"enabled\"]")" = False ]' "$out"
out=$(api send_save_job "{\"job\":$(job top "$C/TV Shows" "$B/TV Shows")}")
check "a copy directly below the pool is warned about" 'echo "$out" | grep -q "one level down"' "$out"
for id in $(echo "$out" | jget '" ".join(j["id"] for j in d["jobs"])'); do api send_delete "{\"id\":\"$id\"}" >/dev/null; done
check "removing the last job really empties the list" 'grep -q "\"jobs\": \[\]" "$ZDC_CONFIG_DIR/send_jobs.json"'
out=$(api send_save_job "{\"job\":{\"name\":\"bad\",\"source\":\"$C/appdata\",\"dest\":\"$C/appdata/copy\"}}")
check "a job whose copy lies inside the source is saved disabled with the reason" 'echo "$out" | grep -q "inside the source" && [ "$(echo "$out" | jget "d[\"job\"][\"enabled\"]")" = False ]' "$out"

group "First full send, incrementals, checkpoints"
jobs_json "$(job a "$C/appdata" "$B/replica/appdata" ',"recursive":true,"compressed":true,"keep_dest":2,"readonly":true')"
run a
check "the first run sends in full and creates the missing parent" 'zfs list "$B/replica/appdata/Home Assistant" >/dev/null 2>&1 && seen "Created $B/replica"' "$(tail -5 "$LOG")"
check "the parent it created is not mounted over anything" '[ "$(zfs get -H -o value mounted "$B/replica")" = no ]'
check "the copy is received unmounted and read-only" '[ "$(zfs get -H -o value mounted "$B/replica/appdata")" = no ] && [ "$(zfs get -H -o value readonly "$B/replica/appdata")" = on ]'
check "the old auto snapshot came along" 'zfs list "$B/replica/appdata@auto-daily-2026-11-01" >/dev/null 2>&1'
for i in 2 3 4; do echo "change $i" >> "/mnt/$C/appdata/plex/prefs.xml"; run a; done
check "later runs are incremental" 'seen "incremental from @zdc-send-a-"' "$(tail -5 "$LOG")"
for side in "$C/appdata" "$B/replica/appdata"; do
    counts=$(zfs list -H -t snapshot -o name -r "$side" | grep "@zdc-send-a-" | sed 's/@.*//' | sort | uniq -c | awk '{print $1}' | sort -u)
    check "two checkpoints on every dataset under $side, the one with a space included" '[ "$counts" = 2 ] && [ "$(ncp "$side/Home Assistant" a)" = 2 ]' "counts: $(echo $counts)"
done
zfs mount "$B/replica/appdata/plex"
check "the last change arrived" 'grep -q "change 4" "/mnt/$B/replica/appdata/plex/prefs.xml"'
zfs unmount "$B/replica/appdata/plex"
st=$("$ZT" api send_jobs "{}")
check "the job list state has the run and the job result" '[ "$(echo "$st" | jget "d[\"state\"][\"errors\"]")" = 0 ] && [ "$(echo "$st" | jget "d[\"state\"][\"last_run\"]")" != "" ] && [ "$(echo "$st" | jget "d[\"state\"][\"jobs\"][\"a\"][\"last\"][\"status\"]")" = ok ]' "$st"
check "a per-run log was written" '[ "$(ls "$ZDC_TMP_DIR/send-logs" | grep -c "^a-")" -ge 4 ]'

group "keep_dest"
jobs_json "$(job a "$C/appdata" "$B/replica/appdata" ',"recursive":true,"keep_dest":3')"
for i in 1 2; do run a; done
check "keep_dest 3 keeps three on the destination, two here" '[ "$(ncp "$B/replica/appdata" a)" = 3 ] && [ "$(ncp "$C/appdata" a)" = 2 ]' "dest $(ncp "$B/replica/appdata" a) src $(ncp "$C/appdata" a) $(grep ERROR "$LOG")"
jobs_json "$(job a "$C/appdata" "$B/replica/appdata" ',"recursive":true,"keep_dest":0')"
run a
check "keep_dest 0 never prunes the destination" '[ "$(ncp "$B/replica/appdata" a)" = 4 ]' "dest $(ncp "$B/replica/appdata" a)"
zfs snapshot "$C/appdata@zdc-send-a-x-20200101-000000"
run a
check "a checkpoint of a job whose id only starts the same is left alone" 'zfs list "$C/appdata@zdc-send-a-x-20200101-000000" >/dev/null 2>&1'
zfs destroy "$C/appdata@zdc-send-a-x-20200101-000000"

group "Dry run"
before=$(zfs list -H -t snapshot -o name | wc -l)
jobs_json "$(job a "$C/appdata" "$B/replica/appdata" ',"recursive":true,"allow_rollback":true')"
run --dry-run a
check "dry run shows the commands with Force reaching zfs recv, raw off" 'seen "Would run: zfs send -R -L -e -c -I @zdc-send-a-" && seen "zfs recv -u -x mountpoint -F" && ! seen "send -R -w"' "$(grep Would "$LOG")"
check "dry run estimates the size" 'seen "Estimated stream size"'
check "dry run changes nothing" '[ "$(zfs list -H -t snapshot -o name | wc -l)" = "$before" ]'
out=$("$ZT" api send_preview "{\"id\":\"a\"}")
check "the preview action lists the commands" 'echo "$out" | grep -q "zfs recv -u -x mountpoint -F"' "$out"

group "A dataset with a space, not recursive"
jobs_json "$(job tv "$C/TV Shows" "$B/replica/TV Shows" ',"readonly":false')"
run tv && run tv
check "it replicates" '[ "$(ncp "$B/replica/TV Shows" tv)" -ge 1 ] && seen "incremental"' "$(tail -3 "$LOG")"

group "Force only when allowed"
zfs snapshot "$B/replica/TV Shows@made-there"
echo more >> "/mnt/$C/TV Shows/episode.mkv"
run tv
check "a destination with a newer snapshot stops the job without Force" 'seen "newer than the last common one" && zfs list "$B/replica/TV Shows@made-there" >/dev/null 2>&1' "$(tail -3 "$LOG")"
check "the failed run left no checkpoint behind" '[ "$(ncp "$C/TV Shows" tv)" -le 2 ]'
zfs destroy "$B/replica/TV Shows@made-there"
zfs mount "$B/replica/TV Shows" 2>/dev/null
echo scribble > "/mnt/$B/replica/TV Shows/scribble"
run tv
check "a destination written to stops the job without Force, with advice" 'seen "was written to the destination\|turn on Force once"' "$(tail -3 "$LOG")"
jobs_json "$(job tv "$C/TV Shows" "$B/replica/TV Shows" ',"allow_rollback":true,"readonly":true')"
run tv
check "with Force the destination is rolled back and receives" 'seen "replicated $C/TV Shows" && [ ! -e "/mnt/$B/replica/TV Shows/scribble" ]' "$(tail -3 "$LOG")"
zfs unmount "$B/replica/TV Shows" 2>/dev/null

group "Recovery"
jobs_json "$(job a "$C/appdata" "$B/replica/appdata" ',"recursive":true')"
oldest=$(zfs list -H -t snapshot -o name -s createtxg -d 1 "$C/appdata" | grep "@zdc-send-a-" | head -1)
zfs destroy -r "$oldest"
run a
check "deleting an older checkpoint here does not matter" 'seen "replicated $C/appdata"' "$(tail -3 "$LOG")"
newest=$(zfs list -H -t snapshot -o name -s createtxg -d 1 "$C/appdata" | grep "@zdc-send-a-" | tail -1)
zfs destroy -r "$newest"
run a
check "after the newest checkpoint was deleted here the job names the snapshots in the way" 'seen "newer than the last common one" && seen "${newest#*@}"' "$(tail -3 "$LOG")"
sleep 1.1; run --force a
check "one run with --force rolls the destination back and continues the chain" 'seen "Force for this run" && seen "replicated $C/appdata" && ! zfs list "$B/replica/appdata@${newest#*@}" >/dev/null 2>&1' "$(grep -c "Force for" "$LOG") $newest $(zfs list -H -t snapshot -o name -d 1 "$B/replica/appdata" | tr '\n' ' ')"
jobs_json "$(job a "$C/appdata" "$B/replica/appdata" ',"recursive":true')"
run a
check "the next normal run is incremental again" 'seen "incremental from @zdc-send-a-"' "$(tail -3 "$LOG")"
for s in $(zfs list -H -t snapshot -o name -d 1 "$C/appdata"); do zfs destroy -r "$s"; done
run a
check "with nothing in common the job fails clearly and leaves the copy alone" 'seen "shares no snapshot" && zfs list "$B/replica/appdata/plex" >/dev/null 2>&1' "$(tail -3 "$LOG")"
zfs create "$B/replica/empty"
jobs_json "$(job e "$C/TV Shows" "$B/replica/empty" ',"allow_rollback":true')"
run e
check "an existing dataset without snapshots is never overwritten, even with Force" 'seen "has no snapshots" && zfs list "$B/replica/empty" >/dev/null' "$(tail -3 "$LOG")"

group "A whole pool"
zfs snapshot -r "$C@root1"
jobs_json "$(job pool "$C" "$B/replica/pool" ',"recursive":true')"
run pool
check "a whole pool's copy does not take the pool's mountpoint" \
      '[ "$(zfs get -H -o value mountpoint "$B/replica/pool")" = "/mnt/$B/replica/pool" ] && mounted_at "/mnt/$C"' \
      "$(zfs get -H -o value,source mountpoint "$B/replica/pool" 2>&1) $(tail -2 "$LOG")"

group "Stop during a run"
zfs create "$C/big"
head -c 160M /dev/urandom > "/mnt/$C/big/blob"
jobs_json "$(job big "$C/big" "$B/replica/big" ',"bwlimit":4')"
api send_run '{"job":"big"}' >/dev/null
for _ in $(seq 1 100); do
    b=$("$ZT" api send_jobs "{}" | jget '(d["state"]["progress"] or {}).get("bytes",0)')
    [ "${b:-0}" -gt 0 ] && break
    sleep 0.2
done
check "progress counts bytes while sending" '[ "${b:-0}" -gt 0 ]' "bytes=$b"
out=$(api send_stop)
st=$("$ZT" api send_jobs "{}")
check "stop ends the run" '[ "$(echo "$st" | jget "d[\"state\"][\"running\"]")" = False ] && [ "$(echo "$st" | jget "d[\"state\"][\"jobs\"][\"big\"][\"last\"][\"status\"]")" = cancelled ]' "$out $st"
check "no zfs send or recv is left running" '! pgrep -f "zfs (send|recv).*big" >/dev/null'
check "the unfinished checkpoint is removed and no partial copy remains" '[ "$(ncp "$C/big" big)" = 0 ] && ! zfs list "$B/replica/big" >/dev/null 2>&1' "$(zfs list -t all -r "$B/replica" 2>&1 | grep big)"
zfs destroy -r "$C/big"

group "Over SSH to localhost"
if command -v sshd >/dev/null && ssh -V >/dev/null 2>&1; then
    mkdir -p /root/.ssh && chmod 700 /root/.ssh
    AK=/root/.ssh/authorized_keys
    [ -f "$AK" ] && cp -p "$AK" "$WORK/ak.bak"
    restore_ak() { if [ -f "$WORK/ak.bak" ]; then cp -p "$WORK/ak.bak" "$AK"; else rm -f "$AK"; fi; }
    trap 'restore_ak; cleanup' EXIT
    out=$(api send_key '{"op":"create"}')
    pub=$(echo "$out" | jget 'd["key"]["public"]')
    check "the plugin makes its own key" '[ -f "$ZDC_CONFIG_DIR/id_send" ] && echo "$pub" | grep -q "^ssh-ed25519 "' "$out"
    echo "$pub" >> "$AK"; chmod 600 "$AK"
    SJ="$(printf '{"id":"s","enabled":true,"name":"s","source":"%s","dest":"%s","transport":"ssh","ssh_host":"root@127.0.0.1","ssh_port":22,"ssh_key":"%s","recursive":true,"readonly":true}' "$C/appdata" "$B/remote copy/appdata" "$ZDC_CONFIG_DIR/id_send")"
    jobs_json "$SJ"
    zfs snapshot -r "$C/appdata@fresh"
    out=$("$ZT" api send_test '{"id":"s"}')
    check "the connection test passes" '[ "$(echo "$out" | jget "d[\"ok\"]")" = True ] && echo "$out" | grep -q "SSH login"' "$out"
    run s; run s
    check "replicates over ssh into a name with a space" 'zfs list "$B/remote copy/appdata/Home Assistant" >/dev/null 2>&1 && seen "incremental"' "$(tail -4 "$LOG")"
    check "remote checkpoints are kept per dataset" '[ "$(ncp "$B/remote copy/appdata/Home Assistant" s)" = 2 ]'
    out=$("$ZT" api send_targets '{"job":{"transport":"ssh","ssh_host":"root@127.0.0.1","ssh_port":22,"ssh_key":"'"$ZDC_CONFIG_DIR/id_send"'"}}')
    check "the target list shows what exists over there" 'echo "$out" | grep -q "remote copy/appdata"' "$out"
    ssh-keygen -q -t ed25519 -N "" -f "$WORK/other" >/dev/null
    out=$("$ZT" api send_test "{\"job\":$(echo "$SJ" | sed "s#$ZDC_CONFIG_DIR/id_send#$WORK/other#")}")
    check "a key the server does not know is reported as such" 'echo "$out" | grep -q "did not accept the key"' "$out"
    restore_ak
    check "authorized_keys is restored" '! grep -qF "$pub" "$AK" 2>/dev/null'
else
    echo "  SKIP no ssh/sshd"
fi

group "Overview"
jobs_json "$(job a "$C/appdata" "$B/replica/appdata" ',"recursive":true'),$(job off "$C/appdata" "$B/replica/x" ',"enabled":false')"
out=$("$ZT" api overview "{}")
check "the overview part counts enabled jobs and reports the last failure" '[ "$(echo "$out" | jget "d[\"replication\"][\"count\"]")" = 1 ] && [ "$(echo "$out" | jget "d[\"replication\"][\"status\"][\"level\"]")" = err ]' "$(echo "$out" | jget 'd["replication"]["status"]')"

finish
