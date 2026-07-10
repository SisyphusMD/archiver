#!/usr/bin/env bash
# Real s3-type runtime coverage against a MinIO sidecar: init, backup, and restore over the
# S3 API through the exact production code path (build_storage_url's s3:// URL + the
# DUPLICACY_<NAME>_* credential env vars). String-level bats tests missed the 0.8.10/0.8.11
# do-spaces breakage; this exercises the wire. MinIO serves TLS with a self-signed cert the
# archiver container is taught to trust (duplicacy's s3 backend is https-only).
#
# HOST-DRIVEN: run on the docker host (not via --entrypoint bash).
#
#   IMAGE=archiver:dev bash tests/integration/s3-minio.sh

set -uo pipefail

IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"
NET="archiver-s3-net-$$"
MINIO="archiver-s3-minio-$$"
ARCH="archiver-s3-arch-$$"
CERTVOL="archiver-s3-certs-$$"
# Digest-pinned: this test gates releases; keep deterministic. (Renovate does not scan
# shell scripts — bump the digest by hand when updating.)
MINIO_IMAGE="minio/minio:RELEASE.2025-04-22T22-12-26Z@sha256:a1ea29fa28355559ef137d71fc570e508a214ec84ff8083e39bc5428980b015e"
S3_KEY="testkey"
S3_SECRET="testsecret123"
BUCKET="archiver-test"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
cleanup() {
  docker rm -f "$MINIO" "$ARCH" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  docker volume rm "$CERTVOL" >/dev/null 2>&1 || true
}
trap cleanup EXIT

log "self-signed TLS cert into a shared volume"
docker network create "$NET" >/dev/null
docker volume create "$CERTVOL" >/dev/null
# duplicacy's AWS SDK addresses buckets virtual-host-style (<bucket>.<endpoint>), exactly
# like real S3/Spaces endpoints — so the cert and the container's network aliases must
# cover the bucket-prefixed name too.
docker run --rm -v "$CERTVOL":/certs --entrypoint bash "$IMAGE" -c "
  set -e
  openssl req -x509 -newkey rsa:2048 -nodes -days 2 \
    -keyout /certs/private.key -out /certs/public.crt \
    -subj '/CN=minio' -addext 'subjectAltName=DNS:minio,DNS:${BUCKET}.minio' 2>/dev/null
  chmod 644 /certs/public.crt /certs/private.key
" || die "cert generation failed"

log "start MinIO (TLS) on the test network"
# MINIO_DOMAIN enables virtual-host-style routing; without it MinIO parses the
# bucket-prefixed Host as path-style and misroutes PutObject to CreateBucket.
docker run -d --name "$MINIO" --network "$NET" \
  --network-alias minio --network-alias "${BUCKET}.minio" \
  -v "$CERTVOL":/root/.minio/certs \
  -e MINIO_ROOT_USER="$S3_KEY" -e MINIO_ROOT_PASSWORD="$S3_SECRET" \
  -e MINIO_DOMAIN=minio \
  "$MINIO_IMAGE" server /data >/dev/null || die "minio failed to start"

log "archiver container on the same network (env-native, s3 target)"
docker run -d --name "$ARCH" --network "$NET" \
  --cap-drop ALL --cap-add DAC_OVERRIDE \
  -e SERVICE_DIRECTORIES=/data/fixtures/ \
  -e STORAGE_TARGET_1_NAME=minio \
  -e STORAGE_TARGET_1_TYPE=s3 \
  -e STORAGE_TARGET_1_S3_ENDPOINT=minio:9000 \
  -e STORAGE_TARGET_1_S3_BUCKETNAME="$BUCKET" \
  -e STORAGE_TARGET_1_S3_REGION=us-east-1 \
  -e ROTATE_BACKUPS=false \
  --entrypoint bash "$IMAGE" -c 'sleep 600' >/dev/null || die "archiver container failed to start"

log "in-container: secrets, CA trust, fixtures; wait for MinIO; create the bucket"
docker exec "$ARCH" bash -c "
  set -e
  mkdir -p /opt/archiver/keys /run/secrets /data/fixtures
  openssl genrsa -aes256 -passout pass:testpassphrase -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null
  openssl rsa -in /opt/archiver/keys/private.pem -passin pass:testpassphrase -pubout -out /opt/archiver/keys/public.pem 2>/dev/null
  chmod 600 /opt/archiver/keys/private.pem
  printf 'testpassword' > /run/secrets/storage_password
  printf 'testpassphrase' > /run/secrets/rsa_passphrase
  printf '%s' '${S3_KEY}'    > /run/secrets/storage_target_1_s3_id
  printf '%s' '${S3_SECRET}' > /run/secrets/storage_target_1_s3_secret
  echo 'hello via s3' > /data/fixtures/file.txt
  head -c 8192 /dev/urandom > /data/fixtures/blob.bin
" || die "in-container setup failed"

# the cert volume is not mounted in the archiver container; pipe the CA in via stdin
docker run --rm -v "$CERTVOL":/certs --entrypoint cat "$IMAGE" /certs/public.crt | \
  docker exec -i "$ARCH" bash -c 'cat > /usr/local/share/ca-certificates/minio.crt && update-ca-certificates >/dev/null' \
  || die "CA trust installation failed"

docker exec "$ARCH" bash -c '
  set -e
  for _ in $(seq 1 60); do
    curl -fsS -o /dev/null https://minio:9000/minio/health/ready && exit 0
    sleep 1
  done
  echo "minio never became ready" >&2; exit 1
' || die "minio not ready"

docker exec "$ARCH" bash -c "
  curl -fsS -X PUT --aws-sigv4 'aws:amz:us-east-1:s3' --user '${S3_KEY}:${S3_SECRET}' 'https://minio:9000/${BUCKET}/'
" || die "bucket creation failed"

log "backup to MinIO over s3://"
docker exec "$ARCH" archiver backup retain || die "s3 backup exited non-zero"

log "restore from MinIO"
docker exec -e SNAPSHOT_ID="$(docker exec "$ARCH" hostname)-fixtures" -e LOCAL_DIR=/data/restore \
  -e OVERWRITE=1 -e HASH_COMPARE=1 "$ARCH" archiver auto-restore || die "s3 auto-restore exited non-zero"

docker exec "$ARCH" bash -c '
  set -e
  diff /data/fixtures/file.txt /data/restore/file.txt
  cmp /data/fixtures/blob.bin /data/restore/blob.bin
' || die "restored content differs from source"

echo "=== S3-MINIO OK: backup + restore over the s3 wire (TLS) ==="
