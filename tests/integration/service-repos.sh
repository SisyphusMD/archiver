#!/usr/bin/env bash
# Each service's Duplicacy repository is Archiver's own, on the logs volume, never the
# service directory's: whoever owns a service directory must not supply the configuration
# a root backup follows. A .duplicacy planted there (preferences pointing at another
# storage and another directory, or a link) is neither used nor written, and is not backed
# up; a repository someone else could have changed is refused; restores keep their
# repository out of the destination.
#
#   docker run -i --rm --hostname sp-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/service-repos.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store
EVIL=/evil-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase
REPO="/opt/archiver/logs/.service-repos/sp-host-app-$(printf %s "$SERVICES/app" | sha256sum | cut -c1-12)"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

log "materialize RSA keypair + file secrets (env-native mode)"
mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE" "$EVIL" /secret-dir
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null \
  || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null \
  || die "openssl rsa -pubout"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${RSA_PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"
echo "not for backing up" >/secret-dir/secret.txt

export STORAGE_TARGET_1_NAME="local"
export STORAGE_TARGET_1_TYPE="local"
export STORAGE_TARGET_1_LOCAL_PATH="${STORE}"
export SERVICE_DIRECTORIES="${SERVICES}/*/"

mkdir -p "$SERVICES/app/.duplicacy"
echo "app data" >"$SERVICES/app/file.txt"
# What a service's owner could leave: preferences sending the backup to another storage, of
# another directory, and filters that exclude the real data.
cat >"$SERVICES/app/.duplicacy/preferences" <<PREFS
[{"name":"local","id":"sp-host-app","repository":"/secret-dir","storage":"$EVIL","encrypted":true,"no_backup":false,"no_restore":false,"no_save_password":true,"keys":null}]
PREFS
printf -- '-*\n' >"$SERVICES/app/.duplicacy/filters"
cp -a "$SERVICES/app/.duplicacy" /tmp/planted
cp -a "$SERVICES/app/.duplicacy" /tmp/planted-orig

log "a backup ignores the planted repository"
archiver backup >/tmp/run1.out 2>&1 || { cat /tmp/run1.out; die "backup failed"; }
[ -z "$(ls -A "$EVIL")" ] || die "the planted preferences sent the backup to another storage"
[ -d "$STORE/snapshots/sp-host-app" ] || die "no revision on the configured storage"
diff -r /tmp/planted "$SERVICES/app/.duplicacy" >/dev/null || die "the planted repository was changed"
grep -q "\"repository\": \"$SERVICES/app\"" "$REPO/.duplicacy/preferences" || die "Archiver's repository does not point at the service: $(cat "$REPO/.duplicacy/preferences")"
[ "$(stat -c '%a %u' "$REPO" /opt/archiver/logs/.service-repos | sort -u)" = "700 0" ] || die "the repositories are not owner-only"

log "what was backed up is the service's data, without the planted .duplicacy"
SNAPSHOT_ID=sp-host-app LOCAL_DIR=/restore/app archiver auto-restore >/tmp/r1.out 2>&1 || { cat /tmp/r1.out; die "restore failed"; }
cmp "$SERVICES/app/file.txt" /restore/app/file.txt || die "file.txt not restored"
[ ! -e /restore/app/secret.txt ] || die "the planted repository path was backed up"
# Duplicacy's restore always makes an empty .duplicacy in its destination.
[ -z "$(ls -A /restore/app/.duplicacy 2>/dev/null)" ] || die "a .duplicacy was backed up or written in the restore's destination"

log "a link planted as .duplicacy is not followed"
rm -rf "$SERVICES/app/.duplicacy"
ln -s /tmp/planted "$SERVICES/app/.duplicacy"
echo "two" >>"$SERVICES/app/file.txt"
archiver backup >/tmp/run2.out 2>&1 || { cat /tmp/run2.out; die "backup with a linked .duplicacy failed"; }
[ -z "$(ls -A "$EVIL")" ] || die "the linked preferences sent the backup to another storage"
diff -r /tmp/planted-orig /tmp/planted >/dev/null || die "something was written through the link"

log "a repository someone else could change is refused and its service fails"
chmod 770 "$REPO"
archiver backup >/tmp/run3.out 2>&1 && die "a backup through a group-writable repository exited 0"
grep -q "Cannot prepare the Duplicacy repository" /opt/archiver/logs/archiver.log || die "the refusal is not logged"
chmod 700 "$REPO"
mv "$REPO" /tmp/moved-repo && ln -s /tmp/moved-repo "$REPO"
archiver backup >/tmp/run4.out 2>&1 && die "a backup through a linked repository exited 0"
rm "$REPO"
archiver backup >/tmp/run5.out 2>&1 || { cat /tmp/run5.out; die "the backup after the repository was removed failed"; }
[ -d "$REPO/.duplicacy" ] || die "the repository was not recreated"

echo "PASS: service repositories"
