#!/usr/bin/env bash
# Proves the daemon actually runs scheduled jobs under the documented cap set, with no
# SETGID (Debian's cron needed it to fork jobs; the daemon runs them as the container
# user). Runs INSIDE the archiver image as root under `--cap-drop ALL`, e.g.:
#   docker run -i --rm --cap-drop ALL \
#     --cap-add DAC_OVERRIDE --cap-add CHOWN --cap-add FOWNER \
#     --entrypoint bash archiver:sched -s < tests/integration/scheduler.sh
#
# A seven-field schedule has a seconds field, so the job fires within seconds. With no
# configuration the backup itself fails, which is fine: the point is that it was started,
# and that a run is reported with its exit status.

set -uo pipefail
die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

archiver daemon --check || die "--check failed with no schedules set (nothing to validate is valid)"
BACKUP_SCHEDULE='0 3 * * *' archiver daemon --check || die "the daemon rejected a plain five-field schedule"
BACKUP_SCHEDULE='not a cron line' archiver daemon --check 2>/dev/null && die "the daemon accepted a malformed schedule"

LOG=/tmp/daemon.log
echo ">>> starting the daemon with an every-2-seconds backup schedule"
BACKUP_SCHEDULE='*/2 * * * * * *' archiver daemon >"$LOG" 2>&1 &
pid=$!

for _ in $(seq 1 60); do
  grep -qs 'archiver daemon: backup exited' "$LOG" && break
  sleep 0.5
done

kill -TERM "$pid" 2>/dev/null
for _ in $(seq 1 60); do kill -0 "$pid" 2>/dev/null || break; sleep 0.5; done
kill -0 "$pid" 2>/dev/null && die "the daemon did not exit on SIGTERM"
wait "$pid"; code=$?

grep -q 'archiver daemon: starting backup' "$LOG" || { cat "$LOG" >&2; die "the daemon never started the scheduled backup"; }
grep -q 'archiver daemon: backup exited' "$LOG" || { cat "$LOG" >&2; die "the daemon did not report the run's exit status"; }
[ "$code" -eq 0 ] || die "the daemon exited $code on SIGTERM, want 0"

cat "$LOG"
echo "=== SCHEDULER OK: the daemon fired a scheduled job under cap-drop ALL, no SETGID, and stopped cleanly ==="
