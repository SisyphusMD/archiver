# 20. Hooks become executables

- Status: Accepted
- Date: 2026-10-01

## Context

Each service directory may hold `service-backup-settings.sh`, which the bash pipeline sources into its own shell: it sets the filter list and defines pre- and post-backup functions that share that shell's variables. The Go pipeline (ADR 10) has no shell to source it into, and ADR 4 says v1 never executes a configuration file as code.

## Options

1. Executable hooks: optional `pre-backup` and `post-backup` programs and a plain `filters` file per service, with a migration command.
2. Keep sourcing the file, through a bash helper the Go pipeline runs per service.
3. Both, with the sourced file deprecated during 1.x.

## Decision

Option 1.

## Consequences

- A hook is any executable the container can run; Archiver runs it in the service directory and reads its exit code. Hooks are not translated to Go.
- Hooks receive `ARCHIVER_SERVICE`, `ARCHIVER_SERVICE_DIR`, `ARCHIVER_SNAPSHOT_ID` and a per-run `ARCHIVER_STATE_DIR`; `post-backup` also receives `ARCHIVER_BACKUP_RESULT`. They never receive storage secrets or the RSA passphrase.
- Exit codes keep their meaning: a failed `pre-backup` skips that service and fails the run, and `post-backup` runs whenever `pre-backup` ran.
- `filters` holds duplicacy filter lines, one per line, as the sourced array did.
- `archiver migrate hooks` writes wrappers that source the old file (kept as `service-backup-settings.legacy.sh`) and call its functions, and writes `filters` from its array. It warns where post appears to read a variable set in pre, which no longer carries over.
- v1 refuses to start while any `service-backup-settings.sh` remains, and names the migration command.
- `restore-service.sh` moves to the same model as a `post-restore` executable.
