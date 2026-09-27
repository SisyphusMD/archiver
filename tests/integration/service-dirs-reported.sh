#!/usr/bin/env bash
# A configured directory that is not backed up must be reported, never dropped in silence.
# An entry matching no directory (a typo, an unmounted path) and a directory whose name
# cannot form a duplicacy snapshot ID (only letters, digits, '_' and '-' are allowed, so a
# space or a dot is fatal to `duplicacy init`) must each log an ERROR and make the run exit
# non-zero, while every valid directory still backs up.
#
#   docker run -i --rm --hostname sd-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/service-dirs-reported.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase

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

log "one valid service, one whose name has a space, one that does not exist"
mkdir -p "$SERVICES/plain" "$SERVICES/with space"
echo "plain data" >"$SERVICES/plain/file.txt"
echo "spaced data" >"$SERVICES/with space/file.txt"

export SERVICE_DIRECTORIES="${SERVICES}/plain/:${SERVICES}/with space/:${SERVICES}/missing/"
export STORAGE_TARGET_1_NAME="local"
export STORAGE_TARGET_1_TYPE="local"
export STORAGE_TARGET_1_LOCAL_PATH="${STORE}"

archiver backup >/tmp/run.out 2>&1
rc=$?
echo "--- errors ---"; grep -rh "\[ERROR\]" /opt/archiver/logs/ | sed 's/^\[[^]]*\] //'

[ -d "$STORE/snapshots/$(hostname)-plain" ] || die "the valid service was not backed up"
[ "$rc" -ne 0 ] || die "run exited 0 although two configured directories were not backed up"
grep -rq "\[ERROR\].*${SERVICES}/missing/.*matches no directory" /opt/archiver/logs/ \
  || die "no ERROR for the entry that matches no directory"
grep -rq "\[ERROR\].*with space.*snapshot ID" /opt/archiver/logs/ \
  || die "no ERROR for the directory whose name cannot form a snapshot ID"

echo "=== SERVICE-DIRS-REPORTED OK: missing and unnameable directories are errors; valid ones still back up ==="
