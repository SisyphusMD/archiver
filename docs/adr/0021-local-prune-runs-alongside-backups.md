# 21. Local prune runs alongside backups

- Status: Accepted
- Date: 2026-10-02
- Supersedes: the consequence "backups still finish before a prune starts" in [ADR 19](0019-local-prune-skips-revisions-in-use.md)

## Context

ADR 19 has the local prune skip revisions in use, and says backups still finish before a prune starts. Built that way, a prune waits for every backup in progress, including one held in a slow or hung pre-backup hook. That breaks the provisional contract that maintenance may overlap a backup: the e2e test holding a backup in its pre-backup hook never sees maintenance finish.

## Options

1. The prune runs alongside a backup, as maintenance does today.
2. The prune waits for any backup in its backup stages, and maintenance no longer completes during one.

## Decision

Option 1.

## Consequences

- Duplicacy's non-exclusive prune is designed for this: it never deletes an ID's newest revision, and a fossil is deleted only once every snapshot ID has backed up since it was collected.
- A long or hung hook never delays maintenance.
- The rest of ADR 19 stands: revisions a copy or restore is reading are still left for the next prune.
- The overlap contract stays as marked, and the e2e test keeps pinning it.
