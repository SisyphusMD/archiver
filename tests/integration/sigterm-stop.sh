#!/usr/bin/env bash
# The production interruption story: `docker stop` (SIGTERM to the real entrypoint) during
# a running backup must stop gracefully WITHIN the grace period — stop recorded, no
# check/prune after the stop, container exits 0 — and must not race PID-namespace teardown
# against the backup's own cleanup.
#
# HOST-DRIVEN: run on the docker host (not via --entrypoint bash). Secrets reach the
# container through a named volume because CI runner workspace bind mounts land empty.
#
#   IMAGE=archiver:dev bash tests/integration/sigterm-stop.sh

set -uo pipefail

IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"
NAME="archiver-sigterm-$$"
NAME_MAINT="archiver-sigterm-maint-$$"
VOL="archiver-sigterm-secrets-$$"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
cleanup() {
  docker rm -f "$NAME" "$NAME_MAINT" >/dev/null 2>&1 || true
  docker volume rm "$VOL" >/dev/null 2>&1 || true
}
trap cleanup EXIT

log "populate a named secrets volume (RSA keypair + file secrets)"
docker volume create "$VOL" >/dev/null
docker run --rm -v "$VOL":/run/secrets --entrypoint bash "$IMAGE" -c '
  set -e
  openssl genrsa -aes256 -passout pass:testpassphrase -out /run/secrets/rsa_private_key -traditional 2048 2>/dev/null
  openssl rsa -in /run/secrets/rsa_private_key -passin pass:testpassphrase -pubout -out /run/secrets/rsa_public_key 2>/dev/null
  printf "testpassword" > /run/secrets/storage_password
  printf "testpassphrase" > /run/secrets/rsa_passphrase
  chmod 600 /run/secrets/rsa_private_key /run/secrets/storage_password /run/secrets/rsa_passphrase
' || die "secret volume population failed"

log "start the real entrypoint (manual mode, env-native, rotation on)"
docker run -d --name "$NAME" \
  --cap-drop ALL --cap-add DAC_OVERRIDE \
  -v "$VOL":/run/secrets \
  -e SERVICE_DIRECTORIES=/data/fixtures/ \
  -e STORAGE_TARGET_1_NAME=local \
  -e STORAGE_TARGET_1_TYPE=local \
  -e STORAGE_TARGET_1_LOCAL_PATH=/backup-store \
  -e ROTATE_BACKUPS=true \
  -e PRUNE_KEEP="-keep 0:1" \
  "$IMAGE" >/dev/null || die "container failed to start"

for _ in $(seq 1 50); do
  docker logs "$NAME" 2>&1 | grep -q "Container is ready" && break
  sleep 0.2
done
docker logs "$NAME" 2>&1 | grep -q "Container is ready" || die "entrypoint never became ready"

log "fixture data + a duplicacy shadow whose 'backup' blocks (stoppable window)"
docker exec "$NAME" bash -c '
  set -e
  mkdir -p /data/fixtures /backup-store
  echo "some content" > /data/fixtures/file.txt
  REAL="$(command -v duplicacy)"
  mv "$REAL" "${REAL}.real"
  printf "#!/usr/bin/env bash\nif [ \"\${1:-}\" = backup ]; then touch /tmp/backup-started; sleep 60; exit 0; fi\nexec \"\$0.real\" \"\$@\"\n" > "$REAL"
  chmod +x "$REAL"
' || die "in-container setup failed"

log "kick off a backup, wait for the backup stage"
docker exec "$NAME" archiver backup --detach >/dev/null || die "archiver start failed"
for _ in $(seq 1 100); do
  docker exec "$NAME" test -f /tmp/backup-started 2>/dev/null && break
  sleep 0.2
done
docker exec "$NAME" test -f /tmp/backup-started || die "backup never reached the backup stage"

log "docker stop (SIGTERM + 120s grace) during the backup"
T0=$(date +%s)
docker stop -t 120 "$NAME" >/dev/null || die "docker stop failed"
ELAPSED=$(( $(date +%s) - T0 ))

EXIT_CODE=$(docker inspect -f '{{.State.ExitCode}}' "$NAME")
LOGS=$(docker logs "$NAME" 2>&1)
# The stdout log tailer (tail -F, 1s poll) races the shutdown teardown for the very last
# lines, so assert against the archiver.log files themselves (docker cp works when stopped).
LOGDIR=$(mktemp -d)
docker cp "$NAME":/opt/archiver/logs/. "$LOGDIR"/ >/dev/null 2>&1 || die "could not copy logs out"

echo "--- container logs (tail) ---"; echo "$LOGS" | tail -10; echo "-----------------------------"
echo "stop took ${ELAPSED}s, container exit code ${EXIT_CODE}"

echo "$LOGS" | grep -q "Received shutdown signal" || die "entrypoint SIGTERM trap never fired"
grep -rq "Backup stopped\." "$LOGDIR" || die "no 'Backup stopped' record: graceful stop did not complete before teardown"
grep -rq "Storage check completed" "$LOGDIR" && die "storage check ran after docker stop"
grep -rq "Prune completed" "$LOGDIR" && die "prune ran after docker stop"
[ "$EXIT_CODE" = "0" ] || die "container exited ${EXIT_CODE}, expected 0 (graceful)"
[ "$ELAPSED" -lt 110 ] || die "stop took ${ELAPSED}s; the grace period nearly ran out"
rm -rf "$LOGDIR"

# ── Second phase: docker stop must ALSO drain a live MAINTENANCE run ────────────
# handle_shutdown runs 'archiver stop' (target all) and waits on BOTH pipeline locks, so a
# SIGTERM during check/prune must stop it gracefully — not SIGKILL duplicacy mid-prune.
log "phase 2: start a fresh container, do one real backup, then a maintenance-SIGTERM test"
docker run -d --name "$NAME_MAINT" \
  --cap-drop ALL --cap-add DAC_OVERRIDE \
  -v "$VOL":/run/secrets \
  -e SERVICE_DIRECTORIES=/data/fixtures/ \
  -e STORAGE_TARGET_1_NAME=local \
  -e STORAGE_TARGET_1_TYPE=local \
  -e STORAGE_TARGET_1_LOCAL_PATH=/backup-store \
  -e PRUNE_KEEP="-keep 0:1" \
  "$IMAGE" >/dev/null || die "phase-2 container failed to start"

for _ in $(seq 1 50); do
  docker logs "$NAME_MAINT" 2>&1 | grep -q "Container is ready" && break
  sleep 0.2
done
docker logs "$NAME_MAINT" 2>&1 | grep -q "Container is ready" || die "phase-2 entrypoint never became ready"

log "fixture + one real backup (so a repository exists for maintenance), then shadow 'check' to block"
docker exec "$NAME_MAINT" bash -c '
  set -e
  mkdir -p /data/fixtures /backup-store
  echo "maint content" > /data/fixtures/file.txt
' || die "phase-2 fixture setup failed"
docker exec "$NAME_MAINT" archiver backup >/dev/null 2>&1 || die "phase-2 initial backup failed"
docker exec "$NAME_MAINT" bash -c '
  set -e
  REAL="$(command -v duplicacy)"
  mv "$REAL" "${REAL}.real"
  printf "#!/usr/bin/env bash\nif [ \"\${1:-}\" = check ]; then touch /tmp/check-started; sleep 60; exit 0; fi\nexec \"\$0.real\" \"\$@\"\n" > "$REAL"
  chmod +x "$REAL"
' || die "phase-2 duplicacy shadow failed"

log "kick off maintenance, wait for the check stage"
docker exec -d "$NAME_MAINT" archiver maintenance || die "archiver maintenance failed to start"
for _ in $(seq 1 100); do
  docker exec "$NAME_MAINT" test -f /tmp/check-started 2>/dev/null && break
  sleep 0.2
done
docker exec "$NAME_MAINT" test -f /tmp/check-started || die "maintenance never reached the check stage"

log "docker stop during maintenance must drain it gracefully, exit 0"
T0=$(date +%s)
docker stop -t 120 "$NAME_MAINT" >/dev/null || die "docker stop (maintenance) failed"
ELAPSED_M=$(( $(date +%s) - T0 ))
EXIT_M=$(docker inspect -f '{{.State.ExitCode}}' "$NAME_MAINT")
LOGS_M=$(docker logs "$NAME_MAINT" 2>&1)
LOGDIR_M=$(mktemp -d)
docker cp "$NAME_MAINT":/opt/archiver/logs/. "$LOGDIR_M"/ >/dev/null 2>&1 || die "could not copy phase-2 logs out"

echo "$LOGS_M" | grep -q "Received shutdown signal" || die "phase-2 SIGTERM trap never fired"
grep -rq "Maintenance session summary: Maintenance stopped" "$LOGDIR_M" || die "maintenance did not record a graceful stop"
[ "$EXIT_M" = "0" ] || die "phase-2 container exited ${EXIT_M}, expected 0"
[ "$ELAPSED_M" -lt 110 ] || die "phase-2 stop took ${ELAPSED_M}s; grace period nearly ran out"
rm -rf "$LOGDIR_M"

echo "=== SIGTERM-STOP OK: backup stop in ${ELAPSED}s + maintenance stop in ${ELAPSED_M}s, both recorded, exit 0 ==="
