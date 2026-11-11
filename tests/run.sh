#!/bin/bash
set -u
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT" || exit 1
fail=0
section() { printf '\n\033[1m%s\033[0m\n' "$1"; }

section "Go"
GO=$(command -v go || echo /usr/local/go/bin/go)
unformatted=$(gofmt -l cmd internal)
if [ -n "$unformatted" ]; then
    echo "  FAIL not gofmt-formatted: $unformatted"
    fail=1
else
    echo "  PASS gofmt"
fi
GOOS=linux "$GO" vet ./... || fail=1
"$GO" test ./internal/zt/ -count=1 || fail=1

section "Web interface"
( cd web && { [ -d node_modules ] || npm ci --silent; } && npm run --silent check ) || fail=1

section "PHP syntax"
if command -v php >/dev/null 2>&1; then
    while IFS= read -r f; do
        out=$(php -l "$f" 2>&1) || { echo "  $out"; fail=1; }
    done < <(find src/include src -maxdepth 1 -name '*.php' -o -maxdepth 1 -name '*.page'; find src/include -name '*.php')
    echo "  checked"
else
    echo "  SKIP php not installed"
fi

section "Pages and manifest"
for p in src/*.page; do
    if ! grep -q '^Markdown="false"' "$p"; then
        echo "  FAIL $p lacks Markdown=\"false\": Unraid would run it through Markdown"
        fail=1
    fi
done
echo "  PASS every page sets Markdown=\"false\""
if command -v php >/dev/null 2>&1; then
    if php -r 'exit(simplexml_load_file($argv[1]) === false ? 1 : 0);' src/zfs.toolkit.plg 2>/dev/null; then
        echo "  PASS the manifest is well-formed XML"
    else
        echo "  FAIL the manifest is not well-formed XML"
        fail=1
    fi
fi
for base in $(grep -ho 'Base: *Name *+ *"[^"]*"\|Base: *Name,' internal/zt/crontab.go | sed -e 's/Base: *Name *+ *"\([^"]*\)"/\1/' -e 's/Base: *Name,//'); do
    if ! grep -q "/etc/cron.d/&name;${base}" src/zfs.toolkit.plg; then
        echo "  FAIL the uninstall block does not remove /etc/cron.d/zfs.toolkit${base}"
        fail=1
    fi
done
echo "  PASS the uninstall block knows every cron file"
for s in src/event/*; do
    bash -n "$s" || fail=1
done
echo "  PASS event hooks parse"

section "Uninstall"
uninstall() {
    local sb="$1" c
    mkdir -p "$sb/cfg" "$sb/tmp" "$sb/plugin/bin" "$sb/bin" "$sb/etc/cron.d" "$sb/run"
    for c in crontab removepkg update_cron; do printf '#!/bin/sh\nexit 0\n' > "$sb/bin/$c"; done
    printf '#!/bin/sh\necho "$*" >> "%s/syslog"\n' "$sb" > "$sb/bin/logger"
    printf '#!/bin/sh\ntrap '\''mkdir -p "%s/cfg"; echo late > "%s/cfg/late"; exit 0'\'' TERM\n: > "%s/ready"\nwhile :; do sleep 1; done\n' "$sb" "$sb" "$sb" > "$sb/plugin/bin/zfs-toolkit"
    chmod +x "$sb"/bin/* "$sb/plugin/bin/zfs-toolkit"
    for c in settings.cfg send_jobs.json id_send id_send.pub send_known_hosts snap_datasets.json conversion_history.json zfs.toolkit-snapshots.cron zfs.toolkit-2026.11.07.04-x86_64-1.txz; do
        echo x > "$sb/cfg/$c"
    done
    echo log > "$sb/tmp/send.log"
    "$sb/plugin/bin/zfs-toolkit" snapshot-tick >/dev/null 2>&1 &
    until [ -e "$sb/ready" ]; do sleep 0.1; done
    sed -n '/Method="remove"/,/<\/INLINE>/p' src/zfs.toolkit.plg | sed '1,2d;$d' \
        | sed -e "s#&configDir;#$sb/cfg#g" -e "s#&tmpDir;#$sb/tmp#g" -e "s#&pluginDir;#$sb/plugin#g" -e 's#&name;#zfs.toolkit#g' \
              -e "s#/etc/#$sb/etc/#g" -e "s#/var/run/#$sb/run/#g" -e 's#&gt;#>#g' -e 's#&lt;#<#g' -e 's#&amp;#\&#g' > "$sb/remove.sh"
    PATH="$sb/bin:$PATH" bash "$sb/remove.sh" > "$sb/out" 2>&1
}
ucheck() {
    if eval "$2"; then echo "  PASS $1"; else echo "  FAIL $1"; sed 's/^/       /' "$sb/out"; fail=1; fi
}
sb=$(mktemp -d)
uninstall "$sb"
ucheck "uninstall deletes the whole config folder, SSH key included" '[ ! -e "$sb/cfg" ]'
ucheck "uninstall stops running zfs-toolkit processes first" '! pgrep -f "$sb/plugin/bin/zfs-toolkit" >/dev/null'
ucheck "uninstall deletes the RAM folder" '[ ! -e "$sb/tmp" ]'
ucheck "the message names what was deleted" 'grep -q "$sb/cfg" "$sb/out" && grep -qi "ssh key" "$sb/out" && ! grep -qi preserved "$sb/out"'
pkill -f "$sb/plugin/bin/zfs-toolkit" 2>/dev/null
rm -rf "$sb"
sb=$(mktemp -d)
mkdir -p "$sb/cfg"
printf 'cache/appdata\nplex\nplex\n/mnt/cache/appdata/plex_temp\n' > "$sb/cfg/conversion_in_progress"
uninstall "$sb"
ucheck "an interrupted conversion keeps only its journal" '[ "$(ls -A "$sb/cfg")" = conversion_in_progress ]'
ucheck "the message names the original folder of the interrupted conversion" 'grep -q "/mnt/cache/appdata/plex_temp" "$sb/out"'
ucheck "the journal goes to the syslog" 'grep -q "/mnt/cache/appdata/plex_temp" "$sb/syslog"'
pkill -f "$sb/plugin/bin/zfs-toolkit" 2>/dev/null
rm -rf "$sb"

if [ "$fail" = 0 ]; then
    printf '\n\033[32mAll passed\033[0m\n'
else
    printf '\n\033[31mFailures above\033[0m\n'
fi
exit $fail
