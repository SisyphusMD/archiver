#!/usr/bin/env bash
# pause/resume/stop depend on the backup process tree: they signal DIRECT CHILDREN of the
# main.sh PID (pkill -STOP/-CONT/-TERM -P). This pins that duplicacy itself is reachable
# that way: pause actually freezes it (state T), resume unfreezes it (backup completes),
# and stop-while-paused kills it (SIGKILL path, since TERM does nothing to a stopped
# process).
#
#   docker run -i --rm --hostname pr-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/pause-resume.sh

set -uo pipefail

FIXTURES=/data/fixtures
STORE=/backup-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
LOG=/opt/archiver/logs/archiver.log
LOCKFILE=/var/lock/archiver-main.lock
MARKER=/tmp/backup-started

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

log "materialize RSA keypair + file secrets (env-native mode)"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE" "$FIXTURES"
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null \
  || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null \
  || die "openssl rsa -pubout"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${RSA_PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"

export SERVICE_DIRECTORIES="${FIXTURES}/"
export STORAGE_TARGET_1_NAME="local"
export STORAGE_TARGET_1_TYPE="local"
export STORAGE_TARGET_1_LOCAL_PATH="${STORE}"
export ROTATE_BACKUPS="false"
echo "some content" >"$FIXTURES/file.txt"

log "shadow duplicacy: 'backup' marks then sleeps briefly (pausable, completes on its own)"
REAL="$(command -v duplicacy)" || die "duplicacy not on PATH"
mv "$REAL" "${REAL}.real"
cat >"$REAL" <<WRAP
#!/usr/bin/env bash
if [ "\${1:-}" = "backup" ]; then
  touch "${MARKER}"
  sleep 6
  exit 0
fi
exec "\$0.real" "\$@"
WRAP
chmod +x "$REAL"

log "round 1: pause freezes duplicacy, resume unfreezes, backup then completes"
archiver start retain >/dev/null || die "archiver start failed"
for _ in $(seq 1 100); do [ -f "$MARKER" ] && break; sleep 0.2; done
[ -f "$MARKER" ] || die "backup never started"
DPID=$(pgrep -f "duplicacy backup" | head -1)
[ -n "$DPID" ] || die "no duplicacy backup process found"

archiver pause >/dev/null || die "archiver pause failed"
sleep 1
STATE=$(ps -o stat= -p "$DPID" | tr -d ' ')
[[ "$STATE" == T* ]] || die "duplicacy not frozen after pause (state ${STATE}); pause is not reaching it"

sleep 3   # while frozen, the 6s sleep must not advance
kill -0 "$DPID" 2>/dev/null || die "duplicacy died while paused"

archiver resume >/dev/null || die "archiver resume failed"
sleep 1
STATE=$(ps -o stat= -p "$DPID" | tr -d ' ')
[[ "$STATE" == T* ]] && die "duplicacy still frozen after resume"

for _ in $(seq 1 60); do [ ! -e "$LOCKFILE" ] && break; sleep 0.5; done
[ ! -e "$LOCKFILE" ] || die "backup did not complete after resume"
grep -q "Backup to local completed for fixtures service" "$LOG" || die "no completion record after resume"
grep -q "Backup Paused\|Backup paused" "$LOG" || die "pause not recorded"
grep -q "Backup resumed" "$LOG" || die "resume not recorded"

log "round 2: stop while paused must kill the frozen backup (SIGKILL path)"
rm -f "$MARKER"
archiver start retain >/dev/null || die "second archiver start failed"
for _ in $(seq 1 100); do [ -f "$MARKER" ] && break; sleep 0.2; done
[ -f "$MARKER" ] || die "second backup never started"
DPID=$(pgrep -f "duplicacy backup" | head -1)

archiver pause >/dev/null || die "second pause failed"
sleep 1
archiver stop >/dev/null 2>&1 || true
sleep 3
kill -0 "$DPID" 2>/dev/null && die "duplicacy survived stop-while-paused (left frozen forever)"
grep -rq "Backup stopped\." /opt/archiver/logs/ || die "stop-while-paused not recorded"

echo "=== PAUSE-RESUME OK: pause freezes (T), resume completes, stop-while-paused kills ==="
