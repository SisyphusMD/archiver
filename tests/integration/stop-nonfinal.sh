#!/usr/bin/env bash
# The OTHER stop path (stop-skips-prune.sh covers the final service): a stop during a
# NON-final service's backup is honored at the next service boundary — the next service's
# duplicacy backup must never start, the run must record "Backup stopped", and no storage
# check/prune may run.
#
#   docker run -i --rm --hostname sn-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/stop-nonfinal.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
LOG=/opt/archiver/logs/archiver.log
MARKER=/tmp/backup-started

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

log "two services; the FIRST one's backup blocks (stop lands there)"
mkdir -p "$SERVICES/svc-a" "$SERVICES/svc-b"
echo "service a data" >"$SERVICES/svc-a/a.txt"
echo "service b data" >"$SERVICES/svc-b/b.txt"

export SERVICE_DIRECTORIES="${SERVICES}/*/"
export STORAGE_TARGET_1_NAME="local"
export STORAGE_TARGET_1_TYPE="local"
export STORAGE_TARGET_1_LOCAL_PATH="${STORE}"
export ROTATE_BACKUPS="true"
export PRUNE_KEEP="-keep 0:1"

REAL="$(command -v duplicacy)" || die "duplicacy not on PATH"
mv "$REAL" "${REAL}.real"
cat >"$REAL" <<WRAP
#!/usr/bin/env bash
if [ "\${1:-}" = "backup" ] && [[ "\$PWD" == */svc-a ]]; then
  touch "${MARKER}"
  sleep 15
  exit 0
fi
exec "\$0.real" "\$@"
WRAP
chmod +x "$REAL"

log "start, wait for svc-a's backup stage, then stop"
archiver start >/dev/null || die "archiver start failed"
for _ in $(seq 1 100); do [ -f "$MARKER" ] && break; sleep 0.2; done
[ -f "$MARKER" ] || die "svc-a backup never started"
archiver stop || die "archiver stop failed"

log "wait for the run to wind down"
for _ in $(seq 1 80); do
  grep -q "Backup stopped\." "$LOG" 2>/dev/null && break
  sleep 0.5
done

echo "--- archiver.log (tail) ---"; tail -12 "$LOG"; echo "---------------------------"

grep -q "Backup stopped\." "$LOG" || die "no 'Backup stopped' record"
grep -q "Starting backup to local for svc-b service" "$LOG" && die "svc-b's backup started despite the stop"
grep -q "Storage check completed" "$LOG" && die "storage check ran after stop"
grep -q "Prune completed" "$LOG" && die "prune ran after stop"

echo "=== STOP-NONFINAL OK: stop during svc-a honored, svc-b never backed up, no check/prune ==="
