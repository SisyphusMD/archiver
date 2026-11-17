# #125 feat: schedule runs with a Go daemon instead of supercronic

- Kind: pull, state: closed, by cody, created 2026-09-30T21:47:53-07:00
- Labels: none; milestone: none
- refs/pull/125/head -> main, merged: True

ADR 10 step 2, first part (ADR 14). archiver daemon replaces supercronic, using supercronic's own cronexpr parser and its no-overlap/skip-on-overrun timing; still runs the bash backup and maintenance commands. Also routes Go modules for the unit-test job and e2e runner through the NAS cache.
