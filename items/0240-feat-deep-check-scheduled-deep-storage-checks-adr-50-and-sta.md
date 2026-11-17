# #240 feat(deep-check): scheduled deep storage checks (ADR 50), and standalone maintenance honours in-use revisions on secondaries

- Kind: pull, state: closed, by cody, created 2026-10-13T12:29:49-07:00
- Labels: none; milestone: none
- refs/pull/240/head -> main, merged: True

Two commits.

1. fix(maintenance): maintenance run without the copy workers pruned secondary storages directly, ignoring the in-use registry, so a restore or drill reading a secondary could lose its revision. It now takes deletions from a dry run and skips revisions in use, reusing the primary's filtered prune.
2. feat(deep-check): STORAGE_TARGET_N_DEEP_CHECK (cron, unset = off) and `archiver deep-check [STORAGE]` run `check -chunks` from a repository of its own on the logs volume. verified_chunks is kept across an interruption and cleared once a check completes (Duplicacy otherwise skips chunks it verified before). A starting backup interrupts it (SIGINT, progress saved) and it stays due; DEEP_CHECK_TIMEOUT (default 7d); stop and pause act on it; shutdown waits for it to save progress; the revision hold and run lock pass to the duplicacy child. Damage raises an incident naming the chunks (zero-length ones included); a pass clears it. Metrics per storage: archiver_deep_check_last_timestamp_seconds, _last_ok, _last_duration_seconds, _last_success_timestamp_seconds; status shows the last check.

Tests: unit tests for config, progress clearing, metrics, incidents, shutdown, the child-held locks; tests/integration/deep-check.sh (clean pass, a corrupted and a zero-length chunk named, resume vs clear) added to CI; maintenance, restore-drill, prune-in-use and copy-workers scripts rerun. Lint and race tests clean. Three Codex review rounds; findings fixed.
