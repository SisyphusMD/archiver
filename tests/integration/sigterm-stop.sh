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
VOL="archiver-sigterm-secrets-$$"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
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
docker exec "$NAME" archiver start >/dev/null || die "archiver start failed"
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

echo "=== SIGTERM-STOP OK: graceful stop in ${ELAPSED}s, stop recorded, no check/prune, exit 0 ==="
