#!/usr/bin/env bash
# Real s3-type runtime coverage against a Garage sidecar: init, backup, and restore over the
# S3 API through the exact production code path (build_storage_url's s3:// URL + the
# DUPLICACY_<NAME>_* credential env vars). String-level bats tests missed the 0.8.10/0.8.11
# do-spaces breakage; this exercises the wire. duplicacy's s3 backend is https-only and
# Garage serves plain HTTP, so a Caddy sidecar terminates TLS with a self-signed cert the
# archiver container is taught to trust.
#
# HOST-DRIVEN: run on the docker host (not via --entrypoint bash).
#
#   IMAGE=archiver:dev bash tests/integration/s3-garage.sh

set -uo pipefail

IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"
NET="archiver-s3-net-$$"
GARAGE="archiver-s3-garage-$$"
PROXY="archiver-s3-proxy-$$"
ARCH="archiver-s3-arch-$$"
CFGVOL="archiver-s3-cfg-$$"
# Digest-pinned: this test gates releases; keep deterministic. (Renovate does not scan
# shell scripts — bump the digests by hand when updating.) Garage matches the NAS's pin.
GARAGE_IMAGE="dxflrs/garage:v2.3.0@sha256:866bd13ed2038ba7e7190e840482bc27234c4afaf77be8cfa439ae088c1e4690"
CADDY_IMAGE="caddy:2.11.4-alpine@sha256:6aeddd44c3078b0f9a35206472a11420648a79c184603ef95957d0a20044cb2b"
# `garage key import` accepts caller-chosen credentials in Garage's own shape.
S3_KEY="GK0123456789abcdef01234567"
S3_SECRET="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
BUCKET="archiver-test"
REGION="garage"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
cleanup() {
  docker rm -f "$GARAGE" "$PROXY" "$ARCH" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  docker volume rm "$CFGVOL" >/dev/null 2>&1 || true
}
trap cleanup EXIT
garage() { docker exec "$GARAGE" /garage -c /cfg/garage.toml "$@"; }

log "Garage config, Caddyfile, and a self-signed TLS cert into a shared volume"
docker network create "$NET" >/dev/null
docker volume create "$CFGVOL" >/dev/null
# duplicacy's AWS SDK addresses buckets virtual-host-style (<bucket>.<endpoint>), exactly
# like real S3/Spaces endpoints — so the cert and the proxy's network aliases must cover the
# bucket-prefixed name too, and Garage's root_domain must strip it back off.
docker run --rm -i -v "$CFGVOL":/cfg --entrypoint bash "$IMAGE" -s <<EOF || die "config generation failed"
set -e
openssl req -x509 -newkey rsa:2048 -nodes -days 2 \
  -keyout /cfg/private.key -out /cfg/public.crt \
  -subj '/CN=garage' -addext 'subjectAltName=DNS:garage,DNS:${BUCKET}.garage' 2>/dev/null
chmod 644 /cfg/public.crt /cfg/private.key
cat > /cfg/garage.toml <<TOML
metadata_dir = "/var/lib/garage/meta"
data_dir = "/var/lib/garage/data"
db_engine = "sqlite"
replication_factor = 1
rpc_bind_addr = "[::]:3901"
rpc_public_addr = "127.0.0.1:3901"
rpc_secret = "\$(openssl rand -hex 32)"

[s3_api]
s3_region = "${REGION}"
api_bind_addr = "[::]:3900"
root_domain = ".garage"
TOML
# Caddy forwards the original Host header, which SigV4 signs and Garage parses the bucket from.
cat > /cfg/Caddyfile <<CADDY
{
	auto_https off
	admin off
}
https://garage, https://${BUCKET}.garage {
	tls /cfg/public.crt /cfg/private.key
	reverse_proxy garage-api:3900
}
CADDY
EOF

log "start Garage and its TLS front on the test network"
docker run -d --name "$GARAGE" --network "$NET" --network-alias garage-api \
  -v "$CFGVOL":/cfg "$GARAGE_IMAGE" /garage -c /cfg/garage.toml server >/dev/null \
  || die "garage failed to start"
docker run -d --name "$PROXY" --network "$NET" \
  --network-alias garage --network-alias "${BUCKET}.garage" \
  -v "$CFGVOL":/cfg "$CADDY_IMAGE" caddy run --config /cfg/Caddyfile --adapter caddyfile >/dev/null \
  || die "caddy failed to start"

log "single-node layout, a known access key, and the bucket"
for _ in $(seq 1 30); do garage status >/dev/null 2>&1 && break; sleep 1; done
NODE="$(garage node id -q 2>/dev/null | cut -d@ -f1)"
[ -n "$NODE" ] || die "garage never reported a node id"
garage layout assign -z dc1 -c 1G "$NODE" >/dev/null || die "garage layout assign failed"
garage layout apply --version 1 >/dev/null || die "garage layout apply failed"
garage key import --yes -n "$BUCKET" "$S3_KEY" "$S3_SECRET" >/dev/null || die "garage key import failed"
garage bucket create "$BUCKET" >/dev/null || die "garage bucket create failed"
garage bucket allow --read --write --owner "$BUCKET" --key "$BUCKET" >/dev/null \
  || die "garage bucket allow failed"

log "archiver container on the same network (env-native, s3 target)"
docker run -d --name "$ARCH" --network "$NET" \
  --cap-drop ALL --cap-add DAC_OVERRIDE \
  -e SERVICE_DIRECTORIES=/data/fixtures/ \
  -e STORAGE_TARGET_1_NAME=garage \
  -e STORAGE_TARGET_1_TYPE=s3 \
  -e STORAGE_TARGET_1_S3_ENDPOINT=garage \
  -e STORAGE_TARGET_1_S3_BUCKETNAME="$BUCKET" \
  -e STORAGE_TARGET_1_S3_REGION="$REGION" \
  -e ROTATE_BACKUPS=false \
  --entrypoint bash "$IMAGE" -c 'sleep 600' >/dev/null || die "archiver container failed to start"

log "in-container: secrets, CA trust, fixtures; wait for the TLS front"
docker exec "$ARCH" bash -c "
  set -e
  mkdir -p /opt/archiver/keys /run/secrets /data/fixtures
  openssl genrsa -aes256 -passout pass:testpassphrase -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null
  openssl rsa -in /opt/archiver/keys/private.pem -passin pass:testpassphrase -pubout -out /opt/archiver/keys/public.pem 2>/dev/null
  chmod 600 /opt/archiver/keys/private.pem
  printf 'testpassword' > /run/secrets/storage_password
  printf 'testpassphrase' > /run/secrets/rsa_passphrase
  printf 'recovery-s3-password' > /run/secrets/recovery_password
  printf '%s' '${S3_KEY}'    > /run/secrets/storage_target_1_s3_id
  printf '%s' '${S3_SECRET}' > /run/secrets/storage_target_1_s3_secret
  echo 'hello via s3' > /data/fixtures/file.txt
  head -c 8192 /dev/urandom > /data/fixtures/blob.bin
" || die "in-container setup failed"

# the config volume is not mounted in the archiver container; pipe the CA in via stdin
docker run --rm -v "$CFGVOL":/cfg --entrypoint cat "$IMAGE" /cfg/public.crt | \
  docker exec -i "$ARCH" bash -c 'cat > /usr/local/share/ca-certificates/garage.crt && update-ca-certificates >/dev/null' \
  || die "CA trust installation failed"

# A signed ListObjectsV2 answering 200 proves the whole chain: TLS, the proxy, Garage's
# applied layout, the key, and the bucket. Any other status (a 502 while Garage is still
# starting) keeps waiting.
docker exec "$ARCH" bash -c "
  for _ in \$(seq 1 60); do
    curl -fsS -o /dev/null --aws-sigv4 'aws:amz:${REGION}:s3' --user '${S3_KEY}:${S3_SECRET}' \
      'https://garage/${BUCKET}/?list-type=2' 2>/dev/null && exit 0
    sleep 1
  done
  echo 'Garage never answered a signed request' >&2; exit 1
" || die "garage not ready over TLS"

log "backup to Garage over s3://"
docker exec "$ARCH" archiver backup || die "s3 backup exited non-zero"

log "recovery kit must be a plain object in the bucket, decryptable with only the password"
HOSTN="$(docker exec "$ARCH" hostname)"
docker exec "$ARCH" bash -c "
  set -e
  curl -fsS --aws-sigv4 'aws:amz:${REGION}:s3' --user '${S3_KEY}:${S3_SECRET}' \
    -o /tmp/kit.enc 'https://garage/${BUCKET}/archiver-recovery-kit-${HOSTN}.tar.enc'
  curl -fsS --aws-sigv4 'aws:amz:${REGION}:s3' --user '${S3_KEY}:${S3_SECRET}' \
    -o /tmp/kit.readme 'https://garage/${BUCKET}/archiver-recovery-kit-${HOSTN}.README.txt'
  grep -q 'openssl enc -d' /tmp/kit.readme
  openssl enc -d -aes-256-cbc -pbkdf2 -pass pass:recovery-s3-password -in /tmp/kit.enc | tar -xf - -C /tmp
  grep -q '^STORAGE_TARGET_1_S3_BUCKETNAME=${BUCKET}\$' /tmp/archiver.env
  [ \"\$(cat /tmp/secrets/storage_target_1_s3_secret)\" = '${S3_SECRET}' ]
" || die "recovery kit missing from the bucket or not decryptable"

log "restore from Garage"
docker exec -e SNAPSHOT_ID="${HOSTN}-fixtures" -e LOCAL_DIR=/data/restore \
  -e OVERWRITE=1 -e HASH_COMPARE=1 "$ARCH" archiver auto-restore || die "s3 auto-restore exited non-zero"

docker exec "$ARCH" bash -c '
  set -e
  diff /data/fixtures/file.txt /data/restore/file.txt
  cmp /data/fixtures/blob.bin /data/restore/blob.bin
' || die "restored content differs from source"

echo "=== S3-GARAGE OK: backup + restore over the s3 wire (TLS) ==="
