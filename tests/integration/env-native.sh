#!/usr/bin/env bash
# Env-native backup -> restore round-trip. Like roundtrip.sh, but proves the Phase 3
# layered-config path: there is NO bundle and NO config.sh. Configuration comes entirely
# from environment variables, secrets from files under ${SECRETS_DIR}, and the RSA keypair
# from files. It must still preserve file CONTENT, MODE, and original UID/GID.
#
#   docker run --rm --hostname en-host \
#     --cap-drop ALL --cap-add DAC_OVERRIDE --cap-add CHOWN --cap-add FOWNER \
#     -v "$PWD/tests/integration:/tests:ro" --entrypoint bash archiver:dev /tests/env-native.sh
#
# The snapshot id is ${HOSTNAME}-${service_basename}; --hostname makes it deterministic.

set -uo pipefail

FIXTURES=/data/fixtures
RESTORE=/data/restore
STORE=/backup-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
SNAPSHOT_ID="$(hostname)-fixtures"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

log "materialize RSA keypair as files (no bundle, no config.sh)"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE" "$FIXTURES" "$RESTORE"
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null \
  || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null \
  || die "openssl rsa -pubout"
chmod 600 /opt/archiver/keys/private.pem
chmod 644 /opt/archiver/keys/public.pem

log "write file-based secrets under ${SECRETS_DIR}"
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${RSA_PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"
chmod 600 "${SECRETS_DIR}/storage_password" "${SECRETS_DIR}/rsa_passphrase"

log "provide non-secret config purely via environment (colon-delimited SERVICE_DIRECTORIES)"
export SERVICE_DIRECTORIES="${FIXTURES}/"
export STORAGE_TARGET_1_NAME="local"
export STORAGE_TARGET_1_TYPE="local"
export STORAGE_TARGET_1_LOCAL_PATH="${STORE}"
export ROTATE_BACKUPS="false"

log "build fixture tree with varied ownership + modes"
echo "root-owned content" >"$FIXTURES/root.txt"
mkdir -p "$FIXTURES/sub"
echo "uid1000 content" >"$FIXTURES/sub/u1000.txt"
head -c 4096 /dev/urandom >"$FIXTURES/sub/u5000.bin"
ln -s root.txt "$FIXTURES/link"
chmod 644 "$FIXTURES/root.txt"
chmod 640 "$FIXTURES/sub/u1000.txt"
chmod 600 "$FIXTURES/sub/u5000.bin"
chown 1000:1000 "$FIXTURES/sub/u1000.txt"
chown 5000:5000 "$FIXTURES/sub/u5000.bin"

FILES=(root.txt sub/u1000.txt sub/u5000.bin)
declare -A OWN MODE HASH
for f in "${FILES[@]}"; do
  OWN[$f]="$(stat -c '%u:%g' "$FIXTURES/$f")"
  MODE[$f]="$(stat -c '%a' "$FIXTURES/$f")"
  HASH[$f]="$(sha256sum "$FIXTURES/$f" | cut -d' ' -f1)"
done
log "originals: $(for f in "${FILES[@]}"; do printf '%s(%s,%s) ' "$f" "${OWN[$f]}" "${MODE[$f]}"; done)"

# Same false-green guard as roundtrip.sh: without CHOWN/FOWNER every file is root-owned
# and the test would pass trivially. Require the varied ownership to have stuck.
{ [ "${OWN[sub/u1000.txt]}" = "1000:1000" ] && [ "${OWN[sub/u5000.bin]}" = "5000:5000" ]; } \
  || die "fixtures lack expected ownership (run with --cap-add CHOWN --cap-add FOWNER): u1000=${OWN[sub/u1000.txt]} u5000=${OWN[sub/u5000.bin]}"

log "backup (blocking, no prune) -> ${STORE}"
archiver backup retain || die "backup exited non-zero"
[ -d "$STORE/snapshots" ] || die "no snapshots written to storage"

log "restore latest of ${SNAPSHOT_ID} -> ${RESTORE}"
SNAPSHOT_ID="$SNAPSHOT_ID" LOCAL_DIR="$RESTORE" OVERWRITE=1 HASH_COMPARE=1 archiver auto-restore \
  || die "auto-restore exited non-zero"

log "assert content + mode + ownership preserved"
rc=0
for f in "${FILES[@]}"; do
  [ -e "$RESTORE/$f" ] || { echo "FAIL missing: $f"; rc=1; continue; }
  ro="$(stat -c '%u:%g' "$RESTORE/$f")"
  rmo="$(stat -c '%a' "$RESTORE/$f")"
  rh="$(sha256sum "$RESTORE/$f" | cut -d' ' -f1)"
  [ "$rh" = "${HASH[$f]}" ] || { echo "FAIL content $f"; rc=1; }
  [ "$rmo" = "${MODE[$f]}" ] || { echo "FAIL mode $f: got $rmo want ${MODE[$f]}"; rc=1; }
  [ "$ro" = "${OWN[$f]}" ] || { echo "FAIL owner $f: got $ro want ${OWN[$f]}"; rc=1; }
  [ "$rc" = 0 ] && echo "ok $f content+mode($rmo)+owner($ro)"
done
[ -L "$RESTORE/link" ] || { echo "FAIL symlink not restored as a symlink"; rc=1; }
[ "$rc" = 0 ] && echo "ok link is a symlink"

if [ "$rc" = 0 ]; then
  echo "=== ENV-NATIVE ROUND-TRIP OK: config from env, secrets+keys from files, no bundle ==="
else
  echo "=== ENV-NATIVE ROUND-TRIP FAILED ==="
fi
exit "$rc"
