#!/usr/bin/env bash
# The post-backup hook undoes what the pre-backup hook did (restarts a stopped database,
# drops a snapshot), so it must run whenever the pre-backup hook ran: after a failed backup
# and after a stop that lands mid-backup, not only after a clean one. And once a stop is
# requested, no further service's pre-backup hook may start.
#
#   docker run -i --rm --hostname hp-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/hooks-always-post.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
LOCKFILE=/var/lock/archiver-main.lock
HOOKLOG=/tmp/hooks.log
MODE=/tmp/duplicacy-mode
MARKER=/tmp/backup-started

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

log "materialize RSA keypair + file secrets (env-native mode)"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE"
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null \
  || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null \
  || die "openssl rsa -pubout"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${RSA_PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"

log "two services whose hooks record themselves"
for s in svc-a svc-b; do
  mkdir -p "$SERVICES/$s"
  echo "$s data" >"$SERVICES/$s/file.txt"
  cat >"$SERVICES/$s/service-backup-settings.sh" <<EOF
service_specific_pre_backup_function() { echo "$s pre" >> $HOOKLOG; }
service_specific_post_backup_function() { echo "$s post" >> $HOOKLOG; }
EOF
done

export SERVICE_DIRECTORIES="${SERVICES}/*/"
export STORAGE_TARGET_1_NAME="local"
export STORAGE_TARGET_1_TYPE="local"
export STORAGE_TARGET_1_LOCAL_PATH="${STORE}"

log "shadow duplicacy: svc-a's 'backup' fails or blocks per $MODE; everything else is real"
REAL="$(command -v duplicacy)" || die "duplicacy not on PATH"
mv "$REAL" "${REAL}.real"
cat >"$REAL" <<WRAP
#!/usr/bin/env bash
if [ "\${1:-}" = "backup" ] && [[ "\$PWD" == */svc-a ]]; then
  case "\$(cat $MODE 2>/dev/null)" in
    fail) echo "SIMULATED: chunk upload failed" >&2; exit 1 ;;
    block) touch "$MARKER"; sleep 30; exit 0 ;;
  esac
fi
exec "\$0.real" "\$@"
WRAP
chmod +x "$REAL"

log "scenario 1: svc-a's backup fails -> its post hook still runs, svc-b still backs up"
echo fail >"$MODE"
rm -f "$HOOKLOG"
archiver backup >/tmp/run1.out 2>&1
rc=$?
[ "$rc" -ne 0 ] || die "backup exited 0 despite svc-a failing"
echo "--- hooks ---"; cat "$HOOKLOG"
[ "$(cat "$HOOKLOG")" = "$(printf 'svc-a pre\nsvc-a post\nsvc-b pre\nsvc-b post')" ] \
  || die "hooks after a failed backup: expected svc-a pre/post then svc-b pre/post"

log "scenario 2: a stop during svc-a's backup -> svc-a's post hook runs, svc-b's hooks never start"
echo block >"$MODE"
rm -f "$HOOKLOG" "$MARKER"
archiver backup --detach >/dev/null || die "backup --detach failed"
for _ in $(seq 1 100); do [ -f "$MARKER" ] && break; sleep 0.2; done
[ -f "$MARKER" ] || die "svc-a's backup never started"
archiver stop >/dev/null || die "archiver stop failed"
for _ in $(seq 1 120); do [ ! -e "$LOCKFILE" ] && break; sleep 0.5; done
[ ! -e "$LOCKFILE" ] || die "lock not released after the stop"
echo "--- hooks ---"; cat "$HOOKLOG"
grep -rq "Backup stopped\." /opt/archiver/logs/ || die "no 'Backup stopped' record"
[ "$(cat "$HOOKLOG")" = "$(printf 'svc-a pre\nsvc-a post')" ] \
  || die "hooks after a stop: expected only svc-a pre then svc-a post"

echo "=== HOOKS-ALWAYS-POST OK: post hook runs after a failed or stopped backup; no new pre hook after a stop ==="
