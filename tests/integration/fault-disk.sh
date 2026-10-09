#!/usr/bin/env bash
# Fault injection, full disks (ADR 31): the local storage, the logs volume and a restore
# target fill up in turn (small tmpfs mounts). Each gives a clear error and a failed exit,
# never a corrupted or half-written newest revision, and works again once space returns.
#
# HOST-DRIVEN: run on the docker host (the mounts need docker run options).
#
#   IMAGE=archiver:dev bash tests/integration/fault-disk.sh

set -uo pipefail

IMAGE="${IMAGE:?set IMAGE to the archiver image under test}"

docker run -i --rm --hostname fd-host --cap-drop ALL --cap-add DAC_OVERRIDE \
  --mount type=tmpfs,dst=/backup-store,tmpfs-size=16m \
  --mount type=tmpfs,dst=/opt/archiver/logs,tmpfs-size=4m \
  --mount type=tmpfs,dst=/restore,tmpfs-size=2m \
  --entrypoint bash "$IMAGE" -s <<'EOF'
set -uo pipefail
SERVICES=/data/services
STORE=/backup-store
ID=fd-host-app
log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
revisions() { ls "$STORE/snapshots/$ID" 2>/dev/null | wc -l; }

mkdir -p /opt/archiver/keys /run/secrets "$SERVICES/app"
openssl genrsa -aes256 -passout pass:pp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:pp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "openssl rsa"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >/run/secrets/storage_password; printf 'pp' >/run/secrets/rsa_passphrase
export STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH="$STORE"
export SERVICE_DIRECTORIES="$SERVICES/*/"

# A stand-in ntfy server that records every notification.
python3 -c '
import http.server
class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        with open("/tmp/notified", "ab") as f:
            f.write(str(self.headers).encode() + body + b"\n----\n")
        self.send_response(200); self.end_headers()
    def log_message(self, *a): pass
http.server.HTTPServer(("127.0.0.1", 8099), H).serve_forever()
' &
export NTFY_URL=http://127.0.0.1:8099/archiver NOTIFY_ON=failures

head -c 100000 /dev/urandom >"$SERVICES/app/small.bin"
archiver backup >/tmp/b0.out 2>&1 || { cat /tmp/b0.out; die "first backup failed"; }
[ "$(revisions)" -eq 1 ] || die "expected one revision"

log "the storage fills mid-backup: the run fails and notifies, no revision is added"
: >/tmp/notified
free=$(df --output=avail -k "$STORE" | tail -1)
dd if=/dev/zero of="$STORE/filler" bs=1k count=$((free - 1024)) 2>/dev/null
head -c 6000000 /dev/urandom >"$SERVICES/app/big.bin"
archiver backup >/tmp/b1.out 2>&1 && { cat /tmp/b1.out; die "a backup to a full storage exited 0"; }
grep -qiE "no space|space left|ERROR" /opt/archiver/logs/archiver.log || die "no error in the log for a full storage"
[ "$(revisions)" -eq 1 ] || die "a backup to a full storage added a revision"
grep -q . /tmp/notified || die "a backup to a full storage did not notify"

log "space returns: the next backup completes and restores"
rm -f "$STORE/filler"
archiver backup >/tmp/b2.out 2>&1 || { cat /tmp/b2.out; die "the backup after space returned failed"; }
[ "$(revisions)" -eq 2 ] || die "the backup after space returned did not add its revision"

log "a full restore target fails the restore loudly"
: >/tmp/notified
SNAPSHOT_ID=$ID LOCAL_DIR=/restore/app archiver auto-restore >/tmp/r1.out 2>&1 && { cat /tmp/r1.out; die "a restore into a full target exited 0"; }
grep -qiE "no space|space left" /tmp/r1.out || { tail -5 /tmp/r1.out; die "no clear error for a full restore target"; }
rm -rf /restore/app

log "the logs volume full: the run fails with a clear message on stdout and notifies, the backup itself stands"
: >/tmp/notified
free=$(df --output=avail -k /opt/archiver/logs | tail -1)
dd if=/dev/zero of=/opt/archiver/logs/filler bs=1k count="$free" 2>/dev/null
echo change >>"$SERVICES/app/small.bin"
archiver backup >/tmp/b3.out 2>&1 && { cat /tmp/b3.out; die "a backup with a full logs volume exited 0"; }
grep -q "Failed to log message" /tmp/b3.out || { tail -5 /tmp/b3.out; die "no clear message for a full logs volume"; }
grep -q . /tmp/notified || die "a backup with a full logs volume did not notify"
rm -f /opt/archiver/logs/filler
mkdir -p /restore2 && SNAPSHOT_ID=$ID LOCAL_DIR=/restore2/app archiver auto-restore >/tmp/r2.out 2>&1 || { cat /tmp/r2.out; die "the newest revision does not restore after the logs filled"; }
cmp "$SERVICES/app/small.bin" /restore2/app/small.bin || die "the newest revision restored different bytes"

echo "PASS: fault injection, full disks"
EOF
