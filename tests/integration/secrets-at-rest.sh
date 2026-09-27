#!/usr/bin/env bash
# Storage secrets must never be written to disk inside the backed-up data. duplicacy
# persists anything given to `duplicacy set -key ... -value` in the repository's plaintext
# .duplicacy/preferences, which lives in every service directory (usually a host bind
# mount), and puts the value on argv while it runs. The credentials must reach duplicacy
# through DUPLICACY_<NAME>_* env vars only. Covers the backup, the copy leg (which needs the
# RSA passphrase for its source), a restore, and a storage literally named `default`, whose
# env vars duplicacy reads without the name prefix.
#
#   docker run -i --rm --hostname sr-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/secrets-at-rest.sh

set -uo pipefail

FIXTURES=/data/fixtures
STORE1=/backup-store-1
STORE2=/backup-store-2
SECRETS_DIR=/run/secrets
STORAGE_PASSWORD=storage-pw-8f2c
RSA_PASSPHRASE=rsa-pass-4d7e

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

# Fails if either secret value appears anywhere under the given directories.
assert_no_secrets() {
  local hits
  hits=$(grep -rlF -e "$STORAGE_PASSWORD" -e "$RSA_PASSPHRASE" "$@" 2>/dev/null)
  [ -z "$hits" ] || die "secret value found on disk in: $(echo "$hits" | tr '\n' ' ')"
}

log "materialize RSA keypair + file secrets (env-native mode)"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE1" "$STORE2" "$FIXTURES"
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null \
  || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null \
  || die "openssl rsa -pubout"
chmod 600 /opt/archiver/keys/private.pem
printf '%s' "$STORAGE_PASSWORD" >"${SECRETS_DIR}/storage_password"
printf '%s' "$RSA_PASSPHRASE" >"${SECRETS_DIR}/rsa_passphrase"
echo "secret-free content" >"$FIXTURES/file.txt"

export SERVICE_DIRECTORIES="${FIXTURES}/"
export STORAGE_TARGET_1_NAME="primary"
export STORAGE_TARGET_1_TYPE="local"
export STORAGE_TARGET_1_LOCAL_PATH="${STORE1}"
export STORAGE_TARGET_2_NAME="secondary"
export STORAGE_TARGET_2_TYPE="local"
export STORAGE_TARGET_2_LOCAL_PATH="${STORE2}"

log "backup + copy to the secondary"
archiver backup || die "backup exited non-zero"
[ -d "$STORE2/snapshots" ] || die "copy to the secondary did not run"
assert_no_secrets "$FIXTURES/.duplicacy"

log "restore from the secondary (the copy's RSA path)"
SNAPSHOT_ID="$(hostname)-fixtures" LOCAL_DIR=/data/restore STORAGE_TARGET=secondary archiver auto-restore \
  || die "auto-restore exited non-zero"
diff "$FIXTURES/file.txt" /data/restore/file.txt || die "restored content differs"
assert_no_secrets /data/restore/.duplicacy

log "a storage named 'default' (duplicacy reads its env vars without the name prefix)"
rm -rf "$FIXTURES/.duplicacy" /data/restore
export STORAGE_TARGET_1_NAME="default"
export STORAGE_TARGET_1_LOCAL_PATH=/backup-store-default
unset STORAGE_TARGET_2_NAME STORAGE_TARGET_2_TYPE STORAGE_TARGET_2_LOCAL_PATH
mkdir -p /backup-store-default
archiver backup || die "backup to a storage named 'default' exited non-zero"
SNAPSHOT_ID="$(hostname)-fixtures" LOCAL_DIR=/data/restore archiver auto-restore \
  || die "restore from a storage named 'default' exited non-zero"
diff "$FIXTURES/file.txt" /data/restore/file.txt || die "restored content differs ('default')"
assert_no_secrets "$FIXTURES/.duplicacy" /data/restore/.duplicacy

echo "=== SECRETS-AT-REST OK: no storage password or RSA passphrase on disk; backup, copy, restore, and 'default' still work ==="
