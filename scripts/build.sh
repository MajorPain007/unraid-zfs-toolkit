#!/bin/bash

set -euo pipefail

PLUGIN="zfs.dataset.converter"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
VERSION="${1:-$(date '+%Y.%m.%d').01}"
STAGE="$(mktemp -d)/staging"
OUT_DIR="${ZDC_OUT_DIR:-${ROOT_DIR}/archive}"
case "$OUT_DIR" in
    /*) ;;
    *) OUT_DIR="${ROOT_DIR}/${OUT_DIR}" ;;
esac
PLG="${ROOT_DIR}/src/${PLUGIN}.plg"

if [[ ! "$VERSION" =~ ^[0-9]{4}\.[0-9]{2}\.[0-9]{2}\.[0-9]+$ ]]; then
    echo "ERROR: version must look like YYYY.MM.DD.NN (got '${VERSION}')" >&2
    exit 1
fi

echo "Building ${PLUGIN}-${VERSION}-x86_64-1.txz ..."

PDIR="${STAGE}/usr/local/emhttp/plugins/${PLUGIN}"
mkdir -p "${PDIR}/scripts" "${PDIR}/event" "${STAGE}/install"

cp "${ROOT_DIR}/src/"*.page "${PDIR}/"
cp "${ROOT_DIR}/src/ZFSDatasetConverterPage.php" "${PDIR}/"
cp "${ROOT_DIR}/src/scripts/"*.php "${PDIR}/scripts/"
cp "${ROOT_DIR}/src/scripts/"*.sh  "${PDIR}/scripts/"
cp "${ROOT_DIR}/src/event/"*       "${PDIR}/event/"
chmod 755 "${PDIR}/scripts/"*.sh "${PDIR}/event/"*
chmod 644 "${PDIR}/scripts/"*.php "${PDIR}/"*.page "${PDIR}/ZFSDatasetConverterPage.php"

for s in "${PDIR}/scripts/"*.sh "${PDIR}/event/"*; do
    bash -n "$s" || { echo "SYNTAX ERROR in $s" >&2; exit 1; }
done
if command -v php >/dev/null 2>&1; then
    for p in "${PDIR}/scripts/"*.php "${PDIR}/"*.page "${PDIR}/ZFSDatasetConverterPage.php"; do
        php -l "$p" >/dev/null || { echo "PHP SYNTAX ERROR in $p" >&2; exit 1; }
    done
else
    echo "  (php not found - skipping PHP lint)"
fi

cat > "${STAGE}/install/slack-desc" << EOF
${PLUGIN}: ZFS Dataset Converter (MajorPain007)
${PLUGIN}:
${PLUGIN}: Converts regular folders to ZFS child datasets, manages native
${PLUGIN}: snapshots with retention, browses and restores them, and
${PLUGIN}: replicates datasets with zfs send/recv.
${PLUGIN}:
EOF

mkdir -p "${OUT_DIR}"
PKG="${PLUGIN}-${VERSION}-x86_64-1.txz"
if ! ( cd "${STAGE}" && COPYFILE_DISABLE=1 tar --no-xattrs -cJf "${OUT_DIR}/${PKG}" install/ usr/ ); then
    echo "ERROR: failed to create ${OUT_DIR}/${PKG}" >&2
    exit 1
fi

if command -v md5sum >/dev/null 2>&1; then
    MD5=$(md5sum "${OUT_DIR}/${PKG}" | cut -d' ' -f1)
else
    MD5=$(md5 -q "${OUT_DIR}/${PKG}")
fi

if [ -f "$PLG" ] && [ "${ZDC_SKIP_PLG:-0}" != "1" ]; then
    tmp="${PLG}.tmp"
    sed -e "s|<!ENTITY version   \"[^\"]*\">|<!ENTITY version   \"${VERSION}\">|" \
        -e "s|<!ENTITY md5       \"[^\"]*\">|<!ENTITY md5       \"${MD5}\">|" \
        "$PLG" > "$tmp"
    mv "$tmp" "$PLG"
    echo "  Updated ${PLG#"$ROOT_DIR"/}"
fi

rm -rf "$(dirname "$STAGE")"

echo ""
echo "Done!"
echo "  File : ${OUT_DIR#"$ROOT_DIR"/}/${PKG}"
echo "  MD5  : ${MD5}"
echo "  Size : $(du -h "${OUT_DIR}/${PKG}" | cut -f1)"
echo ""
echo "Next:"
echo "  tests/run.sh"
echo "  git add -A && git commit -m \"v${VERSION}: ...\" && git push"
echo "  git tag ${VERSION} && git push origin ${VERSION}    # publishes a GitHub Release"
