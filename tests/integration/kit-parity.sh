#!/usr/bin/env bash
# The Go recovery kit is the bash kit: for the same configuration both record the same
# fingerprint (so switching re-uploads nothing) and their kits decrypt, with stock openssl, to
# identical files and modes. Deleted with the bash kit.
#
#   docker run -i --rm --hostname kp-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/kit-parity.sh

set -uo pipefail

SECRETS_DIR=/run/secrets
STATE=/opt/archiver/logs/.recovery-kit-state
KIT="archiver-recovery-kit-$(hostname).tar.enc"
LOG=/opt/archiver/logs/archiver.log

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; tail -20 "$LOG" >&2 2>/dev/null; exit 1; }
decrypt() { mkdir -p "$2" && openssl enc -d -aes-256-cbc -pbkdf2 -pass pass:recovery-pw-123 -in "$1" | tar -xf - -C "$2"; }

log "a configuration with every awkward part"
mkdir -p /opt/archiver/keys "$SECRETS_DIR" /store1 /store2 /data/a /data/b /opt/archiver/deployment/..data /srv/extras/sub /srv/other
openssl genrsa -aes256 -passout pass:rp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:rp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "pubout"
printf 'testpassword\n' >"$SECRETS_DIR/storage_password"
printf 'rp\r\n' >"$SECRETS_DIR/rsa_passphrase"
printf 'recovery-pw-123' >"$SECRETS_DIR/recovery_password"
: >"$SECRETS_DIR/pushover_user_key"
printf 'not-used' >"$SECRETS_DIR/storage_target_1_b2_id"   # target 1 is local: not loaded
echo "services: {}" >/opt/archiver/deployment/compose.yaml
echo hidden >/opt/archiver/deployment/..data/compose.yaml
ln -s compose.yaml /opt/archiver/deployment/linked.yaml
echo "# runbook" >"/srv/extras/DR Runbook.md"
printf '#!/bin/sh\necho restore\n' >/srv/extras/restore.sh && chmod 750 /srv/extras/restore.sh
echo nested >/srv/extras/sub/notes.txt
echo other >/srv/other/restore.sh
export SERVICE_DIRECTORIES=$'/data/a/\n/data/b/'
export ROTATE_BACKUPS=false TZ=America/Los_Angeles BACKUP_SCHEDULE="0 3 * * *"
export STORAGE_TARGET_1_NAME=store1 STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH=/store1
export STORAGE_TARGET_2_NAME=store-2 STORAGE_TARGET_2_TYPE=local STORAGE_TARGET_2_LOCAL_PATH=/store2
export STORAGE_TARGET_10_NAME=not-a-target STORAGE_TARGET_1_S3_REGION=
export RECOVERY_KIT_EXTRA_PATHS=$'/srv/extras/DR Runbook.md:/srv/extras/restore.sh\n/srv/extras/sub:/srv/other/restore.sh'
for s in /store1 /store2; do : >"$s/config"; chmod 640 "$s/config"; done

log "the bash kit"
bash -c 'source /opt/archiver/lib/core/common.sh
  source_if_not_sourced "${CONFIG_LOADER_CORE}"
  source_if_not_sourced "${RECOVERY_KIT_FEATURE}"
  STORAGE_TARGET_COUNT=2 run_recovery_kit' >/tmp/bash.out 2>&1 || { cat /tmp/bash.out; die "the bash kit failed"; }
bash_fp="$(head -1 "$STATE")"
decrypt "/store1/$KIT" /tmp/bash-kit || die "the bash kit does not decrypt"

log "the Go kit finds it current: the same fingerprint"
archiver recovery-kit >/tmp/go.out 2>&1 || { cat /tmp/go.out; die "the Go kit failed"; }
grep -q 'Recovery kit is up to date on all storage targets' "$LOG" || { cat "$STATE"; die "the Go kit did not find the bash kit current (fingerprints differ)"; }
[ "$(head -1 "$STATE")" = "$bash_fp" ] || die "the fingerprints differ"

log "forced, the Go kit decrypts to the same files and modes"
archiver recovery-kit force >/tmp/go.out 2>&1 || { cat /tmp/go.out; die "the forced Go kit failed"; }
decrypt "/store1/$KIT" /tmp/go-kit || die "the Go kit does not decrypt"
diff -r /tmp/bash-kit /tmp/go-kit || die "the kits' contents differ"
modes() { (cd "$1" && find . -printf '%y %m %p\n' | LC_ALL=C sort); }
diff <(modes /tmp/bash-kit) <(modes /tmp/go-kit) || die "the kits' modes differ"
[ "$(stat -c %a "/store2/$KIT")" = 640 ] || die "the kit does not carry the storage's mode"
[ -f "/tmp/go-kit/extra/restore.sh.2" ] && [ -d /tmp/go-kit/extra/sub ] || die "extras missing: $(ls /tmp/go-kit/extra)"
[ ! -e /tmp/go-kit/deployment/..data ] || die "a hidden ConfigMap entry is in the kit"

echo "=== KIT-PARITY OK: same fingerprint as the bash kit; identical decrypted contents and modes ==="
