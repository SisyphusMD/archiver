#!/usr/bin/env bash
# A malformed CRON_SCHEDULE must fail the container fast at startup (supercronic -test)
# with a clear message — not crash-loop or silently run without a scheduler.
#
# HOST-DRIVEN: run on the docker host (not via --entrypoint bash).
#
#   IMAGE=archiver:dev bash tests/integration/invalid-cron.sh

set -uo pipefail

IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"
NAME="archiver-badcron-$$"
VOL="archiver-badcron-secrets-$$"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker volume rm "$VOL" >/dev/null 2>&1 || true
}
trap cleanup EXIT

log "populate a named secrets volume (valid env-native config, so only the cron is bad)"
docker volume create "$VOL" >/dev/null
docker run --rm -v "$VOL":/run/secrets --entrypoint bash "$IMAGE" -c '
  set -e
  openssl genrsa -aes256 -passout pass:testpassphrase -out /run/secrets/rsa_private_key -traditional 2048 2>/dev/null
  openssl rsa -in /run/secrets/rsa_private_key -passin pass:testpassphrase -pubout -out /run/secrets/rsa_public_key 2>/dev/null
  printf "testpassword" > /run/secrets/storage_password
  printf "testpassphrase" > /run/secrets/rsa_passphrase
' || die "secret volume population failed"

log "start with a malformed CRON_SCHEDULE"
docker run -d --name "$NAME" \
  --cap-drop ALL --cap-add DAC_OVERRIDE \
  -v "$VOL":/run/secrets \
  -e CRON_SCHEDULE="not a cron line" \
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
[ "$RUNNING" = "false" ] || die "container still running with a malformed CRON_SCHEDULE"

EXIT_CODE=$(docker inspect -f '{{.State.ExitCode}}' "$NAME")
LOGS=$(docker logs "$NAME" 2>&1)
[ "$EXIT_CODE" -ne 0 ] || die "container exited 0 despite the malformed schedule"
echo "$LOGS" | grep -q "CRON_SCHEDULE is invalid" || { echo "$LOGS" | tail -5; die "no clear invalid-schedule message"; }

echo "=== INVALID-CRON OK: fail-fast exit ${EXIT_CODE} with a clear message ==="
