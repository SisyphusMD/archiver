# #255 fix(pause): pause and program starts share one lock (the e2e flakes item)

- Kind: pull, state: closed, by cody, created 2026-10-17T15:17:40-07:00
- Labels: none; milestone: none
- refs/pull/255/head -> main, merged: True

Item: the known flakes TestRunsPauseResume and TestRunsRefusedBackupNotifies.

Evidence (Forgejo Actions logs, all black-box failures since 2026-09-29):
- TestRunsRefusedBackupNotifies failed on main once (2026-10-07, task 104176): a single TLS handshake timeout to the fake notifier lost the alert. Fixed that night by 0ea32e9 (transient notification failures are retried). Its earlier stall (2026-10-04, task 97523) was on release/0.11 (bash).
- TestRunsPauseResume failed only on v0.11.4 (bash, task 97766); the bash pause gap was fixed in 0.11.6.
- Neither has failed on main since; the other black-box failures were image-build or network errors.

Residual race fixed here: archiver pause recorded the pause, signalled, slept 300 ms and signalled again to catch a program the pipeline started between its pause check and the start. A busy host can outrun 300 ms, leaving that program running while the backup reports paused. Now the pause (runlock.Pause, as the drill already uses) and every program start (duplicacy and hooks, runlock.(*Lock).Unpaused) hold the lock-file edit lock: a start either finishes first and is stopped, or sees the pause and waits. A pre-backup hook held back by a pause and then stopped does not start (Codex review); post-backup hooks still do.

Tests: TestUnpausedExcludesPause, TestStopDuringPauseSkipsOnlyThePreHook (fails without the skip); go test -race ./cmd/... ./internal/...; golangci-lint 0 issues; e2e on the bench daemon: the two tests 20x each (40/40) and, after the review fix, all TestRuns* 3x plus the two 10x each (68/68).
