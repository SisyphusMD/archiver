#!/usr/bin/env bash
# The non-interactive restore commands' interface, which nas, vps and the restore drills
# script against: snapshot-exists answers EXISTS/NOT FOUND/UNDETERMINED with exit 0/1/2;
# auto-restore falls back across targets, honors STORAGE_TARGET and REVISION, and exits
# 1 (not found, failed, hook failed), 2 (unreachable or invalid env) or 3 (backup running);
# auto-restore-all restores every service with RUN_RESTORE_SERVICE and reports failures.
#
#   docker run -i --rm --hostname rc-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/restore-commands.sh

set -uo pipefail

STORE=/backup-store
OFFSITE=/offsite-store
SECRETS_DIR=/run/secrets
LOCKFILE=/var/lock/archiver-main.lock
HOST="$(hostname)"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
# expect CODE|fail OUTPUT_PATTERN DESCRIPTION -- ENV... : runs `archiver` with ENV, checks both.
expect() {
  local want="$1" pattern="$2" what="$3"; shift 4
  env "$@" >/tmp/out 2>&1
  local rc=$?
  if [ "$want" = fail ]; then
    [ "$rc" -ne 0 ] || { cat /tmp/out; die "$what: exit 0, want a failure"; }
  else
    [ "$rc" -eq "$want" ] || { cat /tmp/out; die "$what: exit $rc, want $want"; }
  fi
  [ -z "$pattern" ] || grep -qE "$pattern" /tmp/out || { cat /tmp/out; die "$what: output lacks /$pattern/"; }
}

log "two services backed up to a local primary and copied to a local secondary"
mkdir -p /opt/archiver/keys "$SECRETS_DIR" "$STORE" "$OFFSITE" /data/services/app /data/services/web
openssl genrsa -aes256 -passout pass:rp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:rp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "pubout"
printf 'testpassword' >"$SECRETS_DIR/storage_password"
printf 'rp' >"$SECRETS_DIR/rsa_passphrase"
export SERVICE_DIRECTORIES="/data/services/*/"
export STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH="$STORE"
export STORAGE_TARGET_2_NAME=offsite STORAGE_TARGET_2_TYPE=local STORAGE_TARGET_2_LOCAL_PATH="$OFFSITE"
echo "app one" >/data/services/app/file.txt
echo "web one" >/data/services/web/file.txt
printf '#!/bin/bash\necho "app hook in $PWD" > hook-marker.txt\n' >/data/services/app/restore-service.sh
printf '#!/bin/bash\nexit 7\n' >/data/services/web/restore-service.sh
archiver backup >/tmp/b1 2>&1 || { cat /tmp/b1; die "backup 1 failed"; }
sleep 1.1
echo "app two (changed)" >/data/services/app/file.txt
archiver backup >/tmp/b2 2>&1 || { cat /tmp/b2; die "backup 2 failed"; }

log "snapshot-exists"
expect 0 '^EXISTS$' "an existing snapshot" -- SNAPSHOT_ID="$HOST-app" archiver snapshot-exists
expect 1 '^NOT FOUND$' "a missing snapshot" -- SNAPSHOT_ID="$HOST-nope" archiver snapshot-exists
expect 2 '' "no SNAPSHOT_ID" -- archiver snapshot-exists
expect 2 '^UNDETERMINED$' "every target unreachable" -- SNAPSHOT_ID="$HOST-app" \
  STORAGE_TARGET_1_LOCAL_PATH=/etc/hostname STORAGE_TARGET_2_LOCAL_PATH=/etc/hostname archiver snapshot-exists
expect 0 '^EXISTS$' "found on the second target when the first is unreachable" -- SNAPSHOT_ID="$HOST-app" \
  STORAGE_TARGET_1_LOCAL_PATH=/etc/hostname archiver snapshot-exists

log "auto-restore: argument and environment errors"
expect 2 'SNAPSHOT_ID' "no SNAPSHOT_ID" -- LOCAL_DIR=/data/r archiver auto-restore
expect 2 'LOCAL_DIR' "no LOCAL_DIR" -- SNAPSHOT_ID="$HOST-app" archiver auto-restore
expect 2 'STORAGE_TARGET' "an unknown STORAGE_TARGET name" -- SNAPSHOT_ID="$HOST-app" LOCAL_DIR=/data/r STORAGE_TARGET=nope archiver auto-restore
expect 2 'STORAGE_TARGET' "an out-of-range STORAGE_TARGET id" -- SNAPSHOT_ID="$HOST-app" LOCAL_DIR=/data/r STORAGE_TARGET=9 archiver auto-restore

log "auto-restore: not found, missing revision, unreachable"
expect 1 'not found' "a missing snapshot" -- SNAPSHOT_ID="$HOST-nope" LOCAL_DIR=/data/r-missing archiver auto-restore
expect 1 '' "a missing revision" -- SNAPSHOT_ID="$HOST-app" LOCAL_DIR=/data/r-rev REVISION=99 archiver auto-restore
expect 2 'unreachable' "every target unreachable" -- SNAPSHOT_ID="$HOST-app" LOCAL_DIR=/data/r-unr \
  STORAGE_TARGET_1_LOCAL_PATH=/etc/hostname STORAGE_TARGET_2_LOCAL_PATH=/etc/hostname archiver auto-restore

log "auto-restore: falls back to the secondary, and STORAGE_TARGET pins one"
expect 0 '' "fallback restore" -- SNAPSHOT_ID="$HOST-app" LOCAL_DIR=/data/r-fallback STORAGE_TARGET_1_LOCAL_PATH=/etc/hostname archiver auto-restore
grep -q "app two" /data/r-fallback/file.txt || die "the fallback restore is not the latest revision"
expect 0 '' "pinned to offsite by name, revision 1" -- SNAPSHOT_ID="$HOST-app" LOCAL_DIR=/data/r-pin STORAGE_TARGET=offsite REVISION=1 archiver auto-restore
grep -q "app one" /data/r-pin/file.txt || die "REVISION=1 from offsite did not restore revision 1"
expect 1 '' "pinned to an unreachable target, never falling back" -- SNAPSHOT_ID="$HOST-app" LOCAL_DIR=/data/r-pin2 STORAGE_TARGET=1 \
  STORAGE_TARGET_1_LOCAL_PATH=/var/empty-store archiver auto-restore
[ ! -e /data/r-pin2/file.txt ] || die "a pinned restore fell back to another target"

log "auto-restore: the restore hook runs only when asked, and its failure fails the restore"
expect 0 '' "restore without RUN_RESTORE_SERVICE" -- SNAPSHOT_ID="$HOST-app" LOCAL_DIR=/data/r-nohook archiver auto-restore
[ ! -e /data/r-nohook/hook-marker.txt ] || die "the restore hook ran without RUN_RESTORE_SERVICE"
expect fail '' "a failing restore hook" -- SNAPSHOT_ID="$HOST-web" LOCAL_DIR=/data/r-webhook RUN_RESTORE_SERVICE=1 archiver auto-restore
grep -q "web one" /data/r-webhook/file.txt || die "the files were not restored before the hook"

log "a running backup refuses restores and probes with exit 3"
sleep 300 &
holder=$!
printf '%s backup backup\n' "$holder" >"$LOCKFILE"
expect 3 '' "snapshot-exists under a backup" -- SNAPSHOT_ID="$HOST-app" archiver snapshot-exists
expect 3 '' "auto-restore under a backup" -- SNAPSHOT_ID="$HOST-app" LOCAL_DIR=/data/r-lock archiver auto-restore
expect 3 '' "auto-restore-all under a backup" -- archiver auto-restore-all
kill "$holder"; wait "$holder" 2>/dev/null; rm -f "$LOCKFILE"

log "auto-restore-all onto blank service directories"
rm -rf /data/services/app /data/services/web
mkdir -p /data/services/app /data/services/web
expect 1 'FAILED: +web' "auto-restore-all with web's hook failing" -- RUN_RESTORE_SERVICE=1 archiver auto-restore-all
grep -q "app two" /data/services/app/file.txt || die "auto-restore-all did not restore app's latest revision"
grep -q "app hook in /data/services/app" /data/services/app/hook-marker.txt || die "app's restore hook did not run in its directory"
grep -q "web one" /data/services/web/file.txt || die "auto-restore-all did not restore web"
grep -qE 'restored: +app' /tmp/out || { cat /tmp/out; die "the summary does not list app as restored"; }
rm /data/services/web/restore-service.sh
expect 0 'All services restored' "auto-restore-all, all succeeding" -- archiver auto-restore-all

log "a snapshot from before the hooks migration: a settings file and a post-restore hook"
mkdir -p /data/old-src
echo "old data" >/data/old-src/file.txt
printf 'service_specific_pre_backup_function() { echo pre; }\n' >/data/old-src/service-backup-settings.sh
printf '#!/bin/sh\necho "post-restore $ARCHIVER_SNAPSHOT_ID r$ARCHIVER_RESTORE_REVISION" > post-restore-ran\n' >/data/old-src/post-restore
chmod +x /data/old-src/post-restore
# Written by duplicacy itself, as a pre-migration release's backup would have been.
(cd /data/old-src && DUPLICACY_LOCAL_PASSWORD=testpassword duplicacy init -e -key /opt/archiver/keys/public.pem \
  -storage-name local "$HOST-old-src" "$STORE" >/tmp/b3 2>&1 && DUPLICACY_LOCAL_PASSWORD=testpassword duplicacy backup >>/tmp/b3 2>&1 \
  && rm -rf .duplicacy) || { cat /tmp/b3; die "backup of the legacy service failed"; }

log "restored into a scratch directory, the settings file stays as backed up"
expect 0 '' "scratch restore" -- SNAPSHOT_ID="$HOST-old-src" LOCAL_DIR=/data/scratch archiver auto-restore
[ -f /data/scratch/service-backup-settings.sh ] && [ ! -e /data/scratch/pre-backup ] || die "a scratch restore was migrated"

log "restored into a service directory, it is migrated and the post-restore hook runs"
mkdir -p /data/services/old
expect 0 '' "service restore" -- SNAPSHOT_ID="$HOST-old-src" LOCAL_DIR=/data/services/old RUN_RESTORE_SERVICE=1 archiver auto-restore
[ -x /data/services/old/pre-backup ] && [ ! -e /data/services/old/service-backup-settings.sh ] || { cat /tmp/out; die "the restored settings file was not migrated"; }
grep -q "post-restore $HOST-old-src r1" /data/services/old/post-restore-ran || die "the post-restore hook did not run with its variables"
archiver backup >/tmp/b4 2>&1 || { cat /tmp/b4; die "the backup after a migrating restore failed"; }

log "restored over a directory whose own hooks survive, the conflict is reported, not silent"
mkdir -p /data/services/old3 && echo '+*' >/data/services/old3/filters
expect 0 'filters already exists' "restore over surviving hooks" -- SNAPSHOT_ID="$HOST-old-src" LOCAL_DIR=/data/services/old3 archiver auto-restore

log "a link in the destination is never written through: refused without OVERWRITE, replaced with it"
mkdir -p /restore/links /tmp/outside && echo untouched >/tmp/outside/f && ln -s /tmp/outside/f /restore/links/file.txt
SNAPSHOT_ID="$HOST-app" LOCAL_DIR=/restore/links archiver auto-restore >/tmp/l1 2>&1 && { cat /tmp/l1; die "a restore over a linked file succeeded without OVERWRITE"; }
grep -q 'is a symbolic link where the snapshot has a file' /tmp/l1 /opt/archiver/logs/archiver.log || { cat /tmp/l1; die "no word of the refused link"; }
[ "$(cat /tmp/outside/f)" = untouched ] || die "the restore wrote through the link"
OVERWRITE=1 SNAPSHOT_ID="$HOST-app" LOCAL_DIR=/restore/links archiver auto-restore >/tmp/l2 2>&1 || { cat /tmp/l2; die "the restore with OVERWRITE failed"; }
[ ! -L /restore/links/file.txt ] && [ "$(cat /restore/links/file.txt)" = "app two (changed)" ] || die "the link was not replaced by the restored file"
[ "$(cat /tmp/outside/f)" = untouched ] || die "the restore with OVERWRITE wrote through the link"

log "a restore killed mid-hook takes its hook with it (it would run on without the restore lock)"
mkdir -p /restore/hooked && printf '#!/bin/sh\ntrap "" TERM\ntouch /tmp/hook-started\nwhile :; do sleep 0.2; done\n' >/data/services/web/post-restore
chmod 755 /data/services/web/post-restore
rm -rf /data/services/old3 # left unmigrated on purpose above; it would fail this backup
archiver backup >/tmp/b5 2>&1 || { cat /tmp/b5; grep -E "ERROR" /opt/archiver/logs/archiver.log | tail -5; die "the backup with the looping hook failed"; }
rm -f /data/services/web/post-restore
SNAPSHOT_ID="$HOST-web" LOCAL_DIR=/restore/hooked RUN_RESTORE_SERVICE=1 archiver auto-restore >/tmp/h1 2>&1 &
pid=$!
for _ in $(seq 1 60); do [ -e /tmp/hook-started ] && break; sleep 0.5; done
[ -e /tmp/hook-started ] || { cat /tmp/h1; die "the restore hook never started"; }
kill -9 "$pid"; wait "$pid" 2>/dev/null
sleep 2
# Through /proc, not pgrep: the slim image has no procps.
for c in /proc/[0-9]*/cmdline; do
  tr '\0' ' ' <"$c" 2>/dev/null | grep -q /restore/hooked/post-restore && die "the restore hook outlived the killed restore"
done

echo "=== RESTORE-COMMANDS OK: snapshot-exists answers and codes; auto-restore fallback, pinning, errors, hook; auto-restore-all; busy refusal; post-restore and migration of restored settings ==="
