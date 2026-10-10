#!/usr/bin/env bash
# Interrupted runs resume (ADR 46): a backup killed between a service's hooks has that
# post-backup hook run on the next start (ARCHIVER_BACKUP_RESULT=interrupted, with what its
# pre-backup hook left), then backs up its unfinished services first; the daemon runs an
# interrupted backup, maintenance or drill at once; a stop for a shutdown keeps the run to
# resume and a stop asked for does not; a stopped duplicacy saves its resume point and the
# next backup uses it; an interrupted drill's copies are deleted.
#
#   docker run -i --rm --hostname fi-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/fault-interrupt.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store
LOGS=/opt/archiver/logs

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; tail -25 $LOGS/archiver.log >&2 2>/dev/null; exit 1; }
wait_for() { local _; for _ in $(seq 1 "$2"); do eval "$1" && return 0; sleep 1; done; return 1; }

mkdir -p /opt/archiver/keys /run/secrets "$STORE" "$SERVICES/db" "$SERVICES/app"
openssl genrsa -aes256 -passout pass:pp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:pp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "openssl rsa"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >/run/secrets/storage_password; printf 'pp' >/run/secrets/rsa_passphrase
export STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH="$STORE"
export SERVICE_DIRECTORIES="$SERVICES/db:$SERVICES/app" BACKUP_PARALLELISM=1
echo d >"$SERVICES/db/f"; echo a >"$SERVICES/app/f"
# db's hooks: pre leaves a token in the state directory, post records how the run went and
# whether it found the token.
printf '#!/bin/sh\necho token > "$ARCHIVER_STATE_DIR/token"\ntouch /tmp/db-down\n' >"$SERVICES/db/pre-backup"
printf '#!/bin/sh\necho "$ARCHIVER_BACKUP_RESULT $(cat "$ARCHIVER_STATE_DIR/token" 2>/dev/null)" >> /tmp/db-post\nrm -f /tmp/db-down\n' >"$SERVICES/db/post-backup"
chmod 755 "$SERVICES/db/pre-backup" "$SERVICES/db/post-backup"
archiver backup >/tmp/b0.out 2>&1 || { cat /tmp/b0.out; die "first backup failed"; }
: >/tmp/db-post

# A shadow duplicacy holds a backup of the directory named in /tmp/hold, to be killed there.
REAL="$(command -v duplicacy)"
mv "$REAL" "${REAL}.real"
cat >"$REAL" <<'WRAP'
#!/usr/bin/env bash
if [ "${2:-}" = backup ] && [ -s /tmp/hold ] && [ "$PWD" = "$(cat /tmp/hold)" ]; then
  touch /tmp/holding
  exec sleep 300
fi
exec "$0.real" "$@"
WRAP
chmod +x "$REAL"
crash() { # archiver backup, killed while duplicacy backs up $1
  echo "$1" >/tmp/hold
  archiver backup >/tmp/crash.out 2>&1 &
  local pid=$!
  wait_for '[ -e /tmp/holding ]' 30 || die "the backup never reached $1"
  kill -9 "$pid"; pkill -9 -f "sleep 300"; wait "$pid" 2>/dev/null
  : >/tmp/hold; rm -f /tmp/holding
}

log "killed between db's hooks: the next run first runs db's post hook as interrupted, with its pre hook's token"
crash "$SERVICES/db"
[ -e /tmp/db-down ] || die "db's pre hook did not run before the crash"
[ -s /tmp/db-post ] && die "db's post hook ran although the run was killed"
grep -q '"hooked"' $LOGS/.run-backup.json || die "the record does not show db between its hooks"
archiver backup >/tmp/b1.out 2>&1 || { cat /tmp/b1.out; die "the backup after the crash failed"; }
[ "$(head -1 /tmp/db-post)" = "interrupted token" ] || die "db's post hook did not run as interrupted with its token: $(cat /tmp/db-post)"
[ ! -e /tmp/db-down ] || die "db was left down"
[ "$(wc -l </tmp/db-post)" = 2 ] || die "db's hooks did not run again in the backup itself"
[ ! -e $LOGS/.run-backup.json ] || die "a finished run left its record"

log "killed during app, after db finished: the next run backs up app first"
crash "$SERVICES/app"
: >/tmp/db-post
archiver backup >/tmp/b2.out 2>&1 || { cat /tmp/b2.out; die "the backup after the crash failed"; }
order=$(grep -o 'Processing [a-z]* service' $LOGS/archiver.log | tr '\n' ' ')
[ "$order" = "Processing app service Processing db service " ] || die "unfinished app was not first: $order"
[ "$(cat /tmp/db-post)" = "success token" ] || die "db's post hook saw $(cat /tmp/db-post)"

log "the daemon runs an interrupted backup and maintenance at once"
crash "$SERVICES/app"
printf '{"started":1}' >$LOGS/.run-maintenance.json
BACKUP_SCHEDULE="0 3 1 1 *" MAINTENANCE_SCHEDULE="0 4 1 1 *" archiver daemon >/tmp/d.out 2>&1 &
d=$!
wait_for '! [ -e '$LOGS'/.run-backup.json ] && ! [ -e '$LOGS'/.run-maintenance.json ]' 120 || { cat /tmp/d.out; die "the daemon did not finish the interrupted runs"; }
grep -q 'starting backup now: the last one was interrupted' /tmp/d.out || die "the backup was not run at once"
grep -q 'starting maintenance now: the last one was interrupted' /tmp/d.out || die "maintenance was not run at once"
kill $d; wait $d 2>/dev/null

log "a stop for a shutdown keeps the run to resume; a stop asked for does not"
printf '#!/bin/sh\necho token > "$ARCHIVER_STATE_DIR/token"\nsleep 4\n' >"$SERVICES/db/pre-backup"
for how in --shutdown ""; do
  archiver backup >/tmp/b3.out 2>&1 &
  pid=$!
  wait_for 'grep -q "\"hooked\"" '$LOGS'/.run-backup.json 2>/dev/null' 30 || die "db's pre hook never started"
  archiver stop $how >/dev/null 2>&1
  wait "$pid" 2>/dev/null
  if [ -n "$how" ]; then
    [ -e $LOGS/.run-backup.json ] || die "a stop for a shutdown did not keep the run"
    grep -q '"hooked"' $LOGS/.run-backup.json && die "db's post hook ran, yet the record still has it between its hooks"
  else
    [ ! -e $LOGS/.run-backup.json ] || die "a stop asked for kept the run to resume"
  fi
  rm -f /var/lock/archiver-shutting-down
done
printf '#!/bin/sh\necho token > "$ARCHIVER_STATE_DIR/token"\n' >"$SERVICES/db/pre-backup"

log "a stopped first backup saves duplicacy's resume point, and the next backup uses it"
mkdir -p "$SERVICES/big"; head -c 400000000 /dev/urandom >"$SERVICES/big/blob"
SERVICE_DIRECTORIES="$SERVICES/big" DUPLICACY_THREADS=1 archiver backup >/tmp/b4.out 2>&1 &
pid=$!
wait_for '[ "$(find '$STORE'/chunks -type f -newer /tmp/b4.out | wc -l)" -gt 3 ]' 60 || die "the big backup never uploaded"
archiver stop >/dev/null 2>&1
wait "$pid" 2>/dev/null
grep -q 'Incomplete snapshot saved' $LOGS/archiver.log || die "duplicacy did not save its resume point when stopped"
SERVICE_DIRECTORIES="$SERVICES/big" archiver backup >/tmp/b5.out 2>&1 || { cat /tmp/b5.out; die "the backup after the stop failed"; }
grep -q 'Previous incomplete backup contains' $LOGS/archiver.log || die "the next backup did not use the resume point"
rm -rf "$SERVICES/big"

log "an incremental backup killed outright (no signal handled, as in a power cut) resumes from its last periodic save"
mkdir -p "$SERVICES/inc"; echo first >"$SERVICES/inc/small"
SERVICE_DIRECTORIES="$SERVICES/inc" archiver backup >/tmp/b6.out 2>&1 || { cat /tmp/b6.out; die "the first backup of inc failed"; }
head -c 300000000 /dev/urandom >"$SERVICES/inc/blob1"; head -c 100000000 /dev/urandom >"$SERVICES/inc/blob2"
SERVICE_DIRECTORIES="$SERVICES/inc" DUPLICACY_THREADS=1 DUPLICACY_RESUME_INTERVAL=0 archiver backup >/tmp/b7.out 2>&1 &
pid=$!
wait_for '[ -e '$SERVICES'/inc/.duplicacy/cache/local/incomplete_chunks ] && [ "$(find '$STORE'/chunks -type f -newer /tmp/b6.out | wc -l)" -gt 20 ]' 90 || die "the incremental backup never saved a resume point"
kill -9 "$pid"; pkill -9 -f 'duplicacy.real .*backup'; wait "$pid" 2>/dev/null
grep -q 'Incomplete snapshot saved' $LOGS/archiver.log && die "the killed backup handled a signal; this case is for one that cannot"
SERVICE_DIRECTORIES="$SERVICES/inc" archiver backup >/tmp/b8.out 2>&1 || { cat /tmp/b8.out; die "the backup after the kill failed"; }
grep -q 'Previous incomplete backup contains' $LOGS/archiver.log || die "the backup after the kill did not resume from the periodic save"
SNAPSHOT_ID=fi-host-inc LOCAL_DIR=/restore/inc archiver auto-restore >/tmp/r1.out 2>&1 || { cat /tmp/r1.out; die "the resumed revision does not restore"; }
for f in small blob1 blob2; do cmp "$SERVICES/inc/$f" "/restore/inc/$f" || die "$f restored different bytes"; done
rm -rf "$SERVICES/inc" /restore/inc

log "an interrupted drill's copies are deleted by the next drill"
mkdir -p /tmp/archiver-drill/drill-local-app-leftover && echo x >/tmp/archiver-drill/drill-local-app-leftover/f
# The record also names a service's data, as a remount since the drill could make it: never
# deleted.
printf '{"started":1,"services":{"/tmp/archiver-drill/drill-local-app-leftover":"pending","'"$SERVICES"'/app":"pending"},"dir":"/tmp/archiver-drill"}' >$LOGS/.run-drill.json
archiver drill >/tmp/dr.out 2>&1 || { cat /tmp/dr.out; tail -20 $LOGS/drill.log; die "the drill failed"; }
[ ! -e /tmp/archiver-drill/drill-local-app-leftover ] || die "the interrupted drill's copy was left"
[ -e "$SERVICES/app/f" ] || die "a recorded path in a service directory was deleted"
grep -q "Not deleting $SERVICES/app" $LOGS/drill.log || die "no word of the refused deletion"
[ ! -e $LOGS/.run-drill.json ] || die "the drill left its record"

echo "PASS: interrupted runs resume"
