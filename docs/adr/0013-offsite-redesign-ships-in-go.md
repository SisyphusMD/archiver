# 13. The offsite redesign ships with the Go pipeline

- Status: Accepted
- Date: 2026-09-29

## Context

ADRs 11 and 12 change the backup pipeline, which ADR 10 moves to Go in its next step. Until then, a copy that runs past the next scheduled start still makes that backup refuse, silently. That happens occasionally, not nightly, and grows rarer as target listings shrink.

## Options

1. Build the copy worker and mirroring in the Go pipeline port, next.
2. Build a bash version in 0.11.x first, then rebuild it in Go.

## Decision

Option 1, plus a small bash change now: a notification whenever a scheduled backup is refused because a run still holds the lock.

## Consequences

- The design is written once, in the language suited to per-target state, retries, and cancellation, and held to the existing e2e contract tests.
- The concurrency model (backup, copy workers, maintenance, and their locks) is shown to Cody before the pipeline port is implemented.
- Until the port ships, a refused backup is reported instead of silent, and the monthly exhaustive prune keeps orphaned fossils in check.
