#!/bin/bash

ZDC_NAME="zfs.dataset.converter"
ZDC_CONFIG_DIR="${ZDC_CONFIG_DIR:-/boot/config/plugins/${ZDC_NAME}}"
ZDC_SETTINGS="${ZDC_SETTINGS:-${ZDC_CONFIG_DIR}/settings.cfg}"
ZDC_TMP_DIR="${ZDC_TMP_DIR:-/tmp/${ZDC_NAME}}"
ZDC_CRON_DIR="${ZDC_CRON_DIR:-/etc/cron.d}"

mkdir -p "$ZDC_TMP_DIR" 2>/dev/null

declare -gA CFG

zdc_load_cfg() {
    local file="${1:-$ZDC_SETTINGS}" line key val
    [ -f "$file" ] || return 0
    while IFS= read -r line || [ -n "$line" ]; do
        line="${line%$'\r'}"
        [[ "$line" =~ ^[[:space:]]*(#|$) ]] && continue
        [[ "$line" != *=* ]] && continue
        key="${line%%=*}"
        val="${line#*=}"
        key="${key#"${key%%[![:space:]]*}"}"; key="${key%"${key##*[![:space:]]}"}"
        val="${val#"${val%%[![:space:]]*}"}"; val="${val%"${val##*[![:space:]]}"}"
        [[ "$key" =~ ^[A-Za-z0-9_]+$ ]] || continue
        CFG["$key"]="$val"
    done < "$file"
}

zdc_cfg() {
    local key="$1" def="${2:-}"
    if [ -n "${CFG[$key]+x}" ] && [ -n "${CFG[$key]}" ]; then
        printf '%s' "${CFG[$key]}"
    else
        printf '%s' "$def"
    fi
}

zdc_valid_cron() {
    local expr="$1" fields
    [ -n "$expr" ] || return 1
    case "$expr" in
        *$'\n'*|*$'\r'*|*';'*|*'&'*|*'|'*|*'$'*|*'`'*|*'('*|*')'*|*'<'*|*'>'*) return 1 ;;
    esac
    [[ "$expr" =~ ^[0-9A-Za-z*/,\ -]+$ ]] || return 1
    fields=$(awk '{print NF}' <<< "$expr")
    [ "$fields" = "5" ] || return 1
    return 0
}

zdc_valid_dataset() {
    local name="$1"
    [ -n "$name" ] || return 1
    [[ "$name" =~ ^[A-Za-z0-9][A-Za-z0-9_.:-]*(/[A-Za-z0-9_.:-]+)*$ ]] || return 1
    return 0
}

zdc_valid_int() {
    [[ "$1" =~ ^[0-9]+$ ]]
}

zdc_remove_cron() {
    local cronfile="${ZDC_CRON_DIR}/$1" pattern="$2"
    rm -f "$cronfile"
    if crontab -l 2>/dev/null | grep -q -- "$pattern"; then
        crontab -l 2>/dev/null | grep -v -- "$pattern" | grep -v '^# zfs-dataset-converter' | crontab - 2>/dev/null
    fi
    zdc_refresh_cron
}

zdc_refresh_cron() {
    local refreshed=0
    if command -v update_cron >/dev/null 2>&1; then
        update_cron >/dev/null 2>&1 && refreshed=1
    fi
    if [ "$refreshed" = "0" ] && [ -x /etc/rc.d/rc.crond ]; then
        /etc/rc.d/rc.crond restart >/dev/null 2>&1 && refreshed=1
    fi
    return 0
}

zdc_install_cron() {
    local base="$1" expr="$2" cmd="$3" pattern="$4"
    local cronfile="${ZDC_CRON_DIR}/${base}"

    if ! zdc_valid_cron "$expr"; then
        echo "ERROR: refusing to install invalid cron expression: '${expr}'" >&2
        return 1
    fi

    if crontab -l 2>/dev/null | grep -q -- "$pattern"; then
        crontab -l 2>/dev/null | grep -v -- "$pattern" | grep -v '^# zfs-dataset-converter' | crontab - 2>/dev/null
    fi

    mkdir -p "$ZDC_CRON_DIR"
    local tmp
    tmp=$(mktemp "${ZDC_CRON_DIR}/.${base}.XXXXXX") || { echo "ERROR: mktemp failed" >&2; return 1; }
    {
        echo "# Managed by ${ZDC_NAME} - edit via Settings > ZFS Dataset Converter"
        echo "${expr} ${cmd}"
    } > "$tmp"
    chmod 0644 "$tmp"
    mv -f "$tmp" "$cronfile"

    zdc_refresh_cron

    if crontab -l 2>/dev/null | grep -q -- "$pattern"; then
        echo "Cron installed via ${cronfile}: ${expr}"
        return 0
    fi

    ( crontab -l 2>/dev/null; echo "# zfs-dataset-converter"; echo "${expr} ${cmd}" ) | crontab - 2>/dev/null
    if crontab -l 2>/dev/null | grep -q -- "$pattern"; then
        echo "Cron installed via crontab fallback: ${expr} (${cronfile} was not picked up)"
        return 0
    fi

    echo "ERROR: cron entry could not be installed (${expr})" >&2
    return 1
}

zdc_trim_log() {
    local file="$1" max="${2:-1048576}" keep="${3:-2000}" size
    [ -f "$file" ] || return 0
    size=$(stat -c %s "$file" 2>/dev/null || echo 0)
    [ "$size" -le "$max" ] 2>/dev/null && return 0
    local tmp="${file}.trim.$$"
    if tail -n "$keep" "$file" > "$tmp" 2>/dev/null; then
        printf '[%s] --- log trimmed to last %s lines ---\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$keep" \
            | cat - "$tmp" > "${tmp}.2" 2>/dev/null && mv -f "${tmp}.2" "$file"
    fi
    rm -f "$tmp" "${tmp}.2" 2>/dev/null
}

zdc_prune_old_logs() {
    local dir="$1" glob="$2" keep="${3:-10}"
    [ -d "$dir" ] || return 0
    local old
    old=$(ls -1t "${dir}"/${glob} 2>/dev/null | tail -n +$((keep + 1)))
    [ -n "$old" ] && echo "$old" | xargs -r rm -f 2>/dev/null
    return 0
}

zdc_notify() {
    local subject="$1" message="$2" importance="${3:-normal}"
    local notify=/usr/local/emhttp/webGui/scripts/notify
    [ -x "$notify" ] || return 0
    "$notify" -e "ZFS Dataset Converter" -s "$subject" -d "$message" -i "$importance" >/dev/null 2>&1 || true
}

zdc_zfs_ready() {
    command -v zfs >/dev/null 2>&1 || return 1
    command -v zpool >/dev/null 2>&1 || return 1
    local pools
    pools=$(zpool list -H -o name 2>/dev/null)
    [ -n "$pools" ] || return 1
    return 0
}

zdc_pool_capacity() {
    local pool="${1%%/*}"
    zpool list -H -o capacity "$pool" 2>/dev/null | tr -d '%'
}

zdc_pool_free_bytes() {
    local pool="${1%%/*}"
    zpool list -Hp -o free "$pool" 2>/dev/null
}

zdc_pool_size_bytes() {
    local pool="${1%%/*}"
    zpool list -Hp -o size "$pool" 2>/dev/null
}

zdc_to_bytes() {
    local spec total num unit
    spec=$(printf '%s' "${1:-}" | tr -d '[:space:]' | tr '[:lower:]' '[:upper:]')
    total="${2:-0}"
    [ -n "$spec" ] || return 1

    if [[ "$spec" =~ ^([0-9]+)%$ ]]; then
        [ "$total" -gt 0 ] 2>/dev/null || return 1
        printf '%s' $(( total * ${BASH_REMATCH[1]} / 100 ))
        return 0
    fi

    [[ "$spec" =~ ^([0-9]+)([KMGTP]?)I?B?$ ]] || return 1
    num="${BASH_REMATCH[1]}"; unit="${BASH_REMATCH[2]}"
    case "$unit" in
        K) printf '%s' $(( num * 1024 )) ;;
        M) printf '%s' $(( num * 1024 ** 2 )) ;;
        G) printf '%s' $(( num * 1024 ** 3 )) ;;
        T) printf '%s' $(( num * 1024 ** 4 )) ;;
        P) printf '%s' $(( num * 1024 ** 5 )) ;;
        *) printf '%s' "$num" ;;
    esac
}

zdc_fmt_bytes() {
    local b="${1:-0}"
    zdc_valid_int "$b" || { printf '%s' "-"; return; }
    numfmt --to=iec-i --suffix=B "$b" 2>/dev/null || printf '%sB' "$b"
}

zdc_valid_snapshot() {
    local s="$1"
    [[ "$s" == *@* ]] || return 1
    zdc_valid_dataset "${s%%@*}" || return 1
    [[ "${s#*@}" =~ ^[A-Za-z0-9][A-Za-z0-9_.:-]*$ ]] || return 1
    return 0
}

zdc_lock() {
    local name="$1"
    command -v flock >/dev/null 2>&1 || return 0
    exec 9>"${ZDC_TMP_DIR}/${name}.lock"
    flock -n 9
}
