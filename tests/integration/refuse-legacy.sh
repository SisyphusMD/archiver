#!/usr/bin/env bash
# A deployment whose services still hold service-backup-settings.sh refuses to start (ADR
# 20), naming them and the conversion, and the conversion runs from the container's own
# command (`migrate hooks`), after which the container starts.
#
# HOST-DRIVEN: run on the docker host (not via --entrypoint bash). Named volumes, not bind
# mounts: a CI runner's workspace bind mounts arrive empty.
#
#   IMAGE=archiver:dev bash tests/integration/refuse-legacy.sh

set -uo pipefail

IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"
SECVOL="archiver-rl-secrets-$$"
SVCVOL="archiver-rl-services-$$"
NAME="archiver-rl-$$"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1; docker volume rm "$SECVOL" "$SVCVOL" >/dev/null 2>&1 || true; }
trap cleanup EXIT
docker volume create "$SECVOL" >/dev/null
docker volume create "$SVCVOL" >/dev/null

log "secrets, and a service with a legacy settings file"
docker run --rm -v "$SECVOL":/run/secrets -v "$SVCVOL":/data/services --entrypoint bash "$IMAGE" -c '
  set -e
  openssl genrsa -aes256 -passout pass:tp -out /run/secrets/rsa_private_key -traditional 2048 2>/dev/null
  openssl rsa -in /run/secrets/rsa_private_key -passin pass:tp -pubout -out /run/secrets/rsa_public_key 2>/dev/null
  printf "testpassword" >/run/secrets/storage_password
  printf "tp" >/run/secrets/rsa_passphrase
  mkdir -p /data/services/app
  printf "service_specific_pre_backup_function() { echo pre; }\n" >/data/services/app/service-backup-settings.sh
' || die "setup failed"
ARGS=(-v "$SECVOL":/run/secrets -v "$SVCVOL":/data/services -e SERVICE_DIRECTORIES=/data/services/*/
  -e STORAGE_TARGET_1_NAME=local -e STORAGE_TARGET_1_TYPE=local -e STORAGE_TARGET_1_LOCAL_PATH=/backup-store)

log "the container refuses to start, naming the service and the conversion"
out="$(docker run --rm --network none "${ARGS[@]}" "$IMAGE" 2>&1)"; rc=$?
[ "$rc" -ne 0 ] || die "started with a legacy settings file"
grep -q "is still in: /data/services/app\." <<<"$out" || { echo "$out" | tail -8; die "the service is not named"; }
grep -q "docker compose run --rm archiver migrate hooks" <<<"$out" || die "no conversion command"

log "run backup refuses too"
out="$(docker run --rm --network none "${ARGS[@]}" "$IMAGE" run backup 2>&1)" && die "run backup started"
grep -q "migrate hooks" <<<"$out" || die "run backup does not name the conversion"

log "the conversion runs as the container's command, and then it starts"
docker run --rm --network none "${ARGS[@]}" "$IMAGE" migrate hooks >/tmp/rl-migrate.out 2>&1 || { cat /tmp/rl-migrate.out; die "migrate hooks failed"; }
grep -q "Migrated 1 service directories" /tmp/rl-migrate.out || { cat /tmp/rl-migrate.out; die "nothing migrated"; }
docker run -d --name "$NAME" --network none "${ARGS[@]}" "$IMAGE" >/dev/null || die "start after migration"
for _ in $(seq 1 60); do grep -q "Container is ready" <<<"$(docker logs "$NAME" 2>&1)" && break; sleep 0.5; done
grep -q "Container is ready" <<<"$(docker logs "$NAME" 2>&1)" || { docker logs "$NAME" 2>&1 | tail -8; die "did not start after migration"; }

echo "=== REFUSE-LEGACY OK: legacy settings refuse start with the conversion; migrate hooks runs as the command; then it starts ==="
