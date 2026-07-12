#!/usr/bin/env bash
# Lock semantics guard data integrity (two concurrent duplicacy backups into one storage
# corrupt trust in the snapshots): a second backup while one is running must be refused
# with an explanation; a stale lock (dead PID) must be recovered AND the new run must
# actually hold the lock (a recovered-but-lockless run would admit a concurrent backup).
#
#   docker run -i --rm --hostname lc-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/lock-concurrency.sh

set -uo pipefail

FIXTURES=/data/fixtures
STORE=/backup-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
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

log "shadow duplicacy: 'backup' blocks so the lock stays held"
REAL="$(command -v duplicacy)" || die "duplicacy not on PATH"
mv "$REAL" "${REAL}.real"
cat >"$REAL" <<WRAP
#!/usr/bin/env bash
if [ "\${1:-}" = "backup" ]; then
  touch "${MARKER}"
  sleep 30
  exit 0
fi
exec "\$0.real" "\$@"
WRAP
chmod +x "$REAL"

log "start a backup, wait until it holds the lock in the backup stage"
archiver backup --detach >/dev/null || die "archiver start failed"
for _ in $(seq 1 100); do [ -f "$MARKER" ] && break; sleep 0.2; done
[ -f "$MARKER" ] || die "backup never reached the duplicacy backup stage"
[ -e "$LOCKFILE" ] || die "no lockfile while a backup is running"

log "a second backup must be refused with an explanation"
set +e
OUT=$(archiver backup 2>&1)
rc=$?
set -e
[ "$rc" -ne 0 ] || die "second concurrent backup was admitted (exit 0)"
echo "$OUT" | grep -q "already running" || die "refusal carries no explanation; got: $OUT"

log "detached 'archiver backup --detach' must ALSO refuse visibly (never a false success line)"
set +e
OUT2=$(archiver backup --detach 2>&1)
rc_start=$?
set -e
[ "$rc_start" -ne 0 ] || die "'backup --detach' exited 0 while a backup is running"
echo "$OUT2" | grep -q "already running" || die "detach refusal carries no explanation; got: $OUT2"
echo "$OUT2" | grep -q "started in the background" && die "detach printed the success line while refusing"

log "removed commands must error with guidance, not act"
set +e
OUT3=$(archiver start 2>&1); rc3=$?
OUT4=$(archiver restart 2>&1); rc4=$?
OUT5=$(archiver backup retain 2>&1); rc5=$?
set -e
[ "$rc3" -ne 0 ] && echo "$OUT3" | grep -q "was removed" || die "'start' did not error with removal guidance"
[ "$rc4" -ne 0 ] && echo "$OUT4" | grep -q "was removed" || die "'restart' did not error with removal guidance"
[ "$rc5" -ne 0 ] && echo "$OUT5" | grep -q "was removed" || die "'backup retain' did not error with removal guidance"

log "stop the running backup with an explicit target + --immediate, wait for it to wind down"
archiver stop backup --immediate >/dev/null 2>&1 || true
for _ in $(seq 1 120); do [ ! -e "$LOCKFILE" ] && break; sleep 0.5; done
[ ! -e "$LOCKFILE" ] || die "lock not released after 'stop backup --immediate'"

log "'archiver stop backup maintenance' (two targets) must be rejected, not silently drop one"
set +e
OUT_MT=$(archiver stop backup maintenance 2>&1); rc_mt=$?
set -e
[ "$rc_mt" -ne 0 ] || die "'stop backup maintenance' was accepted (should reject multiple targets)"
echo "$OUT_MT" | grep -q "at most one target" || die "no guidance rejecting multiple stop targets; got: $OUT_MT"

log "stale lock: a dead PID must be recovered AND the new run must hold the lock"
echo "999999 duplicacy pre-backup" >"$LOCKFILE"
rm -f "$MARKER"
archiver backup --detach >/dev/null || die "archiver start failed on stale lock"
for _ in $(seq 1 100); do [ -f "$MARKER" ] && break; sleep 0.2; done
[ -f "$MARKER" ] || die "backup never started after stale-lock recovery"

NEW_PID="$(head -n1 "$LOCKFILE" | cut -d' ' -f1)"
[ -n "$NEW_PID" ] && [ "$NEW_PID" != "999999" ] || die "lockfile still carries the stale PID"
kill -0 "$NEW_PID" 2>/dev/null || die "lockfile PID ${NEW_PID} is not a live process (recovered run is lockless)"
# The stale-lock warning is logged before rotate_logs switches to this run's fresh file,
# so it lands in the previous file: search the whole log dir.
grep -rq "Stale lock file found" /opt/archiver/logs/ || die "stale-lock recovery not logged"

log "and the recovered run must again refuse a concurrent backup"
set +e
archiver backup >/dev/null 2>&1
rc2=$?
set -e
[ "$rc2" -ne 0 ] || die "concurrent backup admitted during a stale-recovered run"

archiver stop >/dev/null 2>&1 || true
echo "=== LOCK-CONCURRENCY OK: busy refused sync (rc=${rc}) + detach (rc=${rc_start}) + removals, stale recovered and re-held (pid ${NEW_PID}), busy re-refused (rc=${rc2}) ==="
