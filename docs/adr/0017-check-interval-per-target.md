# 17. Each target is checked on its own interval

- Status: Accepted
- Date: 2026-09-30

## Context

A check lists every chunk on its storage. That takes minutes on local and object storage but 5 to 6 hours on the SFTP target, where a daily check occupies its worker for a quarter of the day. The right frequency depends on the storage, not on one global schedule.

## Options

1. A check interval per target, with defaults by storage type and a global default.
2. One global check schedule for every storage.

## Decision

Option 1: `STORAGE_TARGET_N_CHECK_INTERVAL`, falling back to a global default, falling back to the type default: 1 day for local, B2 and S3, 7 days for SFTP.

## Consequences

- A target's check runs in its worker when due and the worker is otherwise idle, so it never delays a copy.
- `status` warns when a storage's last successful check is more than twice its interval old.
- The primary's check stays in local maintenance.
- New storage types in Phase 5 each declare a default interval.
