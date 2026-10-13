# 49. A drill checks every restored file against the revision's record

- Status: Accepted
- Date: 2026-10-13
- Supersedes: the file-count check in [ADR 28](0028-restore-drills-opt-in-rotating.md)

## Context

ADR 28's drill compares only the number of restored files. Duplicacy already verifies
every restored file's content against the hash the revision records, so content is
covered; names, sizes, times, permissions and owners are not.

## Options

1. Check each restored entry against the revision's own record.
2. Restore the revision from two storages and compare the copies (the NAS's old drill).
3. Keep the file count.

## Decision

Option 1. After the restore, every entry the revision lists must exist in the scratch copy
with its type, size, modification time and mode, and its owner and group when the drill
restores with ownership.

## Consequences

- One restore per storage, as before; a mismatch fails the drill and names the entries.
- Option 2's second download, which mostly repeated Duplicacy's own hash check, is dropped.
