#!/usr/bin/env bash
# archiver forget: a retired service's snapshots leave every storage, a configured one's
# never do, and doctor names what is left behind.
#
#   docker run -i --rm --hostname fg-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/forget.sh

set -uo pipefail

STORE=/backup-store
OFFSITE=/offsite-store
SECRETS_DIR=/run/secrets
HOST="$(hostname)"

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

log "two services backed up to a local primary and copied to a local secondary"
mkdir -p /opt/archiver/keys "$SECRETS_DIR" "$STORE" "$OFFSITE" /data/services/app /data/services/old
openssl genrsa -aes256 -passout pass:rp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:rp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "pubout"
printf 'testpassword' >"$SECRETS_DIR/storage_password"
printf 'rp' >"$SECRETS_DIR/rsa_passphrase"
export SERVICE_DIRECTORIES="/data/services/*/"
export STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH="$STORE"
export STORAGE_TARGET_2_NAME=offsite STORAGE_TARGET_2_TYPE=local STORAGE_TARGET_2_LOCAL_PATH="$OFFSITE"
echo "app data" >/data/services/app/file.txt
head -c 200000 /dev/urandom >/data/services/old/file.bin
archiver backup >/tmp/b1 2>&1 || { cat /tmp/b1; die "backup 1 failed"; }
sleep 1.1
echo "old again" >/data/services/old/more.txt
archiver backup >/tmp/b2 2>&1 || { cat /tmp/b2; die "backup 2 failed"; }
for s in "$STORE" "$OFFSITE"; do
  [ -d "$s/snapshots/$HOST-old" ] || die "$s has no $HOST-old snapshots to begin with"
done

log "a configured service is refused"
archiver forget app --yes >/tmp/out 2>&1 && { cat /tmp/out; die "forget of a configured service succeeded"; }
grep -q "still a configured service" /tmp/out || { cat /tmp/out; die "no reason given for the refusal"; }
[ -d "$STORE/snapshots/$HOST-app" ] || die "a refused forget deleted app"

log "the retired service: doctor names it, forget removes it everywhere"
mv /data/services/old /data/retired
archiver doctor >/tmp/doc 2>&1
grep -q "WARN.*no configured service backs up.*$HOST-old" /tmp/doc || { cat /tmp/doc; die "doctor did not name $HOST-old"; }
archiver forget old </dev/null >/tmp/out 2>&1 && { cat /tmp/out; die "forget without --yes and no terminal went ahead"; }
[ -d "$STORE/snapshots/$HOST-old" ] || die "forget without --yes deleted something"
archiver forget old --yes >/tmp/out 2>&1 || { cat /tmp/out; die "forget failed"; }
grep -q "'local': forgot $HOST-old" /tmp/out || { cat /tmp/out; die "no report for local"; }
grep -q "'offsite': forgot $HOST-old" /tmp/out || { cat /tmp/out; die "no report for offsite"; }
for s in "$STORE" "$OFFSITE"; do
  [ ! -e "$s/snapshots/$HOST-old" ] || die "$s still has $HOST-old"
  [ -d "$s/snapshots/$HOST-app" ] || die "$s lost $HOST-app"
done
SNAPSHOT_ID="$HOST-old" archiver snapshot-exists >/tmp/out 2>&1
[ $? -eq 1 ] || { cat /tmp/out; die "$HOST-old still found"; }
archiver doctor >/tmp/doc 2>&1
! grep -q "no configured service backs up" /tmp/doc || { cat /tmp/doc; die "doctor still warns after forget"; }

log "the exhaustive prune collects its chunks, and the rest still restores"
archiver maintenance exhaustive >/tmp/m 2>&1 || { cat /tmp/m; die "exhaustive maintenance failed"; }
find "$STORE/chunks" -name '*.fsl' | grep -q . || { cat /tmp/m; die "the exhaustive prune turned no chunk into a fossil"; }
SNAPSHOT_ID="$HOST-app" LOCAL_DIR=/data/r archiver auto-restore >/tmp/out 2>&1 || { cat /tmp/out; die "restore of app failed"; }
grep -q "app data" /data/r/file.txt || die "app did not restore"
archiver forget old --yes >/tmp/out 2>&1 || { cat /tmp/out; die "a second forget failed"; }
grep -q "Nothing to forget" /tmp/out || { cat /tmp/out; die "a second forget found something"; }

log "PASS"
