#!/usr/bin/env bash
# Fault injection, killed processes (ADR 31): duplicacy, a hook, archiver itself and a copy
# worker's copy are killed at chosen points. Each run fails or retries as designed, its lock
# is released (or found stale and cleared), nothing half-written becomes the newest revision,
# and the next run proceeds and restores.
#
#   docker run -i --rm --hostname fk-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/fault-kill.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store
OFFSITE=/backup-offsite
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
ID=fk-host-app

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; tail -15 /opt/archiver/logs/archiver.log >&2 2>/dev/null; exit 1; }
wait_for() { local _; for _ in $(seq 1 "$2"); do eval "$1" && return 0; sleep 1; done; return 1; }
revisions() { ls "$1/snapshots/$ID" 2>/dev/null | wc -l; }

mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE" "$OFFSITE"
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "openssl rsa"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${RSA_PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"
export STORAGE_TARGET_1_NAME="local" STORAGE_TARGET_1_TYPE="local" STORAGE_TARGET_1_LOCAL_PATH="${STORE}"
export SERVICE_DIRECTORIES="${SERVICES}/*/"
mkdir -p "$SERVICES/app" && head -c 20000 /dev/urandom >"$SERVICES/app/data.bin"
archiver backup >/tmp/b0.out 2>&1 || { cat /tmp/b0.out; die "first backup failed"; }
[ "$(revisions "$STORE")" -eq 1 ] || die "expected one revision"

# The real duplicacy stays; a shadow holds the commands named in /tmp/hold so they can be
# killed mid-run.
REAL="$(command -v duplicacy)"
mv "$REAL" "${REAL}.real"
cat >"$REAL" <<'WRAP'
#!/usr/bin/env bash
if grep -qx "${2:-}" /tmp/hold 2>/dev/null; then
  touch "/tmp/holding-$2"
  exec sleep 300 # killing it is killing the duplicacy process itself
fi
exec "$0.real" "$@"
WRAP
chmod +x "$REAL"

log "duplicacy killed mid-backup: the run fails, no revision is added, the lock is released"
echo backup >/tmp/hold
archiver backup >/tmp/b1.out 2>&1 &
pid=$!
wait_for '[ -e /tmp/holding-backup ]' 30 || die "the backup never reached duplicacy"
pkill -9 -f "sleep 300"
wait "$pid" && die "a backup whose duplicacy was killed exited 0"
[ "$(revisions "$STORE")" -eq 1 ] || die "a killed backup added a revision"
grep -q "Backup: not running" <<<"$(archiver status)" || die "the lock was not released"
: >/tmp/hold; rm -f /tmp/holding-*

log "a hook killed mid-run: its service fails and post-backup still runs"
printf '#!/bin/sh\ntouch /tmp/pre-started\nsleep 300\n' >"$SERVICES/app/pre-backup"
printf '#!/bin/sh\ntouch /tmp/post-ran\n' >"$SERVICES/app/post-backup"
chmod 755 "$SERVICES/app/pre-backup" "$SERVICES/app/post-backup"
archiver backup >/tmp/b2.out 2>&1 &
pid=$!
wait_for '[ -e /tmp/pre-started ]' 30 || die "the pre-backup hook never started"
pkill -9 -f "sleep 300"
wait "$pid" && die "a backup whose pre-backup hook was killed exited 0"
[ -e /tmp/post-ran ] || die "post-backup did not run after the pre-backup hook was killed"
[ "$(revisions "$STORE")" -eq 1 ] || die "a service whose pre-backup hook was killed was backed up"
rm -f "$SERVICES/app/pre-backup" "$SERVICES/app/post-backup" /tmp/pre-started /tmp/post-ran

log "archiver itself killed mid-backup (as in a crash): the next backup clears the stale lock and proceeds"
echo backup >/tmp/hold
archiver backup >/tmp/b3.out 2>&1 &
pid=$!
wait_for '[ -e /tmp/holding-backup ]' 30 || die "the backup never reached duplicacy"
kill -9 "$pid"; pkill -9 -f "sleep 300"; wait "$pid" 2>/dev/null
: >/tmp/hold; rm -f /tmp/holding-*
[ "$(revisions "$STORE")" -eq 1 ] || die "a crashed backup added a revision"
echo more >>"$SERVICES/app/data.bin"
archiver backup >/tmp/b4.out 2>&1 || { cat /tmp/b4.out; die "the backup after a crash failed"; }
[ "$(revisions "$STORE")" -eq 2 ] || die "the backup after a crash did not add its revision"
SNAPSHOT_ID=$ID LOCAL_DIR=/restore/after-crash archiver auto-restore >/tmp/r.out 2>&1 || { cat /tmp/r.out; die "the revision after a crash does not restore"; }
cmp "$SERVICES/app/data.bin" /restore/after-crash/data.bin || die "the revision after a crash restored different bytes"

log "a copy worker's copy killed: the worker retries and catches up"
export STORAGE_TARGET_2_NAME="offsite" STORAGE_TARGET_2_TYPE="local" STORAGE_TARGET_2_LOCAL_PATH="${OFFSITE}"
echo copy >/tmp/hold
BACKUP_SCHEDULE="0 3 1 1 *" archiver daemon >/tmp/daemon.out 2>&1 &
dpid=$!
wait_for '[ -e /tmp/holding-copy ]' 90 || die "the worker never started its copy"
: >/tmp/hold
pkill -9 -f "sleep 300"
wait_for 'grep -q "offsite: retrying in" <<<"$(archiver status)"' 60 || die "the worker did not retry after its copy was killed"
wait_for '[ "$(revisions "$OFFSITE")" -eq 2 ]' 150 || die "the worker did not catch up after its copy was killed"
kill "$dpid"; wait "$dpid" 2>/dev/null

echo "PASS: fault injection, killed processes"
