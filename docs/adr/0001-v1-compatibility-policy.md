# 1. v1.0.0 compatibility policy

- Status: Accepted
- Date: 2026-09-26

## Context

The ground-up rewrite needs a rule for what it may change. The Phase 1 audit found many behaviors that users or tests could depend on: command names, log wording, the lock-file format, bundle mode, sourced-bash hooks, and internal paths. Treating all of them as fixed would force the rewrite to reproduce bash internals, bugs included.

## Options

1. Keep every user-visible behavior, or ship a migration for each one.
2. Release the rewrite as v1.0.0 with a short never-break list, and let everything else change with a migration command and an upgrade guide.
3. Allow any change and record it only in the CHANGELOG.

## Decision

Option 2. The rewrite ships as v1.0.0, and breaking changes are welcome wherever they make Archiver better.

Never break:

- existing storages
- snapshot IDs
- encryption keys
- restoring old snapshots
- recovering the recovery kit with plain `openssl`

Anything else may change if it ships with a migration command and an upgrade guide. Sourced-bash hooks (`service-backup-settings.sh`) may be replaced by executable hooks that receive environment variables and whose exit codes count.

## Consequences

- The never-break list is proven by tests that cross versions: data written by the bash releases must be read and continued by v1.
- Every other change ships with a migration command, an upgrade guide, and a CHANGELOG entry under Breaking.
- Log wording, the lock-file format, internal paths, and command names are free to change.
- Each deployment (nas, vps, do-volume-ops) upgrades deliberately, following the guide.
