#!/usr/bin/env bash
# A pause that lands between two steps of a backup holds the next one: nothing new starts
# while the run is recorded paused. (Pause stops what is running; a step started after it
# must not run unpaused.)
#
#   docker run -i --rm --hostname pg-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/pause-gap.sh

set -uo pipefail

SVC=/data/services/app
STORE=/backup-store
SECRETS_DIR=/run/secrets
LOG=/opt/archiver/logs/archiver.log
LOCKFILE=/var/lock/archiver-main.lock
MARKER=/tmp/duplicacy-backup-started

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; tail -15 "$LOG" >&2 2>/dev/null; exit 1; }
wait_for() { local _; for _ in $(seq 1 "$2"); do eval "$1" && return 0; sleep 0.25; done; return 1; }

log "env-native config, one service whose pre-backup function takes two seconds"
mkdir -p /opt/archiver/keys "$SECRETS_DIR" "$STORE" "$SVC"
openssl genrsa -aes256 -passout pass:rp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:rp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "pubout"
printf 'testpassword' >"$SECRETS_DIR/storage_password"
printf 'rp' >"$SECRETS_DIR/rsa_passphrase"
export SERVICE_DIRECTORIES="/data/services/*/"
export STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH="$STORE"
echo "content" >"$SVC/file.txt"
cat >"$SVC/service-backup-settings.sh" <<'SETTINGS'
service_specific_pre_backup_function() { touch /tmp/hook-running; sleep 2; }
SETTINGS

REAL="$(command -v duplicacy)"
mv "$REAL" "${REAL}.real"
cat >"$REAL" <<WRAP
#!/usr/bin/env bash
[ "\${1:-}" = "backup" ] && touch "${MARKER}"
exec "\$0.real" "\$@"
WRAP
chmod +x "$REAL"

log "a pause recorded while the hook runs holds duplicacy until resume"
archiver backup --detach >/dev/null || die "backup did not start"
wait_for '[ -e /tmp/hook-running ]' 40 || die "the hook never ran"
# What a pause landing between steps leaves: the run recorded paused, nothing stopped yet.
echo "$(date +%s) paused" >>"$LOCKFILE"
sleep 4
[ ! -e "$MARKER" ] || die "duplicacy started while the run was recorded paused"
echo "$(date +%s) running" >>"$LOCKFILE"
wait_for '[ -e "$MARKER" ]' 40 || die "duplicacy did not start after the run resumed"
wait_for '[ ! -e "$LOCKFILE" ]' 120 || die "the backup did not finish"
grep -q "Backup to local completed for app service" "$LOG" || die "the backup did not complete"

echo "=== PAUSE-GAP OK: a pause between steps holds the next one until resume ==="
