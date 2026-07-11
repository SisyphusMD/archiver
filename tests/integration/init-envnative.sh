#!/usr/bin/env bash
# `archiver init` is the front door: driven with scripted answers, it must produce BOTH the
# encrypted bundle and the env-native materials (archiver.env + secrets/, the primary
# deployment path) — and it must serialize the user's ANSWERS, not the container's inherited
# environment or mounted secrets ('docker compose run ... init' under an existing deployment
# must not fold the old config into the new artifacts). Then prove the materials are real:
# wipe config.sh and the keys, rebuild the runtime state purely from env-native/, and run a
# backup with no bundle involved.
#
#   docker run -i --rm --hostname in-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/init-envnative.sh

set -uo pipefail

FIXTURES=/data/fixtures
OTHER=/data/other
STORE=/backup-store
ENVNATIVE=/opt/archiver/setup/env-native
SECRETS_DIR=/run/secrets

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

mkdir -p "$FIXTURES" "$OTHER" "$STORE" "$SECRETS_DIR"
echo "init test content" >"$FIXTURES/file.txt"
echo "other service content" >"$OTHER/other.txt"

log "plant DECOYS: deployment-style env vars and a mounted secret that init must IGNORE"
export SERVICE_DIRECTORIES="/decoy/"
export STORAGE_TARGET_1_NAME="decoy"
export STORAGE_TARGET_1_TYPE="b2"
export STORAGE_TARGET_1_B2_BUCKETNAME="decoy-bucket"
printf 'decoy-secret' >"$SECRETS_DIR/storage_password"

log "drive init with scripted answers (TWO directories, local target, no pushover)"
# In production the ENTRYPOINT dispatches 'init' (docker run ... init); here the entrypoint
# is bypassed, so call the script directly. Answer order: Directories (comma-separated) /
# storage name / type / local path, then two single-char (y/N) reads with no trailing
# newline, then the bundle password twice.
printf '/data/fixtures/,/data/other/\nlocal\nlocal\n/backup-store\nnntestbundlepw\ntestbundlepw\n' \
  | bash /opt/archiver/lib/scripts/init.sh >/tmp/init.out 2>&1 || { tail -30 /tmp/init.out; die "init exited non-zero"; }

log "init must have produced the bundle AND the env-native materials"
[ -f /opt/archiver/setup/bundle.tar.enc ] || die "no bundle.tar.enc"
[ -f "$ENVNATIVE/archiver.env" ] || die "no env-native/archiver.env"
[ -d "$ENVNATIVE/secrets" ] || die "no env-native/secrets/"
grep -q "RECOMMENDED (env-native)" /tmp/init.out || die "init guidance does not lead with env-native"
grep -q "RECOVERY PASSWORD" /tmp/init.out || die "init did not display the recovery password callout"

log "the materials must reflect the ANSWERS, not the decoys, and keep BOTH directories"
grep -q "^STORAGE_TARGET_1_NAME=local$" "$ENVNATIVE/archiver.env" || die "archiver.env storage name is not the answer given"
grep -q "^SERVICE_DIRECTORIES=/data/fixtures/:/data/other/$" "$ENVNATIVE/archiver.env" \
  || die "archiver.env SERVICE_DIRECTORIES lost a directory or took the decoy: $(grep SERVICE_DIRECTORIES "$ENVNATIVE/archiver.env")"
grep -q "decoy" "$ENVNATIVE/archiver.env" && die "a decoy env value leaked into archiver.env"
grep -q "STORAGE_PASSWORD" "$ENVNATIVE/archiver.env" && die "a secret leaked into archiver.env"
[ "$(cat "$ENVNATIVE/secrets/storage_password")" = "decoy-secret" ] && die "the mounted decoy secret overrode the generated one"
for f in storage_password rsa_passphrase recovery_password rsa_private_key rsa_public_key ssh_private_key ssh_public_key; do
  [ -s "$ENVNATIVE/secrets/$f" ] || die "missing secret file: $f"
done
[ "$(cat "$ENVNATIVE/secrets/recovery_password")" != "$(cat "$ENVNATIVE/secrets/storage_password")" ] \
  || die "generated recovery password equals the storage password"

log "the bundle decrypts with the OLD-style openssl -k (compat) and records config.sh as 0600"
openssl enc -d -aes-256-cbc -pbkdf2 -in /opt/archiver/setup/bundle.tar.enc -out /tmp/bundle.tar -k testbundlepw \
  || die "bundle does not decrypt with legacy-style -k (password-derivation compat broken)"
tar -tvf /tmp/bundle.tar | grep "config.sh" | grep -q -- "^-rw-------" || die "config.sh not 0600 inside the bundle tar"
tar -xf /tmp/bundle.tar -C /tmp config.sh
grep -q "decoy" /tmp/config.sh && die "a decoy value leaked into the bundle's config.sh"
grep -q "^RECOVERY_PASSWORD=" /tmp/config.sh || die "bundle config.sh lacks the generated recovery password"
rm -f /tmp/bundle.tar /tmp/config.sh

log "wipe the in-container config state; rebuild purely from env-native/"
rm -f /opt/archiver/config.sh
rm -rf /opt/archiver/keys
mkdir -p /opt/archiver/keys
unset SERVICE_DIRECTORIES STORAGE_TARGET_1_NAME STORAGE_TARGET_1_TYPE STORAGE_TARGET_1_B2_BUCKETNAME
cp "$ENVNATIVE/secrets/storage_password" "$SECRETS_DIR/storage_password"
cp "$ENVNATIVE/secrets/rsa_passphrase"   "$SECRETS_DIR/rsa_passphrase"
cp "$ENVNATIVE/secrets/recovery_password" "$SECRETS_DIR/recovery_password"
# what the entrypoint's overlay_key_files does with the key secrets:
cp "$ENVNATIVE/secrets/rsa_private_key" /opt/archiver/keys/private.pem && chmod 600 /opt/archiver/keys/private.pem
cp "$ENVNATIVE/secrets/rsa_public_key"  /opt/archiver/keys/public.pem
# export the non-secret settings the way compose env_file would
while IFS= read -r line; do
  case "$line" in ''|'#'*) continue ;; esac
  export "${line%%=*}"="${line#*=}"
done <"$ENVNATIVE/archiver.env"

log "backup must run BOTH services from env-native materials alone (no bundle, no config.sh)"
archiver backup retain || die "env-native backup from init materials failed"
[ -d "$STORE/snapshots/$(hostname)-fixtures" ] || die "no snapshot for fixtures"
[ -d "$STORE/snapshots/$(hostname)-other" ] || die "no snapshot for other (multi-dir lost)"

log "recovery kit is on by default from init: the first backup must place a decryptable kit"
EKIT="$STORE/archiver-recovery-kit-$(hostname).tar.enc"
[ -f "$EKIT" ] || die "no recovery kit on the storage after the first backup"
openssl enc -d -aes-256-cbc -pbkdf2 -pass "pass:$(cat "$SECRETS_DIR/recovery_password")" -in "$EKIT" \
  | tar -xOf - archiver.env | grep -q "^STORAGE_TARGET_1_NAME=local$" \
  || die "recovery kit does not decrypt with the init-generated password"

echo "=== INIT-ENVNATIVE OK: decoys ignored, both dirs kept, bundle 0600 + -k compat, bundle-free backup, recovery kit on by default ==="
