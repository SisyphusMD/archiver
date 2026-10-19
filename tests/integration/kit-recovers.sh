#!/usr/bin/env bash
# The recovery kit reaches a secondary that missed it, with nobody acting: once the copy
# worker catches up, the daemon places the kit there, before any further backup or
# `archiver recovery-kit`. Two ways to miss it: the kit upload failed while copies worked (the
# kit incident opens, and clears once placed), and the storage was down at backup time (the
# kit step skipped it).
#
# HOST-DRIVEN: run on the docker host (not via --entrypoint bash).
#
#   IMAGE=archiver:dev bash tests/integration/kit-recovers.sh

set -uo pipefail

IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"
NET="archiver-kitr-$$"
SFTP="archiver-kitr-sftp-$$"
TOXI="archiver-kitr-toxi-$$"
ARCH="archiver-kitr-arch-$$"
KEYVOL="archiver-kitr-keys-$$"
# Digest-pinned like every sidecar here (Renovate does not scan shell scripts).
SFTP_IMAGE="atmoz/sftp:alpine@sha256:7e439fabeae536fa254340724c042e11999924d906417dbb5c73602ad518813f"
TOXI_IMAGE="ghcr.io/shopify/toxiproxy:2.12.0@sha256:9378ed52a28bc50edc1350f936f518f31fa95f0d15917d6eb40b8e376d1a214e"
KIT=archiver-recovery-kit-kr-host.tar.enc

log() { printf '>>> %s\n' "$*"; }
die() {
  printf 'FAIL: %s\n' "$*" >&2
  docker exec "$ARCH" sh -c 'tail -15 /opt/archiver/logs/archiver.log; tail -15 /opt/archiver/logs/copies.log; cat /tmp/daemon.out' >&2 2>/dev/null
  exit 1
}
cleanup() {
  docker rm -f "$SFTP" "$TOXI" "$ARCH" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  docker volume rm "$KEYVOL" >/dev/null 2>&1 || true
}
trap cleanup EXIT
toxi() { docker exec "$TOXI" /toxiproxy-cli "$@" >/dev/null; }
kit_on_secondary() { docker exec "$SFTP" sh -c "test -f /home/backup/upload/copy/$KIT && test -s /home/backup/upload/copy/$KIT"; }
status() { docker exec "$ARCH" archiver status 2>&1; }

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
for _ in $(seq 1 30); do docker exec "$SFTP" test -d /home/backup/upload && break; sleep 1; done
docker exec "$SFTP" sh -c "mkdir -p /home/backup/upload/copy && chown 1001 /home/backup/upload/copy"

docker run -d --name "$ARCH" --network "$NET" --hostname kr-host --cap-drop ALL --cap-add DAC_OVERRIDE \
  -v "$KEYVOL":/client-keys:ro \
  -e SERVICE_DIRECTORIES=/data/services/app \
  -e STORAGE_TARGET_1_NAME=local -e STORAGE_TARGET_1_TYPE=local -e STORAGE_TARGET_1_LOCAL_PATH=/backup-store \
  -e STORAGE_TARGET_2_NAME=offsite -e STORAGE_TARGET_2_TYPE=sftp -e STORAGE_TARGET_2_SFTP_URL=storage-link \
  -e STORAGE_TARGET_2_SFTP_PORT=2222 -e STORAGE_TARGET_2_SFTP_USER=backup -e STORAGE_TARGET_2_SFTP_PATH=upload/copy \
  -e BACKUP_SCHEDULE="0 3 1 1 *" -e MAINTENANCE_SCHEDULE="0 4 1 1 *" \
  --entrypoint bash "$IMAGE" -c 'sleep 900' >/dev/null || die "archiver container failed to start"
docker exec "$ARCH" bash -c '
  set -e
  mkdir -p /opt/archiver/keys /run/secrets /data/services/app /backup-store /root/.ssh
  openssl genrsa -aes256 -passout pass:pp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null
  openssl rsa -in /opt/archiver/keys/private.pem -passin pass:pp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null
  chmod 600 /opt/archiver/keys/private.pem
  cp /client-keys/id_ed25519 /client-keys/id_ed25519.pub /opt/archiver/keys/ && chmod 600 /opt/archiver/keys/id_ed25519
  printf testpassword > /run/secrets/storage_password; printf pp > /run/secrets/rsa_passphrase
  printf recoverypassword > /run/secrets/recovery_password
  head -c 200000 /dev/urandom > /data/services/app/first.bin
  for _ in $(seq 1 30); do
    ssh-keyscan -T 3 -p 2222 storage-link > /root/.ssh/known_hosts 2>/dev/null && [ -s /root/.ssh/known_hosts ] && exit 0
    sleep 1
  done
  exit 1
' || die "in-container setup failed"

placements() { docker exec "$ARCH" sh -c 'grep -c "lacks the current recovery kit; placing it now" /opt/archiver/logs/copies.log 2>/dev/null || true'; }
# Each backup run starts a new archiver.log, so this counts what began since it was read.
backups() { docker exec "$ARCH" sh -c 'grep -c "Starting backup for " /opt/archiver/logs/archiver.log'; }

log "the kit upload fails while copies work: the kit is on the primary only, and the kit incident opens"
# A directory where the kit goes fails its upload, and nothing else.
docker exec "$SFTP" sh -c "mkdir /home/backup/upload/copy/$KIT && chown 1001 /home/backup/upload/copy/$KIT"
docker exec "$ARCH" archiver backup >/tmp/kitr-b1.out 2>&1 && { cat /tmp/kitr-b1.out; die "a backup whose kit upload failed exited 0"; }
docker exec "$ARCH" test -s "/backup-store/$KIT" || die "the kit is not on the primary"
docker exec "$SFTP" sh -c "ls /home/backup/upload/copy/snapshots/kr-host-app | grep -q ." || die "the copy did not reach the secondary"
status | grep -q "Recovery Kit Failed" || { status; die "no kit incident for a failed kit upload"; }
ran=$(backups)

log "the upload can work again: the daemon's worker catches up and places the kit, the incident clears, no backup runs"
docker exec "$SFTP" rm -rf "/home/backup/upload/copy/$KIT" || die "could not clear the blocking directory"
docker exec -d "$ARCH" sh -c 'archiver daemon > /tmp/daemon.out 2>&1'
for _ in $(seq 1 120); do kit_on_secondary && break; sleep 1; done
kit_on_secondary || { status; die "the kit never reached the secondary"; }
for _ in $(seq 1 60); do status | grep -q "Recovery Kit Failed" || break; sleep 1; done
status | grep -q "Recovery Kit Failed" && { status; die "the kit incident stayed open after the kit was placed"; }
docker exec "$ARCH" grep -qx "offsite" /opt/archiver/logs/.recovery-kit-state || die "the kit state does not record the secondary"
[ "$(placements)" -eq 1 ] || die "expected one placement after recovery, logged $(placements)"
[ "$(backups)" = "$ran" ] || die "a backup ran to place the kit"

log "a later caught-up pass, with the kit placed, places nothing"
docker exec "$ARCH" archiver daemon ctl local-changed >/dev/null
sleep 10
[ "$(placements)" -eq 1 ] || die "the kit was placed again on a storage that already had it"

log "the secondary down at backup time: the kit step skips it, and it gets the kit once back"
docker exec "$SFTP" rm -f "/home/backup/upload/copy/$KIT"
docker exec "$ARCH" sed -i '/^offsite$/d' /opt/archiver/logs/.recovery-kit-state
toxi toggle link
docker exec "$ARCH" sh -c 'head -c 100000 /dev/urandom > /data/services/app/second.bin'
docker exec "$ARCH" archiver daemon ctl local-changed >/dev/null
for _ in $(seq 1 120); do status | grep -qE "offsite: (retrying|down)" && break; sleep 1; done
status | grep -qE "offsite: (retrying|down)" || { status; die "the worker never reported its failing copy"; }
docker exec "$ARCH" archiver backup >/tmp/kitr-b2.out 2>&1
ran=$(backups)
docker exec "$ARCH" grep -q "Recovery kit: skipping offsite" /opt/archiver/logs/archiver.log || die "the kit step did not skip the down storage"
kit_on_secondary && die "the kit reached a storage that was cut off"
toxi toggle link
# Its backoff has grown to minutes by now; a wake stands in for the retry falling due.
docker exec "$ARCH" archiver daemon ctl local-changed >/dev/null
for _ in $(seq 1 120); do kit_on_secondary && break; sleep 1; done
kit_on_secondary || { status; die "the kit never reached the secondary after it came back"; }
[ "$(placements)" -eq 2 ] || die "expected a second placement after the link returned, logged $(placements)"
[ "$(backups)" = "$ran" ] || die "a backup ran to place the kit"

echo "PASS: the kit reaches a secondary that missed it"
