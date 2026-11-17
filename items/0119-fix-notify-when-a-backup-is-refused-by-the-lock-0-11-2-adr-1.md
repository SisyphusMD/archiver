# #119 fix: notify when a backup is refused by the lock (0.11.2, ADR 13 stopgap)

- Kind: pull, state: closed, by cody, created 2026-09-29T16:24:47-07:00
- Labels: none; milestone: none
- refs/pull/119/head -> release/0.11, merged: True

ADR 13 stopgap until the Go copy worker ships: a scheduled backup refused because the previous run (usually a long offsite copy) still holds the lock now sends a notification with the holder's PID, stage, and start time. On 2026-09-28 the NAS skipped a backup this way and the only trace was a docker logs line.

Test first: TestRunsRefusedBackupNotifies fails on 0.11.1 (no notification in 30s) and passes with the fix. The e2e runner also gets an explicit 30-minute timeout (go test's 10-minute default failed the suite on a busy runner; same fix is in #118 for main).

Local on the built image: 28/28 integration, 30/30 e2e. Codex: one point declined (--detach refusals are interactive, so the person sees them; scheduled runs use the synchronous verb covered here).

Port to main after merge.
