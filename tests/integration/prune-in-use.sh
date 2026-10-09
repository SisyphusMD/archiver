#!/usr/bin/env bash
# The local prune leaves out revisions still in use (ADR 19): a revision a restore is reading
# survives the prune that would delete it and goes at the next one; a reader that died holds
# nothing.
#
# Retention needs snapshots days old, which a test cannot make, so a duplicacy wrapper answers
# only the dry run (with /tmp/plan as the policy's choice) and passes every other command to
# the real binary: the deletions are real, and retention itself stays duplicacy's.
#
#   docker run -i --rm --hostname pu-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/prune-in-use.sh

set -uo pipefail

SVC=/data/services/app
STORE=/backup-store
SECRETS_DIR=/run/secrets
PASSPHRASE=testpassphrase
MLOG=/opt/archiver/logs/maintenance.log
HOST="$(hostname)"
ID="${HOST}-app"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; tail -25 "$MLOG" >&2 2>/dev/null; exit 1; }
present() { local r; for r in "$@"; do [ -e "$STORE/snapshots/$ID/$r" ] || return 1; done; }
absent() { local r; for r in "$@"; do [ ! -e "$STORE/snapshots/$ID/$r" ] || return 1; done; }
wait_for() { local _; for _ in $(seq 1 "$2"); do eval "$1" && return 0; sleep 1; done; return 1; }
maintain() { PATH="/tmp/fakebin:$PATH" archiver maintenance >/tmp/maint.out 2>&1; }

log "env-native config with a local primary and five revisions"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE" "$SVC" /tmp/fakebin
openssl genrsa -aes256 -passout "pass:${PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "pubout"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"
export SERVICE_DIRECTORIES="/data/services/*/"
export STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH="$STORE"
export CHECK_BACKUPS=false PRUNE_BACKUPS=true PRUNE_KEEP="-keep 0:30" PRUNE_EXHAUSTIVE_FREQUENCY=off
for i in 1 2 3 4 5; do
  sleep 1.1; echo "content $i $i" >"$SVC/file.txt"
  archiver backup >/dev/null 2>&1 || die "backup $i failed"
done
present 1 2 3 4 5 || die "expected revisions 1-5"

REAL="$(command -v duplicacy)"
cat >/tmp/fakebin/duplicacy <<EOF
#!/usr/bin/env bash
# A restore waits while /tmp/hold-restore exists, as a long restore would.
if [ "\${2:-}" = "restore" ] && [ -e /tmp/hold-restore ]; then
  touch /tmp/holding
  while [ -e /tmp/hold-restore ]; do sleep 0.2; done
fi
for a in "\$@"; do
  if [ "\$a" = "-dry-run" ]; then
    echo "Storage set to $STORE"
    for r in \$(cat /tmp/plan); do echo "Deleting snapshot $ID at revision \$r"; done
    exit 0
  fi
done
exec "$REAL" "\$@"
EOF
chmod +x /tmp/fakebin/duplicacy

log "a restore reading revision 2 keeps it through a prune that selects 1, 2 and 3"
touch /tmp/hold-restore
PATH="/tmp/fakebin:$PATH" SNAPSHOT_ID="$ID" LOCAL_DIR=/tmp/restore REVISION=2 archiver auto-restore >/tmp/restore.out 2>&1 &
RESTORE=$!
wait_for '[ -e /tmp/holding ]' 30 || { cat /tmp/restore.out; die "the restore never started reading"; }
echo "1 2 3" >/tmp/plan
maintain || { cat /tmp/maint.out; die "maintenance failed"; }
absent 1 3 || die "revisions 1 and 3 were not pruned"
present 2 4 5 || die "the prune deleted revision 2, which a restore is reading"
grep -q "Leaving revisions 2 of $ID for the next prune" "$MLOG" || die "kept revision not logged"

log "once the restore ends, the next prune deletes it"
rm /tmp/hold-restore
wait "$RESTORE" || { cat /tmp/restore.out; die "the held restore failed"; }
echo "2" >/tmp/plan
maintain || { cat /tmp/maint.out; die "maintenance failed"; }
absent 2 || die "revision 2 survived after the restore ended"
present 4 5 || die "the prune deleted more than planned"

log "a reader that died holds nothing"
mkdir -p /var/lock/archiver-in-use
echo "local $ID 4" >/var/lock/archiver-in-use/restore-dead
echo "4" >/tmp/plan
maintain || { cat /tmp/maint.out; die "maintenance failed"; }
absent 4 || die "a dead reader's registration kept revision 4"
[ ! -e /var/lock/archiver-in-use/restore-dead ] || die "the dead reader's file was not removed"

echo "=== PRUNE-IN-USE OK: in-use revisions survive the prune and go at the next, dead readers hold nothing ==="
