#!/usr/bin/env bash
# A restore that preserves ownership in a container WITHOUT CHOWN/FOWNER warns (files would
# land root-owned) and still restores; with the caps, or when ownership is not preserved
# (IGNORE_OWNERSHIP), it does not warn. The caller sets EXPECT_WARN to match the cap set:
#
#   docker run -i --rm --cap-drop ALL --cap-add DAC_OVERRIDE -e EXPECT_WARN=1 \
#     --entrypoint bash archiver:dev -s < tests/integration/restore-caps.sh
#   docker run -i --rm --cap-drop ALL --cap-add DAC_OVERRIDE --cap-add CHOWN --cap-add FOWNER \
#     -e EXPECT_WARN=0 --entrypoint bash archiver:dev -s < tests/integration/restore-caps.sh

set -uo pipefail

die() { echo "FAIL: $*"; exit 1; }
mkdir -p /opt/archiver/keys /run/secrets /store /data/app
openssl genrsa -aes256 -passout pass:rp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die genrsa
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:rp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die pubout
printf testpassword >/run/secrets/storage_password; printf rp >/run/secrets/rsa_passphrase
export SERVICE_DIRECTORIES=/data/*/ STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH=/store
echo data >/data/app/file.txt
archiver backup >/tmp/b.out 2>&1 || { cat /tmp/b.out; die "backup failed"; }
warned() { grep -q "capability is not granted" "$1" && echo 1 || echo 0; }

SNAPSHOT_ID="$(hostname)-app" LOCAL_DIR=/tmp/r1 archiver auto-restore >/tmp/r1.out 2>&1 || { cat /tmp/r1.out; die "restore failed"; }
[ "$(warned /tmp/r1.out)" = "${EXPECT_WARN:?EXPECT_WARN must be set}" ] || { cat /tmp/r1.out; die "ownership-preserving warn=$(warned /tmp/r1.out), expected ${EXPECT_WARN}"; }
grep -q data /tmp/r1/file.txt || die "not restored"

SNAPSHOT_ID="$(hostname)-app" LOCAL_DIR=/tmp/r2 IGNORE_OWNERSHIP=1 archiver auto-restore >/tmp/r2.out 2>&1 || die "restore failed"
[ "$(warned /tmp/r2.out)" = 0 ] || die "IGNORE_OWNERSHIP must never warn"

echo "=== RESTORE-CAPS OK: ownership-preserving warn=${EXPECT_WARN}; IGNORE_OWNERSHIP never warns ==="
