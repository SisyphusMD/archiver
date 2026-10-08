# 28. Restore drills are opt-in and rotate through services

- Status: Accepted
- Date: 2026-10-08

## Context

A backup is only proven by a restore. Archiver can restore by itself on a schedule and
report the result, but a restore needs scratch space and downloads data (egress on
offsite storages), which not every deployment has to spare.

## Options

1. Weekly rotating sample, on by default.
2. Monthly full restore of everything.
3. Verify without restoring (`check -files`).
4. Off by default; opt in with a schedule.

## Decision

Option 4, rotating like option 1 and configurable:
`RESTORE_DRILL_SCHEDULE` (unset = off), `RESTORE_DRILL_SERVICES` (1 by default, rotating
per storage, or `all`), `RESTORE_DRILL_STORAGES` (`all`, `primary` or names),
`RESTORE_DRILL_DIR` (default `/tmp/archiver-drill`), `RESTORE_DRILL_EXCLUDE`.

## Consequences

- A drill restores a service's newest revision into the drill directory, with duplicacy
  verifying every chunk and the revision held in use, checks the file count against the
  revision's listing, then deletes the copy. Restore hooks never run.
- Too little free space skips the service with a warning; it does not fail the drill.
- A failed drill always notifies; status shows the last drill per service and storage; the
  healthcheck flags a failure, or no passing drill run at all within twice the schedule's
  interval (a check on the runs, since rotation drills each service less often).
- Drills run alongside backups; stop and pause act on them; `archiver drill` runs one now.
