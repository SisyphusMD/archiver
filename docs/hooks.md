# Hooks

Per-service scripts that run around a backup or a restore, and the filters that choose what is backed up.

## Custom Service Scripts

Any service directory may hold up to three optional files:

- `filters`: [Duplicacy include/exclude patterns](https://forum.duplicacy.com/t/filters-include-exclude-patterns/1089), one per line, first match wins. Without it, everything is backed up.
- `pre-backup`: an executable run before the backup, for example to dump a database.
- `post-backup`: an executable run after it, for example to clean up or restart what `pre-backup` stopped.

```bash
# filters
+backup.sql
+data/
+data/*
+filters
+pre-backup
+post-backup
-*
```

```bash
#!/bin/bash
# pre-backup (chmod +x)
docker exec postgres-container pg_dump -U user dbname > backup.sql
```

```bash
#!/bin/bash
# post-backup (chmod +x)
rm -f backup.sql
```

A hook can be any program the container can run (most are shell scripts). It runs in the service directory, and its output goes to the Archiver log; a line starting `[ERROR] ` or `[WARNING] ` is logged at that level, and an `[ERROR]` line counts as an error of the run (with a notification) without skipping the service. A background process a hook starts must redirect its own output: Archiver stops reading a hook's output two seconds after the hook exits. It receives `ARCHIVER_SERVICE`, `ARCHIVER_SERVICE_DIR`, `ARCHIVER_SNAPSHOT_ID`, and `ARCHIVER_STATE_DIR`, a scratch directory shared by that run's `pre-backup` and `post-backup` (to pass a value from one to the other; it lives on the logs volume, so it is for small values, not dumps). `post-backup` also receives `ARCHIVER_BACKUP_RESULT`: `success`, `failed`, `skipped`, `stopped`, or `interrupted` (run when the container next starts, after a crash or a kill left the service between its hooks; see [Interrupted runs](installation.md#interrupted-runs)). Hooks never receive storage credentials or the RSA passphrase.

Exit codes count. If `pre-backup` exits non-zero (the dump above failing, say), that service is **not** backed up that run: its newest revision stays the last good one instead of one holding a broken dump, and the run reports an error while every other service still backs up. `post-backup` always runs once `pre-backup` has, even after a failed `pre-backup`, a failed backup, or a stop, so it can restart whatever `pre-backup` stopped; a non-zero exit from it is reported as an error. A hook file that exists but is not executable is an error, and that service is skipped.

**Who can change a hook.** Hooks run as root in the container, which can read every storage credential and the RSA key. So a hook runs only if nobody but its owner can change it: the hook file and every directory above it must not be writable by group or others, except a sticky directory such as `/tmp` (for a symlinked hook, its target's path is checked the same way). Otherwise the service is skipped with an error that names the fix (`chmod go-w <path>`). The owner is not checked, so a service's own user may keep its hooks; but anyone who can write a service directory can then make the backup run code as root. If an app (or a user) you trust less than Archiver owns a service directory, keep that service's hooks in `HOOKS_DIR` instead: a directory outside the backed-up data, mounted read-only, holding `<service>/pre-backup`, `<service>/post-backup` and `<service>/post-restore`, where `<service>` is the service directory's name. With `HOOKS_DIR` set, hooks come only from there, and any left in a service directory are ignored with a warning. Duplicacy's own `.duplicacy/scripts` never run.

**Upgrading from `service-backup-settings.sh`:** convert once with `archiver migrate hooks`. For every configured service it writes `pre-backup` and `post-backup` wrappers that call your existing functions, writes your `DUPLICACY_FILTERS_PATTERNS` to `filters` (a pattern naming the old file is rewritten to name the new ones), and keeps the old file as `service-backup-settings.legacy.sh`, which the wrappers source. It warns if a post-backup function reads a variable its pre-backup function sets, which no longer carries over between the two processes. A container whose services still hold `service-backup-settings.sh` refuses to start and says so; run the conversion as its command, with the same mounts and environment (`docker compose run --rm archiver migrate hooks`), then start it again.

## Custom Restore Scripts

Create an executable `post-restore` in any service directory (and include it in the service's `filters`, if it has one) to run post-restore tasks. `archiver restore` offers to run it once the files are back; `auto-restore` and `auto-restore-all` run it when `RUN_RESTORE_SERVICE` is set. It runs in the restored directory and receives `ARCHIVER_SERVICE`, `ARCHIVER_SERVICE_DIR`, `ARCHIVER_SNAPSHOT_ID`, `ARCHIVER_RESTORE_REVISION` and `ARCHIVER_RESTORE_STORAGE`, never storage credentials or the RSA passphrase. A non-zero exit fails the restore. A directory without `post-restore` but with the older `restore-service.sh` runs that instead, with `bash restore-service.sh`, so existing scripts and backups need no change.

A restore that brings a `service-backup-settings.sh` back into a configured service directory migrates it as `archiver migrate hooks` would. Restores anywhere else leave the files exactly as backed up.

```bash
#!/bin/bash
# post-restore: runs after restoration completes

echo "Importing database..."
docker exec postgres-container psql -U user -d dbname -f /backup/dump.sql

echo "Setting permissions..."
chown -R 1000:1000 /mnt/restored-data

echo "Starting services..."
docker compose up -d
```
