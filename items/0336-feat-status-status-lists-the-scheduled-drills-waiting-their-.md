# #336 feat(status): status lists the scheduled drills waiting their turn

- Kind: pull, state: closed, by cody, created 2026-11-13T09:19:59-08:00
- Labels: none; milestone: none
- refs/pull/336/head -> main, merged: True

Since #334 a scheduled drill waits for the running one, but only the daemon log said so. `archiver status` now prints `Drills waiting their turn: ...` and `status --json` carries `drills_waiting`. The daemon keeps the list in the lock directory, written atomically and cleared at its start.

tests/integration/restore-drill.sh checks status while a drill waits and after a stop.
