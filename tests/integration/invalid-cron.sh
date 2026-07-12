#!/usr/bin/env bash
# Schedule handling at startup: a malformed BACKUP_SCHEDULE (or MAINTENANCE_SCHEDULE) must
# fail the container fast (supercronic -test) with a clear message — not crash-loop or
# silently run without a scheduler; a legacy CRON_SCHEDULE must fail fast with the rename
# message; and a valid pair of schedules must register both jobs and announce them.
#
# HOST-DRIVEN: run on the docker host (not via --entrypoint bash).
#
#   IMAGE=archiver:dev bash tests/integration/invalid-cron.sh

set -uo pipefail

IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"
NAME="archiver-badcron-$$"
NAME_MAINT="archiver-badmaint-$$"
NAME_RENAME="archiver-rename-$$"
NAME_OK="archiver-schedok-$$"
VOL="archiver-badcron-secrets-$$"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
cleanup() {
  docker rm -f "$NAME" "$NAME_MAINT" "$NAME_RENAME" "$NAME_OK" >/dev/null 2>&1 || true
  docker volume rm "$VOL" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Poll until a container exits, then echo its exit code (or "running" if it never stopped).
wait_exit() {
  local name="$1" running
  for _ in $(seq 1 60); do
    running=$(docker inspect -f '{{.State.Running}}' "$name" 2>/dev/null)
    [ "$running" = "false" ] && { docker inspect -f '{{.State.ExitCode}}' "$name"; return; }
    sleep 1
  done
  echo "running"
}

log "populate a named secrets volume (valid env-native config, so only the cron is bad)"
docker volume create "$VOL" >/dev/null
docker run --rm -v "$VOL":/run/secrets --entrypoint bash "$IMAGE" -c '
  set -e
  openssl genrsa -aes256 -passout pass:testpassphrase -out /run/secrets/rsa_private_key -traditional 2048 2>/dev/null
  openssl rsa -in /run/secrets/rsa_private_key -passin pass:testpassphrase -pubout -out /run/secrets/rsa_public_key 2>/dev/null
  printf "testpassword" > /run/secrets/storage_password
  printf "testpassphrase" > /run/secrets/rsa_passphrase
' || die "secret volume population failed"

log "start with a malformed BACKUP_SCHEDULE"
docker run -d --name "$NAME" \
  --cap-drop ALL --cap-add DAC_OVERRIDE \
  -v "$VOL":/run/secrets \
  -e BACKUP_SCHEDULE="not a cron line" \
  -e SERVICE_DIRECTORIES=/data/fixtures/ \
  -e STORAGE_TARGET_1_NAME=local \
  -e STORAGE_TARGET_1_TYPE=local \
  -e STORAGE_TARGET_1_LOCAL_PATH=/backup-store \
  "$IMAGE" >/dev/null || die "container failed to start"

log "container must exit non-zero quickly with a clear message"
for _ in $(seq 1 60); do
  RUNNING=$(docker inspect -f '{{.State.Running}}' "$NAME")
  [ "$RUNNING" = "false" ] && break
  sleep 1
done
[ "$RUNNING" = "false" ] || die "container still running with a malformed BACKUP_SCHEDULE"

EXIT_CODE=$(docker inspect -f '{{.State.ExitCode}}' "$NAME")
LOGS=$(docker logs "$NAME" 2>&1)
[ "$EXIT_CODE" -ne 0 ] || die "container exited 0 despite the malformed schedule"
echo "$LOGS" | grep -q "BACKUP_SCHEDULE or MAINTENANCE_SCHEDULE is invalid" || { echo "$LOGS" | tail -5; die "no clear invalid-schedule message"; }

log "a malformed MAINTENANCE_SCHEDULE (valid BACKUP_SCHEDULE) must fail fast too"
docker run -d --name "$NAME_MAINT" \
  --cap-drop ALL --cap-add DAC_OVERRIDE \
  -v "$VOL":/run/secrets \
  -e BACKUP_SCHEDULE="0 3 * * *" \
  -e MAINTENANCE_SCHEDULE="also not a cron line" \
  -e SERVICE_DIRECTORIES=/data/fixtures/ \
  -e STORAGE_TARGET_1_NAME=local \
  -e STORAGE_TARGET_1_TYPE=local \
  -e STORAGE_TARGET_1_LOCAL_PATH=/backup-store \
  "$IMAGE" >/dev/null || die "maintenance-schedule container failed to start"
MAINT_EXIT=$(wait_exit "$NAME_MAINT")
[ "$MAINT_EXIT" != "running" ] || die "container still running with a malformed MAINTENANCE_SCHEDULE"
[ "$MAINT_EXIT" -ne 0 ] || die "container exited 0 despite the malformed MAINTENANCE_SCHEDULE"
docker logs "$NAME_MAINT" 2>&1 | grep -q "BACKUP_SCHEDULE or MAINTENANCE_SCHEDULE is invalid" \
  || die "no clear invalid-schedule message for MAINTENANCE_SCHEDULE"

log "a legacy CRON_SCHEDULE (valid cron) must fail fast with the rename message"
docker run -d --name "$NAME_RENAME" \
  --cap-drop ALL --cap-add DAC_OVERRIDE \
  -v "$VOL":/run/secrets \
  -e CRON_SCHEDULE="0 3 * * *" \
  -e SERVICE_DIRECTORIES=/data/fixtures/ \
  -e STORAGE_TARGET_1_NAME=local \
  -e STORAGE_TARGET_1_TYPE=local \
  -e STORAGE_TARGET_1_LOCAL_PATH=/backup-store \
  "$IMAGE" >/dev/null || die "rename container failed to start"
RENAME_EXIT=$(wait_exit "$NAME_RENAME")
[ "$RENAME_EXIT" != "running" ] || die "container still running despite legacy CRON_SCHEDULE"
[ "$RENAME_EXIT" -ne 0 ] || die "container exited 0 despite legacy CRON_SCHEDULE"
docker logs "$NAME_RENAME" 2>&1 | grep -q "CRON_SCHEDULE was renamed to BACKUP_SCHEDULE" \
  || die "no rename guidance for legacy CRON_SCHEDULE"

log "a valid BACKUP_SCHEDULE + MAINTENANCE_SCHEDULE registers both jobs and announces them"
docker run -d --name "$NAME_OK" \
  --cap-drop ALL --cap-add DAC_OVERRIDE \
  -v "$VOL":/run/secrets \
  -e BACKUP_SCHEDULE="0 3 * * *" \
  -e MAINTENANCE_SCHEDULE="0 13 * * *" \
  -e SERVICE_DIRECTORIES=/data/fixtures/ \
  -e STORAGE_TARGET_1_NAME=local \
  -e STORAGE_TARGET_1_TYPE=local \
  -e STORAGE_TARGET_1_LOCAL_PATH=/backup-store \
  "$IMAGE" >/dev/null || die "valid-schedule container failed to start"
for _ in $(seq 1 30); do
  docker logs "$NAME_OK" 2>&1 | grep -q "Starting supercronic" && break
  sleep 1
done
OK_LOGS=$(docker logs "$NAME_OK" 2>&1)
echo "$OK_LOGS" | grep -q "Backups scheduled: 0 3 \* \* \*" || die "backup schedule not announced"
echo "$OK_LOGS" | grep -q "Maintenance scheduled: 0 13 \* \* \*" || die "maintenance schedule not announced"
CRONTAB=$(docker exec "$NAME_OK" cat /tmp/archiver.crontab 2>&1) || die "could not read crontab"
echo "$CRONTAB" | grep -q "archiver.sh backup$" || die "crontab missing the backup job"
echo "$CRONTAB" | grep -q "archiver.sh maintenance$" || die "crontab missing the maintenance job"
[ "$(printf '%s\n' "$CRONTAB" | grep -c 'archiver.sh')" -eq 2 ] || die "crontab does not have exactly two jobs"

echo "=== INVALID-CRON OK: fail-fast on bad backup/maintenance schedule + CRON_SCHEDULE rename; valid pair registers both jobs ==="
