# #307 fix(copier): recount a busy target's behind count when a backup finishes

- Kind: pull, state: closed, by cody, created 2026-11-02T09:16:26-08:00
- Labels: none; milestone: none
- refs/pull/307/head -> main, merged: True

A backup that finished while a copy worker was mirroring, exhaustively pruning, copying or held by its copy window left the target's behind count (status, backup health, `archiver_copy_behind_revisions`) at its pre-backup value until that work ended. On the NAS, cowanserver read 0 behind while really 9 behind during a 7h mirror prune, so a copy-lag alert could not fire.

- A wake now recounts from a listing of the primary only (the target is not touched): still-missing revisions from the last listing plus everything local gained since.
- A listing that finds the target caught up clears the count; a pass that catches up does not zero a count a backup raised mid-pass.
- New gauge `archiver_copy_busy_since_timestamp_seconds{target,activity}` for alerting on a prune or copy that runs too long.
- Status lines show "N revisions behind" while busy.

Tests: TestBackupDuringPruneRaisesBehind (fails on main), TestCopyThenUpkeepNotBehind, TestIdleWakeLeavesCountToPass; full unit suite with -race; copier package 20x stable.
