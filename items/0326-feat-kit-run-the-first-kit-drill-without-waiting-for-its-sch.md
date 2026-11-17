# #326 feat(kit): run the first kit drill without waiting for its schedule

- Kind: pull, state: closed, by cody, created 2026-11-06T17:55:51-08:00
- Labels: none; milestone: none
- refs/pull/326/head -> main, merged: True

ADR 54. A kit drill that has never run waited for its first slot, so a monthly schedule could leave the envelope path unproven for weeks.

With KIT_DRILL_SCHEDULE set, the daemon now starts one when an offsite storage has no drill on record and every offsite storage holds the current kit. It checks at container start and after each successful scheduled backup, on its own goroutine so a stalled or paused drill never holds up a backup. Restore drills and deep checks keep their first slots.

The kit carries the settings, the schedule among them, so setting the schedule makes the placed kit stale: tests/integration/kit-drill.sh covers that case, the first drill at start, and no repeat once every storage is drilled.
