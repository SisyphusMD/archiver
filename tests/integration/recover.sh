#!/usr/bin/env bash
# archiver recover (ADR 29) end to end: back up two services, lose the host (keys, secrets,
# configuration and data all gone, nothing left but the storage and the kit password), then
# recover from the kit alone: the configuration and secrets come back owner-only, the keys
# are placed, and every service returns byte for byte.
#
#   docker run -i --rm --hostname rc-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --cap-add CHOWN --cap-add FOWNER --entrypoint bash archiver:dev -s < tests/integration/recover.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

log "a deployment with two services and a recovery kit"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE"
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null \
  || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null \
  || die "openssl rsa -pubout"
chmod 600 /opt/archiver/keys/private.pem
cp /opt/archiver/keys/private.pem "${SECRETS_DIR}/rsa_private_key"; cp /opt/archiver/keys/public.pem "${SECRETS_DIR}/rsa_public_key"
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${RSA_PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"
printf 'a-long-recovery-pw' >"${SECRETS_DIR}/recovery_password"
export STORAGE_TARGET_1_NAME="local" STORAGE_TARGET_1_TYPE="local" STORAGE_TARGET_1_LOCAL_PATH="${STORE}"
# Two globs, either of which would take either name: each service must come back to the
# directory it was backed up from.
export SERVICE_DIRECTORIES="${SERVICES}/apps/*/:${SERVICES}/dbs/*/"
for s in apps/app dbs/db; do
  mkdir -p "$SERVICES/$s/sub"
  head -c 30000 /dev/urandom >"$SERVICES/$s/blob.bin"; echo "$s" >"$SERVICES/$s/sub/name.txt"
done
(cd "$SERVICES" && find . -type f | sort | xargs sha256sum) >/tmp/before.sums
archiver backup >/tmp/backup.out 2>&1 || { cat /tmp/backup.out; die "backup failed"; }
kit=$(ls "$STORE"/archiver-recovery-kit-rc-host.tar.enc) || die "no kit on the storage"

log "the host is lost: keys, configuration, secrets (all but the password) and data"
cp "$kit" /tmp/kit.tar.enc
rm -rf /opt/archiver/keys "$SERVICES" /opt/archiver/logs/.recovery-kit-state
find "$SECRETS_DIR" -type f ! -name recovery_password -delete
unset STORAGE_TARGET_1_NAME STORAGE_TARGET_1_TYPE STORAGE_TARGET_1_LOCAL_PATH SERVICE_DIRECTORIES

log "a wrong password is refused and nothing is written"
printf 'not-the-password' >/tmp/wrongpw
RECOVERY_PASSWORD_FILE=/tmp/wrongpw archiver recover /tmp/kit.tar.enc --yes >/tmp/wrong.out 2>&1 && die "a wrong password recovered"
grep -q "does not open with that password" /tmp/wrong.out || { cat /tmp/wrong.out; die "no clear message for a wrong password"; }
[ -e /opt/archiver/recovered/archiver.env ] && die "a failed recovery wrote the configuration"

log "an existing empty output directory others could write to is made owner-only first"
mkdir -p /opt/archiver/recovered && chmod 777 /opt/archiver/recovered

log "recover from the kit alone"
# Through the entrypoint, as `docker run <image> recover ...` runs it: no keys exist yet.
archiver entrypoint recover /tmp/kit.tar.enc --yes >/tmp/recover.out 2>&1 || { cat /tmp/recover.out; die "recover failed"; }
grep -qF "All 2 services restored" /tmp/recover.out || { cat /tmp/recover.out; die "recover did not restore both services"; }
grep -qF "hostname 'rc-host'" /tmp/recover.out || { cat /tmp/recover.out; die "recover does not say which hostname to keep"; }
[ -f /opt/archiver/recovered/archiver.env ] || die "no recovered archiver.env"
[ "$(stat -c %a /opt/archiver/recovered/secrets/storage_password)" = 600 ] || die "a recovered secret is not owner-only"
[ -f /opt/archiver/keys/private.pem ] || die "the RSA key was not placed"
[ "$(stat -c %a /opt/archiver/recovered)" = 700 ] || die "the output directory was left writable by others"
(cd "$SERVICES" && find . -path '*/.duplicacy' -prune -o -type f -print | sort | xargs sha256sum) >/tmp/after.sums
diff /tmp/before.sums /tmp/after.sums || die "the recovered services differ from the originals"

log "running it again with the same kit reuses the recovered directory"
archiver recover /tmp/kit.tar.enc --yes >/tmp/again.out 2>&1 || { cat /tmp/again.out; die "a second recovery with the same kit failed"; }
grep -qF "finishing and reusing it" /tmp/again.out || { cat /tmp/again.out; die "the second recovery did not reuse the directory"; }

echo "PASS: recover"
