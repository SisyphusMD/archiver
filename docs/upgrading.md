# Upgrading

Archiver 1.0 keeps your storages, snapshot IDs, keys and every revision written so far: backups continue where they left off, and old revisions restore as before. What changes is how a deployment is configured and how its hooks run.

## Upgrading to 1.0

Nothing about your data changes: the same storages, snapshot IDs (`<hostname>-<service>`) and keys, every revision written by 0.x restores, and recovery kits still decrypt with stock `openssl`. Take a backup with your current release first, and make sure your recovery password is saved.

Go through this list before starting the 1.0 image; the container refuses to start over the first two, and says why.

1. **Configuration is environment variables and secret files only.** A mounted bundle (`bundle.tar.enc`, `config.sh`, a `bundle_password` secret or `BUNDLE_PASSWORD`) stops the container. Convert it first, with the 0.11 image: [Upgrading from a bundle](#upgrading-from-a-bundle).
2. **Hooks are executables.** A service directory that still holds `service-backup-settings.sh` stops the container. Convert every service once with `docker compose run --rm archiver migrate hooks` (same mounts and environment as the container), which writes `pre-backup`, `post-backup` and `filters` that call your existing functions; see [Hooks](hooks.md#custom-service-scripts). A `restore-service.sh` still runs as the restore hook; an executable `post-restore` takes its place if present.
3. **Only their owner may change a hook.** A hook writable by group or others, or below a directory that is (sticky ones such as `/tmp` aside), is refused and its service skipped, with the `chmod go-w` that fixes it. If a less trusted app owns a service directory, keep its hooks in `HOOKS_DIR` instead ([Who can change a hook](hooks.md#custom-service-scripts)).
4. **Routine notifications are off by default.** With `NOTIFY_ON=failures`, "Backup Complete", "Maintenance Complete" and pause, resume and stop notices are no longer sent; set `NOTIFY_ON=everything` (or `PUSHOVER_NOTIFY_ON=everything`) to keep them. Failures are one message per incident, repeated every `ALERT_REPEAT_INTERVAL` (24 hours by default) while it lasts, with one notice when it clears ([Notifications](configuring.md#notifications)).
5. **Services back up two at a time** (`BACKUP_PARALLELISM`, default 2). If one service's hook depends on another's having finished, set `BACKUP_PARALLELISM=1` to keep the configured order.
6. **The entrypoint is `archiver entrypoint` under tini.** An `entrypoint:` override naming `/usr/local/bin/docker-entrypoint.sh` must drop it.
7. **A schedule that can never fire** (such as `0 0 30 2 *`) now fails container start instead of being accepted.
8. **Restore hooks no longer receive storage credentials,** and a failing restore hook makes `auto-restore` exit 1.
9. **Mount the logs directory as a volume** (`/opt/archiver/logs`), if you do not already: it now also holds what lets an interrupted run be finished when the container comes back ([Interrupted runs](installation.md#interrupted-runs)), the copy workers' state and the open incidents.

After starting 1.0, run `docker exec archiver archiver doctor`, which checks the configuration, secrets, every storage and the hooks, and `archiver status`. The first backup uploads the recovery kit once more (its `RECREATE.txt` now lists each service's directory), and if you printed the [break-glass envelope](recovery.md#break-glass-envelope), `archiver status` says whether it is still current. The [CHANGELOG](../CHANGELOG.md) lists every change.

## Upgrading from a bundle

**v1 no longer reads bundles** (`bundle.tar.enc` and its `config.sh`). Configuration is env-native only: environment variables for settings, files under `/run/secrets` for secrets and keys. A v1 container that finds a mounted bundle, a `config.sh`, a `bundle_password` secret, or `BUNDLE_PASSWORD` in its environment refuses to start and prints these steps.

Convert the bundle once with the 0.11 image (any 0.11 release from 0.11.4 on), which reads it and writes the same configuration as env-native materials:

```bash
docker run --rm \
  -v ./archiver-bundle:/opt/archiver/bundle:ro \
  -v ./secrets/bundle_password:/run/secrets/bundle_password:ro \
  -v ./archiver-migrate:/opt/archiver/migrate \
  ghcr.io/sisyphusmd/archiver:0.11 run migrate
```

It writes `archiver-migrate/archiver.env` (the settings, as `KEY=value`) and `archiver-migrate/secrets/` (one file per secret, plus the RSA and SSH keys). Load the first as environment variables (a Compose `env_file:` or a Kubernetes ConfigMap) and mount the second under `/run/secrets` (Compose `secrets:` or a Kubernetes Secret), remove the bundle mount and the `bundle_password` secret, and start v1. The snapshot IDs, storages and keys are unchanged, so backups continue where they left off. The secret files are plaintext: move them into your secret store and delete `archiver-migrate/`.

`archiver bundle export`, `bundle import` and `migrate` are gone with bundles; the [recovery kit](recovery.md#automatic-recovery-kit) is the disaster-recovery copy of the configuration.

## Upgrading from a host installation (before 0.7.0)

**Direct installation on host systems is no longer supported.** All deployments must now run inside a container (Docker, Podman, Kubernetes, etc.).

If you're currently running Archiver v0.6.5 or earlier directly on your host system, see the [Legacy to Docker Migration Guide](guides/migration/legacy-to-docker.md) for step-by-step upgrade instructions.
