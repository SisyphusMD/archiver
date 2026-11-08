# 54. The first kit drill does not wait for its schedule

- Status: Accepted
- Date: 2026-11-06

## Context

Catch-up on start covers interrupted runs only (ADR 46), so a drill that has never run waits
for its first slot. A monthly `KIT_DRILL_SCHEDULE` can leave the envelope's recovery path
unproven for weeks: the NAS's first slot was 2026-11-15, so its first kit drills were run by
hand on 2026-11-05. The kit drill takes seconds; a restore drill or deep check can take hours.

## Options

1. Leave every first run to its slot.
2. Run a never-run kit drill as soon as it can pass; other drills keep their slots.
3. Run every never-run scheduled drill at container start.

## Decision

Option 2. With `KIT_DRILL_SCHEDULE` set, the daemon starts a kit drill when an offsite storage
has no drill on record and every offsite storage holds the kit for the current configuration.
It checks at container start and after each successful scheduled backup, and runs beside
whatever else is running, so a stalled or paused drill never holds up a backup.

## Consequences

- The kit carries the settings, the schedule among them, so setting the schedule makes the
  placed kit stale. Checking at start alone would miss that case or fail on it; the backup
  that refreshes the kit is what lets the drill run.
- A fresh deployment, or a secondary still catching up, raises no false alarm: nothing runs
  until the kit has reached every offsite storage.
- A new secondary is drilled once it holds the kit.
- Any recorded result counts, including a failure: failed drills stay on their schedule (ADR
  52) and restarts do not repeat them.
- Restore drills share the lock, so an interrupted or running one goes first.
- Manual mode runs no scheduled drills, so nothing changes there.
