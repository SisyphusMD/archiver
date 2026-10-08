# 31. Fault injection is part of the test suite

- Status: Accepted
- Date: 2026-10-08

## Context

The contract says failures exit non-zero, notify, and never leave a bad revision as the
newest; it is tested with clean failures (a failing hook, an unreachable storage). Real
failures are messier: connections dropping mid-transfer, disks filling, processes killed,
storage bit rot.

## Options

Which faults CI injects (CI only, never against a real deployment): network drops; full
disks; killed processes; corrupted chunks.

## Decision

All four.

## Consequences

- Network: a proxy (toxiproxy) cuts or slows a storage connection mid-backup and mid-copy;
  the run fails or retries as designed, never reports success for a transfer that did not
  complete, and a retry that completes it, or the next run, leaves a restorable revision.
- Disk: the local storage, the logs volume and a restore target fill in turn; each gives a
  clear error and notification, no corrupted revision, and recovery once space returns.
- Processes: duplicacy, a hook, a copy worker and archiver itself are killed at chosen
  points; locks are cleaned up, nothing half-written becomes the newest revision, the next
  run proceeds.
- Corruption: changed or deleted chunks are reported by check and drills naming the damaged
  revisions; restoring one fails loudly, and undamaged revisions still restore.
