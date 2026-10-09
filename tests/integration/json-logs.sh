#!/usr/bin/env bash
# LOG_FORMAT=json: every line the running container prints to docker logs (startup
# messages, the forwarded archiver log, a backup's output) is one JSON object with time,
# level, log and msg, and a backup's lines carry their service; without it the logs stay
# text.
#
# HOST-DRIVEN: run on the docker host (not via --entrypoint bash).
#
#   IMAGE=archiver:dev bash tests/integration/json-logs.sh

set -uo pipefail

IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"
NAME="archiver-jsonlogs-$$"
VOL="archiver-jsonlogs-secrets-$$"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; docker logs "$NAME" 2>&1 | tail -15 >&2; exit 1; }
cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker volume rm "$VOL" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker volume create "$VOL" >/dev/null
docker run --rm -v "$VOL":/run/secrets --entrypoint bash "$IMAGE" -c '
  set -e
  openssl genrsa -aes256 -passout pass:testpassphrase -out /run/secrets/rsa_private_key -traditional 2048 2>/dev/null
  openssl rsa -in /run/secrets/rsa_private_key -passin pass:testpassphrase -pubout -out /run/secrets/rsa_public_key 2>/dev/null
  printf "testpassword" > /run/secrets/storage_password
  printf "testpassphrase" > /run/secrets/rsa_passphrase
' || die "secret volume population failed"

log "start with LOG_FORMAT=json and one service"
docker run -d --name "$NAME" --hostname json-host \
  --cap-drop ALL --cap-add DAC_OVERRIDE \
  -v "$VOL":/run/secrets \
  -e LOG_FORMAT=json \
  -e SERVICE_DIRECTORIES=/data/app/ \
  -e STORAGE_TARGET_1_NAME=local -e STORAGE_TARGET_1_TYPE=local -e STORAGE_TARGET_1_LOCAL_PATH=/backup-store \
  "$IMAGE" >/dev/null || die "container failed to start"
for _ in $(seq 1 60); do docker logs "$NAME" 2>&1 | grep -q 'Container is ready' && break; sleep 1; done
docker exec "$NAME" sh -c 'mkdir -p /data/app /backup-store && echo hi > /data/app/f.txt' || die "fixture"

log "a backup's output reaches docker logs as JSON"
docker exec "$NAME" archiver backup >/dev/null 2>&1 || die "backup failed"
for _ in $(seq 1 20); do docker logs "$NAME" 2>&1 | grep -q 'session summary' && break; sleep 1; done

LOGS=$(docker logs "$NAME" 2>&1)
[ -n "$LOGS" ] || die "no logs"
echo "$LOGS" | docker run -i --rm --entrypoint python3 "$IMAGE" -c '
import json, sys
n = 0
service = False
for i, line in enumerate(sys.stdin, 1):
    try:
        o = json.loads(line)
    except ValueError:
        sys.exit("line %d is not JSON: %r" % (i, line[:120]))
    for k in ("time", "level", "log", "msg"):
        if k not in o:
            sys.exit("line %d lacks %s: %r" % (i, k, line[:120]))
    if o.get("service") == "app" and o["log"] == "archiver":
        service = True
    n += 1
if not service:
    sys.exit("no archiver line carried service app")
print("%d JSON lines" % n)
' || die "docker logs is not all JSON"

echo "=== JSON-LOGS OK: every docker logs line is JSON; backup lines carry their service ==="
