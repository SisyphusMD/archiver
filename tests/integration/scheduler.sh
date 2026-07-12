#!/usr/bin/env bash
# Proves supercronic (the Debian-cron replacement) actually executes scheduled jobs
# WITHOUT the SETGID capability — the guarantee that let Phase 2 drop SETGID from the
# documented cap set. Runs INSIDE the archiver image as root under `--cap-drop ALL`
# (only DAC_OVERRIDE/CHOWN/FOWNER added back, NO SETGID), e.g.:
#   docker run -i --rm --cap-drop ALL \
#     --cap-add DAC_OVERRIDE --cap-add CHOWN --cap-add FOWNER \
#     --entrypoint bash archiver:sched -s < tests/integration/scheduler.sh
#
# Debian's cron forks setgid to exec jobs, so without SETGID its jobs silently never
# run. supercronic runs jobs as the container user — if that assumption were wrong,
# the marker file below would never appear and this test fails.
#
# supercronic's granularity here is one MINUTE (this build supports neither `@every`
# nor a seconds field — a 6-field line is parsed as ordinary per-minute cron), so the
# job fires on the next minute boundary and the test waits up to ~70s for it.

set -uo pipefail
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

command -v supercronic >/dev/null 2>&1 || die "supercronic is not installed in the image"

# The exact schedule format the entrypoint writes must parse.
echo '0 3 * * * /opt/archiver/archiver.sh backup' > /tmp/real.crontab
supercronic -test /tmp/real.crontab >/dev/null 2>&1 \
  || die "supercronic rejected archiver's crontab format ('m h dom mon dow <cmd>')"

# An every-minute job whose only effect is to append to a marker file.
MARKER=/tmp/fired
CT=/tmp/smoke.crontab
rm -f "$MARKER"
echo '* * * * * echo fired >> /tmp/fired' > "$CT"
supercronic -test "$CT" >/dev/null 2>&1 || die "supercronic rejected an every-minute crontab"

echo ">>> starting supercronic under the current cap set (no SETGID); waiting for the next minute tick"
supercronic "$CT" >/tmp/supercronic.log 2>&1 &
sc_pid=$!

# Wait up to ~70s (one minute boundary + margin) for the first fire, breaking as soon
# as the marker lands.
for _ in $(seq 1 140); do
  [ -s "$MARKER" ] && break
  sleep 0.5
done

kill "$sc_pid" 2>/dev/null || true
wait "$sc_pid" 2>/dev/null || true

if [ ! -s "$MARKER" ]; then
  echo "--- supercronic log ---" >&2
  cat /tmp/supercronic.log >&2 || true
  die "supercronic did not execute the job within 70s (does it still need SETGID?)"
fi

echo "=== SCHEDULER OK: supercronic fired a scheduled job under cap-drop ALL, no SETGID ==="
