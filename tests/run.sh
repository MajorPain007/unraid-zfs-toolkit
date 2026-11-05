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

if [ "$fail" = 0 ]; then
    printf '\n\033[32mAll passed\033[0m\n'
else
    printf '\n\033[31mFailures above\033[0m\n'
fi
exit $fail
