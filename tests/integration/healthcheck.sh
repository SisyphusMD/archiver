#!/usr/bin/env bash
# `archiver healthcheck` backs the image's Docker HEALTHCHECK: it must be healthy for a
# fresh env-native deployment (no config.sh — config from env/secrets), healthy after a
# finished run even one with errors (transient failures must not flip a daily-backup
# container UNHEALTHY), and UNHEALTHY when recent errors have no finished run (crash/hang)
# or when there is no configuration at all.
#
#   docker run -i --rm --hostname hc-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/healthcheck.sh

set -uo pipefail

FIXTURES=/data/fixtures
STORE=/backup-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
LOG=/opt/archiver/logs/archiver.log

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

log "no configuration at all: must be UNHEALTHY"
set +e
archiver healthcheck >/tmp/hc0.out 2>&1
rc=$?
set -e
[ "$rc" -ne 0 ] || { cat /tmp/hc0.out; die "healthcheck healthy with no config and no keys"; }
grep -q "No configuration found" /tmp/hc0.out || die "missing no-configuration error"

log "env-native deployment (keys + secrets, NO config.sh): must be healthy"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE" "$FIXTURES"
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null \
  || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null \
  || die "openssl rsa -pubout"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${RSA_PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"

set +e
archiver healthcheck >/tmp/hc1.out 2>&1
rc=$?
set -e
[ "$rc" -eq 0 ] || { cat /tmp/hc1.out; die "healthcheck UNHEALTHY for a valid env-native deployment (exit $rc)"; }
grep -q "env-native configuration" /tmp/hc1.out || die "env-native mode not recognized"

log "after a successful backup: healthy, no errors"
export SERVICE_DIRECTORIES="${FIXTURES}/"
export STORAGE_TARGET_1_NAME="local"
export STORAGE_TARGET_1_TYPE="local"
export STORAGE_TARGET_1_LOCAL_PATH="${STORE}"
export ROTATE_BACKUPS="false"
echo "some content" >"$FIXTURES/file.txt"
archiver backup || die "backup failed"

set +e
archiver healthcheck >/tmp/hc2.out 2>&1
rc=$?
set -e
[ "$rc" -eq 0 ] || { cat /tmp/hc2.out; die "healthcheck UNHEALTHY after a clean backup (exit $rc)"; }
grep -q "No errors in recent logs" /tmp/hc2.out || die "clean run not reflected"

log "a FAILED but finished run: healthy-with-warning (transient error, run completed)"
REAL="$(command -v duplicacy)"
mv "$REAL" "${REAL}.real"
printf '#!/usr/bin/env bash\nif [ "${1:-}" = backup ]; then echo boom >&2; exit 1; fi\nexec "$0.real" "$@"\n' >"$REAL"
chmod +x "$REAL"
set +e
archiver backup >/dev/null 2>&1
set -e
mv "${REAL}.real" "$REAL"

set +e
archiver healthcheck >/tmp/hc3.out 2>&1
rc=$?
set -e
[ "$rc" -eq 0 ] || { cat /tmp/hc3.out; die "one failed-but-finished run flipped UNHEALTHY (exit $rc)"; }
grep -q "but the run finished" /tmp/hc3.out || die "finished-run detection broken"

log "errors WITHOUT a finished run (crash/hang signature): must be UNHEALTHY"
TS="$(date +'%Y-%m-%d %H:%M:%S')"
printf '[%s] [ERROR] [Service: fixtures] something broke mid-run\n' "$TS" >"$(readlink -f "$LOG")"
set +e
archiver healthcheck >/tmp/hc4.out 2>&1
rc=$?
set -e
[ "$rc" -ne 0 ] || { cat /tmp/hc4.out; die "errors without a finished run reported healthy"; }
grep -q "without a finished run" /tmp/hc4.out || die "crash signature not detected"

echo "=== HEALTHCHECK OK: env-native healthy, finished-with-errors warns, crash signature unhealthy ==="
