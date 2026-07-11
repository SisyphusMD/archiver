#!/usr/bin/env bash
# Entrypoint secret-handling contract, on the REAL entrypoint: (A) a mounted bundle with no
# resolvable password fails fast naming the password path (never a misleading "no bundle"
# error); (B) an explicitly set BUNDLE_PASSWORD_FILE pointing nowhere is a hard error;
# (C) bundle + RSA keys but no password must NOT silently start env-native (a container that
# never backs up); (D) after a successful bundle import, no process in the container carries
# the bundle password in its environment (/proc leak hygiene).
#
# HOST-DRIVEN: run on the docker host (not via --entrypoint bash).
#
#   IMAGE=archiver:dev bash tests/integration/entrypoint-secrets.sh

set -uo pipefail

IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"
NAME="archiver-es-$$"
BUNVOL="archiver-es-bundle-$$"
SECVOL="archiver-es-secrets-$$"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker volume rm "$BUNVOL" "$SECVOL" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker volume create "$BUNVOL" >/dev/null
docker volume create "$SECVOL" >/dev/null

log "generate a REAL bundle via scripted init into the bundle volume"
# init writes to the neutral SETUP_DIR; runtime bundle mode mounts the same volume at
# /opt/archiver/bundle (the bundle sits at the volume root either way).
docker run --rm -v "$BUNVOL":/opt/archiver/setup --entrypoint bash "$IMAGE" -c '
  printf "/data/fixtures/\nlocal\nlocal\n/backup-store\nnntestbundlepw\ntestbundlepw\n" \
    | bash /opt/archiver/lib/scripts/init.sh >/dev/null 2>&1
  test -f /opt/archiver/setup/bundle.tar.enc
' || die "bundle generation via init failed"

log "(A) bundle mounted, no password anywhere: fail fast, name the password path"
set +e
OUT=$(docker run --rm --network none -v "$BUNVOL":/opt/archiver/bundle "$IMAGE" 2>&1); RC=$?
set -e
[ "$RC" -ne 0 ] || die "(A) container exited 0 with an unreadable bundle"
echo "$OUT" | grep -q "bundle found at .* but no bundle password" || { echo "$OUT" | tail -4; die "(A) wrong error message"; }
echo "$OUT" | grep -q "no bundle and no RSA key files" && die "(A) claimed no bundle exists while one is mounted"

log "(B) BUNDLE_PASSWORD_FILE set but missing: hard error, not a silent fallback"
set +e
OUT=$(docker run --rm --network none -e BUNDLE_PASSWORD_FILE=/nonexistent "$IMAGE" 2>&1); RC=$?
set -e
[ "$RC" -ne 0 ] || die "(B) container exited 0 with a dangling BUNDLE_PASSWORD_FILE"
echo "$OUT" | grep -q "BUNDLE_PASSWORD_FILE is set to" || { echo "$OUT" | tail -4; die "(B) wrong error message"; }

log "(C) bundle + RSA key secrets mounted, no password: must NOT silently start env-native"
docker run --rm -v "$SECVOL":/run/secrets --entrypoint bash "$IMAGE" -c '
  set -e
  openssl genrsa -aes256 -passout pass:tp -out /run/secrets/rsa_private_key -traditional 2048 2>/dev/null
  openssl rsa -in /run/secrets/rsa_private_key -passin pass:tp -pubout -out /run/secrets/rsa_public_key 2>/dev/null
  printf "testpassword" > /run/secrets/storage_password
  printf "tp" > /run/secrets/rsa_passphrase
' || die "(C) secret volume population failed"
set +e
OUT=$(docker run --rm --network none -v "$BUNVOL":/opt/archiver/bundle -v "$SECVOL":/run/secrets "$IMAGE" 2>&1); RC=$?
set -e
[ "$RC" -ne 0 ] || die "(C) container started env-native while a bundle sat unreadable"
echo "$OUT" | grep -q "bundle found at .* but no bundle password" || { echo "$OUT" | tail -4; die "(C) wrong error message"; }

log "(D) with the password provided, import succeeds and NO process keeps the password in its environment"
docker run --rm -v "$SECVOL":/run/secrets --entrypoint bash "$IMAGE" -c 'printf "testbundlepw" > /run/secrets/bundle_password' \
  || die "(D) password file write failed"
docker run -d --name "$NAME" --cap-drop ALL --cap-add DAC_OVERRIDE \
  -v "$BUNVOL":/opt/archiver/bundle -v "$SECVOL":/run/secrets "$IMAGE" >/dev/null || die "(D) container failed to start"
# Generous window: under a loaded docker host (CI runners, full local gates) container
# startup can take well over the happy-path second or two.
for _ in $(seq 1 300); do
  docker logs "$NAME" 2>&1 | grep -q "Configuration imported successfully" && break
  sleep 0.2
done
docker logs "$NAME" 2>&1 | grep -q "Configuration imported successfully" || {
  echo "--- (D) diagnostics ---"
  docker inspect "$NAME" --format 'state={{.State.Status}} exit={{.State.ExitCode}} oom={{.State.OOMKilled}} started={{.State.StartedAt}}'
  docker logs "$NAME" 2>&1 | tail -20
  die "(D) bundle import did not succeed"
}

LEAKS=$(docker exec "$NAME" bash -c '
  found=0
  for p in /proc/[0-9]*/environ; do
    if tr "\0" "\n" < "$p" 2>/dev/null | grep -qE "^(BUNDLE_PASSWORD|ARCHIVER_BUNDLE_PASSWORD)="; then
      echo "leak in $p"; found=1
    fi
  done
  exit $found
') || die "(D) bundle password present in a process environment: $LEAKS"

echo "=== ENTRYPOINT-SECRETS OK: precise errors (A,B,C); no password in any environ after import (D) ==="
