#!/usr/bin/env bash
# Check-ins (ADR 37): CHECKIN_URL is pinged after a successful backup and its fail variant
# after a failed one (healthchecks.io style /fail; Uptime Kuma push URLs status=down); a
# copy worker pings STORAGE_TARGET_N_CHECKIN_URL when its target is caught up.
#
#   docker run -i --rm --hostname ck-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/checkins.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store
OFFSITE=/backup-offsite
PINGS=/tmp/pings

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; cat "$PINGS" >&2 2>/dev/null; exit 1; }
wait_for() { local _; for _ in $(seq 1 "$2"); do eval "$1" && return 0; sleep 1; done; return 1; }

mkdir -p /opt/archiver/keys /run/secrets "$STORE" "$OFFSITE" "$SERVICES/app"
openssl genrsa -aes256 -passout pass:pp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:pp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "openssl rsa"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >/run/secrets/storage_password; printf 'pp' >/run/secrets/rsa_passphrase
export STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH="$STORE"
export SERVICE_DIRECTORIES="$SERVICES/*/"
echo a >"$SERVICES/app/f"

# A stand-in monitor recording each ping's path and query.
python3 -c '
import http.server
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        with open("/tmp/pings", "a") as f: f.write(self.path + "\n")
        self.send_response(200); self.end_headers()
    def log_message(self, *a): pass
http.server.HTTPServer(("127.0.0.1", 8098), H).serve_forever()
' &
sleep 1

log "a successful backup pings CHECKIN_URL"
export CHECKIN_URL=http://127.0.0.1:8098/hc/uuid-1
archiver backup >/tmp/b1.out 2>&1 || { cat /tmp/b1.out; die "backup failed"; }
grep -qx '/hc/uuid-1' "$PINGS" || die "no success ping"

log "a failed backup pings the /fail variant"
: >"$PINGS"
printf '#!/bin/sh\nexit 1\n' >"$SERVICES/app/pre-backup"; chmod 755 "$SERVICES/app/pre-backup"
archiver backup >/tmp/b2.out 2>&1 && die "a failing backup exited 0"
grep -qx '/hc/uuid-1/fail' "$PINGS" || die "no fail ping"
rm "$SERVICES/app/pre-backup"

log "an Uptime Kuma push URL gets status=up"
: >"$PINGS"
export CHECKIN_URL="http://127.0.0.1:8098/api/push/tok"
archiver backup >/tmp/b3.out 2>&1 || { cat /tmp/b3.out; die "backup failed"; }
grep -q '^/api/push/tok?.*status=up' "$PINGS" || die "no status=up ping"

log "a copy worker pings its target's check-in URL when caught up"
: >"$PINGS"
unset CHECKIN_URL
export STORAGE_TARGET_2_NAME=offsite STORAGE_TARGET_2_TYPE=local STORAGE_TARGET_2_LOCAL_PATH="$OFFSITE" \
  STORAGE_TARGET_2_CHECKIN_URL=http://127.0.0.1:8098/hc/offsite
BACKUP_SCHEDULE="0 3 1 1 *" archiver daemon >/tmp/d.out 2>&1 &
d=$!
wait_for "grep -qx '/hc/offsite' $PINGS" 90 || { tail -20 /opt/archiver/logs/copies.log; die "no ping when the target caught up"; }
kill $d; wait $d 2>/dev/null

log "an invalid CHECKIN_URL is refused"
CHECKIN_URL="ftp://nope" archiver backup >/tmp/b4.out 2>&1 && die "an ftp CHECKIN_URL was accepted"
grep -q "CHECKIN_URL" /tmp/b4.out /opt/archiver/logs/archiver.log || die "no clear message for a bad CHECKIN_URL"

echo "PASS: check-ins"
