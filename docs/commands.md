# Commands

Everything `archiver` does, run with `docker exec archiver archiver <command>` (or `docker compose exec`).

With `BACKUP_SCHEDULE`/`MAINTENANCE_SCHEDULE` set, the pipelines run automatically. Without them, run commands manually.

## View logs

```bash
docker exec -it archiver archiver logs
docker logs --tail 20 -f archiver
```

## Check status

```bash
docker exec archiver archiver status
docker exec archiver archiver healthcheck
```

## Start backup

```bash
docker exec archiver archiver backup --detach   # run a backup in the background
docker exec archiver archiver logs              # follow it
docker exec archiver archiver maintenance       # run check + prune now
```

## Manage active backups

```bash
docker exec archiver archiver pause
docker exec archiver archiver resume
docker exec archiver archiver stop
```

## Full Command Reference

```bash
archiver backup            # Run the backup pipeline now (synchronous, exit code propagates)
archiver backup --detach   # Run it in the background (follow with 'archiver logs')
archiver maintenance       # Run per-storage check + prune now (synchronous)
archiver maintenance exhaustive  # Same, forcing the full-listing exhaustive prune
archiver drill [SERVICE] [STORAGE]  # Run a restore drill now (maintenance.md, "Restore drills")
archiver doctor [--notify]  # Check everything read-only (maintenance.md, "Checking a deployment"); --notify sends a test
archiver stop [backup|maintenance|drill|all]  # Stop gracefully (default all: backup, maintenance and a drill)
archiver stop --immediate  # Stop immediately (skip cleanup); combine with a target
archiver pause             # Pause backup (experimental)
archiver resume            # Resume paused backup (experimental)
archiver logs              # Follow backup logs
archiver status            # Both pipelines' state + per-storage last check/prune ages
archiver restore           # Restore data from backup (interactive)
archiver auto-restore      # Restore one snapshot from backup (non-interactive, env-driven)
archiver auto-restore-all  # Restore every service in one pass (non-interactive)
archiver snapshot-exists   # Check if a snapshot exists on any storage target
archiver migrate hooks [DIR...]  # Convert service-backup-settings.sh into executable hooks
archiver recovery-kit [force]  # Upload the encrypted recovery kit to every storage target now
archiver envelope [DIR]    # Write the printable break-glass envelope (PDF + HTML; default /opt/archiver/envelope)
archiver envelope confirm  # Record the envelope as printed, so status can say when it goes out of date
archiver healthcheck       # Liveness check (Docker HEALTHCHECK uses this; on Kubernetes wire it as an exec probe)
archiver health --backups  # Backup health: OK, DEGRADED or FAILING and why; exits 0, 1 or 2 for monitors
archiver status --json     # Everything status shows, and backup health, as JSON
archiver completion bash|zsh|fish  # Print shell completions for these commands
archiver help              # Show help
```

Shell completions, for typing `archiver` commands where the binary runs (inside the container, or a shell with it on the PATH):

```bash
eval "$(archiver completion bash)"                 # bash, e.g. in ~/.bashrc
archiver completion zsh > "${fpath[1]}/_archiver"  # zsh
archiver completion fish | source                  # fish
```

## Running a Backup from an External Scheduler (`run backup`)

For most users, a long-lived container with `BACKUP_SCHEDULE` set is the simplest way to get scheduled backups — the in-container scheduler runs `archiver backup` on schedule, and you don't have to manage anything. Skip this section unless you specifically need to drive scheduling from *outside* the container.

If your environment already owns scheduling — e.g., a Kubernetes `CronJob`, a GitHub Actions scheduled workflow, a systemd timer on the host, or any other platform that spawns a short-lived container per run and expects a meaningful exit code — use the entrypoint's `run backup` mode instead. It loads the configuration, runs a backup **synchronously**, and exits with the backup's result code. The container terminates when the backup finishes; your scheduler then reports success or failure based on the exit code.

Exit codes:
- `0` — backup completed
- `1` — lock contention (another backup already in progress) or catastrophic startup failure
- non-zero — see stderr / logs for details

**Example: one-shot `docker run`** (`archiver.env` + `secrets/` as emitted by `init`)

```bash
docker run --rm \
  --env-file /path/to/archiver.env \
  -v /path/to/secrets:/run/secrets:ro \
  -v /path/to/host/backup-dir:/mnt/backup-dir \
  forgejo.bryantserver.com/sisyphusmd/archiver:0.11.1 run backup
```

The same pattern drives maintenance from an external scheduler: `run maintenance` blocks through the per-storage check + prune and propagates its exit code.

**Example: Kubernetes CronJob**

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: archiver
spec:
  schedule: "0 3 * * *"
  concurrencyPolicy: Forbid
  jobTemplate:
    spec:
      template:
        spec:
          # Required: snapshot IDs are <hostname>-<service>, and a Job pod's default
          # hostname is its random pod name — every run would start a new snapshot ID.
          hostname: backup-server
          restartPolicy: OnFailure
          containers:
            - name: archiver
              image: forgejo.bryantserver.com/sisyphusmd/archiver:0.11.1
              args: ["run", "backup"]
              envFrom:
                - configMapRef: { name: archiver-config }   # the archiver.env keys (non-secret settings)
              volumeMounts:
                - { name: archiver-secrets, mountPath: /run/secrets, readOnly: true }
                - { name: backup-dir,       mountPath: /mnt/backup-dir }
          volumes:
            - name: archiver-secrets
              secret: { secretName: archiver-secrets }   # storage_password, rsa_passphrase, rsa_private_key, rsa_public_key, ...
            - name: backup-dir
              persistentVolumeClaim: { claimName: backup-data }
```

Create the ConfigMap and Secret straight from what `init` emitted: `kubectl create configmap archiver-config --from-env-file=archiver.env` and `kubectl create secret generic archiver-secrets --from-file=secrets/`.

The Pod lives for the duration of one backup and exits. If the backup fails, the Pod exits non-zero and Kubernetes marks the Job failed — the usual CronJob semantics apply.

> **Why not `--detach` here?** A detached backup returns exit `0` immediately, before any real work happens — fine interactively, useless to an external scheduler that needs a meaningful exit code. `run backup` blocks until the backup finishes.
