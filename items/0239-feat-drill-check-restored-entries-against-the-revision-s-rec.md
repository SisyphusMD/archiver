# #239 feat(drill): check restored entries against the revision's record, and run the service's drill hook (ADRs 48, 49)

- Kind: pull, state: closed, by cody, created 2026-10-13T11:37:50-07:00
- Labels: none; milestone: none
- refs/pull/239/head -> main, merged: True

- After a drill restores a revision, every entry the revision records (duplicacy cat) must be in the copy with its type, size, mtime and mode (a link's target); nothing extra but duplicacy's top-level .duplicacy. Duplicacy has already verified content hashes. Replaces the file count (ADR 49).
- Then the service's executable `drill` hook (service dir or HOOKS_DIR, ADR 45 rules) runs in the restored copy with RestoreEnviron + ARCHIVER_DRILL_DIR; exit 0 passes; RESTORE_DRILL_HOOK_TIMEOUT (default 6h, paused time not counted); its whole process group is ended on exit, stop or timeout (ADR 48).
- doctor checks a drill hook like the others; backups warn about restore/drill hooks left in a service dir under HOOKS_DIR.

Tests: verify_test.go, drill_hook_test.go (pass/fail/timeout/unsafe/env/cwd/child cleanup/pause), restore-drill.sh cases for a passing and a failing hook; lint, race tests; restore-drill.sh, restore-commands.sh, hook-safety.sh on a local image. Depends on the ADR PR #238 for the ADR files.
