# 50. A scheduled deep check reads every chunk of a storage

- Status: Accepted
- Date: 2026-10-13

## Context

Maintenance's check confirms every chunk a revision needs exists; it does not read them,
so a chunk damaged in place goes unnoticed until a restore needs it (ADR 31's fault tests
showed this). Drills restore only a few services.

## Options

1. A deep check (`check -chunks`) for any storage, on its own schedule.
2. The local storage only.
3. None.

## Decision

Option 1. `STORAGE_TARGET_N_DEEP_CHECK` (a cron schedule, unset = off) has Archiver read
and verify every chunk of that storage. An interrupted check resumes where it stopped; a
completed one is forgotten, so the next check reads every chunk again (Duplicacy otherwise
skips chunks it verified before, and damage that came later would go unread). It gives way
to backups and resumes after them, and has a time limit.

## Consequences

- On local storage it costs disk reads; on offsite storage it downloads everything, so each
  storage chooses its own schedule (for example local monthly, offsite rarely or never).
- A damaged chunk raises an incident naming it and the revisions that need it.
