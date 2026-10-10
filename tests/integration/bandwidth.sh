#!/usr/bin/env bash
# Bandwidth limits and copy windows (ADR 44): STORAGE_TARGET_N_UPLOAD_LIMIT reaches duplicacy
# as -limit-rate on a backup and -upload-limit-rate on a copy (inline and by a worker), and
# STORAGE_TARGET_N_COPY_WINDOW skips an inline copy and holds a worker outside its hours.
#
#   docker run -i --rm --hostname bw-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/bandwidth.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store
OFFSITE=/backup-offsite
CALLS=/tmp/duplicacy-calls

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; cat "$CALLS" >&2 2>/dev/null; exit 1; }
wait_for() { local _; for _ in $(seq 1 "$2"); do eval "$1" && return 0; sleep 1; done; return 1; }

mkdir -p /opt/archiver/keys /run/secrets "$STORE" "$OFFSITE" "$SERVICES/app" /tmp/shim
openssl genrsa -aes256 -passout pass:pp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:pp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "openssl rsa"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >/run/secrets/storage_password; printf 'pp' >/run/secrets/rsa_passphrase
echo a >"$SERVICES/app/f"

# Every duplicacy call is recorded, then run for real: the backups and copies succeeding is
# what shows duplicacy takes the flags.
real=$(command -v duplicacy) || die "no duplicacy"
cat >/tmp/shim/duplicacy <<SHIM
#!/bin/sh
echo "\$*" >>$CALLS
exec $real "\$@"
SHIM
chmod +x /tmp/shim/duplicacy
export PATH=/tmp/shim:$PATH

h=$((10#$(date -u +%H)))
open_window=$(printf '%02d:00-%02d:00' "$h" $(((h + 2) % 24)))
closed_window=$(printf '%02d:00-%02d:00' $(((h + 2) % 24)) $(((h + 3) % 24)))

export SERVICE_DIRECTORIES="$SERVICES/*/" \
  STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH="$STORE" STORAGE_TARGET_1_UPLOAD_LIMIT=50000 \
  STORAGE_TARGET_2_NAME=offsite STORAGE_TARGET_2_TYPE=local STORAGE_TARGET_2_LOCAL_PATH="$OFFSITE" STORAGE_TARGET_2_UPLOAD_LIMIT=40000

log "inside the window: the backup and the inline copy carry their limits"
STORAGE_TARGET_2_COPY_WINDOW=$open_window archiver backup >/tmp/b1.out 2>&1 || { cat /tmp/b1.out; die "backup failed"; }
grep -q '^-no-script backup .*-limit-rate 50000' "$CALLS" || die "the backup had no -limit-rate"
grep -q '^-no-script copy .*-to offsite.*-upload-limit-rate 40000' "$CALLS" || die "the inline copy had no -upload-limit-rate"
[ -n "$(ls -A "$OFFSITE/snapshots" 2>/dev/null)" ] || die "nothing reached the offsite storage"

log "outside the window: the inline copy is skipped and said so"
: >"$CALLS"
echo b >>"$SERVICES/app/f"
STORAGE_TARGET_2_COPY_WINDOW=$closed_window archiver backup >/tmp/b2.out 2>&1 || { cat /tmp/b2.out; die "backup failed"; }
grep -q '^-no-script copy ' "$CALLS" && die "a copy ran outside its window"
grep -q "outside its copy window ($closed_window)" /opt/archiver/logs/archiver.log || die "no word of the skipped copy"

log "outside the window an unreachable secondary is neither probed nor a failure, nor sent the kit"
printf 'kitpassword' >/run/secrets/recovery_password
STORAGE_TARGET_2_COPY_WINDOW=$closed_window STORAGE_TARGET_2_TYPE=sftp STORAGE_TARGET_2_SFTP_URL=127.0.0.1:1 \
  STORAGE_TARGET_2_SFTP_USER=u STORAGE_TARGET_2_SFTP_PATH=p archiver backup >/tmp/b2b.out 2>&1 || { cat /tmp/b2b.out; tail -5 /opt/archiver/logs/archiver.log; die "a held, unreachable secondary failed the backup"; }
grep -q 'Recovery kit: offsite is outside its copy window' /opt/archiver/logs/archiver.log || die "the kit step did not leave the held secondary alone"
rm /run/secrets/recovery_password

log "a worker outside its window waits, and status says until when"
BACKUP_SCHEDULE="0 3 1 1 *" STORAGE_TARGET_2_COPY_WINDOW=$closed_window archiver daemon >/tmp/d1.out 2>&1 &
d=$!
wait_for "grep -q 'wait for its copy window ($closed_window)' /opt/archiver/logs/copies.log 2>/dev/null" 60 || { tail -20 /opt/archiver/logs/copies.log; die "the worker did not wait"; }
STORAGE_TARGET_2_COPY_WINDOW=$closed_window archiver status >/tmp/s1.out 2>&1
grep -q "waiting for its copy window ($closed_window)" /tmp/s1.out || { cat /tmp/s1.out; die "status does not show the wait"; }
grep -q '^-no-script copy ' "$CALLS" && die "the worker copied outside its window"
kill $d; wait $d 2>/dev/null

log "with workers too, a secondary outside its window is not sent the kit"
printf 'kitpassword' >/run/secrets/recovery_password
BACKUP_SCHEDULE="0 3 1 1 *" STORAGE_TARGET_2_COPY_WINDOW=$closed_window archiver daemon >/tmp/d1b.out 2>&1 &
d=$!
wait_for "test -S /var/lock/archiver-daemon.sock" 30 || die "the daemon did not start"
echo d >>"$SERVICES/app/f"
STORAGE_TARGET_2_COPY_WINDOW=$closed_window archiver backup >/tmp/b2c.out 2>&1 || { cat /tmp/b2c.out; die "backup failed"; }
grep -q 'Recovery kit: offsite is outside its copy window' /opt/archiver/logs/archiver.log || die "the kit went to a held secondary under the daemon"
[ -z "$(ls "$OFFSITE" | grep -i kit)" ] || die "a kit reached the offsite storage outside its window"
STORAGE_TARGET_2_COPY_WINDOW=$closed_window archiver recovery-kit force >/tmp/k1.out 2>&1 || { cat /tmp/k1.out; die "recovery-kit failed"; }
grep -q 'current everywhere but offsite, outside its copy window' /tmp/k1.out || { cat /tmp/k1.out; die "recovery-kit claimed the kit current everywhere"; }
log "a secondary already holding the current kit stays current outside its window"
# The window is part of the kit's configuration, so one value must serve both runs: this
# minute only, open for the first and closed for the second.
while [ "$((10#$(date -u +%S)))" -gt 40 ]; do sleep 1; done
m=$((10#$(date -u +%H) * 60 + 10#$(date -u +%M)))
minute_window=$(printf '%02d:%02d-%02d:%02d' $((m / 60)) $((m % 60)) $(((m + 1) / 60 % 24)) $(((m + 1) % 60)))
STORAGE_TARGET_2_COPY_WINDOW=$minute_window archiver recovery-kit >/tmp/k2.out 2>&1 || { cat /tmp/k2.out; die "recovery-kit failed inside the window"; }
grep -q 'is current on all storage targets' /tmp/k2.out || { cat /tmp/k2.out; die "the kit was not placed inside the window"; }
while [ "$((10#$(date -u +%H) * 60 + 10#$(date -u +%M)))" = "$m" ]; do sleep 1; done
STORAGE_TARGET_2_COPY_WINDOW=$minute_window archiver recovery-kit >/tmp/k3.out 2>&1 || { cat /tmp/k3.out; die "recovery-kit failed"; }
grep -q 'is current on all storage targets' /tmp/k3.out || { cat /tmp/k3.out; die "a secondary holding the current kit was reported held"; }
rm /run/secrets/recovery_password
kill $d; wait $d 2>/dev/null

log "inside its window the worker copies, with its limit"
BACKUP_SCHEDULE="0 3 1 1 *" STORAGE_TARGET_2_COPY_WINDOW=$open_window archiver daemon >/tmp/d2.out 2>&1 &
d=$!
wait_for "grep -q '^-no-script copy .*-upload-limit-rate 40000' $CALLS" 90 || { tail -20 /opt/archiver/logs/copies.log; die "the worker did not copy with its limit"; }
wait_for "grep -q 'Copy to offsite storage completed' /opt/archiver/logs/copies.log" 90 || { tail -20 /opt/archiver/logs/copies.log; die "the worker's copy did not finish"; }
kill $d; wait $d 2>/dev/null

log "services backing up at once share the primary's limit; the copy after them has its own"
: >"$CALLS"
mkdir -p "$SERVICES/web"; echo w >"$SERVICES/web/f"; echo c >>"$SERVICES/app/f"
BACKUP_PARALLELISM=2 archiver backup >/tmp/b5.out 2>&1 || { cat /tmp/b5.out; die "backup failed"; }
[ "$(grep -c '^-no-script backup .*-limit-rate 25000$' "$CALLS")" = 2 ] || die "two backups at once did not each get half the limit"
grep -q '^-no-script copy .*-upload-limit-rate 40000$' "$CALLS" || die "the copy, run once after every service, did not get the whole limit"

log "a bad limit or window is refused"
STORAGE_TARGET_2_UPLOAD_LIMIT=fast archiver backup >/tmp/b3.out 2>&1 && die "UPLOAD_LIMIT=fast was accepted"
grep -q "STORAGE_TARGET_2_UPLOAD_LIMIT" /tmp/b3.out /opt/archiver/logs/archiver.log || die "no clear message for a bad UPLOAD_LIMIT"
STORAGE_TARGET_1_COPY_WINDOW=01:00-02:00 archiver backup >/tmp/b4.out 2>&1 && die "a window on the primary was accepted"
BACKUP_PARALLELISM=2 STORAGE_TARGET_1_UPLOAD_LIMIT=1 archiver backup >/tmp/b6.out 2>&1 && die "a primary limit below BACKUP_PARALLELISM was accepted"

echo "PASS: bandwidth limits and copy windows"
