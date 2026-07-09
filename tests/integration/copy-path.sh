#!/usr/bin/env bash
# Secondary-storage (3-2-1) coverage: with two storage targets, a backup must be copied to
# the secondary and be restorable FROM the secondary. Then a failing copy must be reported
# (non-zero exit), not swallowed. Runs entirely against real duplicacy with local storage.
#
#   docker run -i --rm --hostname cp-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/copy-path.sh

set -uo pipefail

FIXTURES=/data/fixtures
RESTORE=/data/restore
STORE1=/backup-primary
STORE2=/backup-secondary
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
SNAPSHOT_ID="$(hostname)-fixtures"
LOG=/opt/archiver/logs/archiver.log

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

log "materialize RSA keypair + file secrets (env-native mode)"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE1" "$STORE2" "$FIXTURES" "$RESTORE"
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null \
  || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null \
  || die "openssl rsa -pubout"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${RSA_PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"

log "two local storage targets: primary + copy"
export SERVICE_DIRECTORIES="${FIXTURES}/"
export STORAGE_TARGET_1_NAME="primary"
export STORAGE_TARGET_1_TYPE="local"
export STORAGE_TARGET_1_LOCAL_PATH="${STORE1}"
export STORAGE_TARGET_2_NAME="secondary"
export STORAGE_TARGET_2_TYPE="local"
export STORAGE_TARGET_2_LOCAL_PATH="${STORE2}"
export ROTATE_BACKUPS="false"

echo "copy me to both storages" >"$FIXTURES/file.txt"
head -c 8192 /dev/urandom >"$FIXTURES/blob.bin"
HASH_TXT="$(sha256sum "$FIXTURES/file.txt" | cut -d' ' -f1)"
HASH_BIN="$(sha256sum "$FIXTURES/blob.bin" | cut -d' ' -f1)"

log "backup -> primary, copy -> secondary"
archiver backup retain || die "backup exited non-zero"
[ -d "$STORE1/snapshots" ] || die "no snapshots on primary"
[ -d "$STORE2/snapshots" ] || die "no snapshots on secondary (copy did not run)"
grep -q "Copy to secondary storage completed" "$LOG" || die "no copy-completed record in log"

log "restore from the SECONDARY storage"
SNAPSHOT_ID="$SNAPSHOT_ID" LOCAL_DIR="$RESTORE" STORAGE_TARGET=2 OVERWRITE=1 HASH_COMPARE=1 archiver auto-restore \
  || die "auto-restore from secondary exited non-zero"
[ "$(sha256sum "$RESTORE/file.txt" | cut -d' ' -f1)" = "$HASH_TXT" ] || die "file.txt content mismatch from secondary"
[ "$(sha256sum "$RESTORE/blob.bin" | cut -d' ' -f1)" = "$HASH_BIN" ] || die "blob.bin content mismatch from secondary"

log "now make 'duplicacy copy' fail: it must be reported, not swallowed"
REAL="$(command -v duplicacy)" || die "duplicacy not on PATH"
mv "$REAL" "${REAL}.real"
cat >"$REAL" <<'WRAP'
#!/usr/bin/env bash
if [ "${1:-}" = "copy" ]; then
  echo "SIMULATED: copy to secondary failed" >&2
  exit 1
fi
exec "$0.real" "$@"
WRAP
chmod +x "$REAL"

echo "new content forces a new revision" >>"$FIXTURES/file.txt"
set +e
archiver backup retain
rc=$?
set -e
[ "$rc" -ne 0 ] || die "backup exited 0 despite the copy failing"
grep -q "\[ERROR\].*Copy to secondary storage failed" "$LOG" || die "no ERROR logged for the failed copy"

echo "=== COPY-PATH OK: copy runs, secondary restores, copy failure exits ${rc} ==="
