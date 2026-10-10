#!/usr/bin/env bash
# Status page (ADR 38): with WEB_PORT the daemon serves a read-only page showing backup health,
# the services and the log, and /status.json; anything but GET is refused; a startup line
# warns that it has no login; without WEB_PORT nothing listens.
#
#   docker run -i --rm --hostname wb-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/web.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
wait_for() { local _; for _ in $(seq 1 "$2"); do eval "$1" && return 0; sleep 1; done; return 1; }

mkdir -p /opt/archiver/keys /run/secrets "$STORE" "$SERVICES/app"
openssl genrsa -aes256 -passout pass:pp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:pp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "openssl rsa"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >/run/secrets/storage_password; printf 'pp' >/run/secrets/rsa_passphrase
export STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH="$STORE"
export SERVICE_DIRECTORIES="$SERVICES/*/"
echo a >"$SERVICES/app/f"
archiver backup >/tmp/b.out 2>&1 || { cat /tmp/b.out; die "backup failed"; }

log "WEB_PORT set: the page shows backup health, the service and the log"
WEB_PORT=8470 BACKUP_SCHEDULE="0 3 1 1 *" archiver daemon >/tmp/d.out 2>&1 &
d=$!
wait_for 'curl -sf --max-time 3 http://127.0.0.1:8470/ >/tmp/page.html' 30 || { cat /tmp/d.out; die "no page on WEB_PORT"; }
grep -q '>OK<' /tmp/page.html || die "the page does not show backup health OK"
grep -q '<td>app</td>' /tmp/page.html || die "the page does not list the service"
grep -q 'Backup session summary' /tmp/page.html || die "the page does not show the log"
grep -q 'has no login of its own' /tmp/d.out || die "no startup warning that the page has no login"
curl -sf --max-time 3 http://127.0.0.1:8470/status.json | python3 -c 'import json,sys; assert json.load(sys.stdin)["backup_health"]["state"]=="OK"' || die "no /status.json"
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST --max-time 3 http://127.0.0.1:8470/)
[ "$code" = 405 ] || die "a POST answered $code"
grep -q 'testpassword' /tmp/page.html && die "a secret appears on the page"
kill $d; wait $d 2>/dev/null

log "WEB_PORT unset: nothing listens"
BACKUP_SCHEDULE="0 3 1 1 *" archiver daemon >/tmp/d2.out 2>&1 &
d=$!
sleep 3
curl -s -o /dev/null --max-time 3 http://127.0.0.1:8470/ && die "something listens with WEB_PORT unset"
kill $d; wait $d 2>/dev/null

log "the same port for METRICS_PORT and WEB_PORT is refused"
METRICS_PORT=8470 WEB_PORT=8470 archiver daemon --check >/tmp/c.out 2>&1 && die "the same port twice was accepted"
grep -q "give them different ports" /tmp/c.out || { cat /tmp/c.out; die "no clear message for the same port"; }

echo "PASS: status page"
