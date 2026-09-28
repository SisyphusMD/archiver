# 10. Port read-only commands first

- Status: Accepted
- Date: 2026-09-28

## Context

Under ADR 8, subcommands move from bash to Go one group at a time. Whichever side writes the lock files decides which format the other side must understand.

## Options

1. Read-only first: `status`, `healthcheck`, and `logs`, then the backup pipeline together with `stop`, `pause`, and `resume`.
2. Pipeline first: the backup pipeline first, for early v1 value, with Go writing the bash lock format until `stop`, `pause`, and `status` move.

## Decision

Option 1, in this order:

1. `status`, `healthcheck`, `logs`
2. The backup pipeline with `stop`, `pause`, and `resume`
3. Maintenance, restore, auto-restore, and snapshot-exists, then the recovery kit, `init`, and `migrate`
4. Delete the bash once nothing delegates to it

## Consequences

- Until step 2, Go only reads the bash lock format and never writes it.
- Step 1 fixes Ctrl+C in `archiver logs` (ADR 5) and gives the Go toolchain, CI, and image layout a low-risk first trip.
- Step 2 is the largest and riskiest move; it carries the circuit breaker, bounded concurrency, and the new lock format together.
- Maintenance stays in bash until step 3, so from step 2 until then `status` and `stop` also read and honor the bash maintenance lock and stop flag.
- Bundle commands are not ported: they are removed with bundle mode (ADR 4).
