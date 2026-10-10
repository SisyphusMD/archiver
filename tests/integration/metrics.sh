#!/usr/bin/env bash
# Metrics (ADR 35): a backup leaves logs/archiver.prom with backup health and the service's
# last backup (time, revision, bytes uploaded); the daemon keeps the file and, with
# METRICS_PORT, serves the same on /metrics; nothing listens without it.
#
#   docker run -i --rm --hostname mx-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/metrics.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store
PROM=/opt/archiver/logs/archiver.prom

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
head -c 200000 /dev/urandom >"$SERVICES/app/blob.bin"

log "a backup writes the textfile"
archiver backup >/tmp/b.out 2>&1 || { cat /tmp/b.out; die "backup failed"; }
[ -f "$PROM" ] || die "no metrics textfile after a backup"
grep -q '^archiver_backup_health 0$' "$PROM" || { cat "$PROM"; die "backup health not OK in the textfile"; }
grep -q '^archiver_service_last_ok{service="app",directory="/data/services/app"} 1$' "$PROM" || { cat "$PROM"; die "the service's result is not in the textfile"; }
grep -q '^archiver_service_last_revision{service="app",directory="/data/services/app"} 1$' "$PROM" || { cat "$PROM"; die "no revision"; }
up=$(sed -n 's/^archiver_service_last_uploaded_bytes{service="app",directory="\/data\/services\/app"} //p' "$PROM")
python3 -c "import sys; sys.exit(0 if float('$up') > 150000 else 1)" || die "uploaded bytes $up do not reflect the 200 KB backed up"

log "METRICS_PORT unset: the daemon keeps the file and listens nowhere"
BACKUP_SCHEDULE="0 3 1 1 *" archiver daemon >/tmp/d1.out 2>&1 &
d=$!
sleep 3
rm -f "$PROM"
kill $d; wait $d 2>/dev/null
curl -s -o /dev/null --max-time 3 http://127.0.0.1:9469/metrics && die "something listens with METRICS_PORT unset"

log "METRICS_PORT set: /metrics serves the same"
METRICS_PORT=9469 BACKUP_SCHEDULE="0 3 1 1 *" archiver daemon >/tmp/d2.out 2>&1 &
d=$!
wait_for 'curl -sf --max-time 3 http://127.0.0.1:9469/metrics >/tmp/m.txt' 30 || { cat /tmp/d2.out; die "no /metrics on METRICS_PORT"; }
grep -q '^archiver_service_last_ok{service="app",directory="/data/services/app"} 1$' /tmp/m.txt || { cat /tmp/m.txt; die "/metrics lacks the service"; }
wait_for "[ -f $PROM ]" 10 || die "the daemon did not write the textfile"
kill $d; wait $d 2>/dev/null

log "an invalid METRICS_PORT is refused"
METRICS_PORT=99999 archiver backup >/tmp/b2.out 2>&1 && die "METRICS_PORT=99999 was accepted"
grep -q "METRICS_PORT must be a port number" /tmp/b2.out /opt/archiver/logs/archiver.log || { cat /tmp/b2.out; die "no clear message for a bad METRICS_PORT"; }

echo "PASS: metrics"
