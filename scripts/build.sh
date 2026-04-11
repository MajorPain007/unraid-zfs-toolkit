#!/bin/bash
# Build script for ZFS Dataset Converter Unraid plugin
# Usage: ./scripts/build.sh [version]
# Example: ./scripts/build.sh 2025.04.15
#
# After running:
#   1. Update &version; and &md5; in src/zfs.dataset.converter.plg
#   2. git add archive/ src/ && git commit -m "..." && git push

set -e

PLUGIN="zfs.dataset.converter"
VERSION="${1:-$(date '+%Y.%m.%d')}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
STAGE="/tmp/${PLUGIN}_build/staging"
ARCHIVE_DIR="${ROOT_DIR}/archive"

echo "Building ${PLUGIN}-${VERSION}-x86_64-1.txz ..."

# Clean staging
rm -rf "/tmp/${PLUGIN}_build"
mkdir -p "${STAGE}/usr/local/emhttp/plugins/${PLUGIN}/scripts"
mkdir -p "${STAGE}/install"

# Copy plugin files
cp "${ROOT_DIR}/src/ZFSDatasetConverter.page" \
   "${STAGE}/usr/local/emhttp/plugins/${PLUGIN}/"
cp "${ROOT_DIR}/src/scripts/"*.php \
   "${STAGE}/usr/local/emhttp/plugins/${PLUGIN}/scripts/"
cp "${ROOT_DIR}/src/scripts/"*.sh \
   "${STAGE}/usr/local/emhttp/plugins/${PLUGIN}/scripts/"
chmod 755 "${STAGE}/usr/local/emhttp/plugins/${PLUGIN}/scripts/"*.sh

# Slack description
cat > "${STAGE}/install/slack-desc" << EOF
${PLUGIN}: ZFS Dataset Converter (MajorPain007)
${PLUGIN}:
${PLUGIN}: Converts regular folders to ZFS child datasets.
${PLUGIN}: Docker & VM awareness, schedule, dry-run, live logs.
${PLUGIN}:
EOF

# Build archive (no Apple metadata)
mkdir -p "${ARCHIVE_DIR}"
PKG="${PLUGIN}-${VERSION}-x86_64-1.txz"
( cd "${STAGE}" && COPYFILE_DISABLE=1 tar --no-xattrs -cJf "${ARCHIVE_DIR}/${PKG}" install/ usr/ )

MD5=$(md5 -q "${ARCHIVE_DIR}/${PKG}")

echo ""
echo "Done!"
echo "  File : archive/${PKG}"
echo "  MD5  : ${MD5}"
echo "  Size : $(du -sh "${ARCHIVE_DIR}/${PKG}" | cut -f1)"
echo ""
echo "Next steps:"
echo "  1. Update src/zfs.dataset.converter.plg:"
echo "     <!ENTITY version   \"${VERSION}\">"
echo "     <!ENTITY md5       \"${MD5}\">"
echo "  2. git add archive/ src/ && git commit && git push"
