#!/usr/bin/env bash
# archiver doctor (ADR 30) against real duplicacy and rclone: a healthy deployment passes
# and doctor writes nothing to a storage; a storage that does not exist yet is reported, not
# created; a deleted kit is noticed; a key pair that cannot decrypt the storage, a wrong
# passphrase and a group-writable hook fail the run.
#
#   docker run -i --rm --hostname dr-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --cap-add CHOWN --cap-add FOWNER --entrypoint bash archiver:dev -s < tests/integration/doctor.sh

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
printf 'a-long-recovery-pw' >"${SECRETS_DIR}/recovery_password"

export STORAGE_TARGET_1_NAME="local" STORAGE_TARGET_1_TYPE="local" STORAGE_TARGET_1_LOCAL_PATH="${STORE}"
export SERVICE_DIRECTORIES="${SERVICES}/*/"
mkdir -p "$SERVICES/app" && echo data >"$SERVICES/app/file.txt"
chmod 755 "$SERVICES" "$SERVICES/app"

log "back up, then doctor passes"
archiver backup >/tmp/backup.out 2>&1 || { cat /tmp/backup.out; die "backup failed"; }
before=$(find "$STORE" -type f | sort | xargs sha256sum)
out=$(archiver doctor) || { echo "$out"; die "doctor failed a healthy deployment"; }
for want in "RSA key pair decrypts" "Storage 'local' (local, primary) is reachable" "dr-host-app: newest revision 1" \
  "recovery kit for this configuration is on every storage"; do
  grep -qF "$want" <<<"$out" || { echo "$out"; die "doctor output lacks: $want"; }
done
after=$(find "$STORE" -type f | sort | xargs sha256sum)
[ "$before" = "$after" ] || die "doctor changed the storage"

log "a storage that does not exist is reported, never created"
export STORAGE_TARGET_2_NAME="missing" STORAGE_TARGET_2_TYPE="local" STORAGE_TARGET_2_LOCAL_PATH="/nonexistent-store"
out=$(archiver doctor) && { echo "$out"; die "doctor passed with a missing storage"; }
grep -qF "Storage 'missing' (local, secondary) cannot be read" <<<"$out" || { echo "$out"; die "the missing storage is not reported"; }
[ -e /nonexistent-store ] && die "doctor created the missing storage"
unset STORAGE_TARGET_2_NAME STORAGE_TARGET_2_TYPE STORAGE_TARGET_2_LOCAL_PATH

log "a kit deleted from the storage is no longer counted as placed"
rm -f "$STORE"/archiver-recovery-kit-*.tar.enc
out=$(archiver doctor)
grep -qF "recovery kit recorded as placed is gone from: local" <<<"$out" || { echo "$out"; die "a deleted kit still counts as placed"; }

log "a key pair that matches itself but not the storage's data fails"
cp -a /opt/archiver/keys /tmp/keys.orig
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null
out=$(archiver doctor) && { echo "$out"; die "doctor passed with another key pair"; }
grep -qF "does not decrypt with the mounted RSA private key" <<<"$out" || { echo "$out"; die "the foreign key pair is not reported"; }
rm -rf /opt/archiver/keys && cp -a /tmp/keys.orig /opt/archiver/keys

log "a wrong passphrase and a group-writable hook fail the run"
printf 'wrong' >"${SECRETS_DIR}/rsa_passphrase"
printf '#!/bin/sh\n' >"$SERVICES/app/pre-backup" && chmod 775 "$SERVICES/app/pre-backup"
out=$(archiver doctor) && { echo "$out"; die "doctor passed with a wrong passphrase"; }
grep -qF "does not decrypt with rsa_passphrase" <<<"$out" || { echo "$out"; die "the wrong passphrase is not reported"; }
grep -qF "chmod go-w" <<<"$out" || { echo "$out"; die "the unsafe hook is not reported"; }

echo "PASS: doctor"
