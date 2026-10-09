#!/usr/bin/env bash
# Notifications are one per incident (ADRs 33, 36): a failing backup notifies once with its
# errors, a second failure while the incident is open is not notified again, a clean backup
# sends one recovery notice, and with ALERT_REPEAT_INTERVAL elapsed an ongoing failure is
# notified again. `archiver notify test` reaches the destination.
#
#   docker run -i --rm --hostname ic-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/incidents.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store
NOTES=/tmp/notified

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; cat "$NOTES" >&2 2>/dev/null; exit 1; }
titles() { grep -c "^Title: $1\$" "$NOTES" 2>/dev/null || true; }

mkdir -p /opt/archiver/keys /run/secrets "$STORE" "$SERVICES/app"
openssl genrsa -aes256 -passout pass:pp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:pp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "openssl rsa"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >/run/secrets/storage_password; printf 'pp' >/run/secrets/rsa_passphrase
export STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH="$STORE"
export SERVICE_DIRECTORIES="$SERVICES/*/"
echo data >"$SERVICES/app/file.txt"

# A stand-in ntfy server that records each notification's title and body.
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
export NTFY_URL=http://127.0.0.1:8099/archiver NOTIFY_ON=everything
sleep 1

log "notify test reaches the destination"
archiver notify test >/tmp/nt.out 2>&1 || { cat /tmp/nt.out; die "notify test failed"; }
[ "$(titles 'Archiver Test')" = 1 ] || die "notify test sent no test message"
grep -q "receives everything" "$NOTES" || die "the test does not say what the destination receives"

# duplicacy's backup fails while /tmp/fail exists; everything else passes through.
REAL="$(command -v duplicacy)"
mv "$REAL" "${REAL}.real"
cat >"$REAL" <<'WRAP'
#!/usr/bin/env bash
if [ -e /tmp/fail ] && [ "${2:-}" = backup ]; then echo "SIMULATED: upload refused" >&2; exit 1; fi
exec "$0.real" "$@"
WRAP
chmod +x "$REAL"

log "a failing backup notifies once, with its errors"
: >"$NOTES"; touch /tmp/fail
archiver backup >/tmp/b1.out 2>&1 && die "a failing backup exited 0"
[ "$(titles 'Backup Failed')" = 1 ] || die "the failed backup did not send exactly one Backup Failed"
grep -q "Backup to local failed for app service" "$NOTES" || die "the notification does not carry the error"
[ "$(titles 'Backup Error')" = 0 ] || die "error lines still notify one by one"

log "failing again while the incident is open is not notified again"
archiver backup >/tmp/b2.out 2>&1 && die "a failing backup exited 0"
[ "$(titles 'Backup Failed')" = 1 ] || die "an open incident was notified again"

log "a clean backup sends one recovery notice"
rm -f /tmp/fail
archiver backup >/tmp/b3.out 2>&1 || { cat /tmp/b3.out; die "the clean backup failed"; }
[ "$(titles 'Backup Recovered')" = 1 ] || die "no recovery notice"
archiver backup >/tmp/b4.out 2>&1 || die "a second clean backup failed"
[ "$(titles 'Backup Recovered')" = 1 ] || die "a recovery was announced twice"

log "with the repeat interval elapsed, an ongoing failure is notified again"
: >"$NOTES"; touch /tmp/fail
export ALERT_REPEAT_INTERVAL=1s
archiver backup >/tmp/b5.out 2>&1
sleep 2
archiver backup >/tmp/b6.out 2>&1
[ "$(titles 'Backup Failed')" = 2 ] || die "the ongoing failure was not repeated after the interval"
grep -q "Still happening, since" "$NOTES" || die "the repeat does not say it is ongoing"

log "ALERT_REPEAT_INTERVAL=0 never repeats"
rm -f /tmp/fail; archiver backup >/dev/null 2>&1; touch /tmp/fail; : >"$NOTES"
export ALERT_REPEAT_INTERVAL=0
archiver backup >/dev/null 2>&1; sleep 2; archiver backup >/dev/null 2>&1
[ "$(titles 'Backup Failed')" = 1 ] || die "a repeat interval of 0 repeated"

echo "PASS: incidents"
