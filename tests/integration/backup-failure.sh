#!/usr/bin/env bash
# A failed primary backup must be reported, not swallowed: `archiver backup` must exit
# non-zero, log an ERROR for the failed service, and not claim success. The duplicacy
# binary is shadowed by a wrapper that fails only the `backup` subcommand, so storage
# init/verify succeed and exactly the backup step's exit status is under test.
#
#   docker run -i --rm --hostname bf-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/backup-failure.sh

set -uo pipefail

FIXTURES=/data/fixtures
STORE=/backup-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
LOG=/opt/archiver/logs/archiver.log

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

echo "some content" >"$FIXTURES/file.txt"

log "shadow duplicacy: fail only the 'backup' subcommand, pass everything else through"
REAL="$(command -v duplicacy)" || die "duplicacy not on PATH"
mv "$REAL" "${REAL}.real"
cat >"$REAL" <<'WRAP'
#!/usr/bin/env bash
if [ "${1:-}" = "backup" ]; then
  echo "SIMULATED: chunk upload failed" >&2
  exit 1
fi
exec "$0.real" "$@"
WRAP
chmod +x "$REAL"

log "run a backup that must fail"
set +e
archiver backup
rc=$?
set -e

echo "--- archiver.log ---"; cat "$LOG"; echo "--------------------"

[ "$rc" -ne 0 ] || die "backup exited 0 despite duplicacy backup failing"
grep -q "\[ERROR\].*Backup to local failed for fixtures service" "$LOG" \
  || die "no ERROR logged for the failed backup"
grep -q "Backup to local completed for fixtures service" "$LOG" \
  && die "log claims the failed backup completed"
grep -q "Completed successfully" "$LOG" \
  && die "completion notification claims success despite the failure"

echo "=== BACKUP-FAILURE OK: failed backup exits ${rc}, logs ERROR, no false success ==="
