#!/usr/bin/env bash
# `archiver stop` during the final (here: only) service's backup must terminate the run
# before storage wrap-up: no check, no prune, and a "Backup stopped" record instead of a
# completion. The duplicacy binary is shadowed so `backup` blocks long enough to stop it
# deterministically; every other subcommand passes through to the real binary.
#
#   docker run -i --rm --hostname sp-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/stop-skips-prune.sh

set -uo pipefail

FIXTURES=/data/fixtures
STORE=/backup-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
LOG=/opt/archiver/logs/archiver.log
MARKER=/tmp/backup-started

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
export ROTATE_BACKUPS="true"
export PRUNE_KEEP="-keep 0:1"

echo "some content" >"$FIXTURES/file.txt"

log "shadow duplicacy: 'backup' blocks (stoppable window), everything else passes through"
REAL="$(command -v duplicacy)" || die "duplicacy not on PATH"
mv "$REAL" "${REAL}.real"
cat >"$REAL" <<WRAP
#!/usr/bin/env bash
if [ "\${1:-}" = "backup" ]; then
  touch "${MARKER}"
  sleep 15
  exit 0
fi
exec "\$0.real" "\$@"
WRAP
chmod +x "$REAL"

log "start backup (background mode, rotation on so wrap-up would prune), wait for the backup stage"
archiver start >/dev/null || die "archiver start failed"
for _ in $(seq 1 100); do [ -f "$MARKER" ] && break; sleep 0.2; done
[ -f "$MARKER" ] || die "backup never reached the duplicacy backup stage"

log "request stop during the backup"
archiver stop || die "archiver stop failed"

log "wait for the run to wind down"
for _ in $(seq 1 60); do
  grep -q "Backup stopped\." "$LOG" 2>/dev/null && break
  sleep 0.5
done

echo "--- archiver.log ---"; cat "$LOG"; echo "--------------------"

grep -q "Backup stopped\." "$LOG" \
  || die "no 'Backup stopped' record: the stop request was not honored"
grep -q "Storage check completed" "$LOG" \
  && die "storage check ran after stop was requested"
grep -q "Prune completed" "$LOG" \
  && die "prune ran after stop was requested"
grep -q "Backup Complete" "$LOG" \
  && die "run recorded a completion despite the stop"

echo "=== STOP-SKIPS-PRUNE OK: stop during backup halts before check/prune ==="
