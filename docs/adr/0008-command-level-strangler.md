# 8. Migrate command by command behind a Go dispatcher

- Status: Accepted
- Date: 2026-09-28

## Context

ADR 6 chose Go and ADR 2 requires cross-version tests before rewrite code. The brief requires that every intermediate commit ships a working image. Old and new code will coexist for months, and they share state: the lock files (PID, stage, pause and stop events), the stop flag, and the log files.

## Options

1. Command level: a Go dispatcher becomes `archiver` and delegates every subcommand not yet ported to `archiver.sh`, which keeps its argument handling (such as `backup --detach`); subcommands move to Go in groups.
2. Bottom up: bash stays the orchestrator and calls small Go helpers (breaker, notify, config).
3. Parallel build: write v1 separately and switch over once.

## Decision

Option 1. The first Go commit is a dispatcher that delegates everything, so the image behaves exactly as before. ADR 10 sets the order in which subcommands move.

## Consequences

- The entrypoint calls bash directly today: its crontab, its SIGTERM stop, and its command fallback run `archiver.sh`, and `init` runs `lib/scripts/init.sh`. The dispatcher commit points every one of them at the dispatcher, so a ported command also covers scheduled runs, shutdown, and image-level `init`.
- Every commit runs the full suite against a hybrid image, and a subcommand moves only when its black-box tests pass.
- The circuit breaker, bounded concurrency, and alert deduplication arrive when the backup pipeline moves, not at the end.
- Subcommands that share state move together, or the Go side keeps the bash format until they do.
- Bash files are deleted only after the cross-version tests pass against the Go path (ADR 2).
