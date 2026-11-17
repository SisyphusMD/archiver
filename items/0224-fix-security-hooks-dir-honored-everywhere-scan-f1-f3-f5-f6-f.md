# #224 fix(security): HOOKS_DIR honored everywhere (scan F1, F3, F5, F6, F16)

- Kind: pull, state: closed, by cody, created 2026-10-11T09:06:46-07:00
- Labels: none; milestone: none
- refs/pull/224/head -> main, merged: True

Security scan 2026-10-11, findings F1, F3, F5, F6, F16: a service directory's owner could get a service-backup-settings.sh run as root despite HOOKS_DIR.

- migrate hooks with HOOKS_DIR set skips configured service directories holding the file unless named explicitly.
- The start-up refusal and the backup's skip message say to remove the file under HOOKS_DIR.
- recover without --hook never migrates (runs) a restored settings file; it warns. With --hook, HOOKS_DIR is kept, so the existing refusal applies.
- Tests: recover.sh plants the file in a revision and asserts a default recover never runs it; migrate-hooks.sh asserts migrate hooks, backup and start under HOOKS_DIR never run it.
