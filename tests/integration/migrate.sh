#!/usr/bin/env bash
# Two-way round-trip for `archiver migrate` + mode-agnostic `bundle export`:
#   bundle-style config  ->  migrate  ->  env-native materials  ->  backup works
#   env-native config    ->  bundle export  ->  re-import       ->  config restored
# Proves migrate emits complete, valid, secret-free env materials, and that bundle export
# captures the effective (env-native) config into a re-importable bundle.
#
#   docker run -i --rm --hostname mig-host \
#     --cap-drop ALL --cap-add DAC_OVERRIDE --cap-add CHOWN --cap-add FOWNER \
#     --entrypoint bash archiver:dev -s < tests/integration/migrate.sh

set -uo pipefail

FIXTURES=/data/fixtures
STORE=/backup-store
SECRETS_DIR=/run/secrets
BUNDLE_DIR=/opt/archiver/bundle
MIG=/tmp/migrated
RSA_PW=rsapass123
BUNDLE_PW=bundlepass123

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

mkdir -p /opt/archiver/keys "${STORE}" "${FIXTURES}" "${SECRETS_DIR}" "${BUNDLE_DIR}"
echo "migrate content" >"${FIXTURES}/f.txt"

log "set up a bundle-style config.sh + keys"
openssl genrsa -aes256 -passout "pass:${RSA_PW}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PW}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "pubout"
ssh-keygen -t ed25519 -N "" -f /opt/archiver/keys/id_ed25519 -q || die "ssh-keygen"
chmod 600 /opt/archiver/keys/private.pem /opt/archiver/keys/id_ed25519
cat >/opt/archiver/config.sh <<CFG
SERVICE_DIRECTORIES="${FIXTURES}/"
STORAGE_TARGET_1_NAME="local"
STORAGE_TARGET_1_TYPE="local"
STORAGE_TARGET_1_LOCAL_PATH="${STORE}"
STORAGE_PASSWORD="storagepassword1"
RSA_PASSPHRASE="${RSA_PW}"
ROTATE_BACKUPS="false"
CFG

log "archiver migrate -> ${MIG}"
archiver migrate "${MIG}" || die "migrate exited non-zero"
[ -f "${MIG}/archiver.env" ] || die "no archiver.env produced"
grep -q '^STORAGE_TARGET_1_TYPE=local' "${MIG}/archiver.env" || die "archiver.env missing storage target"
grep -q '^SERVICE_DIRECTORIES=' "${MIG}/archiver.env" || die "archiver.env missing SERVICE_DIRECTORIES"
grep -q '^STORAGE_PASSWORD=' "${MIG}/archiver.env" && die "a secret leaked into archiver.env"
[ "$(cat "${MIG}/secrets/storage_password")" = "storagepassword1" ] || die "storage_password secret file wrong/missing"
[ -f "${MIG}/secrets/rsa_private_key" ] && [ -f "${MIG}/secrets/rsa_public_key" ] || die "key files not emitted"

log "go env-native from the migrated materials (wipe config.sh + keys; load env + secret files)"
rm -f /opt/archiver/config.sh
find /opt/archiver/keys -type f -delete
rm -f "${SECRETS_DIR:?}"/*
cp "${MIG}"/secrets/* "${SECRETS_DIR}"/
cp "${SECRETS_DIR}/rsa_private_key" /opt/archiver/keys/private.pem && chmod 600 /opt/archiver/keys/private.pem
cp "${SECRETS_DIR}/rsa_public_key" /opt/archiver/keys/public.pem && chmod 644 /opt/archiver/keys/public.pem
cp "${SECRETS_DIR}/ssh_private_key" /opt/archiver/keys/id_ed25519 && chmod 600 /opt/archiver/keys/id_ed25519
set -a; source "${MIG}/archiver.env"; set +a
archiver backup || die "env-native backup from migrated materials failed"
[ -d "${STORE}/snapshots" ] || die "no snapshots after the env-native backup"

log "env-native -> bundle export (mode-agnostic), password from a file"
printf '%s' "${BUNDLE_PW}" >"${SECRETS_DIR}/bundle_password"
archiver bundle export </dev/null || die "bundle export from env-native failed"
[ -f "${BUNDLE_DIR}/bundle.tar.enc" ] || die "no bundle.tar.enc produced by export"

log "re-import the exported bundle (wipe env + config; password read from the file)"
unset SERVICE_DIRECTORIES STORAGE_TARGET_1_NAME STORAGE_TARGET_1_TYPE STORAGE_TARGET_1_LOCAL_PATH ROTATE_BACKUPS
rm -f /opt/archiver/config.sh
find /opt/archiver/keys -type f -delete
ARCHIVER_BUNDLE_PASSWORD="$(<"${SECRETS_DIR}/bundle_password")"; export ARCHIVER_BUNDLE_PASSWORD
export ARCHIVER_BUNDLE_FILE="${BUNDLE_DIR}/bundle.tar.enc"
/opt/archiver/lib/scripts/bundle-import.sh || die "re-import of the exported bundle failed"
[ -f /opt/archiver/config.sh ] || die "config.sh not restored from the exported bundle"
grep -q 'STORAGE_TARGET_1_TYPE' /opt/archiver/config.sh || die "exported bundle's config.sh missing the storage target"
[ -f /opt/archiver/keys/private.pem ] || die "keys not restored from the exported bundle"

echo "=== MIGRATE ROUND-TRIP OK: bundle -> migrate -> env-native backup -> export -> re-import ==="
