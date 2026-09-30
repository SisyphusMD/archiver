# 19. Local prune skips revisions still in use

- Status: Accepted
- Date: 2026-09-30

## Context

A local prune that deletes a revision a running copy is still sending fails that copy, which then repeats its whole destination listing (hours on SFTP). Waiting for copies to finish can delay a prune by up to a day; interrupting them wastes the listing. The only real conflict is a revision both want.

## Options

1. The prune runs on schedule and leaves out any revision a running copy or restore still needs.
2. The prune waits for running copies, and no new copy starts meanwhile.
3. The prune interrupts running copies.

## Decision

Option 1.

## Consequences

- Archiver takes the deletion set from duplicacy's own `prune -dry-run` with the retention policy, removes the revisions in use, and prunes the rest with explicit `-r`, one snapshot ID at a time. It never reimplements retention.
- A revision left out is pruned at the next run, costing only its space until then; in practice the set is almost always empty.
- Local pruning no longer needs local to itself; backups still finish before a prune starts.
- The in-use set must be exact, so it gets its own tests.
