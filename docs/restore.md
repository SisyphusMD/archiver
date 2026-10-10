# Restoring Data

Restore a service interactively, from a temporary container, or unattended. To rebuild a whole host from its recovery kit, see [Recovering a Lost Host](recovery.md#recovering-a-lost-host-archiver-recover).

## Interactive Restore with Existing Container

**Before restoring**, ensure you have a volume mount for the restore destination directory. The restore script will interactively prompt you for:
- Which storage target to restore from
- Snapshot ID to restore
- Local directory path (where to restore the files)
- Which revision to restore

```bash
# Check status first (ensure no backup is running)
docker exec archiver archiver status

# Run interactive restore
docker exec -it archiver archiver restore
```

The restore destination can be any path accessible within the container. If you need to restore to a new location not currently mounted, add a volume mount and restart the container first.

**Before it restores**, the interactive restore shows a preview: how many files it would add, replace (or leave, without overwrite) and delete, and their sizes, judged by size and modification time against what the destination holds. It reads only the revision's file list, so it costs seconds to a minute, and cancelling leaves the destination exactly as it was. At a terminal it then asks "Restore now?"; `NO_PREVIEW=1` skips it. When the snapshot belongs to a configured service, the suggested destination is another directory: restoring over a service that is still running can leave it reading half-restored files, and a copy beside it can be checked first. The advanced options also restore only some paths of the snapshot (see `RESTORE_PATHS` below).

A restore refuses to start while a backup runs. A restore into a configured service directory (or a directory inside or above one) also keeps backups out until it ends, its restore hook included: a backup that starts meanwhile is skipped with a notification, since it would save the directory half-restored. Restores elsewhere do not affect backups.

**Links in the destination.** A restore never writes through a symbolic link already in its destination: a link where the snapshot has a directory or a file is replaced with it when overwriting, and the restore stops otherwise, and nothing is restored below a linked directory it does not replace. What it cannot guard against is someone changing the destination *while* it restores (swapping a directory for a link between Duplicacy's check and its write), so restore into a directory only you and Archiver can write, or one nothing else is changing meanwhile: a fresh directory, or a service that is stopped.

## One-Off Restore with Temporary Container

For a one-time restore without modifying your running container, start a temporary container and exec the interactive restore into it:

```bash
docker run -d --name archiver-restore \
  --env-file /path/to/archiver.env \
  -v /path/to/secrets:/run/secrets:ro \
  -v /path/to/restore/destination:/mnt/restore \
  forgejo.bryantserver.com/sisyphusmd/archiver:0.11.1

docker exec -it archiver-restore archiver restore

docker rm -f archiver-restore
```

When prompted for the local directory path during restore, enter the container path (e.g., `/mnt/restore`). The restored files will appear on your host at `/path/to/restore/destination`.

## Non-interactive Restore (CI / Kubernetes)

Snapshot IDs are `<hostname>-<service directory basename>` (e.g. `backup-server-nextcloud`). When restoring on a different machine, run the container with `hostname:` set to the ORIGINAL value or pass the full `SNAPSHOT_ID` explicitly.

For automated disaster recovery flows (e.g. Kubernetes init containers), Archiver exposes two non-interactive commands driven by environment variables. Exit codes are the machine-readable answer; stdout is informational.

### `archiver snapshot-exists`

Probes every configured storage target for `SNAPSHOT_ID` and short-circuits on the first hit. Useful to gate a restore on whether a backup is actually available.

| Env Var | Required | Description |
|---------|----------|-------------|
| `SNAPSHOT_ID` | Yes | Snapshot ID to look up |

Exit codes:
- `0` — snapshot exists on at least one target (prints `EXISTS`)
- `1` — no target has the snapshot (prints `NOT FOUND`)
- `2` — all targets unreachable or invalid env (prints `UNDETERMINED`)
- `3` — an Archiver backup is in progress; check skipped

### `archiver auto-restore`

Iterates storage targets in configured order and restores from the first target that has the requested snapshot. Once a restore begins, it does not fall through to another target — a failure at that point exits `1`.

| Env Var | Required | Description |
|---------|----------|-------------|
| `SNAPSHOT_ID` | Yes | Snapshot ID to restore |
| `LOCAL_DIR` | Yes | Destination directory inside the container |
| `REVISION` | No | Specific revision number, or `latest` (default) |
| `STORAGE_TARGET` | No | Pin to a single target by name or numeric id |
| `OVERWRITE` | No | Non-empty enables `-overwrite` |
| `DELETE_EXTRA` | No | Non-empty enables `-delete` |
| `HASH_COMPARE` | No | Non-empty enables `-hash` |
| `IGNORE_OWNERSHIP` | No | Non-empty enables `-ignore-owner` |
| `RESTORE_PATHS` | No | Restore only these paths in the snapshot (comma-separated; a directory brings everything under it), for example `config/,data/app.db` |
| `DRY_RUN` | No | Non-empty shows what the restore would add, replace and delete, and restores nothing (the destination is not created or changed) |
| `RUN_RESTORE_SERVICE` | No | Non-empty runs the restored directory's restore hook (`post-restore`, or `restore-service.sh`) after a successful file restore (DB reload, stack restart); its failure fails the restore |
| `RESTORE_THREADS` | No | Override download thread count (default matches `DUPLICACY_THREADS`) |

Exit codes:
- `0` — snapshot restored (and, if `RUN_RESTORE_SERVICE` set, the restore hook succeeded)
- `1` — snapshot not found on any reachable target, the restore itself failed, or the restore hook failed (whatever code it exited with)
- `2` — all targets unreachable, or invalid env
- `3` — an Archiver backup is in progress, or another restore into a service directory is running; restore skipped

Example (gate-and-restore against a running container):

```bash
docker exec \
  -e SNAPSHOT_ID=myservice \
  archiver archiver snapshot-exists \
  && docker exec \
       -e SNAPSHOT_ID=myservice \
       -e LOCAL_DIR=/mnt/restore \
       archiver archiver auto-restore
```

### Running Without a Long-Lived Container (`run` mode)

For Kubernetes Jobs, init containers, or one-shot `docker run` invocations, use the entrypoint's `run` mode (see [Container Modes](installation.md#container-modes)). The configuration is loaded, the subcommand runs, and the container's exit code equals the subcommand's exit code:

```bash
# Probe whether a backup is available
docker run --rm \
  --env-file /path/to/archiver.env \
  -v /path/to/secrets:/run/secrets:ro \
  -e SNAPSHOT_ID=myservice \
  forgejo.bryantserver.com/sisyphusmd/archiver:0.11.1 run snapshot-exists

# Restore a snapshot into a mounted destination
docker run --rm \
  --env-file /path/to/archiver.env \
  -v /path/to/secrets:/run/secrets:ro \
  -e SNAPSHOT_ID=myservice \
  -e LOCAL_DIR=/mnt/restore \
  -e OVERWRITE=1 \
  -v /path/to/restore/destination:/mnt/restore \
  forgejo.bryantserver.com/sisyphusmd/archiver:0.11.1 run auto-restore
```

In Kubernetes this is typically an init container on the workload pod: probe with `run snapshot-exists`, and if a backup exists, run `run auto-restore` to seed the data volume before the main container starts. The exit-code contract means the pod's `restartPolicy` and init-container failure handling behave as expected.
