#!/usr/bin/env bash
# Fault injection, corruption (ADR 31): chunks a later revision added are damaged on the
# storage. Maintenance's check names the damaged revision, fails and notifies; a drill of
# the newest revision fails and notifies; restoring the damaged revision fails loudly; the
# earlier revision, whose chunks are intact, still restores byte for byte.
#
#   docker run -i --rm --hostname fc-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/fault-corruption.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store
SECRETS_DIR=/run/secrets
RSA_PASSPHRASE=testpassphrase

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

mkdir -p /opt/archiver/keys "${SECRETS_DIR}" "$STORE"
openssl genrsa -aes256 -passout "pass:${RSA_PASSPHRASE}" -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin "pass:${RSA_PASSPHRASE}" -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "openssl rsa"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >"${SECRETS_DIR}/storage_password"
printf '%s' "${RSA_PASSPHRASE}" >"${SECRETS_DIR}/rsa_passphrase"
export STORAGE_TARGET_1_NAME="local" STORAGE_TARGET_1_TYPE="local" STORAGE_TARGET_1_LOCAL_PATH="${STORE}"
export SERVICE_DIRECTORIES="${SERVICES}/*/"

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
export NTFY_URL=http://127.0.0.1:8099/archiver

log "revision 1, then revision 2 with a new file"
mkdir -p "$SERVICES/app" && head -c 20000 /dev/urandom >"$SERVICES/app/first.bin"
archiver backup >/tmp/b1.out 2>&1 || { cat /tmp/b1.out; die "first backup failed"; }
sha256sum "$SERVICES/app/first.bin" | cut -d' ' -f1 >/tmp/first.sum
find "$STORE/chunks" -type f | sort >/tmp/chunks1
head -c 50000 /dev/urandom >"$SERVICES/app/second.bin"
archiver backup >/tmp/b2.out 2>&1 || { cat /tmp/b2.out; die "second backup failed"; }
find "$STORE/chunks" -type f | sort >/tmp/chunks2

log "damage the chunks only revision 2 has: one deleted, the rest altered"
new=$(comm -13 /tmp/chunks1 /tmp/chunks2)
[ -n "$new" ] || die "revision 2 added no chunks"
first=1
for c in $new; do
  if [ $first = 1 ]; then rm -f "$c"; first=0; else printf 'garbage' | dd of="$c" bs=1 seek=40 conv=notrunc 2>/dev/null; fi
done

log "maintenance's check names the damaged revision, fails and notifies"
: >/tmp/notified
archiver maintenance >/tmp/m.out 2>&1 && { cat /tmp/m.out; die "maintenance passed over a missing chunk"; }
grep -q "Storage check failed for local; damaged revisions: fc-host-app revision 2" /opt/archiver/logs/maintenance.log \
  || { tail -20 /opt/archiver/logs/maintenance.log; die "the check does not name the damaged revision"; }
grep -q "damaged revisions: fc-host-app revision 2" /tmp/notified || { cat /tmp/notified; die "the check's notification does not name the damaged revision"; }
grep -q "Maintenance completed successfully" /opt/archiver/logs/maintenance.log && die "maintenance claims success after a failed check"

log "a drill of the newest revision fails and notifies"
: >/tmp/notified
archiver drill app >/tmp/d.out 2>&1 && { cat /tmp/d.out; die "a drill over damaged chunks exited 0"; }
grep -q "Restore drill of fc-host-app from 'local' failed" /opt/archiver/logs/drill.log || { tail -20 /opt/archiver/logs/drill.log; die "the drill failure is not logged"; }
grep -q . /tmp/notified || die "a failed drill did not notify"

log "restoring revision 2 fails loudly"
SNAPSHOT_ID=fc-host-app LOCAL_DIR=/restore/r2 REVISION=2 archiver auto-restore >/tmp/r2.out 2>&1 && { cat /tmp/r2.out; die "a damaged revision restored"; }

log "revision 1 still restores byte for byte"
SNAPSHOT_ID=fc-host-app LOCAL_DIR=/restore/r1 REVISION=1 archiver auto-restore >/tmp/r1.out 2>&1 || { cat /tmp/r1.out; die "the intact revision did not restore"; }
[ "$(sha256sum /restore/r1/first.bin | cut -d' ' -f1)" = "$(cat /tmp/first.sum)" ] || die "the intact revision restored different bytes"

echo "PASS: fault injection, corruption"
