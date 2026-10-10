#!/usr/bin/env bash
# Storage outages (ADR 34): a primary that cannot be reached fails the backup at once with
# one PRIMARY DOWN notification (not a Backup Failed besides), starts no service (no pre
# hook runs), and makes backup health FAILING; once it is back, the next backup clears it.
# An unreachable secondary is skipped with one line instead of tried.
#
#   docker run -i --rm --hostname pd-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/primary-down.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store
NOTES=/tmp/notified

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; cat "$NOTES" >&2 2>/dev/null; exit 1; }
titles() { grep -c "^Title: $1\$" "$NOTES" 2>/dev/null || true; }

mkdir -p /opt/archiver/keys /run/secrets "$STORE" "$SERVICES/app" "$SERVICES/db"
openssl genrsa -aes256 -passout pass:pp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:pp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "openssl rsa"
chmod 600 /opt/archiver/keys/private.pem
ssh-keygen -q -t ed25519 -N "" -f /opt/archiver/keys/id_ed25519 || die "ssh-keygen"
printf 'testpassword' >/run/secrets/storage_password; printf 'pp' >/run/secrets/rsa_passphrase
echo a >"$SERVICES/app/f"; echo d >"$SERVICES/db/f"
printf '#!/bin/sh\ntouch /tmp/pre-ran\n' >"$SERVICES/app/pre-backup"; chmod 755 "$SERVICES/app/pre-backup"
export SERVICE_DIRECTORIES="$SERVICES/*/"

python3 -c '
import http.server
class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        with open("/tmp/notified", "ab") as f:
            f.write(b"Title: " + self.headers.get("Title", "").encode() + b"\n" + body + b"\n")
        self.send_response(200); self.end_headers()
    def log_message(self, *a): pass
http.server.HTTPServer(("127.0.0.1", 8099), H).serve_forever()
' &
export NTFY_URL=http://127.0.0.1:8099/archiver
sleep 1

# Nothing listens on port 2: the connection is refused at once.
down=(STORAGE_TARGET_1_NAME=offsite STORAGE_TARGET_1_TYPE=sftp STORAGE_TARGET_1_SFTP_URL=127.0.0.1
  STORAGE_TARGET_1_SFTP_PORT=2 STORAGE_TARGET_1_SFTP_USER=backup STORAGE_TARGET_1_SFTP_PATH=upload)

log "the primary down: the backup fails at once, one PRIMARY DOWN, no service started"
: >"$NOTES"
began=$(date +%s)
env "${down[@]}" archiver backup >/tmp/b1.out 2>&1 && die "a backup with its primary down exited 0"
[ $(( $(date +%s) - began )) -lt 120 ] || die "the primary-down backup took over two minutes"
grep -q "PRIMARY DOWN" /opt/archiver/logs/archiver.log || { tail -20 /opt/archiver/logs/archiver.log; die "no PRIMARY DOWN in the log"; }
[ "$(grep -c "Skipped: primary storage 'offsite' is down" /opt/archiver/logs/archiver.log)" = 2 ] || die "both services were not reported skipped"
[ -e /tmp/pre-ran ] && die "a pre hook ran with the primary down"
[ "$(titles 'PRIMARY DOWN')" = 1 ] || die "not exactly one PRIMARY DOWN notification"
[ "$(titles 'Backup Failed')" = 0 ] || die "a Backup Failed was sent besides PRIMARY DOWN"
env "${down[@]}" archiver health --backups >/tmp/h1.out; rc=$?
[ "$rc" = 2 ] && grep -q "PRIMARY DOWN (primary-down)" /tmp/h1.out || { cat /tmp/h1.out; die "backup health is not FAILING for the primary down ($rc)"; }

log "the primary back (a local one): the backup runs and PRIMARY DOWN clears"
export STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH="$STORE"
archiver backup >/tmp/b2.out 2>&1 || { cat /tmp/b2.out; die "the backup with a reachable primary failed"; }
[ "$(titles 'Primary Storage Back')" = 1 ] || die "no recovery notice for the primary"
[ -e /tmp/pre-ran ] || die "the pre hook did not run once the primary was back"

log "the established primary unmounted (empty directory left): PRIMARY DOWN, no new storage created there"
mv "$STORE" /backup-store-moved && mkdir "$STORE"
: >"$NOTES"
archiver backup >/tmp/b2b.out 2>&1 && die "a backup to an emptied primary exited 0"
[ -e "$STORE/config" ] && die "a new storage was created in the emptied primary's place"
[ "$(titles 'PRIMARY DOWN')" = 1 ] || die "no PRIMARY DOWN for the emptied primary"
rmdir "$STORE" && mv /backup-store-moved "$STORE"

log "an unreachable secondary is skipped with one line, not tried"
export STORAGE_TARGET_2_NAME=offsite STORAGE_TARGET_2_TYPE=sftp STORAGE_TARGET_2_SFTP_URL=127.0.0.1 \
  STORAGE_TARGET_2_SFTP_PORT=2 STORAGE_TARGET_2_SFTP_USER=backup STORAGE_TARGET_2_SFTP_PATH=upload
echo more >>"$SERVICES/db/f"
archiver backup >/tmp/b3.out 2>&1 && die "a backup whose copy was skipped exited 0"
grep -q "Copy to offsite storage skipped: it cannot be reached" /opt/archiver/logs/archiver.log || { tail -20 /opt/archiver/logs/archiver.log; die "no skip line for the unreachable secondary"; }
grep -q "Retrying failed copies" /opt/archiver/logs/archiver.log && die "an unreachable secondary was tried and retried"

echo "PASS: primary down"
