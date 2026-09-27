# 5. Refactor backlog scope

- Status: Accepted
- Date: 2026-09-26

## Context

Cody's earlier notes listed six backlog items. The Phase 1 audit checked each one against the code.

## Options

Keep or drop each item individually.

## Decision

Kept on the refactor backlog:

- `archiver stop` reports the wrong error count (still present)
- Ctrl+C does not exit `archiver logs` (still present)
- a host-side `archiver` command for Synology and Podman (not started)
- Podman parity, including socket-based hooks, with CI coverage (partial today)

Closed: `PRUNE_EXHAUSTIVE_FREQUENCY`, which shipped in 0.10.0 and is covered by `tests/integration/maintenance.sh`.

Open: overlapping runs, pending an interview about what actually happened.

## Consequences

- The first two are fixed in v1 by design, not in the bash patch release (ADR 3).
- Overlapping runs gets its own ADR once it is decided.
