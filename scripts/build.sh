#!/bin/bash

set -euo pipefail

PLUGIN="${ZDC_PLUGIN_NAME:-zfs.toolkit}"
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${1:-$(date '+%Y.%m.%d').01}"
OUT_DIR="${ZDC_OUT_DIR:-${ROOT_DIR}/archive}"
case "$OUT_DIR" in
    /*) ;;
    *) OUT_DIR="${ROOT_DIR}/${OUT_DIR}" ;;
esac
PLG="${ROOT_DIR}/src/zfs.toolkit.plg"

if [[ ! "$VERSION" =~ ^[0-9]{4}\.[0-9]{2}\.[0-9]{2}\.[0-9]+$ ]]; then
    echo "ERROR: version must look like YYYY.MM.DD.NN (got '${VERSION}')" >&2
    exit 1
fi

echo "Building ${PLUGIN}-${VERSION}-x86_64-1.txz ..."

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT
PDIR="${STAGE}/usr/local/emhttp/plugins/${PLUGIN}"
mkdir -p "${PDIR}/include" "${PDIR}/bin" "${PDIR}/assets" "${STAGE}/install"

( cd "${ROOT_DIR}" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/MajorPain007/unraid-zfs-toolkit/internal/zt.buildName=${PLUGIN}" \
    -o "${PDIR}/bin/zfs-toolkit" ./cmd/zfs-toolkit )

( cd "${ROOT_DIR}/web" && { [ -d node_modules ] || npm ci --silent; } && npm run --silent build >/dev/null ) \
    || { echo "the web interface did not build" >&2; exit 1; }

cp "${ROOT_DIR}/src/"*.page "${ROOT_DIR}/src/README.md" "${PDIR}/"
cp "${ROOT_DIR}/src/include/page.php" "${ROOT_DIR}/src/include/api.php" "${PDIR}/include/"
cp "${ROOT_DIR}/web/dist/app.js" "${ROOT_DIR}/web/dist/app.css" "${PDIR}/assets/"
cp -R "${ROOT_DIR}/src/event" "${PDIR}/"
find "${PDIR}" -name '.DS_Store' -delete

if [ "$PLUGIN" != "zfs.toolkit" ]; then
    WORD="${PLUGIN#zfs.toolkit.}"
    SUFFIX="$(printf '%s' "${WORD:0:1}" | tr '[:lower:]' '[:upper:]')${WORD:1}"
    for f in "${PDIR}/"*.page; do
        sed -i.bak -e "s|zfs\.toolkit|${PLUGIN}|g" -e "s|^Title=\"\(.*\)\"|Title=\"\1 ${SUFFIX}\"|" "$f"
        rm -f "$f.bak"
        mv "$f" "${f%.page}${SUFFIX}.page"
    done
    for f in "${PDIR}/include/page.php" "${PDIR}/event/"* "${PDIR}/README.md"; do
        sed -i.bak "s|zfs\.toolkit|${PLUGIN}|g" "$f"
        rm -f "$f.bak"
    done
fi

find "${PDIR}" -type d -exec chmod 755 {} +
find "${PDIR}" -type f -exec chmod 644 {} +
chmod 755 "${PDIR}/event/"* "${PDIR}/bin/zfs-toolkit"

for s in "${PDIR}/event/"*; do
    bash -n "$s" || { echo "SYNTAX ERROR in $s" >&2; exit 1; }
done
if command -v php >/dev/null 2>&1; then
    while IFS= read -r f; do
        php -l "$f" >/dev/null || { echo "PHP SYNTAX ERROR in $f" >&2; exit 1; }
    done < <(find "${PDIR}" -name '*.php' -o -name '*.page')
fi

cat > "${STAGE}/install/slack-desc" << EOF2
${PLUGIN}: ZFS Toolkit (MajorPain007)
${PLUGIN}:
${PLUGIN}: Converts regular folders to ZFS child datasets, manages native
${PLUGIN}: snapshots with retention, browses and restores them, and
${PLUGIN}: replicates datasets with zfs send/recv.
${PLUGIN}:
EOF2

mkdir -p "${OUT_DIR}"
PKG="${PLUGIN}-${VERSION}-x86_64-1.txz"
( cd "${STAGE}" && COPYFILE_DISABLE=1 tar --owner=0 --group=0 --no-xattrs -cJf "${OUT_DIR}/${PKG}" install/ usr/ 2>/dev/null \
  || COPYFILE_DISABLE=1 tar --no-xattrs -cJf "${OUT_DIR}/${PKG}" install/ usr/ )

if command -v md5sum >/dev/null 2>&1; then
    MD5=$(md5sum "${OUT_DIR}/${PKG}" | cut -d' ' -f1)
else
    MD5=$(md5 -q "${OUT_DIR}/${PKG}")
fi

if [ "$PLUGIN" = "zfs.toolkit" ] && [ -f "$PLG" ] && [ "${ZDC_SKIP_PLG:-0}" != "1" ]; then
    tmp="${PLG}.tmp"
    sed -e "s|<!ENTITY version   \"[^\"]*\">|<!ENTITY version   \"${VERSION}\">|" \
        -e "s|<!ENTITY md5       \"[^\"]*\">|<!ENTITY md5       \"${MD5}\">|" \
        "$PLG" > "$tmp"
    mv "$tmp" "$PLG"
    echo "  Updated ${PLG#"$ROOT_DIR"/}"
    find "${OUT_DIR}" -maxdepth 1 -name "${PLUGIN}-*-x86_64-1.txz" ! -name "${PKG}" -delete
elif [ "$PLUGIN" != "zfs.toolkit" ]; then
    sed -e "s|<!ENTITY name      \"zfs.toolkit\">|<!ENTITY name      \"${PLUGIN}\">|" \
        -e "s|<!ENTITY version   \"[^\"]*\">|<!ENTITY version   \"${VERSION}\">|" \
        -e "s|<!ENTITY md5       \"[^\"]*\">|<!ENTITY md5       \"${MD5}\">|" \
        -e "s|<!ENTITY launch    \"Settings/ZFSToolkit\">|<!ENTITY launch    \"Settings/ZFSToolkit${SUFFIX}\">|" \
        -e "s|/usr/local/emhttp/plugins/zfs\.toolkit/|/usr/local/emhttp/plugins/${PLUGIN}/|g" \
        -e "s|/tmp/zfs\.toolkit|/tmp/${PLUGIN}|g" \
        "$PLG" > "${OUT_DIR}/${PLUGIN}.plg"
    echo "  Wrote ${OUT_DIR}/${PLUGIN}.plg"
fi

echo ""
echo "Done!"
echo "  File : ${OUT_DIR}/${PKG}"
echo "  MD5  : ${MD5}"
