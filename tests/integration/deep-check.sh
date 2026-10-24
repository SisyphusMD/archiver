#!/usr/bin/env bash
# Run inside the image; all fixtures and repositories live in the container.
set -euo pipefail
log() { printf '>>> %s\n' "$*"; }
die() { printf 'FAIL: %s\n' "$*" >&2; cat /tmp/deep-check.out >&2 2>/dev/null || true; exit 1; }
export SERVICE_DIRECTORIES=/data/services/app/
export STORAGE_TARGET_1_NAME=local STORAGE_TARGET_1_TYPE=local STORAGE_TARGET_1_LOCAL_PATH=/store
export STORAGE_TARGET_2_NAME=offsite STORAGE_TARGET_2_TYPE=local STORAGE_TARGET_2_LOCAL_PATH=/offsite
export DUPLICACY_THREADS=1
DLOG=/opt/archiver/logs/deep-check-local.log
mkdir -p /opt/archiver/keys /run/secrets /store /offsite "$SERVICE_DIRECTORIES"
openssl genrsa -aes256 -passout pass:testpassphrase -out /opt/archiver/keys/private.pem -traditional 2048 2>/dev/null
openssl rsa -in /opt/archiver/keys/private.pem -passin pass:testpassphrase -pubout -out /opt/archiver/keys/public.pem 2>/dev/null
chmod 600 /opt/archiver/keys/private.pem
printf testpassword >/run/secrets/storage_password
printf testpassphrase >/run/secrets/rsa_passphrase

log 'deep checks never create a missing storage; invalid schedules refuse startup'
archiver deep-check local >/tmp/deep-check.out 2>&1 && die 'missing storage passed'
[ ! -e /store/config ] || die 'deep check created storage'
STORAGE_TARGET_1_DEEP_CHECK=invalid archiver daemon --check >/tmp/invalid.out 2>&1 && die 'invalid cron accepted'
grep -q STORAGE_TARGET_1_DEEP_CHECK /tmp/invalid.out || die 'invalid cron not named'

log 'back up enough chunks to interrupt during verification'
head -c 536870912 /dev/urandom >"$SERVICE_DIRECTORIES/data.bin"
archiver backup >/tmp/backup.out 2>&1 || { cat /tmp/backup.out; die 'backup failed'; }

log 'a clean secondary passes without a schedule, and clears progress'
archiver deep-check offsite >/tmp/deep-check.out 2>&1 || die 'clean offsite failed'
[ -z "$(find /opt/archiver/logs/.deep-check-repos -name verified_chunks)" ] || die 'completed check kept progress'
archiver status | grep 'Deep check of offsite: passed' >/dev/null || die 'status omitted result'

log 'pause a real check after verification starts; timeout sends SIGINT and saves progress'
DEEP_CHECK_TIMEOUT=8s archiver deep-check local >/tmp/deep-check.out 2>&1 &
pid=$!
seen=false
for i in $(seq 1 700); do
 if grep -q 'Verified chunk ' "$DLOG" 2>/dev/null; then seen=true; break; fi
 kill -0 "$pid" 2>/dev/null || break
 sleep 0.01
done
[ "$seen" = true ] || die 'check ended before verification was observed'
archiver pause >/tmp/pause.out
wait "$pid" && die 'timed-out check passed'
verified=$(find /opt/archiver/logs/.deep-check-repos -path '*/local/verified_chunks' -print -quit)
[ -n "$verified" ] && [ -s "$verified" ] || die 'interrupted check lost progress'
python3 - "$verified" <<'PY'
import json,sys
assert len(json.load(open(sys.argv[1]))) > 0, 'no verified chunks saved'
PY

log 'the next attempt skips verified chunks and clears progress on completion'
archiver deep-check local >/tmp/deep-check.out 2>&1 || die 'resumed check failed'
grep -q 'Skipped .* chunks that have already been verified' "$DLOG" || die 'check did not resume'
[ ! -e "$verified" ] || die 'completed check retained verified_chunks'

log 'damage in place is found on the next check and names the chunk in its incident'
chunk=$(find /store/chunks -type f -print -quit)
cp "$chunk" /tmp/original-chunk
id=$(printf '%s' "${chunk#/store/chunks/}" | tr -d '/')
python3 - "$chunk" <<'PY'
import sys
with open(sys.argv[1], 'r+b') as f:
 f.seek(100)
 b=f.read(1)
 f.seek(100)
 f.write(bytes([b[0]^0xff]))
PY
archiver deep-check local >/tmp/deep-check.out 2>&1 && die 'damaged storage passed'
grep -q "$id" /tmp/deep-check.out || die 'damaged chunk not named'
python3 - "$id" <<'PY'
import json,sys
s=json.load(open('/opt/archiver/logs/.incidents.json'))
i=s['deep-check:local']
assert sys.argv[1] in i['message'], i
PY
[ ! -e "$verified" ] || die 'finished failed check retained progress'

log 'a zero-length chunk fails and is named in the incident'
cp /tmp/original-chunk "$chunk"
archiver deep-check local >/tmp/deep-check.out 2>&1 || die 'repair before zero-length case failed'
: >"$chunk"
archiver deep-check local >/tmp/deep-check.out 2>&1 && die 'zero-length chunk passed'
grep -q "Chunk $id has a size of 0" /tmp/deep-check.out || die 'zero-length chunk not named'
python3 - "$id" <<'PYINCIDENT'
import json,sys
s=json.load(open('/opt/archiver/logs/.incidents.json'))
i=s['deep-check:local']
assert sys.argv[1] in i['message'], i
PYINCIDENT
[ ! -e "$verified" ] || die 'zero-length check retained progress'

log 'repair and pass clears the incident; no-argument selection uses schedules'
cp /tmp/original-chunk "$chunk"
STORAGE_TARGET_1_DEEP_CHECK='0 4 1 * *' archiver deep-check >/tmp/deep-check.out 2>&1 || die 'repaired storage failed'
python3 <<'PY'
import json
s=json.load(open('/opt/archiver/logs/.incidents.json'))
assert 'deep-check:local' not in s or s['deep-check:local'].get('recovered'), s
PY
grep -q 'archiver_deep_check_last_ok{storage="local"} 1' /opt/archiver/logs/archiver.prom || die 'metrics omitted pass'
printf 'PASS: deep storage checks\n'
