#!/usr/bin/env bash
# Per-service extension points (docs/examples contract): service-backup-settings.sh must be
# sourced per service, its DUPLICACY_FILTERS_PATTERNS must actually shape the snapshot
# (excluded files absent from a restore), its pre/post hooks must run in order around the
# backup, and hook state must not leak into the NEXT service (which uses the defaults).
#
#   docker run -i --rm --hostname ss-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/service-settings.sh

set -uo pipefail

SERVICES=/data/services
RESTORE=/data/restore
STORE=/backup-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
HOOKLOG=/tmp/hook-order

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

log "materialize RSA keypair + file secrets (env-native mode)"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE" "$RESTORE"
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null \
  || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null \
  || die "openssl rsa -pubout"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${RSA_PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"

log "svc-a: custom settings (filters + hooks); svc-b: no settings file (defaults)"
mkdir -p "$SERVICES/svc-a" "$SERVICES/svc-b"
echo "keep this" >"$SERVICES/svc-a/keep.txt"
echo "never back this up" >"$SERVICES/svc-a/exclude.me"
echo "service b data" >"$SERVICES/svc-b/b.txt"

cat >"$SERVICES/svc-a/service-backup-settings.sh" <<EOF
DUPLICACY_FILTERS_PATTERNS=("-exclude.me" "+*")
service_specific_pre_backup_function() { echo "pre-\${SERVICE}" >> "${HOOKLOG}"; }
service_specific_post_backup_function() { echo "post-\${SERVICE}" >> "${HOOKLOG}"; }
EOF

export SERVICE_DIRECTORIES="${SERVICES}/*/"
export STORAGE_TARGET_1_NAME="local"
export STORAGE_TARGET_1_TYPE="local"
export STORAGE_TARGET_1_LOCAL_PATH="${STORE}"
export ROTATE_BACKUPS="false"

log "backup both services"
archiver backup retain || die "backup exited non-zero"

log "hooks ran in order for svc-a only (no leak into svc-b)"
[ -f "$HOOKLOG" ] || die "hooks never ran"
printf 'pre-svc-a\npost-svc-a\n' | diff - "$HOOKLOG" \
  || die "hook order/leak mismatch: $(tr '\n' ' ' <"$HOOKLOG")"

log "restore svc-a: the filtered file must be absent"
SNAPSHOT_ID="$(hostname)-svc-a" LOCAL_DIR="$RESTORE" OVERWRITE=1 archiver auto-restore \
  || die "auto-restore exited non-zero"
[ -f "$RESTORE/keep.txt" ] || die "kept file missing from restore"
[ -e "$RESTORE/exclude.me" ] && die "excluded file WAS backed up (filters not applied)"
grep -q "keep this" "$RESTORE/keep.txt" || die "kept file content mismatch"

echo "=== SERVICE-SETTINGS OK: filters exclude, hooks ordered, defaults isolated ==="
