#!/usr/bin/env bash
# Copy workers (ADR 11): under the daemon, a backup ends after the local backup and hands
# the copies to one worker per secondary, which catches its target up, retries a failed
# copy on its backoff instead of failing the backup, and follows stop/pause/resume.
#
#   docker run -i --rm --hostname cw-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/copy-workers.sh

set -uo pipefail

SVC=/data/services/app
STORE=/backup-store
OFFSITE=/offsite-store
OFFSITE2=/offsite2-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
LOG=/opt/archiver/logs/archiver.log
CLOG=/opt/archiver/logs/copies.log
HOST="$(hostname)"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; tail -20 "$CLOG" >&2 2>/dev/null; archiver status >&2; exit 1; }
revisions() { ls "$1/snapshots/${HOST}-app" 2>/dev/null | wc -l; }
wait_for() { local _; for _ in $(seq 1 "$2"); do eval "$1" && return 0; sleep 1; done; return 1; }

log "env-native config with a local primary and a local offsite"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE" "$OFFSITE" "$OFFSITE2" "$SVC"
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "pubout"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${RSA_PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"
export SERVICE_DIRECTORIES="/data/services/*/"
export STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH="$STORE"
export STORAGE_TARGET_2_NAME=offsite STORAGE_TARGET_2_TYPE=local STORAGE_TARGET_2_LOCAL_PATH="$OFFSITE"
export STORAGE_TARGET_3_NAME=offsite2 STORAGE_TARGET_3_TYPE=local STORAGE_TARGET_3_LOCAL_PATH="$OFFSITE2"
echo "one" >"$SVC/file.txt"

log "without a daemon a backup copies inline, as before"
archiver backup >/dev/null 2>&1 || die "inline backup failed"
[ "$(revisions "$OFFSITE")" -eq 1 ] || die "inline copy did not reach offsite"

log "start the daemon (a far-off schedule, so only the workers act) with offsite2 unreachable"
mv "$OFFSITE2" "${OFFSITE2}.away" && touch "$OFFSITE2"
BACKUP_SCHEDULE="0 3 1 1 *" archiver daemon >/tmp/daemon.out 2>&1 &
DAEMON=$!
wait_for 'grep -q "Copy workers started" "$CLOG" 2>/dev/null' 60 || die "workers never started"
wait_for 'archiver status | grep -q "offsite: caught up"' 30 || die "worker did not report caught up"
wait_for 'archiver status | grep -q "offsite2: retrying in"' 30 || die "unreachable offsite2 not retrying"

log "a backup hands its copies to the worker and ends"
sleep 1.1; echo "two" >"$SVC/file.txt"
archiver backup >/dev/null 2>&1 || die "backup failed"
grep -q "run in the background" "$LOG" || die "backup did not hand off its copies"
grep -q "Copying backup to offsite storage" "$LOG" && die "backup copied inline despite the workers"
wait_for '[ "$(revisions "$OFFSITE")" -eq 2 ]' 60 || die "worker did not copy revision 2 (an unreachable offsite2 must not hold it up)"

log "an unreachable offsite fails the copy, not the backup, and is retried"
mv "$OFFSITE" "${OFFSITE}.away" && touch "$OFFSITE"
sleep 1.1; echo "three" >"$SVC/file.txt"
archiver backup >/dev/null 2>&1 || die "a broken offsite failed the backup itself"
wait_for 'archiver status | grep -q "offsite: retrying in"' 60 || die "status does not show the retry"
grep -q "\[WARNING\] \[Service: offsite\] Copy to offsite storage failed" "$CLOG" || die "failed copy not logged as a warning"

log "pause and resume reach the workers"
archiver pause | grep -q "Copies to the secondary storages paused" || die "pause did not reach the workers"
archiver status | grep -q "(paused)" || die "status does not show the pause"
archiver resume | grep -q "Copies to the secondary storages resumed" || die "resume did not reach the workers"

log "once the offsites are back, a backup's wake catches them up"
rm "$OFFSITE" && mv "${OFFSITE}.away" "$OFFSITE"
rm "$OFFSITE2" && mv "${OFFSITE2}.away" "$OFFSITE2"
sleep 1.1; echo "four" >"$SVC/file.txt"
archiver backup >/dev/null 2>&1 || die "backup failed"
wait_for '[ "$(revisions "$OFFSITE")" -eq 4 ]' 60 || die "offsite not caught up to 4 revisions"
wait_for '[ "$(revisions "$OFFSITE2")" -eq 4 ]' 60 || die "offsite2 not caught up to 4 revisions"
wait_for 'archiver status | grep -q "offsite: caught up"' 30 || die "status not caught up"
wait_for 'archiver status | grep -q "offsite2: caught up"' 30 || die "offsite2 status not caught up"

log "stop reaches the workers"
archiver stop | grep -q "Copies to the secondary storages stopped" || die "stop did not reach the workers"

kill -TERM "$DAEMON"; wait "$DAEMON"
echo "=== COPY-WORKERS OK: inline without a daemon, handed off with one, retried without failing the backup, pause/resume/stop reach the workers ==="
