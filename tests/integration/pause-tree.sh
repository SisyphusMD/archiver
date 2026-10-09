#!/usr/bin/env bash
# Pause reaches everything the backup runs, not just its direct children: a hook's own
# background program freezes with it. And a pause that lands between two programs holds the
# next one back: on the Go pipeline nothing starts while the run is recorded paused.
#
#   docker run -i --rm --hostname pt-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/pause-tree.sh

set -uo pipefail

SVC=/data/services/app
STORE=/backup-store
SECRETS_DIR=/run/secrets
LOG=/opt/archiver/logs/archiver.log
LOCKFILE=/var/lock/archiver-main.lock
MARKER=/tmp/duplicacy-backup-started
TICKS=/tmp/ticks

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; tail -15 "$LOG" >&2 2>/dev/null; exit 1; }
wait_for() { local _; for _ in $(seq 1 "$2"); do eval "$1" && return 0; sleep 0.25; done; return 1; }
ticks() { if [ -f "$TICKS" ]; then wc -l <"$TICKS"; else echo 0; fi; }

log "env-native config, one service with an executable pre-backup hook (the Go pipeline)"
mkdir -p /opt/archiver/keys "$SECRETS_DIR" "$STORE" "$SVC"
openssl genrsa -aes256 -passout pass:rp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:rp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "pubout"
printf 'testpassword' >"$SECRETS_DIR/storage_password"
printf 'rp' >"$SECRETS_DIR/rsa_passphrase"
export SERVICE_DIRECTORIES="/data/services/*/"
export STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH="$STORE"
echo "content" >"$SVC/file.txt"

REAL="$(command -v duplicacy)"
mv "$REAL" "${REAL}.real"
cat >"$REAL" <<WRAP
#!/usr/bin/env bash
if [ "\${2:-}" = "backup" ]; then
  touch "${MARKER}"
fi
exec "\$0.real" "\$@"
WRAP
chmod +x "$REAL"

log "round 1: a pause during a hook freezes the hook's own background program"
cat >"$SVC/pre-backup" <<'HOOK'
#!/usr/bin/env bash
(while :; do echo tick >>/tmp/ticks; sleep 0.2; done) &
writer=$!
sleep 4
kill "$writer"
HOOK
chmod +x "$SVC/pre-backup"
archiver backup --detach >/dev/null || die "backup did not start"
wait_for '[ "$(ticks)" -ge 3 ]' 40 || die "the hook's writer never ran"
archiver pause >/dev/null || die "pause failed"
sleep 0.5
frozen="$(ticks)"
sleep 2
[ "$(ticks)" -eq "$frozen" ] || die "the hook's background program kept running while paused ($frozen -> $(ticks))"
archiver resume >/dev/null || die "resume failed"
wait_for '[ "$(ticks)" -gt '"$frozen"' ]' 20 || die "the hook's background program did not resume"
wait_for '[ ! -e "$LOCKFILE" ]' 120 || die "the backup did not finish after resume"
grep -q "Backup to local completed for app service" "$LOG" || die "the backup did not complete"

log "round 2: a pause recorded between programs holds the next one until resume"
rm -f "$MARKER"
cat >"$SVC/pre-backup" <<'HOOK'
#!/usr/bin/env bash
touch /tmp/hook-running
sleep 2
HOOK
rm -f /tmp/hook-running
sleep 1.1; echo "content two" >"$SVC/file.txt"
archiver backup --detach >/dev/null || die "second backup did not start"
wait_for '[ -e /tmp/hook-running ]' 40 || die "the hook never ran"
# What a pause landing in the gap leaves: the run recorded paused, with nothing stopped.
echo "$(date +%s) paused" >>"$LOCKFILE"
sleep 4
[ ! -e "$MARKER" ] || die "duplicacy started while the run was recorded paused"
echo "$(date +%s) running" >>"$LOCKFILE"
wait_for '[ -e "$MARKER" ]' 40 || die "duplicacy did not start after the run resumed"
wait_for '[ ! -e "$LOCKFILE" ]' 120 || die "the second backup did not finish"
[ "$(grep -c 'Backup to local completed for app service' "$LOG")" -ge 1 ] || die "the second backup did not complete"

echo "=== PAUSE-TREE OK: pause freezes a hook's own programs, and a pause between programs holds the next ==="
