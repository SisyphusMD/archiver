# Maintenance and Verification

Storage checks and retention run on their own schedule; `archiver doctor` checks a deployment end to end, and restore drills prove the backups restore.

## Maintenance (check + prune)

Storage verification and retention run as their own pipeline, on their own schedule, so they can never extend or block a backup run:

```bash
MAINTENANCE_SCHEDULE="0 13 * * *"   # container env var (compose/K8s); unset = only via 'archiver maintenance'
CHECK_BACKUPS="true"                # verify each storage (duplicacy check -all -fossils -resurrect)
PRUNE_BACKUPS="true"                # enforce retention on each storage
PRUNE_KEEP="-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1"
PRUNE_EXHAUSTIVE_FREQUENCY="monthly"  # off | daily | weekly | monthly
```

The two pipelines run concurrently and rely on Duplicacy's own lock-free design — its two-step fossil collection makes a non-exclusive check/prune safe alongside a copy reading or writing the same storage — so neither blocks the other: a backup never waits on maintenance, and maintenance runs on its schedule regardless of an in-progress copy. In the rare case a copy leg loses a race with a concurrent prune (a retryable error, never corruption or a partial copy, since Duplicacy writes the destination snapshot last), the copy is retried once automatically.

**Default retention policy** keeps:
- All backups younger than 1 day old
- 1 backup every 1 day for backups older than 1 day (`-keep 1:1`)
- 1 backup every 7 days for backups older than 7 days (`-keep 7:7`)
- 1 backup every 30 days for backups older than 30 days (`-keep 30:30`)
- Delete all backups older than 180 days (`-keep 0:180`)

**Format:** `-keep n:m` means keep 1 snapshot every `n` days if the snapshot is at least `m` days old.

**Revisions in use are left for the next prune.** The primary's prune asks Duplicacy which revisions the policy deletes (`prune -dry-run`), leaves out any a copy worker is still copying or a restore is still reading, and deletes the rest. A revision left out costs only its space until the next maintenance run. The prune still runs alongside a backup, which Duplicacy is designed for.

### Exhaustive prune frequency

A normal prune is snapshot-metadata work (fast); `-exhaustive` additionally lists every chunk on the storage to garbage-collect orphans — expensive on remote storages (a full sftp listing can take hours) while orphans are rare, so it runs on its own interval. `PRUNE_EXHAUSTIVE_FREQUENCY` is evaluated per storage at each maintenance run: once the interval has elapsed since the last exhaustive success, that run's prune includes `-exhaustive`. An infrequent maintenance schedule simply fires it belatedly at the next opportunity. Force it any time with `archiver maintenance exhaustive`.

### Multi-Repository Shared Storage

If multiple archiver deployments back up to the same storage target, **only ONE should maintain it** to avoid duplicate work and prune races:

1. Set `PRUNE_BACKUPS="false"` and `CHECK_BACKUPS="false"` in all but one deployment
2. The designated deployment maintains all snapshot IDs on the shared storage via the `-all` flag

See the [Duplicacy prune documentation](https://forum.duplicacy.com/t/prune-command-details/1005) for more details on the two-step fossil collection algorithm.

Maintenance runs from a repository of its own in `logs/.maintenance-repo/`, whose cache holds the pending fossil collections of the storages it prunes; mount the logs directory so they survive container restarts (otherwise their chunks wait for the next exhaustive prune). On its first run it takes over the collections earlier versions left in the service directories' repositories.

### Secondary storages under copy workers

When copy workers run (a schedule and at least one secondary), maintenance keeps to the primary, and each worker maintains its own secondary once it has caught up:

- **Mirroring.** The primary's retention is the only one: after each catch-up, and whenever a maintenance run prunes the primary, the worker deletes on its secondary the revisions the primary has pruned, so an offsite never holds a revision the primary dropped or lacks one it kept. It touches only this deployment's snapshot IDs (`<hostname>-...`), never a snapshot ID's newest revision, nothing when the primary's listing fails or lacks the ID entirely, and refuses (with a notification) a pass that would delete more than half of an ID's revisions. After a deliberate retention change, `archiver mirror --allow-large` lets the next pass through. `archiver mirror --dry-run` shows what the next pass would delete. Every deletion is logged in `copies.log`.
- **Exhaustive prune** on `PRUNE_EXHAUSTIVE_FREQUENCY` (`archiver maintenance exhaustive` forces one on the workers' next pass too). Mirroring and the exhaustive prune run only with `PRUNE_BACKUPS="true"`, so the shared-storage rule above still applies.
- **Check** on each secondary's own interval, when its worker is otherwise idle (a backup interrupts it; it runs again later): `STORAGE_TARGET_N_CHECK_INTERVAL`, else `CHECK_INTERVAL`, else 1 day for local, B2 and S3 storages and 7 days for SFTP, where a check lists every chunk. Intervals take `d`, `h` or `m` (`7d`, `12h`). `archiver status` shows when each secondary was last checked and flags a check more than twice its interval overdue. Checks run only with `CHECK_BACKUPS="true"`.

Each worker keeps a small repository in `logs/.copy-repos/` whose cache holds Duplicacy's pending fossil collections; mount the logs directory so they survive container restarts (otherwise their chunks wait for the next exhaustive prune).

## Checking a deployment (`archiver doctor`)

`docker exec archiver archiver doctor` checks the whole deployment in one pass and prints a line per check, `OK`, `WARN` or `FAIL`, saying what to do about anything wrong (exit 1 on any failure). It never creates a storage or writes data to one (it reads one small file to prove the RSA key decrypts it):

- **Configuration and secrets:** every setting and required secret, the RSA private key decrypting with `rsa_passphrase` and matching `public.pem`, the recovery kit password, and the notification destinations (`--notify` sends each a test).
- **Storages:** each reachable with its credentials and holding a Duplicacy storage that opens with the storage password, whose data decrypts with the mounted RSA key (checked on the smallest file up to 64 MB). A storage that does not exist yet is reported, never created.
- **Container:** the capabilities, the logs directory being a mounted volume (its state survives a recreate only then), free space for logs and the drill directory, the service directories, and the hooks (executable, and safe to run).
- **Freshness:** each service's newest revision on every storage (flagged when older than twice `BACKUP_SCHEDULE`'s interval), the copy workers, the recovery kit on every storage, the envelope, and the last restore drill.

Run it after setting up or changing a deployment, and after restoring one.

## Restore drills

A backup nobody has restored is a hope. With `RESTORE_DRILL_SCHEDULE` set (a cron schedule, like the others; unset means no drills), Archiver regularly restores real revisions and proves they come back:

```bash
RESTORE_DRILL_SCHEDULE="0 5 * * 0"   # weekly, Sunday 5am
RESTORE_DRILL_SERVICES="1"           # services per storage per drill (default 1, in rotation), or "all"
RESTORE_DRILL_STORAGES="all"         # "all" (default), "primary", or storage names
RESTORE_DRILL_DIR="/tmp/archiver-drill"  # scratch space for the restored copy (default)
RESTORE_DRILL_EXCLUDE="media"        # service names never drilled (too big for the scratch space, say)
```

Each drill takes, on every chosen storage, the next service in rotation (so one service per week covers every service over time) and restores its newest revision into the drill directory. Duplicacy verifies every chunk as it downloads it, the restored file count is checked against the revision's listing, and the copy is deleted. Restore hooks never run, and the live service and its directory are never touched. A service whose revision would not fit the drill directory's free space is skipped with a warning; one with no revision on a storage yet (a secondary still catching up) is skipped too.

A failed drill always notifies ("Restore Drill Failed"); a passing one is routine news (`NOTIFY_ON=everything`). `archiver status` shows each service's last drill per storage, and the healthcheck warns when the last drill failed or none has passed within twice the schedule's interval. Drills run alongside backups (the revision being restored is held in use so no prune removes it meanwhile); `archiver stop`, `pause` and `resume` act on them. `archiver drill [SERVICE] [STORAGE]` runs one now. The restored copy needs as much space as the revision, so point `RESTORE_DRILL_DIR` at a volume with room, or exclude large services. Drills write to `logs/drill.log`.
