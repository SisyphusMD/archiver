# #118 feat(cli): port status, healthcheck, and logs to Go (ADR 10 step 1)

- Kind: pull, state: closed, by cody, created 2026-09-29T16:02:43-07:00
- Labels: none; milestone: none
- refs/pull/118/head -> main, merged: True

ADR 10 step 1. Five commits: a read-only reader for the bash lock files and maintenance record, then status, healthcheck, logs, and the dispatcher wiring (ported commands run in Go when called with no arguments; anything else still goes to archiver.sh, so usage errors are unchanged).

- status and healthcheck: byte-identical output and exit codes to bash in four states (fresh, after a real backup, after maintenance, finished-with-error), checked in a container. The deliberate finished-but-failed-stays-healthy rule is kept; config.sh is read as plain assignments, never executed (ADR 4).
- logs: follows in-process; Ctrl+C exits in ~1 ms with 130 and leaves no background tail (backlog item), SIGTERM exits 143, follows log rotation, exits 0 when the backup ends. Drops tail's ==> file <== headers.
- CI Go job now covers internal/; Dockerfile copies internal/ into the build stage.

Local: go test -race green, every commit builds and tests on its own, 28/28 integration + 29/29 e2e on the built image, Codex review clean after fixes (SIGTERM status, logo newline, CHANGELOG wording); one Codex point rejected (sourcing config.sh would violate ADR 4; bundle mode has no live users).
