#!/usr/bin/env bash
# Proves the encrypted bundle decrypts from a FILE-based password (a Docker/k8s secret),
# never an env var. Creates a bundle, wipes the plaintext config + keys, then imports it
# using ONLY the password file at /run/secrets/bundle_password (the way the entrypoint
# resolves it), and runs a backup to confirm the imported config works. Guards the DR /
# cold-restore path for the file-only BUNDLE_PASSWORD change.
#
#   docker run -i --rm --hostname bs-host \
#     --cap-drop ALL --cap-add DAC_OVERRIDE --cap-add CHOWN --cap-add FOWNER \
#     --entrypoint bash archiver:dev -s < tests/integration/bundle-secret.sh

set -uo pipefail

FIXTURES=/data/fixtures
STORE=/backup-store
SECRETS_DIR=/run/secrets
BUNDLE_DIR=/opt/archiver/bundle
BUNDLE_IMPORT=/opt/archiver/lib/scripts/bundle-import.sh
BUNDLE_PW=bundlepass123
RSA_PW=rsapass123

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

mkdir -p /opt/archiver/keys "${STORE}" "${FIXTURES}" "${SECRETS_DIR}" "${BUNDLE_DIR}"
echo "bundle content" >"${FIXTURES}/f.txt"

log "generate RSA + SSH keys and a config.sh"
openssl genrsa -aes256 -passout "pass:${RSA_PW}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PW}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "openssl rsa -pubout"
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

log "encrypt the bundle (mimics bundle-export)"
cd /opt/archiver || die "cd /opt/archiver"
tar -cf /tmp/bundle.tar keys config.sh
openssl enc -aes-256-cbc -pbkdf2 -salt -in /tmp/bundle.tar -out "${BUNDLE_DIR}/bundle.tar.enc" -k "${BUNDLE_PW}" || die "openssl enc"
rm -f /tmp/bundle.tar

log "wipe the plaintext config + keys so only the encrypted bundle remains"
rm -f /opt/archiver/config.sh
find /opt/archiver/keys -type f -delete

log "provide the bundle password ONLY as a file (no env var)"
printf '%s' "${BUNDLE_PW}" >"${SECRETS_DIR}/bundle_password"

log "import the bundle the way the entrypoint does: password read from the file"
BUNDLE_PASSWORD="$(<"${SECRETS_DIR}/bundle_password")"
export ARCHIVER_BUNDLE_PASSWORD="${BUNDLE_PASSWORD}"
export ARCHIVER_BUNDLE_FILE="${BUNDLE_DIR}/bundle.tar.enc"
"${BUNDLE_IMPORT}" || die "bundle-import failed with a file-based password"
[ -f /opt/archiver/config.sh ] || die "config.sh was not restored from the bundle"
[ -f /opt/archiver/keys/private.pem ] || die "keys were not restored from the bundle"

log "backup using the imported config"
archiver backup || die "backup exited non-zero"
[ -d "${STORE}/snapshots" ] || die "no snapshots written to storage"

log "standalone import (the documented 'docker exec ... archiver bundle import'): no ARCHIVER_* env"
rm -f /opt/archiver/config.sh
unset ARCHIVER_BUNDLE_PASSWORD ARCHIVER_BUNDLE_FILE BUNDLE_PASSWORD
printf 'y' | archiver bundle import || die "standalone bundle import failed (self-resolve broken)"
[ -f /opt/archiver/config.sh ] || die "config.sh not restored by standalone import"

echo "=== BUNDLE-SECRET OK: file-password import, backup, and standalone exec import ==="
