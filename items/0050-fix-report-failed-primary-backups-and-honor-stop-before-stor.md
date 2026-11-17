# #50 fix: report failed primary backups and honor stop before storage wrap-up

- Kind: pull, state: closed, by cody, created 2026-07-09T09:12:42-07:00
- Labels: none; milestone: none
- refs/pull/50/head -> main, merged: True

Fixes the two archiver findings from the 2026-07-04 fleet audit, both verified live before fixing.

**Critical: a failed primary backup reported success.** `$!` after the backgrounded `duplicacy | log_output &` pipeline is the log reader (last pipeline element), which exits 0 regardless of duplicacy. A failed backup logged "completed", notified success, and exited 0 (regression from the graceful-stop commit; pre-stop code checked `PIPESTATUS[0]`). Fixed with process substitution (`> >(log_output) 2>&1 &`): `$!` is duplicacy itself, `wait` returns its real status, and duplicacy stays a **direct child** of main.sh, which pause/resume/stop (`pkill -P LOCK_PID`) depend on. A subshell+pipefail fix would have made it a grandchild and silently broken all three.

**High: `archiver stop` during the final service backup ran the destructive wrap-up anyway.** The early `return 1` bypassed the per-service stop handler, so the run continued into check + `prune -exhaustive` + secondary copies and recorded a successful completion. main() now honors the stop flag after the service loop, before wrap-up; a stop is no longer counted as an error; and an empty `last_working_dir` (all services failed) skips wrap-up instead of pruning from whatever directory was left (`cd ""` is a silent no-op in bash).

Also: graceful stop now TERMs duplicacy directly (previously it killed the log reader and waited for duplicacy to die of a broken pipe — observed blocking 15s in the repro).

New deterministic regression tests (duplicacy shadowed by a wrapper that fails/blocks only the `backup` subcommand), both wired into CI: `backup-failure.sh`, `stop-skips-prune.sh`. Both FAIL on the pre-fix image and PASS post-fix. Full suite green: 6/6 existing integration, bats 36/36, shellcheck parity, pause/resume/stop verified live under the new process tree.
