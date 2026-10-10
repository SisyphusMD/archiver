#!/usr/bin/env bash
# Backup health (ADR 32): `archiver health --backups` is OK after good backups, FAILING
# (exit 2) once a service's backup fails, and OK again after it recovers; `status` shows it
# and `status --json` carries it. The liveness healthcheck is not affected by a failed
# backup.
#
#   docker run -i --rm --hostname bh-host --cap-drop ALL --cap-add DAC_OVERRIDE \
#     --entrypoint bash archiver:dev -s < tests/integration/backup-health.sh

set -uo pipefail

SERVICES=/data/services
STORE=/backup-store

log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

mkdir -p /opt/archiver/keys /run/secrets "$STORE" "$SERVICES/app" "$SERVICES/db"
openssl genrsa -aes256 -passout pass:pp -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null || die "openssl genrsa"
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:pp -pubout -out /opt/archiver/keys/public.pem 2>/dev/null || die "openssl rsa"
chmod 600 /opt/archiver/keys/private.pem
printf 'testpassword' >/run/secrets/storage_password; printf 'pp' >/run/secrets/rsa_passphrase
export STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH="$STORE"
export SERVICE_DIRECTORIES="$SERVICES/*/" BACKUP_SCHEDULE="0 3 * * *"
echo a >"$SERVICES/app/f"; echo d >"$SERVICES/db/f"

log "before any backup: DEGRADED (not backed up yet), never FAILING"
archiver health --backups >/tmp/h0.out; rc=$?
[ "$rc" = 1 ] && grep -q "DEGRADED" /tmp/h0.out && grep -q "app: not backed up yet" /tmp/h0.out || { cat /tmp/h0.out; die "a fresh deployment is not DEGRADED ($rc)"; }

log "after good backups: OK"
archiver backup >/tmp/b1.out 2>&1 || { cat /tmp/b1.out; die "backup failed"; }
archiver health --backups >/tmp/h1.out || { cat /tmp/h1.out; die "health is not OK after good backups"; }
grep -q "Backup health: OK" /tmp/h1.out || die "no OK line"

log "a failing service: FAILING, exit 2, in status and its JSON; the liveness healthcheck unaffected"
printf '#!/bin/sh\nexit 1\n' >"$SERVICES/db/pre-backup"; chmod 755 "$SERVICES/db/pre-backup"
archiver backup >/tmp/b2.out 2>&1 && die "a backup with a failing pre hook exited 0"
archiver health --backups >/tmp/h2.out; rc=$?
[ "$rc" = 2 ] && grep -q "db: last backup skipped" /tmp/h2.out || { cat /tmp/h2.out; die "a skipped service is not FAILING ($rc)"; }
grep -q "Backup Failed (backup)" /tmp/h2.out || { cat /tmp/h2.out; die "the open backup incident is not listed"; }
archiver status >/tmp/s.out 2>&1; grep -q "^Backup health: FAILING" /tmp/s.out || { cat /tmp/s.out; die "status does not show backup health"; }
archiver status --json >/tmp/s.json || die "status --json failed"
python3 -c 'import json,sys; d=json.load(open("/tmp/s.json")); assert d["backup_health"]["state"]=="FAILING", d; assert d["services"]["/data/services/db"]["result"]=="skipped", d; assert "backup" in d["incidents"], d' \
  || { cat /tmp/s.json; die "status --json does not carry backup health"; }
archiver healthcheck >/tmp/hc.out 2>&1 || { cat /tmp/hc.out; die "the liveness healthcheck failed on a failed backup"; }

log "recovered: OK again"
rm "$SERVICES/db/pre-backup"
archiver backup >/tmp/b3.out 2>&1 || { cat /tmp/b3.out; die "the recovering backup failed"; }
archiver health --backups >/tmp/h3.out || { cat /tmp/h3.out; die "health is not OK after recovery"; }

echo "PASS: backup health"
