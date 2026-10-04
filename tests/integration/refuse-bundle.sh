#!/usr/bin/env bash
# v1 reads no bundles (ADRs 4, 22): anything bundle-era refuses to start, naming what it found
# and the one-off 0.11 conversion, instead of starting a container that backs up nothing or
# half a configuration. A clean env-native deployment still starts.
#
# HOST-DRIVEN: run on the docker host (not via --entrypoint bash). Named volumes, not bind
# mounts: a CI runner's workspace bind mounts arrive empty.
#
#   IMAGE=archiver:dev bash tests/integration/refuse-bundle.sh

set -uo pipefail

IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"
BUNVOL="archiver-rb-bundle-$$"
SECVOL="archiver-rb-secrets-$$"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
cleanup() { docker volume rm "$BUNVOL" "$SECVOL" >/dev/null 2>&1 || true; }
trap cleanup EXIT
docker volume create "$BUNVOL" >/dev/null
docker volume create "$SECVOL" >/dev/null

log "an env-native deployment's secrets (RSA keypair, storage password)"
docker run --rm -v "$SECVOL":/run/secrets --entrypoint bash "$IMAGE" -c '
  set -e
  openssl genrsa -aes256 -passout pass:tp -out /run/secrets/rsa_private_key -traditional 2048 2>/dev/null
  openssl rsa -in /run/secrets/rsa_private_key -passin pass:tp -pubout -out /run/secrets/rsa_public_key 2>/dev/null
  printf "testpassword" >/run/secrets/storage_password
  printf "tp" >/run/secrets/rsa_passphrase
' || die "writing the secrets failed"
docker run --rm -v "$BUNVOL":/b --entrypoint sh "$IMAGE" -c 'head -c 64 /dev/urandom >/b/bundle.tar.enc' || die "writing the bundle failed"

# refused NAME FOUND [docker run args...]: the container exits non-zero, names FOUND, and
# prints the conversion command.
refused() {
  local name="$1" found="$2" out rc
  shift 2
  out="$(docker run --rm --network none "$@" 2>&1)"; rc=$?
  [ "$rc" -ne 0 ] || { echo "$out" | tail -5; die "$name: started (exit 0)"; }
  grep -q "found ${found}" <<<"$out" || { echo "$out" | tail -8; die "$name: does not name ${found}"; }
  grep -q "no longer reads bundles" <<<"$out" || die "$name: no explanation"
  grep -q "ghcr.io/sisyphusmd/archiver:0.11 run migrate" <<<"$out" || die "$name: no conversion command"
}

log "a mounted bundle refuses"
refused "bundle" "/opt/archiver/bundle/bundle.tar.enc" -v "$BUNVOL":/opt/archiver/bundle -v "$SECVOL":/run/secrets "$IMAGE"

log "a bundle_password secret refuses"
docker run --rm -v "$SECVOL":/run/secrets --entrypoint sh "$IMAGE" -c 'printf pw >/run/secrets/bundle_password'
refused "bundle_password" "/run/secrets/bundle_password" -v "$SECVOL":/run/secrets "$IMAGE"
docker run --rm -v "$SECVOL":/run/secrets --entrypoint rm "$IMAGE" /run/secrets/bundle_password

log "BUNDLE_PASSWORD in the environment refuses"
refused "BUNDLE_PASSWORD" "BUNDLE_PASSWORD in the environment" -e BUNDLE_PASSWORD=pw -v "$SECVOL":/run/secrets "$IMAGE"

log "a config.sh refuses"
refused "config.sh" "/opt/archiver/config.sh" -v "$SECVOL":/run/secrets --entrypoint bash "$IMAGE" \
  -c 'touch /opt/archiver/config.sh && exec /usr/local/bin/docker-entrypoint.sh'

log "run mode refuses too"
refused "run backup" "/opt/archiver/bundle/bundle.tar.enc" -v "$BUNVOL":/opt/archiver/bundle -v "$SECVOL":/run/secrets "$IMAGE" run backup

log "the commands that handled bundles point to the conversion"
out="$(docker run --rm --entrypoint archiver "$IMAGE" bundle export 2>&1)" && die "bundle export exited 0"
grep -q "ghcr.io/sisyphusmd/archiver:0.11 run migrate" <<<"$out" || die "bundle export: no conversion command"
out="$(docker run --rm --entrypoint archiver "$IMAGE" migrate 2>&1)" && die "migrate exited 0"
grep -q "ghcr.io/sisyphusmd/archiver:0.11 run migrate" <<<"$out" || die "migrate: no conversion command"

log "an env-native deployment starts"
out="$(docker run --rm --network none -v "$SECVOL":/run/secrets -e SERVICE_DIRECTORIES=/data/ \
  -e STORAGE_TARGET_1_NAME=local -e STORAGE_TARGET_1_TYPE=local -e STORAGE_TARGET_1_LOCAL_PATH=/store \
  "$IMAGE" run healthcheck 2>&1)"
grep -q "no longer reads bundles" <<<"$out" && die "an env-native deployment was refused"
grep -q "Configuration: keys loaded from files" <<<"$out" || { echo "$out" | tail -5; die "an env-native deployment did not start"; }

echo "=== REFUSE-BUNDLE OK: every bundle-era input refuses with the 0.11 conversion; env-native starts ==="
