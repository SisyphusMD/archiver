# 53. Named drill schedules for services that need their own

- Status: Accepted
- Date: 2026-10-13

## Context

A restore drill (ADR 28) rotates through services, from every storage it lists, on one
schedule. A service far larger than the rest (a whole object store, days to download) fits
badly: it is either drilled from every storage or not on a predictable schedule.

## Options

1. Named extra drill schedules.
2. Leave the service out of the rotation and drill it from an outside scheduler.
3. Let it rotate with the rest.

## Decision

Option 1. `RESTORE_DRILL_<NAME>_SCHEDULE` adds a named drill; its
`RESTORE_DRILL_<NAME>_SERVICES` lists the services it drills (always all of them), and
`RESTORE_DRILL_<NAME>_STORAGES` the storages: `all`, `primary`, names, or `alternate` (one
storage per run, in turn). `alternate:NAME,NAME` limits that rotation to the listed
configured storages in their given order. Changing the ordered list resets its persisted
cursor; a pending storage still in the list retries first. `RESTORE_DRILL_<NAME>_TIMEOUT`
sets its time limit. Services a named drill covers can be left out of the regular rotation
with `RESTORE_DRILL_EXCLUDE`.

## Consequences

- Each named drill is its own kind under ADR 52: due until it passes, time-limited, and its
  metrics, status and incidents are labelled with its name.
- Drill hooks (ADR 48) run in named drills as in any other.
