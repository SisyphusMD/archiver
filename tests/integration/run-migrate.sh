#!/usr/bin/env bash
# `run migrate` converts a bundle in one `docker run`, exactly as the README documents it (the
# bundle and its password mounted read-only, output to a mounted directory), with no running
# container to exec into: the path v1 points bundle deployments to (ADR 22). Covers a bundle
# written with the password on openssl's -k flag, as releases before 0.9 wrote them, and a
# restarted container re-importing a replaced bundle rather than keeping its first boot's.
#
# HOST-DRIVEN: run on the docker host (not via --entrypoint bash). Named volumes, not bind
# mounts: a CI runner's workspace bind mounts arrive empty.
#
#   IMAGE=archiver:dev bash tests/integration/run-migrate.sh

set -uo pipefail

IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"
NAME="archiver-rm-$$"
BUNVOL="archiver-rm-bundle-$$"
SECVOL="archiver-rm-secrets-$$"
OUTVOL="archiver-rm-out-$$"
RSA_PW=rsapass123
BUNDLE_PW=bundlepass123

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker volume rm "$BUNVOL" "$SECVOL" "$OUTVOL" >/dev/null 2>&1 || true
}
trap cleanup EXIT
for v in "$BUNVOL" "$SECVOL" "$OUTVOL"; do docker volume create "$v" >/dev/null; done

# write_bundle KEEP [PASSWORD_FILE_CONTENT]: a bundle whose config sets PRUNE_KEEP to
# "-keep 0:KEEP", encrypted with the password on openssl's -k flag.
write_bundle() {
  docker run -i --rm -v "$BUNVOL":/b -v "$SECVOL":/s --entrypoint bash "$IMAGE" -s "$1" "${2:-$BUNDLE_PW}" <<EOF || die "writing the bundle failed"
set -e
mkdir -p /stage/keys
if [ -f /b/private.pem ]; then
  cp /b/private.pem /b/public.pem /stage/keys/
else
  openssl genrsa -aes256 -passout pass:$RSA_PW -out /stage/keys/private.pem -traditional 2048 2>/dev/null
  openssl rsa -in /stage/keys/private.pem -passin pass:$RSA_PW -pubout -out /stage/keys/public.pem 2>/dev/null
  cp /stage/keys/private.pem /stage/keys/public.pem /b/
fi
cat >/stage/config.sh <<CFG
SERVICE_DIRECTORIES=("/data/services/*/")
STORAGE_TARGET_1_NAME="local"
STORAGE_TARGET_1_TYPE="local"
STORAGE_TARGET_1_LOCAL_PATH="/backup-store"
STORAGE_PASSWORD="storagepassword1"
RSA_PASSPHRASE="$RSA_PW"
PRUNE_KEEP="-keep 0:\$1"
CFG
tar -cf /tmp/bundle.tar -C /stage keys config.sh
openssl enc -aes-256-cbc -pbkdf2 -salt -in /tmp/bundle.tar -out /b/bundle.tar.enc -k $BUNDLE_PW
printf '%s' "\$2" >/s/bundle_password
EOF
}

out() { docker run --rm -v "$OUTVOL":/o --entrypoint cat "$IMAGE" "/o/$1" 2>/dev/null; }
clear_out() { docker run --rm -v "$OUTVOL":/o --entrypoint sh "$IMAGE" -c 'rm -rf /o/*' >/dev/null 2>&1; }

log "a bundle as old releases wrote it, mounted read-only, converts in one docker run"
write_bundle 90
docker run --name "$NAME" --network none \
  -v "$BUNVOL":/opt/archiver/bundle:ro \
  -v "$SECVOL":/run/secrets:ro \
  -v "$OUTVOL":/opt/archiver/migrate \
  "$IMAGE" run migrate >/tmp/rm.out 2>&1 || { tail -15 /tmp/rm.out; die "run migrate failed"; }
out archiver.env | grep -q '^STORAGE_TARGET_1_LOCAL_PATH=/backup-store$' || die "archiver.env lacks the storage settings"
out archiver.env | grep -q '^PRUNE_KEEP=-keep 0:90$' || die "archiver.env lacks PRUNE_KEEP"
[ "$(out secrets/storage_password)" = "storagepassword1" ] || die "storage_password not written"
[ "$(out secrets/rsa_passphrase)" = "$RSA_PW" ] || die "rsa_passphrase not written"
out secrets/rsa_private_key | grep -q 'BEGIN RSA PRIVATE KEY' || die "rsa_private_key not written"
out archiver.env | grep -qi 'password' && die "a secret leaked into archiver.env"

log "restarted with a replaced bundle, the same container uses the new one"
clear_out
write_bundle 60
docker start -a "$NAME" >/tmp/rm.out 2>&1 || { tail -15 /tmp/rm.out; die "run migrate failed on restart"; }
out archiver.env | grep -q '^PRUNE_KEEP=-keep 0:60$' || die "the restart kept its first boot's config: $(out archiver.env | grep PRUNE_KEEP)"
docker rm -f "$NAME" >/dev/null

log "a wrong password fails, with no output written"
clear_out
write_bundle 60 wrongpassword
docker run --rm --network none \
  -v "$BUNVOL":/opt/archiver/bundle:ro \
  -v "$SECVOL":/run/secrets:ro \
  -v "$OUTVOL":/opt/archiver/migrate \
  "$IMAGE" run migrate >/tmp/rm.out 2>&1 && die "run migrate succeeded with the wrong password"
out archiver.env >/dev/null && die "wrote output despite the wrong password"

echo "=== RUN-MIGRATE OK: the documented docker run converts a read-only bundle (legacy -k included); a restart uses a replaced bundle ==="
