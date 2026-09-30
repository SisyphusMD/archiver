# 18. Mirroring deletes automatically, guarded by rules

- Status: Accepted
- Date: 2026-09-30

## Context

ADR 12 makes each target delete the revisions local has pruned. The first pass after upgrading may delete many offsite revisions, since targets pruned on their own schedules until now. A retention step that waits for a person's approval gets switched off.

## Options

1. Mirroring runs automatically from the first pass, protected by fixed rules.
2. The first pass, or any large pass, waits for a person to approve its list.

## Decision

Option 1. No user approves deletions.

## Consequences

- A pass never deletes a snapshot ID's newest revision, and deletes nothing when local's revision list failed to load or lacks an ID entirely.
- It touches only this deployment's snapshot IDs, on targets this deployment maintains.
- A pass that would delete more than half of an ID's revisions is refused with an alert. That catches a bug, not ordinary retention; an intended large change, such as a shorter retention policy, goes through ADR 12's explicit override.
- Every deletion is logged. `archiver mirror --dry-run` shows what the next pass would delete, and nothing waits on it.
- Our own rollout dry-runs the first pass on nas and vps as release verification.
