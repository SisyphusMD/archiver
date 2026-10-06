#!/usr/bin/env bash
# `archiver migrate hooks` turns a service's sourced service-backup-settings.sh into
# executable hooks plus a filters file (ADR 20), and the Go pipeline then runs them: the
# pre-backup hook's script still runs and logs, the post-backup hook runs, and the filters
# still select exactly the same files, the hook files included. Before migration a backup
# refuses the service and names the command.
#
#   docker run -i --rm --hostname mh-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/migrate-hooks.sh

set -uo pipefail

SVC=/data/services/app
STORE=/backup-store
RESTORE=/data/restore
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
LOG=/opt/archiver/logs/archiver.log
HOST="$(hostname)"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

log "materialize RSA keypair + file secrets (env-native mode)"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE" "$SVC/data" "$SVC/skip"
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "openssl rsa"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${RSA_PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"

export SERVICE_DIRECTORIES="/data/services/*/"
export STORAGE_TARGET_1_NAME="local"
export STORAGE_TARGET_1_TYPE="local"
export STORAGE_TARGET_1_LOCAL_PATH="${STORE}"

log "a service in the nas/vps shape: filters naming the settings file, a pre hook that runs ./backup.sh"
echo "kept" >"$SVC/data/kept.txt"
echo "excluded" >"$SVC/skip/excluded.txt"
cat >"$SVC/backup.sh" <<'SH'
#!/bin/sh
echo "dump written by backup.sh"
date +%s%N > data/dump.txt
SH
chmod +x "$SVC/backup.sh"
cat >"$SVC/service-backup-settings.sh" <<'SH'
DUPLICACY_FILTERS_PATTERNS=(
  "+data/"
  "+data/*"
  "+backup.sh"
  "+service-backup-settings.sh"
  "-?*"
)
service_specific_pre_backup_function() {
  if ! (set -o pipefail; ./backup.sh 2>&1 | log_output); then
    log_message "WARNING" "Pre-backup script failed."
    return 1
  fi
}
service_specific_post_backup_function() {
  log_message "INFO" "post hook ran"
}
SH

log "before migration a backup refuses the service, naming the migration"
if archiver backup >/tmp/run1.out 2>&1; then cat /tmp/run1.out; die "an unmigrated service backed up as a success"; fi
grep -q "archiver migrate hooks" "$LOG" || die "the refusal does not name the migration: $(tail -5 "$LOG")"
[ ! -d "$STORE/snapshots/${HOST}-app" ] || die "the refused service was backed up"

log "migrate"
archiver migrate hooks >/tmp/migrate.out 2>&1 || { cat /tmp/migrate.out; die "migrate hooks failed"; }
cat /tmp/migrate.out
[ -x "$SVC/pre-backup" ] && [ -x "$SVC/post-backup" ] && [ -f "$SVC/filters" ] || die "hooks or filters not written"
[ ! -e "$SVC/service-backup-settings.sh" ] && [ -f "$SVC/service-backup-settings.legacy.sh" ] || die "settings file not moved aside"
grep -qx "+pre-backup" "$SVC/filters" && grep -qx "+service-backup-settings.legacy.sh" "$SVC/filters" || die "filters do not select the new hook files: $(cat "$SVC/filters")"
archiver migrate hooks >/tmp/migrate2.out 2>&1 || die "a second migration should be a no-op"
grep -q "Migrated 0 service directories" /tmp/migrate2.out || die "second migration was not a no-op: $(cat /tmp/migrate2.out)"

log "after migration the Go pipeline runs the hooks"
echo "kept, revision 1" >"$SVC/data/kept.txt"
archiver backup >/tmp/run2.out 2>&1 || { cat /tmp/run2.out; tail -30 "$LOG"; die "Go-pipeline backup failed"; }
grep -q "\[Service: app\] dump written by backup.sh" "$LOG" || die "migrated pre hook output not logged"
grep -q "\[Service: app\] post hook ran" "$LOG" || die "migrated post hook did not run"
grep -q "Main backup script exited." "$LOG" || die "no completion line"
[ "$(ls "$STORE/snapshots/${HOST}-app" | wc -l)" -eq 1 ] || die "expected 1 revision"

log "restore revision 1: filters still select data/ and the hook files, nothing else"
mkdir -p "$RESTORE" && cd "$RESTORE" || die "restore dir"
export DUPLICACY_LOCAL_PASSWORD=testpassword DUPLICACY_LOCAL_RSA_PASSPHRASE="${RSA_PASSPHRASE}"
duplicacy init -e -storage-name local "${HOST}-app" "$STORE" >/dev/null || die "restore init"
duplicacy restore -r 1 -key /opt/archiver/keys/private.pem >/dev/null || die "restore"
for f in data/kept.txt data/dump.txt backup.sh pre-backup post-backup filters service-backup-settings.legacy.sh; do
  [ -e "$RESTORE/$f" ] || die "restore lacks $f"
done
[ ! -e "$RESTORE/skip/excluded.txt" ] || die "filters no longer exclude skip/"
grep -q "revision 1" "$RESTORE/data/kept.txt" || die "restored the wrong revision"

echo "=== MIGRATE-HOOKS OK: legacy hooks migrated, run by the Go pipeline, filters unchanged ==="
