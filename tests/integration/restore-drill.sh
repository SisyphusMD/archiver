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

log "a service drill hook checks the scratch copy and it is deleted afterwards"
cat >"$SERVICES/app/drill" <<'HOOK'
#!/bin/sh
[ "$PWD" = "$ARCHIVER_DRILL_DIR" ] || exit 2
[ -f "$ARCHIVER_DRILL_DIR/sub/name.txt" ] || exit 3
[ -z "${STORAGE_PASSWORD+x}${RSA_PASSPHRASE+x}${DUPLICACY_LOCAL_PASSWORD+x}" ] || exit 4
printf '%s\n' "$ARCHIVER_DRILL_DIR" >> /tmp/drill-hook-ran
HOOK
chmod 700 "$SERVICES/app/drill"
archiver drill app >/tmp/drill-hook.out 2>&1 || { cat /tmp/drill-hook.out; die "passing hook failed"; }
[ "$(wc -l </tmp/drill-hook-ran)" -eq 2 ] || die "hook did not run on both storages"
while IFS= read -r scratch; do
  case "$scratch" in /tmp/archiver-drill/drill-*) ;; *) die "hook ran outside scratch";; esac
  [ ! -e "$scratch" ] || die "hook scratch was not deleted"
done </tmp/drill-hook-ran
[ ! -e "$SERVICES/app/drill-hook-ran" ] || die "hook touched live service"

log "a failed drill hook fails the drill and its result names the exit"
printf '#!/bin/sh\nexit 1\n' >"$SERVICES/db/drill"
chmod 700 "$SERVICES/db/drill"
archiver drill db >/tmp/drill-hook-fail.out 2>&1 && die "failed hook passed"
state 'not r["local"]["db"]["ok"] and not r["offsite"]["db"]["ok"] and "exit 1" in r["local"]["db"]["message"] and s.get("last_failed")' || die "hook failure was not recorded"
grep -q 'its drill hook failed.*exit 1' /tmp/drill-hook-fail.out || die "hook failure was not reported"
[ -z "$(ls -A /tmp/archiver-drill)" ] || die "failed hook left scratch copies"

log "failed selections retry before rotation advances"
printf '#!/bin/sh\nexit 0\n' >"$SERVICES/db/drill"
archiver drill >/tmp/drill-retry.out 2>&1 || { cat /tmp/drill-retry.out; die "failed selection retry failed"; }
state 'r["local"]["db"]["ok"] and r["offsite"]["db"]["ok"] and not s.get("pending", {}).get("local") and r["local"]["db"].get("last_success", 0) > 0 and r["local"]["db"].get("seconds", 0) > 0' || die "failed selection did not pass on retry"

log "whole-run timeout ends a hook and keeps the selection due"
printf '#!/bin/sh\nsleep 30 & wait\n' >"$SERVICES/db/drill"
RESTORE_DRILL_TIMEOUT=1s archiver drill db local >/tmp/drill-limit.out 2>&1 && die "whole-run timeout passed"
state 's.get("last_failed") and "db" in s["pending"]["local"] and not r["local"]["db"]["ok"]' || die "timeout did not remain due"
printf '#!/bin/sh\nexit 0\n' >"$SERVICES/db/drill"
archiver drill >/tmp/drill-limit-retry.out 2>&1 || { cat /tmp/drill-limit-retry.out; die "timeout retry failed"; }
state 'r["local"]["db"]["ok"] and not s.get("pending", {}).get("local")' || die "timeout selection did not retry"

log "a damaged chunk on the primary fails the drill; the offsite copy still passes"
[ -n "$(find "$STORE1/chunks" -type f)" ] || die "no chunk found to damage"
for c in $(find "$STORE1/chunks" -type f); do printf 'garbage' | dd of="$c" bs=1 seek=100 conv=notrunc 2>/dev/null; done
archiver drill web >/tmp/drill3.out 2>&1 && { cat /tmp/drill3.out; die "a drill over damaged chunks exited 0"; }
state 'not r["local"]["web"]["ok"] and r["offsite"]["web"]["ok"] and s.get("last_failed")'  \
  || { cat "$STATE"; die "the state does not record local failing and offsite passing for web"; }
grep -q "Restore drill of rd-host-web from 'local' failed" /opt/archiver/logs/drill.log \
  || die "the failure is not logged"

echo "PASS: restore drills"
