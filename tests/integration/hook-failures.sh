#!/usr/bin/env bash
# Hook exit codes count. A failed pre-backup hook (a database dump that errored) leaves the
# service's files in an unknown state, so that service is not backed up: its newest
# revision stays the last good one instead of one auto-restore would pick with a broken
# dump inside. Its post-backup hook still runs (the pre hook may have stopped something
# before failing), every other service still backs up, and the run reports the error. A
# settings file that does not parse is the same failure: sourcing it would silently back
# up everything with no pre hook at all. A failed post-backup hook is reported too.
#
#   docker run -i --rm --hostname hf-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/hook-failures.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
HOOKLOG=/tmp/hooks.log

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
snapshot_exists() { [ -d "$STORE/snapshots/$(hostname)-$1" ]; }

log "materialize RSA keypair + file secrets (env-native mode)"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE"
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null \
  || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null \
  || die "openssl rsa -pubout"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${RSA_PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"

export STORAGE_TARGET_1_NAME="local"
export STORAGE_TARGET_1_TYPE="local"
export STORAGE_TARGET_1_LOCAL_PATH="${STORE}"
export SERVICE_DIRECTORIES="${SERVICES}/*/"

log "svc-a's pre hook fails, svc-b is healthy, svc-c's settings file does not parse"
mkdir -p "$SERVICES/svc-a" "$SERVICES/svc-b" "$SERVICES/svc-c"
for s in svc-a svc-b svc-c; do echo "$s data" >"$SERVICES/$s/file.txt"; done
cat >"$SERVICES/svc-a/service-backup-settings.sh" <<EOF
service_specific_pre_backup_function() { echo "svc-a pre" >> $HOOKLOG; return 3; }
service_specific_post_backup_function() { echo "svc-a post" >> $HOOKLOG; }
EOF
cat >"$SERVICES/svc-b/service-backup-settings.sh" <<EOF
service_specific_pre_backup_function() { echo "svc-b pre" >> $HOOKLOG; }
service_specific_post_backup_function() { echo "svc-b post" >> $HOOKLOG; }
EOF
cat >"$SERVICES/svc-c/service-backup-settings.sh" <<EOF
service_specific_pre_backup_function() { echo "svc-c pre" >> $HOOKLOG;
EOF

archiver backup >/tmp/run1.out 2>&1
rc=$?
echo "--- hooks ---"; cat "$HOOKLOG"
echo "--- errors ---"; grep -rh "\[ERROR\]" /opt/archiver/logs/ | sed 's/^\[[^]]*\] //'

[ "$rc" -ne 0 ] || die "run exited 0 despite a failed pre hook and a broken settings file"
snapshot_exists svc-b || die "the healthy service was not backed up"
snapshot_exists svc-a && die "svc-a was backed up although its pre hook failed"
snapshot_exists svc-c && die "svc-c was backed up although its settings file does not parse"
[ "$(cat "$HOOKLOG")" = "$(printf 'svc-a pre\nsvc-a post\nsvc-b pre\nsvc-b post')" ] \
  || die "hook order: expected svc-a pre/post (post despite the failure) then svc-b pre/post, and nothing from svc-c"
grep -rq "\[ERROR\].*svc-a.*[Pp]re-backup hook failed" /opt/archiver/logs/ || die "no ERROR naming svc-a's failed pre hook"
grep -rq "\[ERROR\].*svc-c.*service-backup-settings.sh" /opt/archiver/logs/ || die "no ERROR naming svc-c's broken settings file"
grep -rq "Completed successfully" /opt/archiver/logs/ && die "the run claims success"

log "a failed post hook is reported, but the backup it followed still counts"
rm -rf "${SERVICES:?}"/svc-* "$HOOKLOG" /opt/archiver/logs/*
mkdir -p "$SERVICES/svc-d"
echo "svc-d data" >"$SERVICES/svc-d/file.txt"
cat >"$SERVICES/svc-d/service-backup-settings.sh" <<'EOF'
service_specific_post_backup_function() { return 4; }
EOF
archiver backup >/tmp/run2.out 2>&1
rc=$?
[ "$rc" -ne 0 ] || die "run exited 0 despite a failed post hook"
snapshot_exists svc-d || die "svc-d was not backed up"
grep -rq "\[ERROR\].*svc-d.*[Pp]ost-backup hook failed" /opt/archiver/logs/ || die "no ERROR naming svc-d's failed post hook"

echo "=== HOOK-FAILURES OK: failed pre hook skips only its service (post still runs); broken settings and failed post hooks are errors ==="
