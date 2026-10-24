#!/usr/bin/env bash
# Host-driven: named volumes also work with a remote Docker daemon.
# Garage is the existing S3 stand-in; the minio storage type reaches its HTTP API.
set -euo pipefail
IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"
P="archiver-kd-$$"
NET="$P-net"
CFG="$P-cfg"
DATA="$P-data"
ARCH="$P-arch"
SFTP="$P-sftp"
GARAGE="$P-garage"
GARAGE_IMAGE="dxflrs/garage:v2.4.1@sha256:9c96caa2612d3411acc5b0e6701fb238dbfba33e533a6d7d3d811a4b12d0d020"
SFTP_IMAGE="atmoz/sftp:alpine@sha256:7e439fabeae536fa254340724c042e11999924d906417dbb5c73602ad518813f"
S3_KEY="GK0123456789abcdef01234567"
S3_SECRET="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
BG_KEY="GK1123456789abcdef01234567"
BG_SECRET="1123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
cleanup() {
 local rc=$?
 if [ "$rc" -ne 0 ]; then
  docker exec "$ARCH" tail -n 50 /opt/archiver/logs/archiver.log >&2 2>/dev/null || true
  docker logs "$SFTP" --tail 30 >&2 2>/dev/null || true
 fi
 docker rm -f "$ARCH" "$SFTP" "$GARAGE" >/dev/null 2>&1 || true
 docker network rm "$NET" >/dev/null 2>&1 || true
 docker volume rm "$CFG" "$DATA" >/dev/null 2>&1 || true
}
trap cleanup EXIT
garage() { docker exec "$GARAGE" /garage -c /cfg/garage.toml "$@"; }
expect_failure() {
 local storage="$1" message="$2" out
 if out=$(docker exec "$ARCH" archiver kit-drill "$storage" 2>&1); then die "$storage unexpectedly passed"; fi
 printf '%s\n' "$out"
 [[ "$out" == *"storage '$storage'"* && "$out" == *"$message"* ]] || die "failure did not name $storage and $message"
}
docker network create "$NET" >/dev/null
docker volume create "$CFG" >/dev/null
docker volume create "$DATA" >/dev/null
log "generate distinct backup and break-glass SSH keys"
docker run --rm -i -v "$CFG":/cfg --entrypoint bash "$IMAGE" -s <<'SETUP'
set -e
ssh-keygen -t ed25519 -N '' -f /cfg/backup -q
ssh-keygen -t ed25519 -N '' -f /cfg/glass -q
cat > /cfg/garage.toml <<TOML
metadata_dir = "/var/lib/garage/meta"
data_dir = "/var/lib/garage/data"
db_engine = "sqlite"
replication_factor = 1
rpc_bind_addr = "[::]:3901"
rpc_public_addr = "127.0.0.1:3901"
rpc_secret = "$(openssl rand -hex 32)"
[s3_api]
s3_region = "garage"
api_bind_addr = "[::]:3900"
root_domain = ".garage"
TOML
SETUP
docker run -d --name "$GARAGE" --network "$NET" --network-alias garage-api -v "$CFG":/cfg \
 "$GARAGE_IMAGE" /garage -c /cfg/garage.toml server >/dev/null
docker run -d --name "$SFTP" --network "$NET" --network-alias sftp-server \
 -v "$CFG":/cfg:ro -v "$DATA":/home/backup/upload -v "$DATA":/home/recovery/upload \
 "$SFTP_IMAGE" backup::1001::upload recovery::1001::upload >/dev/null
for _ in $(seq 1 30); do garage status >/dev/null 2>&1 && break; sleep 1; done
NODE="$(garage node id -q | cut -d@ -f1)"
[ -n "$NODE" ] || die "garage did not become ready"
garage layout assign -z dc1 -c 1G "$NODE" >/dev/null
garage layout apply --version 1 >/dev/null
garage key import --yes -n backup "$S3_KEY" "$S3_SECRET" >/dev/null
garage key import --yes -n reader "$BG_KEY" "$BG_SECRET" >/dev/null
garage bucket create kits >/dev/null
garage bucket allow --read --write --owner kits --key backup >/dev/null
garage bucket allow --read kits --key reader >/dev/null
for _ in $(seq 1 30); do docker exec "$SFTP" test -d /home/recovery && break; sleep 1; done
for _ in $(seq 1 30); do docker exec "$SFTP" pgrep sshd >/dev/null 2>&1 && break; sleep 1; done
docker exec "$SFTP" pgrep sshd >/dev/null || die "SFTP server did not become ready"
docker exec "$SFTP" sh -c '
 set -e
 for pair in backup:backup recovery:glass; do
  user=${pair%:*}; key=${pair#*:}
  mkdir -p /home/$user/.ssh
  cp /cfg/$key.pub /home/$user/.ssh/authorized_keys
  chown -R 1001:1001 /home/$user/.ssh
  chmod 700 /home/$user/.ssh
  chmod 600 /home/$user/.ssh/authorized_keys
 done
 chown 1001:1001 /home/backup/upload
 chmod 755 /home/backup/upload
'
docker run -d --name "$ARCH" --hostname kd-host --network "$NET" \
 --cap-drop ALL --cap-add DAC_OVERRIDE -v "$CFG":/cfg:ro \
 -e SERVICE_DIRECTORIES=/data/fixtures/ -e PRUNE_BACKUPS=false \
 -e STORAGE_TARGET_1_NAME=cloud -e STORAGE_TARGET_1_TYPE=minio \
 -e STORAGE_TARGET_1_S3_ENDPOINT=garage-api:3900 -e STORAGE_TARGET_1_S3_BUCKETNAME=kits -e STORAGE_TARGET_1_S3_REGION=garage \
 -e STORAGE_TARGET_2_NAME=offsite -e STORAGE_TARGET_2_TYPE=sftp \
 -e STORAGE_TARGET_2_SFTP_URL=sftp-server -e STORAGE_TARGET_2_SFTP_PORT=22 \
 -e STORAGE_TARGET_2_SFTP_USER=backup -e STORAGE_TARGET_2_SFTP_PATH=upload \
 -e STORAGE_TARGET_2_BREAKGLASS_SFTP_USER=recovery \
 --entrypoint bash "$IMAGE" -c 'sleep 900' >/dev/null
docker exec -i "$ARCH" bash -s -- "$S3_KEY" "$S3_SECRET" "$BG_KEY" "$BG_SECRET" <<'SETUP'
set -e
mkdir -p /opt/archiver/keys /run/secrets /data/fixtures
openssl genrsa -aes256 -passout pass:testpassphrase -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:testpassphrase -pubout -out /opt/archiver/keys/public.pem 2>/dev/null
cp /cfg/backup /opt/archiver/keys/id_ed25519
cp /cfg/backup.pub /opt/archiver/keys/id_ed25519.pub
cp /cfg/glass /run/secrets/storage_target_2_breakglass_ssh_key
chmod 600 /opt/archiver/keys/private.pem /opt/archiver/keys/id_ed25519 /run/secrets/storage_target_2_breakglass_ssh_key
printf testpassword > /run/secrets/storage_password
printf testpassphrase > /run/secrets/rsa_passphrase
printf recovery-kit-password > /run/secrets/recovery_password
printf %s "$1" > /run/secrets/storage_target_1_s3_id
printf %s "$2" > /run/secrets/storage_target_1_s3_secret
printf %s "$3" > /run/secrets/storage_target_1_breakglass_s3_id
printf %s "$4" > /run/secrets/storage_target_1_breakglass_s3_secret
printf 'kit drill fixture\n' > /data/fixtures/file.txt
for _ in $(seq 1 30); do ssh-keyscan -T 3 sftp-server >/dev/null 2>&1 && exit 0; sleep 1; done
exit 1
SETUP
log "backup builds and places kits on the remote primary and SFTP secondary"
docker exec "$ARCH" archiver backup
log "both break-glass paths pass"
docker exec "$ARCH" archiver kit-drill
docker exec "$ARCH" archiver status --json | grep '"kit_drills"' >/dev/null || die "kit drills absent from status"
log "save the old kit, refresh after a secret changes, then replace only the SFTP kit with the stale one"
docker exec "$SFTP" cp /home/backup/upload/archiver-recovery-kit-kd-host.tar.enc /tmp/stale.enc
docker exec "$ARCH" bash -c 'printf changed-storage-password > /run/secrets/storage_password'
docker exec "$ARCH" archiver recovery-kit force
docker exec "$SFTP" cp /tmp/stale.enc /home/backup/upload/archiver-recovery-kit-kd-host.tar.enc
expect_failure offsite 'differs from the local kit'
log "current kit clears the incident"
docker exec "$ARCH" archiver recovery-kit force
docker exec "$ARCH" archiver kit-drill offsite
log "revoke the S3 read-only credential; backup access remains valid"
garage bucket deny --read kits --key reader >/dev/null
expect_failure cloud 'envelope credential'
garage bucket allow --read kits --key reader >/dev/null
docker exec "$ARCH" archiver kit-drill cloud
log "remove only the break-glass SSH authorized key; the drill must not fall back to backup's key"
docker exec "$SFTP" sh -c ': > /home/recovery/.ssh/authorized_keys'
expect_failure offsite 'envelope credential'
docker exec "$SFTP" sh -c 'cat /cfg/glass.pub > /home/recovery/.ssh/authorized_keys'
docker exec "$ARCH" archiver kit-drill offsite
docker exec "$ARCH" bash -c '! grep -q "kit-drill:" /opt/archiver/logs/.incidents.json; ! compgen -G "/tmp/archiver-kit-drill-*"'
log "kit drill integration passed"
