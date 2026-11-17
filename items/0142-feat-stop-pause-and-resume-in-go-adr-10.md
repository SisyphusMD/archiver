# #142 feat: stop, pause and resume in Go (ADR 10)

- Kind: pull, state: closed, by cody, created 2026-10-05T07:59:08-07:00
- Labels: none; milestone: none
- refs/pull/142/head -> main, merged: True

Pause now freezes each child process group (hook subprocesses included) and records the pause first; the Go pipeline starts no program while paused (the TestRunsPauseResume CI flake: a pause between programs let the next run unpaused). Stop on a Go backup sets the flag and the pipeline ends itself like on a signal: running duplicacy and the kit step end, no copy retry, post hooks run, real error count reported; a stop before a pre hook skips both hooks; no copy handoff after a stop, and stop waits for the backup before its final worker stop. Bash-owned backups (legacy hooks) keep the external stop. Lock-file edits are serialized so a pause appended during a stage change is not lost. New tests: pause-tree (fails on main), stop-go.
