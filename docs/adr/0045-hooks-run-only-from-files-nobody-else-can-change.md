# 45. Hooks run only from files nobody else can change

- Status: Accepted
- Date: 2026-10-08

## Context

The early security pass (ADR 42) found that whoever can write a service directory can make
the backup run code as root, which can read every storage credential and the RSA key: an
executable hook there, or Duplicacy's own `.duplicacy/scripts`, which Archiver never turned
off. Requiring root-owned hooks would break the common setup where one user owns both the
data and its hooks, and Cody asked that the fix not make setup meaningfully harder.

## Options

1. Only root-owned hooks run (sshd's StrictModes with an owner check).
2. Refuse only files plainly open to others, plus an optional hooks directory outside the data.
3. Document the risk.

## Decision

Option 2.

## Consequences

- Every duplicacy command runs with `-no-script`.
- A hook (`pre-backup`, `post-backup`, `post-restore`, `restore-service.sh`) is refused, its
  service failing with an error that names the fix, when the file or any directory above it
  is writable by group or others; a sticky directory such as `/tmp` is fine. A symlinked
  hook's target is checked the same way. The owner is not checked. `filters` is not checked:
  it cannot run code.
- `HOOKS_DIR` (optional) holds `<service>/<hook>` outside the backed-up data; when set, hooks
  come only from there and any in a service directory are ignored with a warning.
- Deployments with group-writable hooks or service directories (`UMASK=002`) must
  `chmod go-w` them or use `HOOKS_DIR`; the upgrade notes say so. An app that owns its data
  directory can still plant a hook there; the README points such setups at `HOOKS_DIR`.
