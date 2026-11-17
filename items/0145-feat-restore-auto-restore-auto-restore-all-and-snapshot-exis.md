# #145 feat: restore, auto-restore, auto-restore-all and snapshot-exists in Go (ADR 10)

- Kind: pull, state: closed, by cody, created 2026-10-05T13:15:33-07:00
- Labels: none; milestone: none
- refs/pull/145/head -> main, merged: True

ADR 10 step 3, restore: `archiver restore`, `auto-restore`, `auto-restore-all` and `snapshot-exists` now run in Go (`internal/restore`).

- Same interface nas, vps and the drills script against: `SNAPSHOT_ID`, `LOCAL_DIR`, `REVISION`, `STORAGE_TARGET`, the restore toggles, `RUN_RESTORE_SERVICE`, exit codes 0/1/2/3, `EXISTS`/`NOT FOUND`/`UNDETERMINED`. `tests/integration/restore-commands.sh` pins it; its first part passes on the bash image too.
- Restore hook (ADR 20): an executable `post-restore` receiving `ARCHIVER_SERVICE`, `_SERVICE_DIR`, `_SNAPSHOT_ID`, `_RESTORE_REVISION`, `_RESTORE_STORAGE`; without one, `restore-service.sh` runs as before (`bash restore-service.sh`, with `SNAPSHOT_ID`/`LOCAL_DIR`). Hooks no longer get storage credentials. A failed hook is exit 1, no longer the hook's own code (which could read as 2 or 3).
- A restored `service-backup-settings.sh` in a configured service directory is migrated when executable hooks put the deployment on the Go pipeline; scratch restores (drills) and bash deployments are left as backed up.
- Restores validate only storage settings (`Config.ValidateStorage`), so a bare recovery container needs no `SERVICE_DIRECTORIES` or notifier.
- duplicacy inherits the in-use registration's lock, so a killed restore still protects its revision; `auto-restore-all` re-checks for a backup per service; interactive restore refuses during a backup, before and after the questions.

Local: unit tests, the restore-touching integration tests (s3, sftp included), full e2e suite.
