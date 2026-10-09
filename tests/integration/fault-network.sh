#!/usr/bin/env bash
# Fault injection, network (ADR 31): an SFTP storage behind toxiproxy, which slows, then cuts
# the connection mid-backup and mid-copy. A slow link still completes; a cut backup fails with
# an error, adds no revision and releases the lock; a cut copy is retried and not reported
# done; once the link returns
# the next backup and the copy complete and restore byte for byte.
#
# HOST-DRIVEN: run on the docker host (not via --entrypoint bash).
#
#   IMAGE=archiver:dev bash tests/integration/fault-network.sh

set -uo pipefail

IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"
NET="archiver-fnet-$$"
SFTP="archiver-fnet-sftp-$$"
TOXI="archiver-fnet-toxi-$$"
ARCH="archiver-fnet-arch-$$"
KEYVOL="archiver-fnet-keys-$$"
# Digest-pinned like every sidecar here (Renovate does not scan shell scripts).
SFTP_IMAGE="atmoz/sftp:alpine@sha256:a6cb3eb29202ca7f57e73bb7e527286e66e0e822fff65609207c7e0ef2d135a3"
TOXI_IMAGE="ghcr.io/shopify/toxiproxy:2.12.0@sha256:9378ed52a28bc50edc1350f936f518f31fa95f0d15917d6eb40b8e376d1a214e"
ID=fn-host-app

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; docker exec "$ARCH" tail -15 /opt/archiver/logs/archiver.log >&2 2>/dev/null; exit 1; }
cleanup() {
  docker rm -f "$SFTP" "$TOXI" "$ARCH" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  docker volume rm "$KEYVOL" >/dev/null 2>&1 || true
}
trap cleanup EXIT
toxi() { docker exec "$TOXI" /toxiproxy-cli "$@" >/dev/null; }
revisions() { docker exec "$SFTP" sh -c "ls /home/backup/upload/snapshots/$ID 2>/dev/null | wc -l"; }

docker network create "$NET" >/dev/null
docker volume create "$KEYVOL" >/dev/null
docker run --rm -v "$KEYVOL":/keys --entrypoint bash "$IMAGE" -c '
  ssh-keygen -t ed25519 -N "" -f /keys/id_ed25519 -q && chmod 644 /keys/id_ed25519 /keys/id_ed25519.pub
' || die "ssh keypair generation failed"
docker run -d --name "$SFTP" --network "$NET" --network-alias sftp-server \
  -v "$KEYVOL":/home/backup/.ssh/keys:ro "$SFTP_IMAGE" backup::1001::upload >/dev/null || die "sftp server failed to start"
docker run -d --name "$TOXI" --network "$NET" --network-alias storage-link "$TOXI_IMAGE" >/dev/null || die "toxiproxy failed to start"
for _ in $(seq 1 30); do toxi create -l 0.0.0.0:2222 -u sftp-server:22 link && break; sleep 1; done
docker exec "$TOXI" /toxiproxy-cli list 2>/dev/null | grep -q link || die "toxiproxy proxy not created"

docker run -d --name "$ARCH" --network "$NET" --hostname fn-host --cap-drop ALL --cap-add DAC_OVERRIDE \
  -v "$KEYVOL":/client-keys:ro \
  -e SERVICE_DIRECTORIES=/data/services/app \
  -e STORAGE_TARGET_1_NAME=offsite -e STORAGE_TARGET_1_TYPE=sftp \
  -e STORAGE_TARGET_1_SFTP_URL=storage-link -e STORAGE_TARGET_1_SFTP_PORT=2222 \
  -e STORAGE_TARGET_1_SFTP_USER=backup -e STORAGE_TARGET_1_SFTP_PATH=upload \
  --entrypoint bash "$IMAGE" -c 'sleep 900' >/dev/null || die "archiver container failed to start"
docker exec "$ARCH" bash -c '
  set -e
  mkdir -p /opt/archiver/keys /run/secrets /data/services/app /root/.ssh
  openssl genrsa -aes256 -passout pass:pp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null
  openssl rsa -in /opt/archiver/keys/private.pem -passin pass:pp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null
  chmod 600 /opt/archiver/keys/private.pem
  cp /client-keys/id_ed25519 /client-keys/id_ed25519.pub /opt/archiver/keys/ && chmod 600 /opt/archiver/keys/id_ed25519
  printf testpassword > /run/secrets/storage_password; printf pp > /run/secrets/rsa_passphrase
  head -c 200000 /dev/urandom > /data/services/app/first.bin
  for _ in $(seq 1 30); do
    ssh-keyscan -T 3 -p 2222 storage-link > /root/.ssh/known_hosts 2>/dev/null && [ -s /root/.ssh/known_hosts ] && exit 0
    sleep 1
  done
  exit 1
' || die "in-container setup failed"

log "a slow link (300 ms on every reply) still completes"
toxi toxic add -t latency -a latency=300 -n slow link
docker exec "$ARCH" archiver backup >/tmp/fnet-b1.out 2>&1 || { cat /tmp/fnet-b1.out; die "a backup over a slow link failed"; }
[ "$(revisions)" -eq 1 ] || die "the slow backup did not add its revision"
toxi toxic remove -n slow link

log "the link cut mid-backup: the run fails with an error, adds no revision, releases the lock"
docker exec "$ARCH" sh -c 'head -c 4000000 /dev/urandom > /data/services/app/second.bin'
toxi toxic add -t bandwidth --upstream -a rate=64 -n narrow link
docker exec "$ARCH" archiver backup >/tmp/fnet-b2.out 2>&1 &
pid=$!
for _ in $(seq 1 60); do docker exec "$ARCH" pgrep -f "duplicacy.* backup" >/dev/null && break; sleep 1; done
sleep 5
toxi toggle link
timeout 600 tail --pid="$pid" -f /dev/null || die "a backup over a cut link never ended"
wait "$pid" && { cat /tmp/fnet-b2.out; die "a backup over a cut link exited 0"; }
docker exec "$ARCH" grep -q '\[ERROR\]' /opt/archiver/logs/archiver.log || die "no error logged for a cut link"
toxi toggle link
toxi toxic remove -n narrow link
[ "$(revisions)" -eq 1 ] || die "a backup over a cut link added a revision"
docker exec "$ARCH" archiver status | grep -q "Backup: not running" || die "the lock was not released after a cut link"

log "the link back: the next backup completes and restores byte for byte"
docker exec "$ARCH" archiver backup >/tmp/fnet-b3.out 2>&1 || { cat /tmp/fnet-b3.out; die "the backup after the link returned failed"; }
[ "$(revisions)" -eq 2 ] || die "the backup after the link returned did not add its revision"
docker exec -e SNAPSHOT_ID=$ID -e LOCAL_DIR=/data/restore "$ARCH" archiver auto-restore >/tmp/fnet-r.out 2>&1 || { cat /tmp/fnet-r.out; die "restore after the link returned failed"; }
docker exec "$ARCH" sh -c 'cmp /data/services/app/first.bin /data/restore/first.bin && cmp /data/services/app/second.bin /data/restore/second.bin' \
  || die "the revision after the link returned restored different bytes"

log "the link cut mid-copy (SFTP as a copy worker's secondary): retried, never reported done, caught up once the link returns"
COPYENV=(-e STORAGE_TARGET_1_NAME=local -e STORAGE_TARGET_1_TYPE=local -e STORAGE_TARGET_1_LOCAL_PATH=/backup-store
  -e STORAGE_TARGET_2_NAME=offsite -e STORAGE_TARGET_2_TYPE=sftp -e STORAGE_TARGET_2_SFTP_URL=storage-link
  -e STORAGE_TARGET_2_SFTP_PORT=2222 -e STORAGE_TARGET_2_SFTP_USER=backup -e STORAGE_TARGET_2_SFTP_PATH=upload/copy
  -e STORAGE_TARGET_1_SFTP_URL= -e STORAGE_TARGET_1_SFTP_PORT= -e STORAGE_TARGET_1_SFTP_USER= -e STORAGE_TARGET_1_SFTP_PATH=)
copies() { docker exec "$SFTP" sh -c "ls /home/backup/upload/copy/snapshots/$ID 2>/dev/null | wc -l"; }
docker exec "$ARCH" mkdir -p /backup-store
docker exec "$SFTP" sh -c "mkdir -p /home/backup/upload/copy && chown 1001 /home/backup/upload/copy"
docker exec "${COPYENV[@]}" "$ARCH" archiver backup >/tmp/fnet-b4.out 2>&1 || { cat /tmp/fnet-b4.out; die "the local backup with a copy failed"; }
[ "$(copies)" -eq 1 ] || die "the inline copy did not reach the secondary"
# Under the daemon a backup hands its copy to the worker, whose retry is what this tests.
docker exec -d "${COPYENV[@]}" -e BACKUP_SCHEDULE="0 3 1 1 *" "$ARCH" sh -c 'archiver daemon > /tmp/daemon.out 2>&1'
for _ in $(seq 1 60); do docker exec "${COPYENV[@]}" "$ARCH" archiver status | grep -q "offsite: caught up" && break; sleep 1; done
docker exec "${COPYENV[@]}" "$ARCH" archiver status | grep -q "offsite: caught up" || die "the worker never reported caught up"
docker exec "$ARCH" sh -c 'head -c 4000000 /dev/urandom > /data/services/app/third.bin'
toxi toxic add -t bandwidth --upstream -a rate=64 -n narrow link
docker exec "${COPYENV[@]}" "$ARCH" archiver backup >/tmp/fnet-b5.out 2>&1 || { cat /tmp/fnet-b5.out; die "the local backup handing off its copy failed"; }
for _ in $(seq 1 90); do docker exec "$ARCH" pgrep -f "duplicacy.* copy " >/dev/null && break; sleep 1; done
docker exec "$ARCH" pgrep -f "duplicacy.* copy " >/dev/null || die "the worker never started its copy"
sleep 5
toxi toggle link
# Duplicacy retries the connection itself (1, 2, 4 ... 128 s), then the worker retries the
# copy: either way, nothing is reported done while the link is down.
sleep 20
docker exec "${COPYENV[@]}" "$ARCH" archiver status | grep -q "offsite: caught up" && die "status reports caught up while the copy's link is cut"
[ "$(copies)" -eq 1 ] || die "a cut copy added a revision"
toxi toggle link
toxi toxic remove -n narrow link
for _ in $(seq 1 420); do [ "$(copies)" -eq 2 ] && break; sleep 1; done
[ "$(copies)" -eq 2 ] || die "the worker did not catch up after the link returned"
docker exec "${COPYENV[@]}" -e SNAPSHOT_ID=$ID -e LOCAL_DIR=/data/restore-copy -e STORAGE_TARGET=offsite "$ARCH" archiver auto-restore >/tmp/fnet-r2.out 2>&1 \
  || { cat /tmp/fnet-r2.out; die "restore from the secondary after the cut failed"; }
docker exec "$ARCH" cmp /data/services/app/third.bin /data/restore-copy/third.bin || die "the copied revision restored different bytes"

echo "PASS: fault injection, network"
