# #232 fix(hooks): read the filters file after the pre-backup hook; migrate hooks warns about runtime filter changes

- Kind: pull, state: closed, by cody, created 2026-10-11T22:20:31-07:00
- Labels: none; milestone: none
- refs/pull/232/head -> main, merged: True

Found by the rc1 readiness review of the nas/vps upgrade. nas Garage's settings file switches DUPLICACY_FILTERS_PATTERNS to a btrfs snapshot's blocks inside its pre-backup function. v1 read filters before the pre hook ran and migration keeps only the starting list, so after upgrading Garage would silently back up live blocks.

- The pipeline reads the filters file again after a successful pre-backup hook (the early read stays, so an unreadable file still skips the service before any hook runs).
- migrate hooks warns, naming the function, when a settings file's functions reference DUPLICACY_FILTERS_PATTERNS.
- docs: hooks.md says when filters is read; upgrading.md covers read-only mounted settings files (convert where kept, mount pre-backup, post-backup, filters and the .legacy.sh) and scripts that sourced /opt/archiver/lib.

Tests: hook-failures.sh gains a pre hook that rewrites filters (fails on main, passes here); TestMigrateWarnsWhenFiltersChangeAtRuntime (fails without the warning). All 43 round-trip scripts pass locally.
