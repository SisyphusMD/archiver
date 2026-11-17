# #244 fix: on/off settings parse false-like values as off; drill hooks get a grace period on timeout or stop

- Kind: pull, state: closed, by cody, created 2026-10-13T18:41:08-07:00
- Labels: none; milestone: none
- refs/pull/244/head -> main, merged: True

Found by the rc1 readiness review of the NAS upgrade.

- On/off environment settings (DRY_RUN, RUN_RESTORE_SERVICE, DELETE_EXTRA, HASH_COMPARE, OVERWRITE and the rest) were on for any non-empty value, so DRY_RUN=false meant a dry run. One shared parser: true/1/yes/on on, false/0/no/off/empty off, anything else refuses with a clear error. docs/upgrading.md lists the change.
- A drill hook that times out or is stopped now gets SIGTERM to its process group and RESTORE_DRILL_HOOK_GRACE (default 60s) to clean up (containers it started) before SIGKILL; pause cannot freeze that cleanup; a paused hook is resumed before SIGTERM. docs/hooks.md: trap TERM and clean up.

Tests for both (fail before, pass after); full race suite, lint, restore integration scripts, drill hook timeout/stop cleanup. Two Codex review rounds. Committed together because they share the config, docs and CHANGELOG hunks.
