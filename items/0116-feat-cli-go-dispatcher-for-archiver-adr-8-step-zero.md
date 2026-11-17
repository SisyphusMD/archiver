# #116 feat(cli): Go dispatcher for archiver (ADR 8 step zero)

- Kind: pull, state: closed, by cody, created 2026-09-29T08:08:28-07:00
- Labels: none; milestone: none
- refs/pull/116/head -> main, merged: True

First Go code in the image. `/usr/local/bin/archiver` becomes a static Go binary (cross-compiled in a pinned golang stage, CGO off) that execs `archiver.sh` for every command, or `lib/scripts/init.sh` for `init` (archiver.sh has no init verb). exec replaces the process, so signals, the TTY, and exit codes pass through unchanged. The entrypoint's crontab, SIGTERM stop, run mode, and init now call the dispatcher (ADR 8 consequence), so a ported command will also cover scheduled runs and shutdown.

Small visible changes: usage says `archiver` instead of the script path; `archiver init` works inside a running container.

This is main's first change past 0.11 (ADR 9): from here 0.11 patches come from `release/0.11`.

Second commit: a Go CI job (gofmt, go vet, go test -race) that publishing waits on.

Local: 28/28 integration and 29/29 e2e on the built image, invalid-cron + scheduler rerun after updating their crontab assertions (a real Codex catch), Go job dry-run green, Codex review clean.
