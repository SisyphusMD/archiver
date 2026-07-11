#!/usr/bin/env bash
# Real sftp-type runtime coverage against an sshd sidecar: init, backup, and restore over
# SSH through the exact production code path (build_storage_url's sftp:// URL, the SSH key
# files placed by the entrypoint convention, duplicacy's ssh_key_file wiring). Only
# string-level tests covered sftp before.
#
# HOST-DRIVEN: run on the docker host (not via --entrypoint bash).
#
#   IMAGE=archiver:dev bash tests/integration/sftp-runtime.sh

set -uo pipefail

IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"
NET="archiver-sftp-net-$$"
SFTP="archiver-sftp-srv-$$"
ARCH="archiver-sftp-arch-$$"
KEYVOL="archiver-sftp-keys-$$"
# Digest-pinned: this test gates releases; a floating tag could break a cut. (Renovate does
# not scan shell scripts — bump the digest by hand when updating.)
SFTP_IMAGE="atmoz/sftp:alpine@sha256:a6cb3eb29202ca7f57e73bb7e527286e66e0e822fff65609207c7e0ef2d135a3"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
cleanup() {
  docker rm -f "$SFTP" "$ARCH" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  docker volume rm "$KEYVOL" >/dev/null 2>&1 || true
}
trap cleanup EXIT

log "generate the client SSH keypair into a shared volume"
docker network create "$NET" >/dev/null
docker volume create "$KEYVOL" >/dev/null
docker run --rm -v "$KEYVOL":/keys --entrypoint bash "$IMAGE" -c '
  set -e
  ssh-keygen -t ed25519 -N "" -f /keys/id_ed25519 -q
  chmod 644 /keys/id_ed25519 /keys/id_ed25519.pub
' || die "ssh keypair generation failed"

log "start the sftp server (atmoz/sftp), authorizing the client key"
docker run -d --name "$SFTP" --network "$NET" --network-alias sftp-server \
  -v "$KEYVOL":/home/backup/.ssh/keys:ro \
  "$SFTP_IMAGE" backup::1001::upload >/dev/null || die "sftp server failed to start"

log "archiver container on the same network (env-native, sftp target)"
docker run -d --name "$ARCH" --network "$NET" \
  --cap-drop ALL --cap-add DAC_OVERRIDE \
  -v "$KEYVOL":/client-keys:ro \
  -e SERVICE_DIRECTORIES=/data/fixtures/ \
  -e STORAGE_TARGET_1_NAME=offsite \
  -e STORAGE_TARGET_1_TYPE=sftp \
  -e STORAGE_TARGET_1_SFTP_URL=sftp-server \
  -e STORAGE_TARGET_1_SFTP_PORT=22 \
  -e STORAGE_TARGET_1_SFTP_USER=backup \
  -e STORAGE_TARGET_1_SFTP_PATH=upload \
  -e ROTATE_BACKUPS=false \
  --entrypoint bash "$IMAGE" -c 'sleep 600' >/dev/null || die "archiver container failed to start"

log "in-container: RSA keys, secrets, SSH client key at its canonical path, known_hosts"
docker exec "$ARCH" bash -c '
  set -e
  mkdir -p /opt/archiver/keys /run/secrets /data/fixtures /root/.ssh
  openssl genrsa -aes256 -passout pass:testpassphrase -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null
  openssl rsa -in /opt/archiver/keys/private.pem -passin pass:testpassphrase -pubout -out /opt/archiver/keys/public.pem 2>/dev/null
  chmod 600 /opt/archiver/keys/private.pem
  cp /client-keys/id_ed25519 /opt/archiver/keys/id_ed25519
  cp /client-keys/id_ed25519.pub /opt/archiver/keys/id_ed25519.pub
  chmod 600 /opt/archiver/keys/id_ed25519
  printf "testpassword" > /run/secrets/storage_password
  printf "testpassphrase" > /run/secrets/rsa_passphrase
  printf "recovery-sftp-password" > /run/secrets/recovery_password
  echo "hello via sftp" > /data/fixtures/file.txt
  head -c 8192 /dev/urandom > /data/fixtures/blob.bin
  for _ in $(seq 1 30); do
    ssh-keyscan -T 3 sftp-server > /root/.ssh/known_hosts 2>/dev/null && [ -s /root/.ssh/known_hosts ] && exit 0
    sleep 1
  done
  echo "sftp server never answered keyscan" >&2; exit 1
' || die "in-container setup failed"

log "backup over sftp://"
docker exec "$ARCH" archiver backup retain || die "sftp backup exited non-zero"
docker exec "$SFTP" test -d /home/backup/upload/snapshots || die "no snapshots directory on the sftp server"

log "recovery kit must sit beside the duplicacy data on the sftp server"
HOSTN="$(docker exec "$ARCH" hostname)"
docker exec "$SFTP" test -f "/home/backup/upload/archiver-recovery-kit-${HOSTN}.tar.enc" || die "no recovery kit on the sftp server"
docker exec "$SFTP" test -f "/home/backup/upload/archiver-recovery-kit-${HOSTN}.README.txt" || die "no kit README on the sftp server"
docker exec "$ARCH" bash -c '
  set -e
  sftp -q -P 22 -i /opt/archiver/keys/id_ed25519 -o BatchMode=yes -b - backup@sftp-server \
    <<< "get /upload/archiver-recovery-kit-$(hostname).tar.enc /tmp/kit.enc" >/dev/null
  openssl enc -d -aes-256-cbc -pbkdf2 -pass pass:recovery-sftp-password -in /tmp/kit.enc | tar -xf - -C /tmp
  grep -q "^STORAGE_TARGET_1_SFTP_URL=sftp-server$" /tmp/archiver.env
  [ "$(cat /tmp/secrets/storage_password)" = "testpassword" ]
' || die "recovery kit not retrievable/decryptable from sftp"

log "restore from the sftp storage"
docker exec -e SNAPSHOT_ID="$(docker exec "$ARCH" hostname)-fixtures" -e LOCAL_DIR=/data/restore \
  -e OVERWRITE=1 -e HASH_COMPARE=1 "$ARCH" archiver auto-restore || die "sftp auto-restore exited non-zero"

docker exec "$ARCH" bash -c '
  set -e
  diff /data/fixtures/file.txt /data/restore/file.txt
  cmp /data/fixtures/blob.bin /data/restore/blob.bin
' || die "restored content differs from source"

echo "=== SFTP-RUNTIME OK: backup + restore over ssh ==="
