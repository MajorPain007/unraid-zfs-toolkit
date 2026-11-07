#!/usr/bin/env bash

. "$(dirname "${BASH_SOURCE[0]}")/common.sh"

command -v jq >/dev/null || { echo "needs jq" >&2; exit 2; }

j() { jq -r "$1"; }
ok_of() { jq -r '.ok'; }
wait_restore() {
    local id="$1" i st
    for i in $(seq 1 120); do
        st=$(api restore_status "{\"id\":\"$id\"}" | j .restore.state)
        case "$st" in done|error|cancelled) echo "$st"; return;; esac
        sleep 0.5
    done
    echo timeout
}
restore() {
    local out id
    out=$(api restore_start "$1")
    id=$(echo "$out" | j '.id // empty')
    [ -n "$id" ] || { echo "start-failed: $(echo "$out" | j .error)"; return; }
    echo "$id" > "$WORK/last_id"
    wait_restore "$id"
}

D="$C/data"
M="/mnt/$D"
zfs create "$D"
mkdir -p "$M/dir/sub" "$M/Büro (alt)"
echo v1 > "$M/dir/a.txt"
echo b > "$M/dir/sub/b.txt"
echo x > "$M/Büro (alt)/x.txt"
ln -s /etc "$M/link"
ln -s dir "$M/inner"
zfs create "$D/child"
echo child > "$M/child/c.txt"
zfs create -o snapdir=hidden "$C/TV Shows"
mkdir -p "/mnt/$C/TV Shows/Show A"
echo ep > "/mnt/$C/TV Shows/Show A/e01.mkv"
zfs snapshot -r "$D@s1"
zfs snapshot "$C/TV Shows@before upgrade"
sleep 1.1
echo v2 > "$M/dir/a.txt"
rm "$M/dir/sub/b.txt"
echo new > "$M/new file.txt"
echo y > "$M/Büro (alt)/y.txt"
zfs snapshot "$D@s2"

group "Datasets and snapshots"
out=$(api browse_datasets)
check "browse_datasets lists the dataset with its snapshots" '[ "$(echo "$out" | j ".datasets[] | select(.name==\"$D\") | .snapshots")" = 2 ]' "$out"
check "browse_datasets lists a dataset with a space" '[ "$(echo "$out" | j ".datasets[] | select(.name==\"$C/TV Shows\") | .snapshots")" = 1 ]'
out=$(api browse_snapshots "{\"dataset\":\"$D\"}")
check "browse_snapshots lists newest first" '[ "$(echo "$out" | j ".snapshots | map(.name) | join(\",\")")" = "s2,s1" ]' "$out"
check "browse_snapshots says the dataset is browsable" '[ "$(echo "$out" | j .browsable)" = true ]'
check "an invalid dataset name is refused" '[ "$(api browse_snapshots "{\"dataset\":\"x;rm -rf /\"}" | ok_of)" = false ]'

group "Browsing"
grep -q "/.zfs/snapshot/s1 " /proc/mounts && umount "$M/.zfs/snapshot/s1"
check "s1 is not automounted yet" '! grep -q "/.zfs/snapshot/s1 " /proc/mounts'
out=$(api browse_list "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"path\":\"/\"}")
check "listing a not yet mounted snapshot works" '[ "$(echo "$out" | ok_of)" = true ]' "$out"
check "the snapshot is mounted afterwards" 'grep -q "/.zfs/snapshot/s1 " /proc/mounts'
check "folders come first" '[ "$(echo "$out" | j ".entries[0].type")" = dir ]'
check "a symlink is listed as a link with its target" '[ "$(echo "$out" | j ".entries[] | select(.name==\"link\") | .type + \" \" + .target")" = "symlink /etc" ]'
check "the file added since is counted" '[ "$(echo "$out" | j ".live_added")" = 1 ]' "$out"
out=$(api browse_list "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"path\":\"/dir\"}")
check "a changed file is flagged against the live data" '[ "$(echo "$out" | j ".entries[] | select(.name==\"a.txt\") | .live")" = changed ]' "$out"
out=$(api browse_list "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"path\":\"/dir/sub\"}")
check "a deleted file is flagged as gone" '[ "$(echo "$out" | j ".entries[] | select(.name==\"b.txt\") | .live")" = gone ]' "$out"
out=$(api browse_list "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"path\":\"/Büro (alt)\"}")
check "a folder with an umlaut and brackets opens" '[ "$(echo "$out" | j ".entries[0].name")" = x.txt ]' "$out"
out=$(api browse_list "{\"dataset\":\"$C/TV Shows\",\"snapshot\":\"before upgrade\",\"path\":\"/Show A\"}")
check "a dataset with a space and snapdir=hidden can be browsed" '[ "$(echo "$out" | j ".entries[0].name")" = e01.mkv ]' "$out"
check "snapdir is unchanged" '[ "$(zfs get -H -o value,source snapdir "$C/TV Shows")" = "hidden	local" ]'

group "Refusals"
for p in "/.." "/dir/../.." "../../etc" "/link" "/link/" "/inner"; do
    out=$(api browse_list "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"path\":\"$p\"}")
    check "browsing $p is refused" '[ "$(echo "$out" | ok_of)" = false ]' "$out"
done
out=$(api browse_list "{\"dataset\":\"$D\",\"snapshot\":\"s1/../../..\",\"path\":\"/\"}")
check "a snapshot name with a slash is refused" '[ "$(echo "$out" | ok_of)" = false ]'
out=$(api browse_list "{\"dataset\":\"$D\",\"snapshot\":\"gone\",\"path\":\"/\"}")
check "a missing snapshot says so" 'echo "$out" | j .error | grep -q "no longer exists"' "$out"
out=$(api restore_preview "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/link/passwd\"]}")
check "restoring through a link is refused" '[ "$(echo "$out" | ok_of)" = false ]' "$out"
for t in /etc /mnt /mnt/../etc "/mnt/$C/data/.zfs/snapshot/s1/x" "../x"; do
    out=$(api restore_preview "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/dir\"],\"target\":\"path\",\"target_path\":\"$t\"}")
    check "restore target $t is refused" '[ "$(echo "$out" | ok_of)" = false ]' "$out"
done
mkdir -p "/mnt/$B/elsewhere"
ln -s "/mnt/$B/elsewhere" "$M/dir/sublink"
out=$(api restore_preview "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/dir/sub/b.txt\"],\"target\":\"path\",\"target_path\":\"dir/sublink\"}")
check "a relative target through a live symlink is refused" '[ "$(echo "$out" | ok_of)" = false ]' "$out"
rm "$M/dir/sublink"
mv "$M/dir/sub" "$M/dir/sub.real"; ln -s "/mnt/$B/elsewhere" "$M/dir/sub"
out=$(api restore_preview "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/dir/sub/b.txt\"]}")
check "restoring in place below a folder that is now a symlink is refused" 'echo "$out" | j .error | grep -q "symbolic link"' "$out"
out=$(api restore_preview "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/dir/sub\"]}")
check "restoring a folder in place over a symlink is refused" 'echo "$out" | j .error | grep -q "symbolic link"' "$out"
rm "$M/dir/sub"; mv "$M/dir/sub.real" "$M/dir/sub"
check "nothing was written outside" '[ -z "$(ls -A "/mnt/$B/elsewhere")" ]'

group "Restore"
out=$(api restore_preview "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/dir\"]}")
check "the preview counts one file to replace and one new" '[ "$(echo "$out" | j ".stats | \"\(.replace) \(.new)\"")" = "1 1" ]' "$out"
check "the preview names the file it would overwrite" '[ "$(echo "$out" | j ".stats.replaced[0].path")" = "$M/dir/a.txt" ]'
st=$(restore "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/dir/sub/b.txt\"]}")
check "restore in place of a deleted file finishes" '[ "$st" = done ]' "$st"
check "the deleted file is back" '[ "$(cat "$M/dir/sub/b.txt")" = b ]'
st=$(restore "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/dir/a.txt\"],\"overwrite\":\"never\"}")
check "overwrite=never keeps the newer live file" '[ "$st" = done ] && [ "$(cat "$M/dir/a.txt")" = v2 ]' "$st"
st=$(restore "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/dir/a.txt\"],\"overwrite\":\"changed\"}")
check "overwrite=changed puts the snapshot's file back" '[ "$st" = done ] && [ "$(cat "$M/dir/a.txt")" = v1 ]' "$st"
echo v3 > "$M/dir/a.txt"; touch -d "$(stat -c %y "$M/.zfs/snapshot/s1/dir/a.txt")" "$M/dir/a.txt"
st=$(restore "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/dir/a.txt\"],\"overwrite\":\"changed\"}")
check "overwrite=changed skips a file with the same size and time" '[ "$st" = done ] && [ "$(cat "$M/dir/a.txt")" = v3 ]' "$st"
st=$(restore "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/dir/a.txt\"],\"overwrite\":\"always\"}")
check "overwrite=always rewrites it anyway" '[ "$st" = done ] && [ "$(cat "$M/dir/a.txt")" = v1 ]' "$st"
st=$(restore "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/dir\",\"/dir/sub\"],\"target\":\"path\",\"target_path\":\"restored\"}")
check "a relative target lands inside the dataset" '[ "$st" = done ] && [ "$(cat "$M/restored/dir/sub/b.txt")" = b ]' "$st"
check "a nested selection is restored once, not twice" '[ ! -e "$M/restored/sub" ]'
st=$(restore "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/dir\",\"/Büro (alt)\"],\"target\":\"path\",\"target_path\":\"/mnt/$B/out put\"}")
check "an absolute target outside the dataset works" '[ "$st" = done ] && [ "$(cat "/mnt/$B/out put/dir/a.txt")" = v1 ] && [ -f "/mnt/$B/out put/Büro (alt)/x.txt" ]' "$st"
check "nothing landed below the dataset (old absolute-path bug)" '[ ! -e "$M/mnt" ]'
st=$(restore "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/link\"],\"target\":\"path\",\"target_path\":\"/mnt/$B/links\"}")
check "a symlink is restored as a link, not followed" '[ "$st" = done ] && [ -L "/mnt/$B/links/link" ] && [ "$(readlink "/mnt/$B/links/link")" = /etc ]' "$st"
out=$(api restore_preview "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/dir/a.txt\",\"/Büro (alt)/x.txt\",\"/dir\"],\"target\":\"path\",\"target_path\":\"/mnt/$B/same\"}")
check "a file inside a selected folder is folded into it" '[ "$(echo "$out" | j ".items | length")" = 2 ]' "$out"
echo stray > "$M/dir/stray.txt"
out=$(api restore_preview "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/dir\"],\"delete\":1}")
check "the delete preview lists the stray file" '[ "$(echo "$out" | j .stats.removed[0])" = "$M/dir/stray.txt" ]' "$out"
st=$(restore "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/dir\"],\"delete\":1}")
check "delete removes files that are not in the snapshot" '[ "$st" = done ] && [ ! -e "$M/dir/stray.txt" ] && [ -f "$M/dir/a.txt" ]' "$st"
echo childnew > "$M/child/new.txt"
st=$(restore "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/\"],\"delete\":1}")
check "restoring the whole dataset in place with delete finishes" '[ "$st" = done ]' "$st"
check "it removed files added since" '[ ! -e "$M/new file.txt" ] && [ ! -e "$M/restored" ]'
check "it left the child dataset alone" '[ -f "$M/child/c.txt" ] && [ -f "$M/child/new.txt" ]'
check "all snapshots survive" '[ "$(zfs list -H -t snapshot -d 1 -o name "$D" | wc -l)" = 2 ]'
zfs create -o snapdir=visible "$C/vis"
echo a > "/mnt/$C/vis/a"; zfs snapshot "$C/vis@v1"; echo b > "/mnt/$C/vis/b"
ls "/mnt/$C/vis/.zfs/snapshot/v1" >/dev/null
st=$(restore "{\"dataset\":\"$C/vis\",\"snapshot\":\"v1\",\"items\":[\"/\"],\"delete\":1}")
check "with snapdir=visible a full restore with delete keeps the snapshot" '[ "$st" = done ] && zfs list "$C/vis@v1" >/dev/null 2>&1 && [ ! -e "/mnt/$C/vis/b" ]' "$st"
check "snapdir=visible is unchanged" '[ "$(zfs get -H -o value snapdir "$C/vis")" = visible ]'

group "Status, stop and overview"
LAST_ID=$(cat "$WORK/last_id")
out=$(api restore_status "{\"id\":\"$LAST_ID\"}")
check "the status reports the finished restore" '[ "$(echo "$out" | j .restore.state)" = done ] && [ "$(echo "$out" | j .restore.percent)" = 100 ]' "$out"
check "the log names the item" 'echo "$out" | j .log | grep -q "dir / ->"' "$out"
check "the status file sits in the tmp dir" '[ -f "$ZDC_TMP_DIR/restores/$LAST_ID.json" ]'
check "restore_list knows it" '[ "$(api restore_list | j ".restores[0].id")" = "$LAST_ID" ]'
check "the overview part reports the last restore" '[ "$(api overview | j .browse.status.level)" = ok ]'
cat > "$WORK/bin/slowrsync" <<'SH'
#!/bin/bash
sleep 30
exec rsync "$@"
SH
chmod +x "$WORK/bin/slowrsync"
out=$(ZDC_RSYNC="$WORK/bin/slowrsync" api restore_start "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"items\":[\"/dir\"],\"target\":\"path\",\"target_path\":\"/mnt/$B/stopped\"}")
id=$(echo "$out" | j .id)
sleep 1.5
check "a running restore shows in the overview" '[ "$(api overview | j .browse.running)" = true ]'
api restore_stop "{\"id\":\"$id\"}" >/dev/null
st=$(wait_restore "$id")
check "a stopped restore ends as cancelled" '[ "$st" = cancelled ]' "$st"
check "the stopped restore wrote nothing" '[ ! -e "/mnt/$B/stopped/dir/a.txt" ]'
check "no rsync is left behind" '! pgrep -f slowrsync >/dev/null'
check "the overview warns about the stopped restore" '[ "$(api overview | j .browse.status.level)" = warn ]'
api restore_dismiss "{\"id\":\"$id\"}" >/dev/null
check "dismissing it clears the warning" '[ "$(api overview | j .browse.status.level)" = ok ]'
fake="20200101-000000-abcd"
printf '{"id":"%s","state":"running","pid":999999,"created":1,"items":[]}' "$fake" > "$ZDC_TMP_DIR/restores/$fake.json"
check "a restore whose process died is reported as failed" '[ "$(api restore_status "{\"id\":\"$fake\"}" | j .restore.state)" = error ]'
check "an invalid restore id is refused" '[ "$(api restore_status "{\"id\":\"../../etc/passwd\"}" | ok_of)" = false ]'

group "Diff"
zfs snapshot "$D@s4"
out=$(api diff_snapshots "{\"dataset\":\"$D\",\"from\":\"s1\",\"to\":\"s2\"}")
check "diff s1 to s2 works" '[ "$(echo "$out" | ok_of)" = true ] && [ "$(echo "$out" | j .reversed)" = false ]' "$out"
check "an added file with a space is decoded" 'echo "$out" | j ".entries[] | select(.kind==\"added\") | .path" | grep -qx "/new file.txt"' "$out"
check "an added file below an umlaut folder is decoded" 'echo "$out" | j ".entries[].path" | grep -qx "/Büro (alt)/y.txt"'
check "a removed file is reported" 'echo "$out" | j ".entries[] | select(.kind==\"removed\") | .path" | grep -qx "/dir/sub/b.txt"'
check "a modified file is reported" 'echo "$out" | j ".entries[] | select(.kind==\"modified\") | .path" | grep -qx "/dir/a.txt"'
rev=$(api diff_snapshots "{\"dataset\":\"$D\",\"from\":\"s2\",\"to\":\"s1\"}")
check "reversed order is put right" '[ "$(echo "$rev" | j .reversed)" = true ] && [ "$(echo "$rev" | j .from)" = "$D@s1" ] && [ "$(echo "$rev" | j .counts.added)" = "$(echo "$out" | j .counts.added)" ]' "$rev"
out=$(api diff_snapshots "{\"dataset\":\"$D\",\"from\":\"s1\",\"to\":\"s2\",\"path\":\"/dir\"}")
check "diff can be limited to a folder" '[ -z "$(echo "$out" | j ".entries[].path" | grep -v "^/dir")" ] && [ "$(echo "$out" | j .total)" -ge 2 ]' "$out"
echo live > "$M/live-only.txt"
out=$(api diff_snapshots "{\"dataset\":\"$D\",\"from\":\"s4\",\"to\":\"\"}")
check "diff against the live data works" '[ "$(echo "$out" | j .live)" = true ] && echo "$out" | j ".entries[].path" | grep -qx "/live-only.txt"' "$out"
check "diff of a snapshot with itself is refused" '[ "$(api diff_snapshots "{\"dataset\":\"$D\",\"from\":\"s1\",\"to\":\"s1\"}" | ok_of)" = false ]'

group "Download"
info=$(echo "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"path\":\"/dir/a.txt\"}" | "$ZT" download-info)
check "download-info describes a file" '[ "$(echo "$info" | j ".name + \" \" + (.size|tostring) + \" \" + (.dir|tostring)")" = "a.txt 3 false" ]' "$info"
echo "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"path\":\"/dir/a.txt\"}" | "$ZT" download > "$WORK/a.dl"
check "download streams the file" 'cmp -s "$WORK/a.dl" "$M/.zfs/snapshot/s1/dir/a.txt"'
info=$(echo "{\"dataset\":\"$C/TV Shows\",\"snapshot\":\"before upgrade\",\"path\":\"/Show A\"}" | "$ZT" download-info)
check "download-info names a folder as zip" '[ "$(echo "$info" | j ".name + \" \" + (.dir|tostring)")" = "Show A.zip true" ]' "$info"
echo "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"path\":\"/\"}" | "$ZT" download > "$WORK/all.zip"
names=$(python3 -c 'import sys,zipfile; print("\n".join(zipfile.ZipFile(sys.argv[1]).namelist()))' "$WORK/all.zip")
check "a folder downloads as a zip with its files" 'echo "$names" | grep -qx "data@s1/dir/sub/b.txt" && echo "$names" | grep -qx "data@s1/Büro (alt)/x.txt"' "$names"
check "the zip holds the link, not /etc" 'echo "$names" | grep -qx "data@s1/link" && ! echo "$names" | grep -q "passwd"'
info=$(echo "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"path\":\"/link\"}" | "$ZT" download-info)
check "downloading a symlink is refused" '[ "$(echo "$info" | ok_of)" = false ]' "$info"
info=$(echo "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"path\":\"/../../etc/passwd\"}" | "$ZT" download-info)
check "downloading with .. is refused" '[ "$(echo "$info" | ok_of)" = false ]' "$info"
check "download of a refused path exits non-zero" '! echo "{\"dataset\":\"$D\",\"snapshot\":\"s1\",\"path\":\"/link/passwd\"}" | "$ZT" download >/dev/null 2>&1'

finish
