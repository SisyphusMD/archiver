#!/usr/bin/env bash
# Automatic recovery kit, end to end on a local storage target (env-native, no bundle).
# Proves the full contract: no recovery secret -> clean skip; with the secret -> the backup
# drops an encrypted kit + README beside the duplicacy data; the kit decrypts with ONLY the
# recovery password into the exact effective config (env + every secret + keys, including
# the recovery password itself), recreation notes, and a verbatim copy of anything mounted
# at /opt/archiver/deployment; unchanged content -> the kit is NOT rewritten; changed config
# or manifest -> it is; `recovery-kit force` re-pushes; prune (-all -exhaustive) leaves the
# kit alone; and a recovery password equal to the storage password is refused.
#
#   docker run -i --rm --hostname rk-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/recovery-kit.sh

set -uo pipefail

FIXTURES=/data/fixtures
STORE=/backup-store
SECRETS_DIR=/run/secrets
DEPLOY_DIR=/opt/archiver/deployment
RSA_PASSPHRASE=testpassphrase
KIT_PW=recovery-test-password
KIT="$STORE/archiver-recovery-kit-$(hostname).tar.enc"
README="$STORE/archiver-recovery-kit-$(hostname).README.txt"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

log "env-native setup: keys + secrets + env config, one local target, a mounted manifest"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE" "$FIXTURES" "$DEPLOY_DIR"
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null \
  || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null \
  || die "openssl rsa -pubout"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${RSA_PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"
chmod 600 "${SECRETS_DIR}/storage_password" "${SECRETS_DIR}/rsa_passphrase"
export SERVICE_DIRECTORIES="${FIXTURES}/"
export STORAGE_TARGET_1_NAME="local"
export STORAGE_TARGET_1_TYPE="local"
export STORAGE_TARGET_1_LOCAL_PATH="${STORE}"
export DUPLICACY_THREADS="4"
echo "recovery-kit test content" >"$FIXTURES/file.txt"
# Kubernetes-ConfigMap-style layout: the visible entry is a symlink into a hidden ..data
# dir. The kit must capture the dereferenced file and must NOT drag ..data along.
mkdir -p "$DEPLOY_DIR/..data"
printf 'services:\n  archiver:\n    image: archiver:test\n' >"$DEPLOY_DIR/..data/compose.yaml"
ln -s ..data/compose.yaml "$DEPLOY_DIR/compose.yaml"

log "backup WITHOUT a recovery secret: clean skip, no kit"
archiver backup retain || die "backup (no kit) exited non-zero"
grep -rq "Recovery kit not configured" /opt/archiver/logs/ || die "no 'Recovery kit not configured' skip message"
[ ! -e "$KIT" ] || die "kit appeared without a recovery secret"

log "provide the recovery secret; backup must produce kit + README on the storage"
printf '%s' "${KIT_PW}" >"${SECRETS_DIR}/recovery_password"
chmod 600 "${SECRETS_DIR}/recovery_password"
archiver backup retain || die "backup (kit) exited non-zero"
[ -f "$KIT" ] || die "no recovery kit at storage root"
[ -f "$README" ] || die "no kit README at storage root"
grep -q "openssl enc -d -aes-256-cbc -pbkdf2" "$README" || die "README lacks the decrypt command"

log "the kit must decrypt with ONLY the recovery password into config + notes + manifest"
mkdir -p /tmp/kit
openssl enc -d -aes-256-cbc -pbkdf2 -pass "pass:${KIT_PW}" -in "$KIT" | tar -xf - -C /tmp/kit \
  || die "kit does not decrypt with the recovery password"
grep -q "^STORAGE_TARGET_1_LOCAL_PATH=${STORE}$" /tmp/kit/archiver.env || die "archiver.env missing the local path"
grep -q "^SERVICE_DIRECTORIES=${FIXTURES}/$" /tmp/kit/archiver.env || die "archiver.env missing SERVICE_DIRECTORIES"
grep -q "^DUPLICACY_THREADS=4$" /tmp/kit/archiver.env || die "archiver.env missing DUPLICACY_THREADS"
[ "$(cat /tmp/kit/secrets/storage_password)" = "testpassword" ] || die "kit storage_password wrong"
[ "$(cat /tmp/kit/secrets/recovery_password)" = "${KIT_PW}" ] || die "recovery password not self-contained in the kit"
cmp -s /tmp/kit/secrets/rsa_private_key /opt/archiver/keys/private.pem || die "kit RSA private key differs"
cmp -s /tmp/kit/secrets/rsa_public_key /opt/archiver/keys/public.pem || die "kit RSA public key differs"
cmp -s /tmp/kit/deployment/compose.yaml "$DEPLOY_DIR/compose.yaml" || die "mounted manifest not captured verbatim"
[ -f /tmp/kit/deployment/compose.yaml ] && [ ! -L /tmp/kit/deployment/compose.yaml ] || die "manifest captured as a symlink, not content"
[ ! -e "/tmp/kit/deployment/..data" ] || die "ConfigMap-style hidden ..data dir leaked into the kit"
grep -q "hostname: $(hostname)" /tmp/kit/RECREATE.txt || die "RECREATE.txt lacks the hostname"
grep -q "${FIXTURES}/" /tmp/kit/RECREATE.txt || die "RECREATE.txt lacks the service-dir mount fact"
grep -q "${STORE}" /tmp/kit/RECREATE.txt || die "RECREATE.txt lacks the local storage mount fact"

log "unchanged content: the next backup must NOT rewrite the kit"
sum1="$(sha256sum "$KIT" | cut -d' ' -f1)"
archiver backup retain || die "backup (unchanged) exited non-zero"
grep -rq "Recovery kit is up to date" /opt/archiver/logs/ || die "no up-to-date skip message"
sum2="$(sha256sum "$KIT" | cut -d' ' -f1)"
[ "$sum1" = "$sum2" ] || die "kit rewritten although nothing changed"

log "changed config (DUPLICACY_THREADS 4->5): the kit must update"
export DUPLICACY_THREADS="5"
archiver backup retain || die "backup (changed) exited non-zero"
sum3="$(sha256sum "$KIT" | cut -d' ' -f1)"
[ "$sum3" != "$sum2" ] || die "kit NOT rewritten after a config change"
openssl enc -d -aes-256-cbc -pbkdf2 -pass "pass:${KIT_PW}" -in "$KIT" | tar -xOf - archiver.env \
  | grep -q "^DUPLICACY_THREADS=5$" || die "updated kit does not carry the new value"

log "changed MANIFEST: the kit must update too"
printf '# revised\nservices:\n  archiver:\n    image: archiver:test2\n' >"$DEPLOY_DIR/compose.yaml"
archiver recovery-kit || die "manual recovery-kit (manifest change) exited non-zero"
sum4="$(sha256sum "$KIT" | cut -d' ' -f1)"
[ "$sum4" != "$sum3" ] || die "kit NOT rewritten after a manifest change"
openssl enc -d -aes-256-cbc -pbkdf2 -pass "pass:${KIT_PW}" -in "$KIT" | tar -xOf - deployment/compose.yaml \
  | grep -q "archiver:test2" || die "updated kit does not carry the new manifest"

log "manual 'archiver recovery-kit' with no change: up to date; 'force' re-pushes"
archiver recovery-kit || die "manual recovery-kit exited non-zero"
sum5="$(sha256sum "$KIT" | cut -d' ' -f1)"
[ "$sum5" = "$sum4" ] || die "manual recovery-kit rewrote an up-to-date kit"
archiver recovery-kit force || die "recovery-kit force exited non-zero"
sum6="$(sha256sum "$KIT" | cut -d' ' -f1)"
[ "$sum6" != "$sum5" ] || die "recovery-kit force did not re-push"

log "prune (-all -exhaustive) must leave the kit files alone"
echo "more content" >>"$FIXTURES/file.txt"
archiver backup prune || die "backup prune exited non-zero"
[ -f "$KIT" ] || die "prune removed the recovery kit"
[ -f "$README" ] || die "prune removed the kit README"

log "multi-target: fan-out with a partial failure leaves only that target retryable"
export STORAGE_TARGET_2_NAME="second"
export STORAGE_TARGET_2_TYPE="local"
export STORAGE_TARGET_2_LOCAL_PATH="/backup-store2"    # deliberately missing
KIT2="/backup-store2/archiver-recovery-kit-$(hostname).tar.enc"
if archiver recovery-kit force >/tmp/kit-partial.out 2>&1; then
  die "recovery-kit force exited zero although one target could not be written"
fi
[ -f "$KIT" ] || die "healthy target not updated during the partial failure"
[ ! -e "$KIT2" ] || die "kit appeared on a target whose path does not exist"
sumA="$(sha256sum "$KIT" | cut -d' ' -f1)"
mkdir -p /backup-store2
archiver recovery-kit || die "retry run exited non-zero"
[ -f "$KIT2" ] || die "failed target not retried on the next run"
sumB="$(sha256sum "$KIT" | cut -d' ' -f1)"
[ "$sumB" = "$sumA" ] || die "already-current target was re-uploaded during the retry"

log "recovery password equal to the storage password must be refused"
printf 'testpassword' >"${SECRETS_DIR}/recovery_password"
if archiver recovery-kit force >/tmp/kit-same.out 2>&1; then
  die "recovery-kit accepted a password equal to STORAGE_PASSWORD"
fi
grep -rq "must differ from STORAGE_PASSWORD" /opt/archiver/logs/ /tmp/kit-same.out || die "no 'must differ' message"

echo "=== RECOVERY-KIT OK: skip/produce/decrypt/self-contained/configmap-manifest+notes/no-rewrite/update/force/prune-safe/multi-target-retry/same-password-refused ==="
