#!/usr/bin/env bash
# Restore surface beyond latest-revision: pinning REVISION must restore the older content,
# IGNORE_OWNERSHIP=1 must restore without ownership preservation (files land as the running
# user, no capability warning), and RUN_RESTORE_SERVICE must invoke the backed-up
# restore-service.sh hook after a successful restore.
#
#   docker run -i --rm --hostname re-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/restore-extras.sh

set -uo pipefail

FIXTURES=/data/fixtures
STORE=/backup-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
SNAPSHOT_ID_VAL="$(hostname)-fixtures"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

log "materialize RSA keypair + file secrets (env-native mode)"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE" "$FIXTURES"
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null \
  || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null \
  || die "openssl rsa -pubout"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${RSA_PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"

export SERVICE_DIRECTORIES="${FIXTURES}/"
export STORAGE_TARGET_1_NAME="local"
export STORAGE_TARGET_1_TYPE="local"
export STORAGE_TARGET_1_LOCAL_PATH="${STORE}"
export ROTATE_BACKUPS="false"

log "revision 1: original content + a restore-service.sh hook"
echo "generation one" >"$FIXTURES/file.txt"
cat >"$FIXTURES/restore-service.sh" <<'EOF'
#!/bin/bash
echo "hook ran in $PWD" > hook-marker.txt
EOF
chmod +x "$FIXTURES/restore-service.sh"
archiver backup || die "backup 1 failed"

log "revision 2: changed content"
# duplicacy's quick mode skips files with unchanged size+mtime; a same-second, same-size
# rewrite would silently reuse revision 1's chunks. Tick the clock and change the size.
sleep 1.1
echo "generation two (changed)" >"$FIXTURES/file.txt"
archiver backup || die "backup 2 failed"

log "restore latest: must be generation two"
SNAPSHOT_ID="$SNAPSHOT_ID_VAL" LOCAL_DIR=/data/restore-latest OVERWRITE=1 archiver auto-restore \
  || die "latest restore failed"
grep -q "generation two" /data/restore-latest/file.txt || die "latest restore is not revision 2"

log "restore REVISION=1: must be generation one"
SNAPSHOT_ID="$SNAPSHOT_ID_VAL" LOCAL_DIR=/data/restore-r1 REVISION=1 OVERWRITE=1 archiver auto-restore \
  || die "pinned-revision restore failed"
grep -q "generation one" /data/restore-r1/file.txt || die "REVISION=1 did not restore revision 1"

log "IGNORE_OWNERSHIP=1: restores (no ownership preservation), and must not warn about caps"
SNAPSHOT_ID="$SNAPSHOT_ID_VAL" LOCAL_DIR=/data/restore-noown IGNORE_OWNERSHIP=1 OVERWRITE=1 archiver auto-restore \
  >/tmp/noown.out 2>&1 || { cat /tmp/noown.out; die "ignore-ownership restore failed"; }
grep -q "generation two" /data/restore-noown/file.txt || die "ignore-ownership restore wrong content"
grep -q "capability is not granted" /tmp/noown.out && die "cap warning fired despite -ignore-owner"

log "RUN_RESTORE_SERVICE: the backed-up hook runs after the restore"
SNAPSHOT_ID="$SNAPSHOT_ID_VAL" LOCAL_DIR=/data/restore-hook OVERWRITE=1 RUN_RESTORE_SERVICE=1 archiver auto-restore \
  || die "hooked restore failed"
[ -f /data/restore-hook/hook-marker.txt ] || die "restore-service.sh hook never ran"
grep -q "hook ran in /data/restore-hook" /data/restore-hook/hook-marker.txt || die "hook ran in the wrong directory"

echo "=== RESTORE-EXTRAS OK: revision pinning, ignore-ownership, restore-service hook ==="
