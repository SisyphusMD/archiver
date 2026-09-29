# 12. Offsite targets mirror local retention

- Status: Accepted
- Date: 2026-09-29

## Context

Each storage prunes with its own `-keep` policy at its own time. A target pruned hours after local deletes revisions local still holds, and the next copy re-sends them, because copy fills in every revision the target lacks. Duplicacy then discards that prune's fossil collection ("ghost snapshots"), leaving its fossils in no collection; only an exhaustive prune finds them again. Local reached 2.17 TiB of such orphans. Copy can select specific revisions only one snapshot ID at a time, which ADR 11 rules out.

## Options

1. Targets run no retention of their own; after each copy the worker deletes on the target exactly the revisions local has pruned.
2. Each target keeps its own retention (possibly longer) and a regular exhaustive prune reclaims the leak.

## Decision

Option 1. Local's retention policy is the only one.

## Consequences

- A target never deletes a revision local still holds, so copies never re-send one: no ghost snapshots and no orphaned fossils from this cause.
- Offsite history can be no longer than local's.
- Deleting offsite data is guarded: never an ID's newest revision (non-exclusive prune refuses it anyway), nothing when local's list is missing or failed to load, and no unusually large deletion without an explicit override.
- Target deletions run under the target's lock (ADR 11), after any pending copy, whenever local's revisions change; a local prune alone is enough to trigger them.
- The release that ships mirroring also stops maintenance from pruning targets with `-keep`, even while maintenance is still bash; otherwise targets keep pruning on their own and the race returns.
- Existing orphans on each storage are reclaimed once by an exhaustive prune.
