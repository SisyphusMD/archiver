# #172 feat: services back up two at a time by default

- Kind: pull, state: closed, by cody, created 2026-10-08T04:26:18-07:00
- Labels: none; milestone: none
- refs/pull/172/head -> main, merged: True

Phase 4 leftover, per Cody's decision (2026-10-08): default 2, `BACKUP_PARALLELISM` configurable, 1 keeps today's one-by-one order.

- Each service runs its pre-backup hook, backup and post-backup hook. Up to `BACKUP_PARALLELISM` services run at once.
- Directories sharing a snapshot ID (listed twice, or two with one name) and one directory under two names (a symlink) go one after another, in order. Two backups of one snapshot ID never run at once (never-break), and two never share one `.duplicacy` repository at once.
- With 1, everything runs in the configured order exactly.
- After a stop, no further service starts; running ones end as before (new e2e test).
- Inline copies still run from the last successful service in configured order.
- Shared pipeline state (old preferences per service, failing-target list) is behind the mutex.
- Notifications: sends are serialized, so concurrent alerts all arrive instead of the second being dropped. The anti-loop guard moved to the source: send outcomes are logged with the new `logging.Log.Unnotified`, so a failed send never notifies.
- The setting is recorded in the recovery kit (`IsSetting`) and validated (>= 1).
- Docs: README performance section, env example, CHANGELOG.

Tests: e2e `TestRunsServicesInParallel` (two hooks meet at a barrier by default; with 1 they never overlap) and `TestRunsStopStartsNoMoreServices`; unit tests for grouping, the setting, concurrent notifications and the no-loop wiring. Three integration scripts whose subject is one-by-one order now pin `BACKUP_PARALLELISM=1`. Locally green: TestRuns, ExitStop, Hooks and CrossVersion e2e, plus all 27 round-trip integration scripts.

Review (Codex) found and I fixed: same-snapshot-ID and same-repository concurrency, serial-mode order, and dropped concurrent notifications.
