#!/usr/bin/env bash
# Restore preview and partial restores (ADR 29) against real duplicacy: DRY_RUN reports what
# would be added, replaced and deleted and restores nothing (the destination is not even
# created); RESTORE_PATHS restores only the paths named, a directory with everything under it.
#
#   docker run -i --rm --hostname rp-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --cap-add CHOWN --cap-add FOWNER --entrypoint bash archiver:dev -s < tests/integration/restore-preview.sh

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

export STORAGE_TARGET_1_NAME="local" STORAGE_TARGET_1_TYPE="local" STORAGE_TARGET_1_LOCAL_PATH="${STORE}"
export SERVICE_DIRECTORIES="${SERVICES}/*/"
mkdir -p "$SERVICES/app/config/sub" "$SERVICES/app/data"
echo a >"$SERVICES/app/config/a.conf"; echo b >"$SERVICES/app/config/sub/b.conf"
head -c 5000 /dev/urandom >"$SERVICES/app/data/db"; echo t >"$SERVICES/app/top.txt"
archiver backup >/tmp/backup.out 2>&1 || { cat /tmp/backup.out; die "backup failed"; }

log "DRY_RUN into a new directory: everything would be added, nothing is created"
out=$(SNAPSHOT_ID=rp-host-app LOCAL_DIR=/restore/new DRY_RUN=1 archiver auto-restore 2>&1) || { echo "$out"; die "dry run failed"; }
grep -qF "Dry run: revision 1 of rp-host-app" <<<"$out" || { echo "$out"; die "no dry-run header"; }
grep -qF "4 file(s) added" <<<"$out" || { echo "$out"; die "the dry run does not count 4 files to add"; }
[ -e /restore/new ] && die "the dry run created the destination"

log "DRY_RUN over the live directory with OVERWRITE and DELETE_EXTRA"
echo changed >"$SERVICES/app/top.txt"; echo extra >"$SERVICES/app/extra.txt"
out=$(SNAPSHOT_ID=rp-host-app LOCAL_DIR="$SERVICES/app" DRY_RUN=1 OVERWRITE=1 DELETE_EXTRA=1 archiver auto-restore 2>&1) || { echo "$out"; die "dry run failed"; }
grep -qF "1 file(s) replaced" <<<"$out" || { echo "$out"; die "the changed file is not shown as replaced"; }
grep -qF "1 file(s) deleted" <<<"$out" || { echo "$out"; die "the extra file is not shown as deleted"; }
grep -qF "3 file(s) already match" <<<"$out" || { echo "$out"; die "the unchanged files are not shown as matching"; }
[ "$(cat "$SERVICES/app/top.txt")" = changed ] || die "the dry run changed a file"

log "RESTORE_PATHS restores only the paths named"
SNAPSHOT_ID=rp-host-app LOCAL_DIR=/restore/part RESTORE_PATHS="config/,top.txt" archiver auto-restore >/tmp/part.out 2>&1 \
  || { cat /tmp/part.out; die "the partial restore failed"; }
got=$(cd /restore/part && find . -path ./.duplicacy -prune -o -type f -print | sort | tr '\n' ' ')
[ "$got" = "./config/a.conf ./config/sub/b.conf ./top.txt " ] || die "partial restore produced: $got"

echo "PASS: restore preview"
