# 51. A kit drill rehearses recovery from the envelope alone

- Status: Accepted
- Date: 2026-10-13

## Context

Recovery after losing the host starts from the break-glass envelope (`archiver envelope`): its
credentials fetch the recovery kit from an offsite storage and the recovery password
decrypts it. Nothing checks on a schedule that this path still works; doctor only sees
that a kit exists.

## Options

1. A built-in kit drill.
2. Leave it to deployments, as a drill hook.
3. None.

## Decision

Option 1. `KIT_DRILL_SCHEDULE` (unset = off) fetches the kit from each offsite storage with
only the credentials the envelope prints (`STORAGE_TARGET_N_BREAKGLASS_*`, else the
backup credential it falls back to), decrypts it with stock `openssl`, and requires every
expected entry, the same content as the local kit and secrets still current.

## Consequences

- A rotated credential, a stale or missing kit, or an envelope that no longer matches fails
  the drill.
- Deployment-specific checks of the kit's extras (the NAS's git bundles) belong in a drill
  hook.
- OneDrive (personal and business) cannot be reached with the envelope alone (its token
  changes as it is used, so the envelope tells a person to sign in): the drill fetches the
  kit there with Archiver's own token instead, proving the kit is present, current and
  decrypts, and status notes that the sign-in step is left to a person.
