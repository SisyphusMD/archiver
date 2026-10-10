# Installation

Run Archiver as a long-lived container with Docker Compose (or Podman, or Kubernetes). The [quick start](../README.md#quick-start) is the short version of this page.

## Prerequisites

- A container runtime — [Docker](https://docs.docker.com/get-docker/), [Podman](https://podman.io/), or Kubernetes. These guides use Docker commands; translate them to your runtime as needed.
- [Docker Compose](https://docs.docker.com/compose/install/) (optional, for easier management of a long-lived container)

## Setting up

> **Container image**: Examples below pull from `forgejo.bryantserver.com/sisyphusmd/archiver`. The same image is also published to `ghcr.io/sisyphusmd/archiver` if you prefer that registry — just substitute the registry hostname in any `image:` or `docker run` line.

### Step 1: Generate Configuration (`init`)

**Skip this step if you already have configuration** — env-native materials (`archiver.env` + secret files) from a previous installation. Coming from a bundle? Convert it first: [Upgrading from a bundle](upgrading.md#upgrading-from-a-bundle).

For new installations, run initialization interactively (the mount is just an output directory for the generated materials):

```bash
docker run -it --rm \
  -v ./archiver-setup:/opt/archiver/setup \
  forgejo.bryantserver.com/sisyphusmd/archiver:0.11.1 init
```

This writes your configuration into `archiver-setup/`:

- `env-native/` — `archiver.env` (non-secret settings) plus `secrets/` (one file per secret, including the keys). **This is what you deploy with**: a Compose `environment:` block + `secrets:`, or a Kubernetes ConfigMap + Secret. The files are plaintext — move them into your secret store and delete `env-native/` afterwards.

`init` also generates and displays the **recovery password** — save it in your password manager; it is the one credential you personally keep. Once the deployment is running, archiver automatically maintains an encrypted recovery kit of the full configuration on every storage target, and that password recovers everything (see [Automatic Recovery Kit](recovery.md#automatic-recovery-kit)).

### Step 2: Configure Docker Compose

Create `compose.yaml` (env-native, the primary mode). Place the emitted `archiver.env` next to it — the compose file loads it directly via `env_file:`, so there is nothing to transcribe, and since it holds no secrets it can be committed to git alongside the compose file. Point the `secrets:` files at where you moved `env-native/secrets/`:

```yaml
services:

  archiver:

    container_name: archiver
    image: forgejo.bryantserver.com/sisyphusmd/archiver:0.11.1
    restart: unless-stopped
    stop_grace_period: 2m         # Allow time for graceful shutdown and cleanup

    hostname: backup-server       # forms the Duplicacy snapshot ID (<hostname>-<service>); keep it stable,
                                  # and set it to the ORIGINAL value when restoring on a new machine

    cap_drop:
      - ALL
    cap_add:
      - DAC_OVERRIDE              # Backup + restore: read/write data owned by other UIDs
      - CHOWN                     # Restore only (drop for backup-only): recreate files under their original owner
      - FOWNER                    # Restore only (drop for backup-only): set mode/timestamps on files owned by other UIDs
    security_opt:
      - no-new-privileges:true

    environment:
      BACKUP_SCHEDULE: "0 3 * * *"       # backups: daily at 3am (omit for manual mode)
      MAINTENANCE_SCHEDULE: "0 13 * * *" # check+prune: daily at 1pm, opposite the backup window
      TZ: "America/New_York"      # Timezone for scheduled backups and timestamps (default: UTC)

    env_file:
      - ./archiver.env            # non-secret settings, exactly as emitted by init
                                  # (no secrets inside, so safe to commit to git; values can
                                  # also be inlined under environment: instead)

    secrets:                      # each lands at /run/secrets/<name>
      - storage_password
      - rsa_passphrase
      - rsa_private_key
      - rsa_public_key
      - recovery_password         # enables the automatic recovery kit (init generates it)
      # - ssh_private_key         # only for sftp targets
      # - ssh_public_key          # only for sftp targets
      # - storage_target_1_b2_id  # only for b2 targets
      # - storage_target_1_b2_key
      # - storage_target_1_s3_id  # only for s3 targets
      # - storage_target_1_s3_secret

    volumes:
      - ./archiver-logs:/opt/archiver/logs           # Persistent logs (optional)
      - /path/to/host/backup-dir:/mnt/backup-dir     # Data to backup (must match SERVICE_DIRECTORIES)
      - /path/to/host/backup/storage:/mnt/backup/storage  # Local storage target
      - ./compose.yaml:/opt/archiver/deployment/compose.yaml:ro  # captured into the recovery kit
      # - /var/run/docker.sock:/var/run/docker.sock  # For docker exec in scripts (optional)
      # - /path/to/host/restore-dir:/mnt/restore-dir # Restore location (will be prompted)

secrets:
  storage_password: { file: ./secrets/storage_password }
  rsa_passphrase:   { file: ./secrets/rsa_passphrase }
  rsa_private_key:  { file: ./secrets/rsa_private_key }
  rsa_public_key:   { file: ./secrets/rsa_public_key }
  recovery_password: { file: ./secrets/recovery_password }
  # ssh_private_key: { file: ./secrets/ssh_private_key }
  # ssh_public_key:  { file: ./secrets/ssh_public_key }
```

Update paths, then start:

```bash
docker compose up -d
```

## Container & Host Sockets (Advanced)

If your backup scripts need to control other containers (e.g., `docker exec` for database dumps), mount the container runtime socket:

```yaml
volumes:
  # Docker socket
  - /var/run/docker.sock:/var/run/docker.sock
  # Podman socket (mount as docker.sock so the docker CLI works)
  # - /run/podman/podman.sock:/var/run/docker.sock
```

**Security Warning**: This grants root-level access to the Docker daemon. The container can start/stop/delete any container or access any data. Only use if necessary.

If your restore scripts need to manage host services (e.g., `systemctl mask/stop/start` to orchestrate restores), mount the systemd D-Bus socket and unit directory, and set `SYSTEMCTL_FORCE_BUS=1`:

```yaml
environment:
  SYSTEMCTL_FORCE_BUS: "1"  # Required: forces systemctl to use D-Bus instead of private socket

volumes:
  - /run/dbus/system_bus_socket:/run/dbus/system_bus_socket  # systemctl start/stop/status
  - /etc/systemd/system:/etc/systemd/system                  # systemctl mask/unmask/enable/disable
```

**Security Warning**: This grants the container full control over host systemd services. It can start, stop, mask, unmask, or restart any service on the host. Only use if your restore scripts require service orchestration.

If your restore scripts use ZFS snapshots for pre-restore safety, the ZFS device node is also needed:

```yaml
volumes:
  - /dev/zfs:/dev/zfs
```

**Security Warning**: This grants the container access to all ZFS pools on the host. It can create, destroy, or modify any dataset or snapshot. Only use if your restore scripts take ZFS snapshots.

If a hook takes a btrfs snapshot (for example a read-only snapshot of a service's data right before its backup, so files that must agree are saved from one instant), the image has `btrfs-progs`, and the container needs the `SYS_ADMIN` capability and a mount of the whole btrfs subvolume:

```yaml
cap_add:
  - SYS_ADMIN   # Only for hooks that run btrfs subvolume snapshot
volumes:
  - /volume1/repo:/volume1/repo
```

**Security Warning**: `SYS_ADMIN` is a broad capability (mounts, namespaces and more). Archiver itself never needs it; add it only for a hook that snapshots btrfs. The image also has Python 3 with the `lmdb` module, for hooks that compact an LMDB database (such as Garage's metadata index) from such a snapshot.

## Security Hardening (Advanced)

We recommend dropping all capabilities and adding back only what Archiver needs. The example `compose.yaml` above includes this by default.

```yaml
services:
  archiver:
    cap_drop:
      - ALL
    cap_add:
      - DAC_OVERRIDE # Backup + restore: write to directories owned by other UIDs
      - CHOWN        # Restore only (drop for backup-only): recreate files under their original owner
      - FOWNER       # Restore only (drop for backup-only): set mode/timestamps on files owned by other UIDs
    security_opt:
      - no-new-privileges:true  # Prevent privilege escalation
```

| Capability | When Required | Why |
|-----------|---------------|-----|
| `DAC_OVERRIDE` | Always (with `cap_drop: ALL`) | Archiver runs as root but writes to service data directories that may be owned by other users (e.g., UID 1000) |
| `CHOWN` | Restore with original ownership (the default) | Restore recreates files as their original UID/GID via `chown()`, which requires `CAP_CHOWN`. Without it, restored files land owned by root (Archiver logs a warning when this happens) |
| `FOWNER` | Restore with original ownership (the default) | Lets Archiver set permissions/timestamps on restored files owned by other UIDs |

**Backup-only least privilege:** `CHOWN` and `FOWNER` are used only by restore, so a container that only takes scheduled backups can drop both and run with just `DAC_OVERRIDE`. Add them back when you need to restore with original ownership. Restoring without them does not fail: the data is restored correctly, but files land owned by root and Archiver logs a warning. To restore without preserving ownership on purpose, set `IGNORE_OWNERSHIP=1`.

**Note**: `no-new-privileges` is a kernel security option, not a Linux capability. It is compatible with `DAC_OVERRIDE`, `CHOWN`, and `FOWNER`, and is recommended for defense in depth. Scheduled backups (`BACKUP_SCHEDULE`) no longer need `SETGID` — Archiver's own scheduler runs jobs as the container user rather than forking with `setgid` like Debian's cron.

## Graceful Shutdown

The `stop_grace_period: 2m` setting allows the container time to complete cleanup when stopped. When `docker compose down` or `docker stop` is called, Archiver will:
- Complete any running pre-backup hooks
- Run post-backup hooks to restore services (e.g., restart databases, remove snapshots)
- Terminate gracefully

If your post-backup hooks take longer than 2 minutes, increase this value accordingly.

### Interrupted runs

A backup, maintenance or restore drill that a shutdown, a crash, a kill or a power cut ends is not left to its next scheduled time. Archiver records each run in progress on the logs volume (`logs/.run-*.json`), and when the container starts again:

- A service the run left between its hooks (`pre-backup` ran, `post-backup` did not, as after a crash) gets its `post-backup` hook first, with `ARCHIVER_BACKUP_RESULT=interrupted`, so whatever `pre-backup` stopped runs again. The hook's `ARCHIVER_STATE_DIR` is kept on the logs volume, so it still finds what `pre-backup` left there.
- The backup then runs again at once, the services it had not finished first. Duplicacy picks up where it was: it saves its resume point every five minutes, and again when a stop sends it SIGINT, so after a kill or a power cut too the files it had finished are not read or sent again. (Archiver builds Duplicacy with a patch for this, `build/duplicacy/resume-point.patch`; Duplicacy 3.2.5 itself saves a resume point only on SIGINT, and only for a first backup.) Uploaded chunks a prune has removed since are checked for first, and the resume point is then not used.
- An interrupted maintenance or drill runs again at once, and the drill's leftover copies are deleted.

A stop you ask for (`archiver stop`) is not an interruption: that run is over and is not resumed. Without a schedule, the container's start runs the interrupted runs the same way.

## Container Modes

The entrypoint selects one of three modes based on the first container argument:

| Mode | How it's invoked | Behavior |
|------|------------------|----------|
| `init` | `docker run ... archiver:<tag> init` | Interactive setup: generates env-native materials and the recovery password. Exits when done. |
| _default_ (daemon) | `docker run ... archiver:<tag>` (no args) | Loads the configuration, then either runs the scheduler, `archiver daemon` (if `BACKUP_SCHEDULE` and/or `MAINTENANCE_SCHEDULE` is set) or waits so you can `docker exec` in. |
| `run` | `docker run ... archiver:<tag> run <subcommand>` | Loads the configuration, then `exec`s a single non-interactive subcommand and exits with that subcommand's exit code. Designed for Kubernetes Jobs / init containers and other CI flows. |

`run` mode only accepts subcommands whose exit codes form a meaningful contract: `auto-restore`, `auto-restore-all`, `snapshot-exists`, `healthcheck`, `backup`, and `maintenance` (synchronous paths intended for external schedulers — see [Running a Backup from an External Scheduler](commands.md#running-a-backup-from-an-external-scheduler-run-backup)). Any other subcommand is rejected with exit code `2`.

## Image Tags

Each release is published under two tags, signed alike:

| Tag | Size | Holds |
|---|---|---|
| `:<version>` (and `:<major>.<minor>`, `:<major>`) | about 1.1 GB | Everything below, plus the tools hooks may use: the docker CLI, `systemctl`, `zfs`, `btrfs`, Python 3 with `lmdb`, `sqlite3`, `vim` and `nano`, `ping`, `ps` |
| `:<version>-slim` (and `:<major>.<minor>-slim`, `:<major>-slim`) | about 340 MB | Everything Archiver itself runs: Duplicacy, rclone, OpenSSH, OpenSSL, curl, qrencode, tini, CA certificates and time zones |

Use the slim tag when no hook needs the extra tools (your hooks run plain shell, or none). A hook that calls `docker`, `systemctl` or another of those tools needs the full tag.

## Verifying Images

Release images are signed with cosign, and each carries an SBOM and a build provenance attestation. To check that an image is one this project released, with [cosign](https://github.com/sigstore/cosign) and the public key in this repository ([cosign.pub](../cosign.pub)):

```bash
cosign verify --key cosign.pub --insecure-ignore-tlog=true ghcr.io/sisyphusmd/archiver:<version>
```

The signatures are not entered in Sigstore's public transparency log, so the flag is needed: the signature itself is stored beside the image in each registry. `docker buildx imagetools inspect <image> --format '{{ json .SBOM }}'` shows the SBOM, and `--format '{{ json .Provenance }}'` the provenance.
