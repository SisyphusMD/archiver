#!/usr/bin/env bash
# Only hooks nobody but their owner can change run (ADR 45). Duplicacy's own
# .duplicacy/scripts never run: a service directory's repository is writable by whoever
# owns the service, and Duplicacy would run them as root with every storage credential. A
# hook writable by group or others (or in such a directory) is refused and its service
# fails; with HOOKS_DIR set, hooks come from there and any in the service directory are
# ignored with a warning.
#
#   docker run -i --rm --hostname hs-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/hook-safety.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
HOOKLOG=/tmp/hooks.log

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

export STORAGE_TARGET_1_NAME="local"
export STORAGE_TARGET_1_TYPE="local"
export STORAGE_TARGET_1_LOCAL_PATH="${STORE}"
export SERVICE_DIRECTORIES="${SERVICES}/*/"

mkdir -p "$SERVICES/app"
chmod 755 "$SERVICES" "$SERVICES/app"
echo "app data" >"$SERVICES/app/file.txt"
hook() { printf '#!/bin/sh\n%s\n' "$3" >"$1/$2"; chmod 755 "$1/$2"; }
hook "$SERVICES/app" pre-backup "echo 'app pre' >> $HOOKLOG"

log "a first backup creates the service's repository and runs its safe hook"
archiver backup >/tmp/run1.out 2>&1 || { cat /tmp/run1.out; die "first backup failed"; }
grep -q '^app pre$' "$HOOKLOG" || die "the safe pre-backup hook did not run"

log "a Duplicacy script planted in the repository never runs"
mkdir -p "$SERVICES/app/.duplicacy/scripts"
printf '#!/bin/sh\nenv > /tmp/planted-ran\n' >"$SERVICES/app/.duplicacy/scripts/pre-backup"
chmod 755 "$SERVICES/app/.duplicacy/scripts/pre-backup"
echo "more" >>"$SERVICES/app/file.txt"
archiver backup >/tmp/run2.out 2>&1 || { cat /tmp/run2.out; die "second backup failed"; }
[ -e /tmp/planted-ran ] && die "duplicacy ran the planted .duplicacy/scripts/pre-backup"

log "a group-writable hook is refused and its service fails"
chmod 775 "$SERVICES/app/pre-backup"
: >"$HOOKLOG"
archiver backup >/tmp/run3.out 2>&1 && die "a backup with a group-writable hook exited 0"
grep -q 'app pre' "$HOOKLOG" && die "the group-writable hook ran"
grep -rq "chmod go-w" /opt/archiver/logs/ || die "the refusal does not say how to fix it"
chmod 755 "$SERVICES/app/pre-backup"

log "a hook in a group-writable service directory is refused"
chmod 775 "$SERVICES/app"
archiver backup >/tmp/run4.out 2>&1 && die "a backup with a hook in a group-writable directory exited 0"
grep -q 'app pre' "$HOOKLOG" && die "the hook in the group-writable directory ran"
log "the same directory with no hook backs up: only hooks are checked"
rm "$SERVICES/app/pre-backup"
archiver backup >/tmp/run5.out 2>&1 || { cat /tmp/run5.out; die "a group-writable service directory without hooks failed"; }
chmod 755 "$SERVICES/app"

log "with HOOKS_DIR, hooks come from there and the service directory's are ignored"
mkdir -p /hooks/app
chmod 755 /hooks /hooks/app
hook /hooks/app pre-backup "echo 'hooks-dir pre' >> $HOOKLOG"
hook "$SERVICES/app" pre-backup "echo 'service-dir pre' >> $HOOKLOG"
: >"$HOOKLOG"
HOOKS_DIR=/hooks archiver backup >/tmp/run6.out 2>&1 || { cat /tmp/run6.out; die "backup with HOOKS_DIR failed"; }
grep -q '^hooks-dir pre$' "$HOOKLOG" || die "the HOOKS_DIR hook did not run"
grep -q 'service-dir pre' "$HOOKLOG" && die "the service directory's hook ran although HOOKS_DIR is set"
grep -rq "with HOOKS_DIR set, hooks come from /hooks/app" /opt/archiver/logs/ || die "no warning about the ignored service-directory hook"

echo "PASS: hook safety"
