# #200 feat: notifications per incident, ALERT_REPEAT_INTERVAL, recovery notices, notify test (ADRs 33, 36)

- Kind: pull, state: closed, by cody, created 2026-10-09T15:05:10-07:00
- Labels: none; milestone: none
- refs/pull/200/head -> main, merged: True

ADRs 33 and 36.

- notify.Raise/Clear over logs/.incidents.json (flocked; shared by backup, maintenance, drills, kit, copy workers). An incident notifies when it opens, again only every ALERT_REPEAT_INTERVAL (default 24h, 0 never), and once when it clears, to the destinations its alert reached. Delivery is at least once: owed destinations are persisted before sending and struck off on arrival; a send claims the incident (5 min lease) so processes do not send it twice or out of order; a clear during an in-flight alert is queued for its sender; newly configured destinations are told of open incidents.
- Logging no longer notifies per ERROR line: each run sends one summary (first 10 error lines + count). Notifier failures never count as the run's failure (Reportable()).
- Keys: backup, backup-skipped, maintenance, kit (own incident; ended-at-deadline raises it), drill + drill:<storage>:<service> (re-raised each run until passing), copy:<target> (replaces Storage Down/Still Down/Recovered and the hardcoded 24h Remind), check:<target> (re-raised each pass), mirror:<target> (problem), log:<name> (unwritable, hourly while failing; clears when writable).
- Pushover messages clipped to 1024 chars, ntfy to 4096 bytes.
- `archiver notify test` (second commit).
- tests/integration/incidents.sh; all 47 integration scripts + e2e pass locally. 17 Codex review rounds; rejected: keeping an undelivered recovery after the incident recurs (a stale "recovered" after a newer failure misleads), and parsing the never-released one-line services format.
