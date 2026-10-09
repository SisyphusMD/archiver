#!/usr/bin/env bash
# Restore drills (ADR 28) against real duplicacy: a drill restores a service's newest
# revision into the drill directory, checks it and deletes the copy; the next drill moves to
# the next service; status and the state record each result; a damaged chunk fails the drill
# with an error, and an undamaged storage still passes.
#
#   docker run -i --rm --hostname rd-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/restore-drill.sh

set -uo pipefail

SERVICES=/data/services
STORE1=/backup-store
STORE2=/backup-offsite
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
STATE=/opt/archiver/logs/.drill-state.json

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; tail -20 /opt/archiver/logs/drill.log >&2 2>/dev/null; exit 1; }
# state EXPR: true when the Python expression holds for the drill state r (its "results").
state() { python3 -c 'import json,sys; s=json.load(open(sys.argv[1])); r=s.get("results", {}); sys.exit(0 if eval(sys.argv[2]) else 1)' "$STATE" "$1"; }

log "materialize RSA keypair + file secrets (env-native mode)"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE1" "$STORE2"
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null \
  || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null \
  || die "openssl rsa -pubout"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${RSA_PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"

export STORAGE_TARGET_1_NAME="local" STORAGE_TARGET_1_TYPE="local" STORAGE_TARGET_1_LOCAL_PATH="${STORE1}"
export STORAGE_TARGET_2_NAME="offsite" STORAGE_TARGET_2_TYPE="local" STORAGE_TARGET_2_LOCAL_PATH="${STORE2}"
export SERVICE_DIRECTORIES="${SERVICES}/*/"

for s in app db web; do
  mkdir -p "$SERVICES/$s/sub"
  for i in 1 2 3; do head -c 20000 /dev/urandom >"$SERVICES/$s/f$i.bin"; done
  echo "$s" >"$SERVICES/$s/sub/name.txt"
  : >"$SERVICES/$s/empty.txt"  # duplicacy's listing summary leaves out empty files
done

log "back up three services to both storages"
archiver backup >/tmp/backup.out 2>&1 || { cat /tmp/backup.out; die "backup failed"; }

log "a drill restores one service per storage, verifies it, and deletes the copy"
archiver drill >/tmp/drill1.out 2>&1 || { cat /tmp/drill1.out; die "the first drill failed"; }
state 'r["local"]["app"]["ok"] and r["local"]["app"]["files"] == 5 and r["offsite"]["app"]["ok"]'  \
  || { cat "$STATE"; die "the drill state does not record app passing on both storages with 5 files"; }
[ -z "$(ls -A /tmp/archiver-drill 2>/dev/null)" ] || die "drill copies were left in /tmp/archiver-drill"
status=$(archiver status)
grep -q "app on local: revision 1 passed" <<<"$status" || { echo "$status"; die "status does not show the drill"; }

log "the next drill moves on to the next service"
archiver drill >/tmp/drill2.out 2>&1 || { cat /tmp/drill2.out; die "the second drill failed"; }
state 'r["local"]["db"]["ok"]'  || { cat "$STATE"; die "the second drill did not drill db"; }

log "a damaged chunk on the primary fails the drill; the offsite copy still passes"
[ -n "$(find "$STORE1/chunks" -type f)" ] || die "no chunk found to damage"
for c in $(find "$STORE1/chunks" -type f); do printf 'garbage' | dd of="$c" bs=1 seek=100 conv=notrunc 2>/dev/null; done
archiver drill web >/tmp/drill3.out 2>&1 && { cat /tmp/drill3.out; die "a drill over damaged chunks exited 0"; }
state 'not r["local"]["web"]["ok"] and r["offsite"]["web"]["ok"] and s.get("last_failed")'  \
  || { cat "$STATE"; die "the state does not record local failing and offsite passing for web"; }
grep -q "Restore drill of rd-host-web from 'local' failed" /opt/archiver/logs/drill.log \
  || die "the failure is not logged"

echo "PASS: restore drills"
