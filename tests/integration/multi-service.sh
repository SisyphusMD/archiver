#!/usr/bin/env bash
# Multi-service semantics: a glob SERVICE_DIRECTORIES pattern expands to every service,
# each gets its own snapshot, and the storage wrap-up runs once at the end. Then failure
# isolation: one service's backup failing must not stop the others, and the run must still
# exit non-zero with the failure counted.
#
#   docker run -i --rm --hostname ms-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/multi-service.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
LOG=/opt/archiver/logs/archiver.log
HOST="$(hostname)"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

log "materialize RSA keypair + file secrets (env-native mode)"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE"
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null \
  || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null \
  || die "openssl rsa -pubout"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${RSA_PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"

log "three services behind one glob pattern"
mkdir -p "$SERVICES/svc-a" "$SERVICES/svc-b" "$SERVICES/svc-c"
echo "service a data" >"$SERVICES/svc-a/a.txt"
echo "service b data" >"$SERVICES/svc-b/b.txt"
echo "service c data" >"$SERVICES/svc-c/c.txt"

export SERVICE_DIRECTORIES="${SERVICES}/*/"
export STORAGE_TARGET_1_NAME="local"
export STORAGE_TARGET_1_TYPE="local"
export STORAGE_TARGET_1_LOCAL_PATH="${STORE}"
export ROTATE_BACKUPS="true"
export PRUNE_KEEP="-keep 0:1"

log "backup all three (check/prune belong to maintenance: the backup run must do NEITHER)"
archiver backup || die "backup exited non-zero"

for s in svc-a svc-b svc-c; do
  [ -d "$STORE/snapshots/${HOST}-${s}" ] || die "no snapshot for ${s} (glob expansion or per-service backup broken)"
  grep -q "Backup to local completed for ${s} service" "$LOG" || die "no completion record for ${s}"
done
grep -q "Storage check completed" "$LOG" && die "a storage check ran inside the backup pipeline"
grep -q "Prune completed" "$LOG" && die "a prune ran inside the backup pipeline"

log "failure isolation: svc-b's backup fails, svc-a and svc-c must still complete"
REAL="$(command -v duplicacy)" || die "duplicacy not on PATH"
mv "$REAL" "${REAL}.real"
cat >"$REAL" <<'WRAP'
#!/usr/bin/env bash
if [ "${1:-}" = "backup" ] && [[ "$PWD" == */svc-b ]]; then
  echo "SIMULATED: svc-b backup failed" >&2
  exit 1
fi
exec "$0.real" "$@"
WRAP
chmod +x "$REAL"

for s in svc-a svc-b svc-c; do echo "round two" >>"$SERVICES/$s/${s#svc-}.txt"; done
set +e
archiver backup
rc=$?
set -e

# rotate_logs gives every run a fresh file behind the archiver.log symlink, so $LOG now
# holds only this second run's lines.
[ "$rc" -ne 0 ] || die "run exited 0 despite svc-b failing"
grep -q "\[ERROR\].*Backup to local failed for svc-b service" "$LOG" || die "svc-b failure not logged as ERROR"
grep -q "Backup to local completed for svc-a service" "$LOG" || die "svc-a did not complete after svc-b's failure"
grep -q "Backup to local completed for svc-c service" "$LOG" || die "svc-c did not complete after svc-b's failure"

echo "=== MULTI-SERVICE OK: glob expands, no check/prune in backup, svc-b failure isolated, exit ${rc} ==="
