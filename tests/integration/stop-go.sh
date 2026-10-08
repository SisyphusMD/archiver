#!/usr/bin/env bash
# `archiver stop` on the Go pipeline: the backup stops itself. A copy running inline (no
# daemon) ends at once, is not retried, and no recovery-kit step follows; the run exits
# non-zero and records the stop. A stop while paused, before a service's pre-backup hook
# runs, skips that hook and its post hook.
#
#   docker run -i --rm --hostname sg-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/stop-go.sh

set -uo pipefail

SVC=/data/services/app
STORE=/backup-store
OFFSITE=/offsite-store
SECRETS_DIR=/run/secrets
LOG=/opt/archiver/logs/archiver.log
LOCKFILE=/var/lock/archiver-main.lock
COPYING=/tmp/copy-started

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; tail -20 "$LOG" >&2 2>/dev/null; exit 1; }
wait_for() { local _; for _ in $(seq 1 "$2"); do eval "$1" && return 0; sleep 0.25; done; return 1; }

log "env-native config with a local primary and an offsite, an executable hook (the Go pipeline)"
mkdir -p /opt/archiver/keys "$SECRETS_DIR" "$STORE" "$OFFSITE" "$SVC"
openssl genrsa -aes256 -passout pass:rp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:rp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "pubout"
printf 'testpassword' >"$SECRETS_DIR/storage_password"
printf 'rp' >"$SECRETS_DIR/rsa_passphrase"
printf 'recovery-pass-123' >"$SECRETS_DIR/recovery_password"
export SERVICE_DIRECTORIES="/data/services/*/"
# One service at a time: what this checks is the order services run in.
export BACKUP_PARALLELISM=1
export STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH="$STORE"
export STORAGE_TARGET_2_NAME=offsite STORAGE_TARGET_2_TYPE=local STORAGE_TARGET_2_LOCAL_PATH="$OFFSITE"
echo "content" >"$SVC/file.txt"
printf '#!/usr/bin/env bash\necho pre >>/tmp/hooks\n' >"$SVC/pre-backup"
printf '#!/usr/bin/env bash\necho post >>/tmp/hooks\n' >"$SVC/post-backup"
chmod +x "$SVC/pre-backup" "$SVC/post-backup"

log "a stalled offsite: duplicacy copy hangs"
REAL="$(command -v duplicacy)"
mv "$REAL" "${REAL}.real"
cat >"$REAL" <<WRAP
#!/usr/bin/env bash
if [ "\${1:-}" = "copy" ]; then
  touch "${COPYING}"
  sleep 300
  exit 0
fi
exec "\$0.real" "\$@"
WRAP
chmod +x "$REAL"

log "stop during the inline copy ends the run promptly, with no retry and no kit step"
(archiver backup >/tmp/backup.out 2>&1; echo $? >/tmp/backup.rc) &
wait_for '[ -e "$COPYING" ]' 240 || die "the copy never started"
archiver stop backup >/tmp/stop.out 2>&1 || { cat /tmp/stop.out; die "stop failed"; }
grep -q 'Stop requested. The backup ends its current step' /tmp/stop.out || die "stop did not hand the stop to the Go pipeline: $(cat /tmp/stop.out)"
wait_for '[ -e /tmp/backup.rc ]' 60 || die "the backup did not end within 15s of the stop"
[ "$(cat /tmp/backup.rc)" -ne 0 ] || die "a stopped backup exited 0"
grep -q 'Retrying failed copies' "$LOG" && die "a stopped backup retried its copy"
grep -q 'Recovery kit' "$LOG" && die "a recovery-kit step ran after the stop"
[ ! -e "$LOCKFILE" ] || die "the lock was not released"
pgrep -f 'duplicacy copy' >/dev/null && die "the copy is still running"
grep -qi 'stopped' "$LOG" || die "the stop was not recorded"

log "a stop while paused, before the pre-backup hook, skips both hooks"
rm -f /tmp/hooks /tmp/backup.rc
mv "${REAL}.real" "$REAL"
printf '#!/usr/bin/env bash\necho pre >>/tmp/hooks\n' >"$SVC/pre-backup"
mkdir -p /data/services/aaa && echo x >/data/services/aaa/f
printf '#!/usr/bin/env bash\ntouch /tmp/first-running; sleep 3\n' >/data/services/aaa/pre-backup
chmod +x /data/services/aaa/pre-backup
(archiver backup >/tmp/backup.out 2>&1; echo $? >/tmp/backup.rc) &
wait_for '[ -e /tmp/first-running ]' 40 || die "the first service's hook never ran"
# Recorded paused between programs, so the next service's pre hook waits; then stopped.
echo "$(date +%s) paused" >>"$LOCKFILE"
archiver stop backup >/dev/null 2>&1 || die "second stop failed"
wait_for '[ -e /tmp/backup.rc ]' 80 || die "the paused, stopped backup did not end"
[ ! -e /tmp/hooks ] || die "a hook ran for a service the stop came before: $(cat /tmp/hooks)"

echo "=== STOP-GO OK: stop ends inline copies with no retry or kit step; a stop before a pre hook skips both hooks ==="
