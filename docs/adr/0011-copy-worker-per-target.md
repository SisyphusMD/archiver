# 11. Offsite copies run in a worker per target

- Status: Accepted
- Date: 2026-09-29

## Context

A backup holds one lock through the local backup and every offsite copy. The local backup takes about an hour; copies take 3 to 20 hours, so a slow copy made the next scheduled backup refuse to start (2026-09-28). An ISP outage failed both copies within seconds and nothing retried until the next night. Every `duplicacy copy` first lists all chunks on its destination: about 5 minutes on Backblaze, 4.5 to 6.3 hours on the SFTP target. Chunk writes are atomic, an interrupted copy is safe to rerun, and copy writes snapshot files only after all chunks.

## Options

1. A worker per target that copies whenever local has something new and retries with backoff until caught up.
2. Copies on their own schedule (`COPY_SCHEDULE`) with a few retries.

Within option 1: one copy of all snapshots per run, or one copy per snapshot ID.

## Decision

Option 1, one copy of all snapshots per run. The backup lock covers only the local backup. Each target has its own worker and lock; targets copy in parallel with each other, one copy at a time per target, and a failed copy retries with growing delays until the target has caught up.

## Consequences

- A slow or unreachable target can no longer block a local backup.
- One destination listing per run; per-snapshot copies would multiply it by the number of snapshot IDs (about 85 hours on the SFTP target).
- Nothing lands on a target until its whole run completes; an interrupted run keeps its uploaded chunks and repeats only the listing.
- The worker records which local revisions each target mirrors and acts whenever local's set changes, by a new backup or by a prune, so checking for work costs no listing.
- Every prune on a target, normal or exhaustive, takes that target's lock, so none overlaps a copy into it.
- Target health (down, catching up, recovered) is tracked per target and feeds the Phase 6b alerts.
