#!/usr/bin/env bash
# Maintenance pipeline, end to end (env-native, two local targets so the copy path is
# exercised too). Proves: maintenance refuses to run before any backup exists; check+prune
# run per storage and record last-success state; exhaustive prune fires when due
# (never-run -> due) and NOT when fresh; CHECK_BACKUPS/PRUNE_BACKUPS toggles skip their
# phases; healthcheck warns on stale maintenance; removed rotation args hard-error;
# `archiver stop maintenance` ends a running maintenance gracefully.
#
#   docker run -i --rm --hostname mt-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/maintenance.sh

set -uo pipefail

FIXTURES=/data/fixtures
STORE=/backup-store
STORE2=/backup-store2
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
MLOG=/opt/archiver/logs/maintenance.log

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

log "env-native setup: keys + secrets + env config, TWO local targets"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE" "$STORE2" "$FIXTURES"
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
export STORAGE_TARGET_2_NAME="second"
export STORAGE_TARGET_2_TYPE="local"
export STORAGE_TARGET_2_LOCAL_PATH="${STORE2}"
export PRUNE_EXHAUSTIVE_FREQUENCY="daily"
echo "maintenance test content" >"$FIXTURES/file.txt"

log "maintenance before any backup must refuse with guidance"
set +e
OUT=$(archiver maintenance 2>&1); rc=$?
set -e
[ "$rc" -ne 0 ] || die "maintenance exited 0 with no repository"
grep -rq "Run a backup before maintenance" /opt/archiver/logs/ || die "no 'run a backup first' guidance"

log "backup (parallel copy path to the second target)"
archiver backup || die "backup exited non-zero"
[ -d "$STORE/snapshots" ] || die "no snapshots on primary"
[ -d "$STORE2/snapshots" ] || die "no snapshots on second target (copy did not run)"
grep -rq "Copy to second storage completed" /opt/archiver/logs/archiver.log || die "copy completion not logged"

log "maintenance run 1: check + prune per storage, exhaustive due (never ran)"
archiver maintenance || die "maintenance run 1 exited non-zero"
grep -q "Storage check completed for local" "$MLOG" || die "no check on primary"
grep -q "Storage check completed for second" "$MLOG" || die "no check on second"
grep -q "Prune completed for local" "$MLOG" || die "no prune on primary"
grep -q "Prune completed for second" "$MLOG" || die "no prune on second"
grep -q "Exhaustive prune due for local" "$MLOG" || die "exhaustive not due on first-ever run"
grep -q "Maintenance session summary: Maintenance completed successfully" "$MLOG" || die "no completion summary"
STATE=/opt/archiver/logs/.maintenance-state
[ -s "$STATE" ] || die "no maintenance state file"
grep -q "^local " "$STATE" || die "no state row for local"
grep -q "^second " "$STATE" || die "no state row for second"

log "status shows maintenance recency"
archiver status | grep -q "Storage maintenance (last success)" || die "status lacks maintenance recency"

log "maintenance run 2: exhaustive NOT due (daily frequency, just ran)"
archiver maintenance || die "maintenance run 2 exited non-zero"
grep -q "Exhaustive prune due" "$MLOG" && die "exhaustive ran again although fresh"
grep -q "Prune completed for local" "$MLOG" || die "plain prune did not run"

log "toggles: CHECK_BACKUPS=false PRUNE_BACKUPS=false -> nothing to do"
CHECK_BACKUPS=false PRUNE_BACKUPS=false archiver maintenance || die "toggled-off maintenance exited non-zero"
grep -q "nothing to do" "$MLOG" || die "no 'nothing to do' message"

log "deprecated ROTATE_BACKUPS=false must gate prune like PRUNE_BACKUPS=false"
ROTATE_BACKUPS=false archiver maintenance || die "ROTATE_BACKUPS run exited non-zero"
grep -q "PRUNE_BACKUPS=false" "$MLOG" || die "alias did not translate"
grep -q "Prune completed" "$MLOG" && die "prune ran despite ROTATE_BACKUPS=false"
grep -q "Storage check completed for local" "$MLOG" || die "check skipped although only prune was off"

log "healthcheck warns on stale maintenance state"
printf 'local 1000 1000 1000\nsecond 1000 1000 1000\n' > "$STATE"
set +e
HC=$(archiver healthcheck 2>&1)
set -e
echo "$HC" | grep -q "No successful check on 'local' in over 8 days" || die "no staleness warning for check"
echo "$HC" | grep -q "No successful prune on 'second' in over 8 days" || die "no staleness warning for prune"

log "healthcheck suppresses the stale-check warning when CHECK_BACKUPS=false"
set +e
HC=$(CHECK_BACKUPS=false archiver healthcheck 2>&1)
set -e
echo "$HC" | grep -q "No successful check on" && die "check staleness warned although CHECK_BACKUPS=false"
echo "$HC" | grep -q "No successful prune on 'second' in over 8 days" || die "prune staleness warning suppressed unexpectedly"

log "belated exhaustive: a stale timestamp fires on the next run regardless of frequency"
OLD_CHECK="$(grep '^local ' "$STATE" | cut -d' ' -f2)"
printf 'local %s %s 1000\nsecond %s %s 1000\n' "$OLD_CHECK" "$OLD_CHECK" "$OLD_CHECK" "$OLD_CHECK" > "$STATE"
PRUNE_EXHAUSTIVE_FREQUENCY=monthly archiver maintenance || die "belated-exhaustive run exited non-zero"
grep -q "Exhaustive prune due for local" "$MLOG" || die "stale exhaustive timestamp did not trigger a belated run"

log "PRUNE_EXHAUSTIVE_FREQUENCY=off: prune runs but exhaustive never does, even with a stale ts"
printf 'local %s %s 1000\nsecond %s %s 1000\n' "$OLD_CHECK" "$OLD_CHECK" "$OLD_CHECK" "$OLD_CHECK" > "$STATE"
PRUNE_EXHAUSTIVE_FREQUENCY=off archiver maintenance || die "off-frequency run exited non-zero"
grep -q "Prune completed for local" "$MLOG" || die "plain prune did not run under off-frequency"
grep -q "Exhaustive prune due" "$MLOG" && die "exhaustive ran although frequency is off"

log "'archiver maintenance exhaustive' actually runs an exhaustive prune (state now fresh)"
archiver maintenance exhaustive || die "forced-exhaustive run exited non-zero"
grep -q "Exhaustive prune due for local" "$MLOG" || die "forced exhaustive did not run"

log "CHECK_BACKUPS=false (prune on): prune runs, check is skipped"
CHECK_BACKUPS=false archiver maintenance || die "check-off maintenance exited non-zero"
grep -q "Prune completed for local" "$MLOG" || die "prune skipped although only check was off"
grep -q "Storage check completed for local" "$MLOG" && die "check ran despite CHECK_BACKUPS=false"

log "removed rotation args must hard-error with guidance"
set +e
OUT=$(archiver backup retain 2>&1); rc=$?
set -e
[ "$rc" -ne 0 ] || die "'backup retain' exited 0"
echo "$OUT" | grep -q "was removed" || die "no removal guidance for retain"

log "'archiver stop maintenance' ends a running maintenance gracefully"
REAL="$(command -v duplicacy)"
mv "$REAL" "${REAL}.real"
cat >"$REAL" <<WRAP
#!/usr/bin/env bash
if [ "\${1:-}" = "check" ]; then
  touch /tmp/check-started
  sleep 60
  exit 0
fi
exec "\$0.real" "\$@"
WRAP
chmod +x "$REAL"
rm -f /tmp/check-started
archiver maintenance >/dev/null 2>&1 &
MAINT_BG=$!
for _ in $(seq 1 60); do [ -f /tmp/check-started ] && break; sleep 1; done
[ -f /tmp/check-started ] || die "maintenance never reached the check stage"
archiver stop maintenance | grep -q "Stopping maintenance" || die "stop did not target maintenance"
wait "$MAINT_BG" 2>/dev/null || true
for _ in $(seq 1 30); do [ ! -e /var/lock/archiver-maintenance.lock ] && break; sleep 1; done
[ ! -e /var/lock/archiver-maintenance.lock ] || die "maintenance lock not released after stop"
grep -q "Maintenance session summary: Maintenance stopped" "$MLOG" || die "no stopped summary"
mv "${REAL}.real" "$REAL"

echo "=== MAINTENANCE OK: pre-backup-refusal/check+prune/exhaustive-frequency/toggles/alias/staleness/deprecation/stop ==="
