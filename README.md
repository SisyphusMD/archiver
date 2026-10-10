# Archiver

<p>
  <img src="lib/logos/72x72.png" alt="Logo" align="left" style="margin-right: 10px;">
  Automated encrypted backups with deduplication to local disk, SFTP, BackBlaze B2, and S3 storage. Leverages <a href="https://github.com/gilbertchen/duplicacy/tree/v3.2.5">Duplicacy CLI v3.2.5</a> to follow the <a href="https://www.backblaze.com/blog/the-3-2-1-backup-strategy/">3-2-1 Backup Strategy</a> while removing the complexity of manual configuration.
</p>

> **Primary repository**: This project is developed at [forgejo.bryantserver.com/SisyphusMD/archiver](https://forgejo.bryantserver.com/SisyphusMD/archiver). The GitHub copy is a read-only mirror.

## What is Archiver?

Archiver automates backing up directories to multiple remote storage locations with encryption and deduplication. Configure once, then backups run automatically on a schedule.

Each directory gets backed up independently, with optional pre/post-backup scripts for service-specific needs (like database dumps). Backups are encrypted at rest and deduplicated across all your directories to save storage space.

Supports local disk, SFTP (Synology NAS, etc.), BackBlaze B2, and S3-compatible storage.

## ⚠️ Upgrading from a bundle

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

`archiver bundle export`, `bundle import` and `migrate` are gone with bundles; the [recovery kit](#automatic-recovery-kit) is the disaster-recovery copy of the configuration.

## ⚠️ Breaking Changes in v0.7.0

**Direct installation on host systems is no longer supported.** All deployments must now run inside a container (Docker, Podman, Kubernetes, etc.).

If you're currently running Archiver v0.6.5 or earlier directly on your host system, see the [Legacy to Docker Migration Guide](docs/guides/migration/legacy-to-docker.md) for step-by-step upgrade instructions.

**New users**: Continue reading below to get started.

## Features

- **Encrypted & Deduplicated**: Block-level deduplication minimizes storage, RSA encryption secures data
- **Multiple Backends**: Local disk, SFTP, BackBlaze B2, S3-compatible storage
- **Automated Rotation**: Configurable retention policies (keep daily, weekly, monthly snapshots)
- **Service Integration**: Pre/post-backup scripts, custom restore procedures
- **Notifications**: Pushover, Apprise and ntfy; one alert per incident, repeated on an interval while it lasts, and a notice when it clears
- **Easy Restoration**: Interactive restore script to recover specific revisions

---

## Prerequisites

- A container runtime — [Docker](https://docs.docker.com/get-docker/), [Podman](https://podman.io/), or Kubernetes. The rest of this README uses Docker commands as the default; translate them to your runtime as needed.
- [Docker Compose](https://docs.docker.com/compose/install/) (optional, for easier management of a long-lived container)

---

## Storage Backend Setup

Prepare at least one storage location before running init. Expand the sections below for setup instructions; every other type Duplicacy supports is listed under [Storage types](#storage-types).

### Local Disk

<details>
  <summary>Click to expand local disk setup instructions</summary>

Local disk storage is the simplest and fastest option, ideal as your primary backup target. Backups can then be copied from local storage to remote locations (SFTP, B2, S3) for off-site redundancy.

#### Requirements

- A local directory path (e.g., `/mnt/backup/storage`)
- Sufficient disk space for your backups
- Proper read/write permissions

#### Setup

1. Create the backup directory:
   ```bash
   sudo mkdir -p /mnt/backup/storage
   ```

2. Set appropriate permissions:
   ```bash
   sudo chown -R $USER:$USER /mnt/backup/storage
   sudo chmod 755 /mnt/backup/storage
   ```

3. Verify the directory is accessible:
   ```bash
   ls -la /mnt/backup/storage
   ```

**Tip:** For the 3-2-1 backup strategy, use local disk as your primary storage target for fast backups, then configure additional remote storage targets to copy backups off-site automatically.

</details>

### SFTP - Synology NAS

<details>
  <summary>Click to expand Synology setup instructions</summary>

#### Enable SFTP

1. Login to Synology DSM Web UI (usually `http://<nas-ip>:5000`)
2. Open **Control Panel** → **File Services** → **FTP** tab
3. Enable **SFTP service** (not FTP/FTPS)
4. Default port **22** is fine
5. Click **Apply**

#### Create User

1. **Control Panel** → **User & Group** → **Create**
2. Set **Name** and **Password**
3. Assign to appropriate **Groups**
4. Grant shared folder permissions
5. Under **Application Permissions**, allow **SFTP**
6. Complete and click **Done**

#### Create Shared Folder

1. **Control Panel** → **Shared Folder** → **Create**
2. Name the folder and configure settings
3. Don't enable **WriteOnce** (incompatible with backups)
4. If using BTRFS, enable **data checksum**
5. Grant **Read/Write** access to backup user

#### Add SSH Key

Generate SSH key first (init script can do this), then:

1. **Control Panel** → **User & Group** → **Advanced**
2. Enable **user home service**
3. Open **File Station** → **homes** folder → your user folder
4. Create `.ssh` folder if it doesn't exist
5. Upload or create `authorized_keys` file containing your public key (`ssh-ed25519 AAAA...`)

</details>

### B2 - BackBlaze

<details>
  <summary>Click to expand BackBlaze B2 setup instructions</summary>

#### Account Setup

1. [Create account](https://www.backblaze.com/sign-up/cloud-storage) or [sign in](https://secure.backblaze.com/user_signin.htm)
2. **My Settings** → Enable **B2 Cloud Storage**

#### Create Bucket

1. **Buckets** → **Create a Bucket**
2. Choose unique **Bucket Name**
3. Files: **Private**
4. Encryption: **Enable**
5. Object Lock: **Disable**
6. Lifecycle: **Keep all versions** (default)

#### Application Key

1. **Application Keys** → **Add New**
2. Name the key
3. Allow access to your bucket
4. Type: **Read and Write**
5. Enable **List All Bucket Names**
6. Click **Create New Key**
7. Save the **keyID** and **applicationKey** (shown only once)

</details>

### S3-Compatible Storage

<details>
  <summary>Click to expand S3 setup instructions</summary>

S3 providers vary, but you'll need:

- **Bucket Name** (globally unique)
- **Endpoint** (e.g., `s3.amazonaws.com` or `s3.us-east-1.wasabisys.com`)
- **Region** (optional, provider-specific, e.g., `us-east-1`)
- **Access Key ID** (with read/write permissions)
- **Secret Access Key**

Create these through your S3 provider's console (AWS, Wasabi, Backblaze S3 API, etc.)

</details>

### Notifications (Optional)

<details>
  <summary>Click to expand Pushover setup instructions</summary>

#### Pushover Setup

1. [Create account](https://pushover.net/signup) or [sign in](https://pushover.net/login)
2. Note your **User Key** from the dashboard
3. [Add a device](https://pushover.net/clients) to receive notifications
4. [Create an Application/API Token](https://pushover.net/apps/build)
5. Name your app and agree to terms
6. Save the **API Token/Key**

You'll enter the **User Key** and **API Token** during init.

</details>

---

## Image Tags

Each release is published under two tags, signed alike:

| Tag | Size | Holds |
|---|---|---|
| `:<version>` (and `:<major>.<minor>`, `:<major>`) | about 1.1 GB | Everything below, plus the tools hooks may use: the docker CLI, `systemctl`, `zfs`, `btrfs`, Python 3 with `lmdb`, `sqlite3`, `vim` and `nano`, `ping`, `ps` |
| `:<version>-slim` (and `:<major>.<minor>-slim`, `:<major>-slim`) | about 340 MB | Everything Archiver itself runs: Duplicacy, rclone, OpenSSH, OpenSSL, curl, qrencode, tini, CA certificates and time zones |

Use the slim tag when no hook needs the extra tools (your hooks run plain shell, or none). A hook that calls `docker`, `systemctl` or another of those tools needs the full tag.

## Verifying Images

Release images are signed with cosign, and each carries an SBOM and a build provenance attestation. To check that an image is one this project released, with [cosign](https://github.com/sigstore/cosign) and the public key in this repository ([cosign.pub](cosign.pub)):

```bash
cosign verify --key cosign.pub --insecure-ignore-tlog=true ghcr.io/sisyphusmd/archiver:<version>
```

The signatures are not entered in Sigstore's public transparency log, so the flag is needed: the signature itself is stored beside the image in each registry. `docker buildx imagetools inspect <image> --format '{{ json .SBOM }}'` shows the SBOM, and `--format '{{ json .Provenance }}'` the provenance.

---

## Installation

> **Container image**: Examples below pull from `forgejo.bryantserver.com/sisyphusmd/archiver`. The same image is also published to `ghcr.io/sisyphusmd/archiver` if you prefer that registry — just substitute the registry hostname in any `image:` or `docker run` line.

### Step 1: Generate Configuration (`init`)

**Skip this step if you already have configuration** — env-native materials (`archiver.env` + secret files) from a previous installation. Coming from a bundle? Convert it first: [Upgrading from a bundle](#️-upgrading-from-a-bundle).

For new installations, run initialization interactively (the mount is just an output directory for the generated materials):

```bash
docker run -it --rm \
  -v ./archiver-setup:/opt/archiver/setup \
  forgejo.bryantserver.com/sisyphusmd/archiver:0.11.1 init
```

This writes your configuration into `archiver-setup/`:

- `env-native/` — `archiver.env` (non-secret settings) plus `secrets/` (one file per secret, including the keys). **This is what you deploy with**: a Compose `environment:` block + `secrets:`, or a Kubernetes ConfigMap + Secret. The files are plaintext — move them into your secret store and delete `env-native/` afterwards.

`init` also generates and displays the **recovery password** — save it in your password manager; it is the one credential you personally keep. Once the deployment is running, archiver automatically maintains an encrypted recovery kit of the full configuration on every storage target, and that password recovers everything (see [Automatic Recovery Kit](#automatic-recovery-kit)).

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

### Container & Host Sockets (Advanced)

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

### Security Hardening (Advanced)

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

### Graceful Shutdown

The `stop_grace_period: 2m` setting allows the container time to complete cleanup when stopped. When `docker compose down` or `docker stop` is called, Archiver will:
- Complete any running pre-backup hooks
- Run post-backup hooks to restore services (e.g., restart databases, remove snapshots)
- Terminate gracefully

If your post-backup hooks take longer than 2 minutes, increase this value accordingly.

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `BACKUP_SCHEDULE` | No | Standard 5-field cron expression for the backup pipeline (empty = manual mode) |
| `MAINTENANCE_SCHEDULE` | No | Cron expression for the maintenance pipeline (check + prune); unset = maintenance only runs via `archiver maintenance` |
| `TZ` | No | Timezone for scheduled backups and log timestamps (default: UTC) |
| `LOG_FORMAT` | No | `text` (default) or `json`: how the container prints its logs to `docker logs`. `json` gives one object per line (`time`, `level`, `service`, `log`, `msg`) for a log collector such as Alloy or Loki, covering everything the running container prints (one-shot `run`, `init` and `migrate` stay text); the files in the logs volume stay text |
| `SYSTEMCTL_FORCE_BUS` | No | Set to `1` to enable systemctl access to host services via D-Bus socket (requires socket mounts, see above) |

Archiver's configuration itself (service directories, storage targets, secrets) is likewise environment variables plus file-based secrets. See [Configuration Sources](#configuration-sources).

### Container Modes

The entrypoint selects one of three modes based on the first container argument:

| Mode | How it's invoked | Behavior |
|------|------------------|----------|
| `init` | `docker run ... archiver:<tag> init` | Interactive setup: generates env-native materials and the recovery password. Exits when done. |
| _default_ (daemon) | `docker run ... archiver:<tag>` (no args) | Loads the configuration, then either runs the scheduler, `archiver daemon` (if `BACKUP_SCHEDULE` and/or `MAINTENANCE_SCHEDULE` is set) or waits so you can `docker exec` in. |
| `run` | `docker run ... archiver:<tag> run <subcommand>` | Loads the configuration, then `exec`s a single non-interactive subcommand and exits with that subcommand's exit code. Designed for Kubernetes Jobs / init containers and other CI flows. |

`run` mode only accepts subcommands whose exit codes form a meaningful contract: `auto-restore`, `auto-restore-all`, `snapshot-exists`, `healthcheck`, `backup`, and `maintenance` (synchronous paths intended for external schedulers — see [Running a Backup from an External Scheduler](#running-a-backup-from-an-external-scheduler-run-backup)). Any other subcommand is rejected with exit code `2`.

### Image Tags

- `forgejo.bryantserver.com/sisyphusmd/archiver:0.11.1` - exact version (recommended; this line always names the current release)
- `MAJOR.MINOR` (e.g. `0.9`) - receives patch updates automatically
- `MAJOR` (e.g. `0`) - receives minor and patch updates automatically

---

## Configuration

The settings below define what to backup and where. Supply them as environment variables plus file-based secrets (see [Configuration Sources](#configuration-sources) below), and edit them wherever they live — your compose file, ConfigMap, or secret store.

### Configuration Sources

Environment variables carry the non-secret settings and files under `/run/secrets` carry the secrets and keys, so the configuration stays under version control (compose file / ConfigMap) and the secrets stay in a secret store. Nothing is read from a configuration file, and nothing configured is ever executed.

**Non-secret settings (plain env vars).** `SERVICE_DIRECTORIES`, the non-secret `STORAGE_TARGET_N_*` fields (`NAME`, `TYPE`, and each type's settings in [Storage types](#storage-types)), `CHECK_BACKUPS`, `CHECK_INTERVAL`, `STORAGE_TARGET_N_CHECK_INTERVAL`, `PRUNE_BACKUPS`, `PRUNE_KEEP`, `PRUNE_EXHAUSTIVE_FREQUENCY`, `DUPLICACY_THREADS`, `BACKUP_PARALLELISM`, `HOOKS_DIR`, `RESTORE_DRILL_SERVICES`, `RESTORE_DRILL_STORAGES`, `RESTORE_DRILL_DIR`, `RESTORE_DRILL_EXCLUDE`, `NOTIFICATION_SERVICE`, and `RECOVERY_KIT_EXTRA_PATHS`. As an env var, `SERVICE_DIRECTORIES` is a colon-delimited list rather than a bash array, for example `SERVICE_DIRECTORIES=/srv/*/:/home/user/data/` (newlines also work, so a YAML block scalar is fine).

**Secrets (files only).** Secrets are never read from a plain env var (one would leak through `/proc` and `docker inspect`, and Archiver purges any it finds). Each secret is read from a file: `<NAME>_FILE` if set, otherwise `/run/secrets/<lowercased name>`. The secrets are `STORAGE_PASSWORD`, `RSA_PASSPHRASE`, `PUSHOVER_USER_KEY`, `PUSHOVER_API_TOKEN`, and each target's type's secrets (see [Storage types](#storage-types)) and optional [break-glass credentials](#break-glass-envelope). For example, `STORAGE_PASSWORD` reads `/run/secrets/storage_password` and `STORAGE_TARGET_1_B2_KEY` reads `/run/secrets/storage_target_1_b2_key`. `STORAGE_PASSWORD` must be at least 8 characters (a Duplicacy requirement). Because `/run/secrets` is the native mount path for Docker and Kubernetes secrets, a Compose or Swarm `secrets:` entry named to match (for example `storage_password`) is picked up with no extra configuration.

**Keys (files).** Keys are always files under `/opt/archiver/keys`. The RSA keypair must be provided as files at `/run/secrets/rsa_private_key` and `/run/secrets/rsa_public_key` (override the paths with `RSA_PRIVATE_KEY_FILE` / `RSA_PUBLIC_KEY_FILE`). The SFTP keypair is optional, for sftp targets, at `/run/secrets/ssh_private_key` and `/run/secrets/ssh_public_key` (override with `SSH_PRIVATE_KEY_FILE` / `SSH_PUBLIC_KEY_FILE`; restore needs both halves).

Starting from scratch without `archiver init`? Generate the RSA keypair yourself — Duplicacy needs the traditional PKCS#1 PEM format, and the passphrase must match your `rsa_passphrase` secret:

```bash
openssl genrsa -aes256 -passout pass:YOUR_RSA_PASSPHRASE -traditional -out rsa_private_key 2048
openssl rsa -in rsa_private_key -passin pass:YOUR_RSA_PASSPHRASE -pubout -out rsa_public_key
```

(For sftp targets, also `ssh-keygen -t ed25519 -N "" -f ssh_private_key`, which writes `ssh_private_key` and `ssh_private_key.pub` — supply the latter as `ssh_public_key`.)

**What you must not lose (disaster recovery).** To restore after losing the host you need, stored somewhere that does not burn down with it: `STORAGE_PASSWORD` (unlocks the Duplicacy storage), `RSA_PASSPHRASE` + `rsa_private_key` (decrypt the file data), your storage-target settings (`archiver.env` or equivalents), and for sftp targets the SSH keypair. Missing any of the first three means the backups are permanently undecryptable. The [Automatic Recovery Kit](#automatic-recovery-kit) keeps all of it on every storage target for you — one password in your password manager covers everything.

### Service Directories

Directories to backup, colon-delimited. Use `*` for subdirectories:

```bash
SERVICE_DIRECTORIES=/srv/*/:/home/user/data/
# /srv/*/           -> each subdirectory becomes its own repository
# /home/user/data/  -> a single repository
```

(Newlines work as separators too, so a YAML block scalar is fine.)

Each directory's name becomes part of its snapshot ID (`<hostname>-<name>`), which Duplicacy restricts to letters, digits, `_` and `-`: a directory named with a space or a dot cannot be backed up, so rename it. An entry that matches no directory (a typo or an unmounted volume) is reported as an error on every backup, and so is a directory whose name breaks that rule; the other directories still back up.

### Storage Targets

Define multiple storage locations of any [type](#storage-types):

> **Note:** Storage names should only contain letters, numbers, and underscores. Other characters will be automatically sanitized (e.g., `my-storage` → `my_storage`).

```bash
# Primary storage (required)
STORAGE_TARGET_1_NAME="local"
STORAGE_TARGET_1_TYPE="local"
STORAGE_TARGET_1_LOCAL_PATH="/mnt/backup/storage"

# Secondary storage (optional)
STORAGE_TARGET_2_NAME="nas"
STORAGE_TARGET_2_TYPE="sftp"
STORAGE_TARGET_2_SFTP_URL="192.168.1.100"
STORAGE_TARGET_2_SFTP_PORT="22"
STORAGE_TARGET_2_SFTP_USER="backup"
STORAGE_TARGET_2_SFTP_PATH="/volume1/backups"

# Tertiary storage (optional)
STORAGE_TARGET_3_NAME="backblaze"
STORAGE_TARGET_3_TYPE="b2"
STORAGE_TARGET_3_B2_BUCKETNAME="my-bucket"
STORAGE_TARGET_3_B2_ID="keyID"
STORAGE_TARGET_3_B2_KEY="applicationKey"

# Quarternary storage (optional)
STORAGE_TARGET_4_NAME="hetzner"
STORAGE_TARGET_4_TYPE="s3"
STORAGE_TARGET_4_S3_BUCKETNAME="my-bucket"
STORAGE_TARGET_4_S3_ENDPOINT="endpoint"
STORAGE_TARGET_4_S3_REGION="none"
STORAGE_TARGET_4_S3_ID="id"
STORAGE_TARGET_4_S3_SECRET="secret"
```

### Storage types

Every storage Duplicacy 3.2.5 supports is a type (ADR 23). Settings are plain `STORAGE_TARGET_N_<NAME>` variables; secrets are files (`/run/secrets/storage_target_n_<name>`, or `STORAGE_TARGET_N_<NAME>_FILE`). Settings in *italics* are optional; a `_PATH` places the storage in a folder inside its bucket, share or drive.

| Type | Settings | Secrets | Notes |
|---|---|---|---|
| `local` | `LOCAL_PATH` | | A path mounted into the container. |
| `sftp`, `sftpc` | `SFTP_URL`, `SFTP_PORT` (default 22), `SFTP_USER`, `SFTP_PATH` | the SSH keypair (see [Keys](#configuration-sources)) | `sftpc` also offers older ciphers and key exchanges, for old servers. The path must exist. |
| `b2` | `B2_BUCKETNAME`, *`B2_PATH`* | `B2_ID`, `B2_KEY` | |
| `b2-custom` | `B2_DOWNLOAD_HOST`, `B2_BUCKETNAME`, *`B2_PATH`* | `B2_ID`, `B2_KEY` | Downloads through your own host (a CDN in front of B2). |
| `s3`, `s3c`, `minio`, `minios` | `S3_BUCKETNAME`, `S3_ENDPOINT`, *`S3_REGION`* (default `none`), *`S3_PATH`* | `S3_ID`, `S3_SECRET` | `s3c` is for providers that need V2 signatures; `minio` (HTTP) and `minios` (HTTPS) use path-style addressing for self-hosted servers. |
| `wasabi` | `WASABI_BUCKETNAME`, `WASABI_ENDPOINT` (default `s3.wasabisys.com`), `WASABI_REGION` (default `us-east-1`), *`WASABI_PATH`* | `WASABI_KEY`, `WASABI_SECRET` | |
| `azure` | `AZURE_ACCOUNT`, `AZURE_CONTAINER` | `AZURE_KEY` | The container must exist. |
| `gcs` | `GCS_BUCKETNAME`, *`GCS_PATH`* | `GCS_TOKEN`: a service-account JSON key | |
| `gcd` | `GCD_PATH` and/or *`GCD_DRIVE`* (a shared drive's ID, from its URL) | `GCD_TOKEN`: a service-account JSON key, or a token from your own OAuth app (below) | Google Drive. |
| `one` | `ONE_PATH`, `ONE_CLIENT_ID` | `ONE_TOKEN`, `ONE_CLIENT_SECRET` | OneDrive personal, through your own app registration (below). |
| `odb` | `ODB_PATH` and/or *`ODB_DRIVE_ID`*, `ODB_CLIENT_ID` | `ODB_TOKEN`, `ODB_CLIENT_SECRET` | OneDrive for Business or SharePoint, through your own app registration (below). |
| `dropbox` | `DROPBOX_PATH`, `DROPBOX_APP_KEY` | `DROPBOX_TOKEN` (a refresh token), `DROPBOX_APP_SECRET` | Through your own Dropbox app (below). |
| `swift` | `SWIFT_URL`: `user@auth-host/v3/container[/path][?domain=…&tenant=…]` | `SWIFT_KEY` | OpenStack Swift, in Duplicacy's URL form; add `protocol=http` for plain HTTP. |
| `webdav`, `webdav-http` | `WEBDAV_HOST` (host[:port]), `WEBDAV_USER`, `WEBDAV_PATH` | `WEBDAV_PASSWORD` | HTTPS or plain HTTP. The path must exist. |
| `smb` | `SMB_HOST` (host[:port]), `SMB_USER`, `SMB_SHARE`, *`SMB_PATH`* | `SMB_PASSWORD` | SMB 2/3. The path must exist. |
| `storj` | `STORJ_SATELLITE` (host:port), `STORJ_BUCKET`, *`STORJ_PATH`* | `STORJ_KEY`, `STORJ_PASSPHRASE` | |
| `fabric` | `FABRIC_ENDPOINT`, *`FABRIC_PATH`* | `FABRIC_TOKEN` | Storage Made Easy File Fabric. |

When Duplicacy cannot open a storage, the error says why: the storage's directory does not exist (Duplicacy does not create it on SFTP, WebDAV or SMB), or the storage cannot be reached, with the reason.

Checks default to daily on object storage and weekly on SFTP, WebDAV, SMB, File Fabric and the consumer drives (`STORAGE_TARGET_N_CHECK_INTERVAL` overrides). Every type receives the [recovery kit](#automatic-recovery-kit), and the [envelope](#break-glass-envelope) prints each type's settings and credentials.

**Your own app for Google Drive, OneDrive and Dropbox.** Archiver never refreshes tokens through duplicacy.com. Create an app with the provider (a Google Cloud OAuth client of type Desktop, a Microsoft Entra app registration with a client secret, or a Dropbox app), then get a token with the `rclone` in the image, on any machine with a browser (or with `rclone authorize` on your desktop):

```bash
docker run -it --rm --entrypoint rclone ghcr.io/sisyphusmd/archiver authorize dropbox "APP_KEY" "APP_SECRET"
docker run -it --rm --entrypoint rclone ghcr.io/sisyphusmd/archiver authorize onedrive "CLIENT_ID" "CLIENT_SECRET"
docker run -it --rm --entrypoint rclone ghcr.io/sisyphusmd/archiver authorize drive "CLIENT_ID" "CLIENT_SECRET"
```

Each prints a token as JSON. For Dropbox, `DROPBOX_TOKEN` is its `refresh_token`. For OneDrive, the whole JSON is `ONE_TOKEN` (or `ODB_TOKEN`). Microsoft replaces the token each time it is refreshed, so Duplicacy works from a copy under `logs/.tokens` and the recovery kit carries the latest one; the envelope cannot, and points to signing in to OneDrive instead. For Google Drive, `GCD_TOKEN` is `{"client_id": "CLIENT_ID", "client_secret": "CLIENT_SECRET", "end_point": {"TokenURL": "https://oauth2.googleapis.com/token"}, "token": <the JSON>}`; a service-account key needs none of this.

### Copies to secondary storages

The first target is the primary: every backup writes there. Each further target is a copy of it.

When the container runs on a schedule (`BACKUP_SCHEDULE`), each secondary has its own copy worker. A backup ends once the primary and the recovery kit are written, then wakes the workers, so a slow or unreachable offsite never delays or fails the next backup. Each worker copies every revision its target lacks, one copy at a time, with targets copying in parallel. A failed copy is retried after 1, 5 and 15 minutes, then every 30 minutes until the target has caught up. A target still failing after 30 minutes counts as down: one notification, a reminder every 24 hours while it stays down, and one when it recovers. Workers write to `logs/copies.log`, and `archiver status` shows each target's state. `archiver stop`, `pause` and `resume` act on the workers too; after a stop, a worker copies again after the next backup.

Without a schedule (manual mode or a one-shot `run backup`), there are no workers: the backup copies to every secondary itself and exits non-zero if a copy fails.

### Secrets

```bash
STORAGE_PASSWORD="encryption-password-for-duplicacy-storage"
RSA_PASSPHRASE="passphrase-for-rsa-private-key"
```

### Automatic Recovery Kit

Your configuration and secrets are the keys to every backup: lose them and the backups are unreadable. The recovery kit automates keeping them safe. It activates when the `recovery_password` secret is present (at `/run/secrets/recovery_password`, or via `RECOVERY_PASSWORD_FILE`) — the shipped compose template includes it.

**Where the password comes from.** `archiver init` generates it, includes it in the emitted secrets, and displays it once — **save it in your password manager**; it is the one credential you personally keep. An existing deployment enables the kit with one command plus a `secrets:` entry:

```bash
openssl rand -base64 24 > secrets/recovery_password    # save a copy in your password manager
```

It must be at least 8 characters and **differ from `STORAGE_PASSWORD`** (it is the only thing protecting the kit at rest, and the kit contains the storage password); archiver refuses a violating configuration. To rotate it, replace the secret file and update your password manager — the next backup re-encrypts and re-uploads everywhere automatically.

**What it does.** After each backup, archiver assembles the kit — the full effective configuration (every non-secret setting, every secret, and the keys), generated recreation notes (`RECREATE.txt`: hostname, schedule, every container path that needs a mount, required capabilities), and your deployment manifests if mounted (below) — encrypts it to that password (AES-256, pbkdf2), and uploads it as a **plain file beside the duplicacy data on every storage target**: `archiver-recovery-kit-<hostname>.tar.enc`, with a companion `README.txt` holding the decrypt command. It is not inside duplicacy's storage format, so you can download it from any provider web UI or file browser with no tooling. The kit is re-encrypted and re-uploaded only when its content actually changes (or a new storage target appears); an unchanged config is a nightly no-op. (The name includes the hostname, so deployments sharing a storage target must have distinct hostnames — they already need that for distinct snapshot IDs.)

**Capturing your deployment manifest.** The kit cannot see files outside the container, so it cannot magically include your compose file or Kubernetes YAML — instead, anything mounted (read-only) at `/opt/archiver/deployment/` is captured **verbatim** into the kit. Each runtime uses its native mechanism:

- **Docker/Podman Compose**: `- ./compose.yaml:/opt/archiver/deployment/compose.yaml:ro` (in the shipped template — the manifest captures itself).
- **Kubernetes (incl. Flux/Argo GitOps)**: put the manifests in a ConfigMap and mount it at `/opt/archiver/deployment`. With kustomize, a `configMapGenerator` entry (`files: [archiver.yaml]`) keeps the ConfigMap — and therefore the kit — current on every git change automatically.
- **NixOS / systemd-managed containers**: bind the service definition, e.g. `"/etc/nixos/services/archiver.nix:/opt/archiver/deployment/archiver.nix:ro"`.

In GitOps setups your manifest is already replicated in git; the kit's copy is for the total-loss case where the git host is gone too. Without a mounted manifest the kit still stands alone: `RECREATE.txt` lists every fact archiver knows about how the container must be put together, and the compose template covers the rest.

**Carrying more in the kit.** `RECOVERY_KIT_EXTRA_PATHS` lists paths in the container (colon- or newline-separated; mount them read-only) that go into the kit's `extra/`: a disaster-recovery runbook, the scripts your restore hooks call, `git bundle` copies of the repositories you rebuild from. A recovery then has them before anything else is restored. A path that does not exist is reported as an error, and the kit goes out without it. Everything here is re-encrypted and re-uploaded whenever it changes, so keep large or fast-changing files out.

**Recovery** happens on any machine with stock `openssl` — no archiver, no other files, just the recovery password from your password manager: reach **any one** of your storage locations, download the kit, and run

```bash
openssl enc -d -aes-256-cbc -pbkdf2 -in archiver-recovery-kit-<hostname>.tar.enc | tar -xvf -
```

It prompts for the password and yields `archiver.env` + `secrets/` + `RECREATE.txt` (+ `deployment/` with your manifests) — everything needed to recreate the deployment (the recovery password itself is included, so the recreated deployment maintains its kit immediately). From there, restore your data with `archiver restore` as usual.

`archiver recovery-kit` uploads on demand (useful right after setup); `archiver recovery-kit force` re-uploads everywhere even if unchanged. An upload failure is logged and notified but never fails the backup; the failed target is retried on the next run. The kit is **write-only**: nothing ever reads it back at runtime, so it is never a boot dependency.

On B2, each update creates a new file version; old versions age out per your bucket lifecycle rules (they are ciphertext, so lingering versions are harmless).

#### Break-glass envelope

The kit needs two things to be useful: the recovery password, and a way to reach one storage. If both live only in systems the backups protect (a password manager hosted on the same servers, a login whose 2FA is there), recovery is circular. The envelope breaks the circle: one printed page, kept somewhere that does not share fate with the backups, from which any one storage is enough to recover with no help from anyone.

```bash
docker exec archiver archiver envelope              # writes /opt/archiver/envelope/envelope-<hostname>.{pdf,html}
docker cp archiver:/opt/archiver/envelope ./envelope && docker exec archiver rm -rf /opt/archiver/envelope
# print ./envelope/envelope-<hostname>.pdf, then:
docker exec archiver archiver envelope confirm
rm -rf ./envelope
```

The page holds the recovery password, the decrypt command, and for each storage target where the kit sits (address, user, bucket, path) with a credential that can read it, as text and as QR codes, plus space to write account-recovery codes by hand. Each storage's block also has a command that downloads the kit from any machine: `sftp` for an SFTP storage, and for the others one `rclone copyto` line that needs nothing but rclone (no config file; it carries the credential shown in `RCLONE_CONFIG_KIT_*` variables, so it never appears in the process list). A OneDrive token changes as it is used, so a OneDrive block says to sign in and download the kit instead. The files are plaintext, owner-only, and never sent anywhere: print one and delete them.

**Break-glass credentials.** By default each storage's block carries its backup credential, marked **FULL ACCESS**, because whoever holds the page could also delete those backups. Where the provider allows a narrower credential, create one that can only read the bucket and give it to archiver as a secret file; the page then carries it instead (archiver never uses it for anything else):

| Storage type | Secret files (`/run/secrets/...`) |
|---|---|
| B2 (`b2`, `b2-custom`) | `storage_target_N_breakglass_b2_id`, `storage_target_N_breakglass_b2_key` (a key with `listBuckets`, `listFiles`, `readFiles` on the bucket) |
| S3 (`s3`, `s3c`, `minio`, `minios`) | `storage_target_N_breakglass_s3_id`, `storage_target_N_breakglass_s3_secret` |
| SFTP (`sftp`, `sftpc`) | `storage_target_N_breakglass_ssh_key` (a private key for a read-only account, whose name goes in the `STORAGE_TARGET_N_BREAKGLASS_SFTP_USER` env var; otherwise the page carries the backup key, since the kit holding it is itself on that server) |
| Any other type | `storage_target_N_breakglass_<secret>` for each of its keys, passwords and tokens that has one: `wasabi_key`/`wasabi_secret`, `azure_key`, `swift_key`, `webdav_password`, `smb_password`, `storj_key`, `fabric_token` (the S3 and B2 variants use the rows above). The page uses them only when all of a target's are set. Drive tokens, app secrets, service-account keys and the Storj passphrase have none: the page carries the backup ones, marked **FULL ACCESS**. |

**Keeping it current.** `archiver envelope confirm` records a fingerprint of what the page says (a hash keyed by the recovery password; it reveals nothing). After every recovery-kit run archiver compares it with what the page would say now: when a secret or storage on it changes, `archiver status` shows `Envelope: OUT OF DATE`, healthcheck warns, and one notification is sent. A year after confirming, the same happens as a reminder to check the envelope is still there and readable.

### Maintenance (check + prune)

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

#### Exhaustive prune frequency

A normal prune is snapshot-metadata work (fast); `-exhaustive` additionally lists every chunk on the storage to garbage-collect orphans — expensive on remote storages (a full sftp listing can take hours) while orphans are rare, so it runs on its own interval. `PRUNE_EXHAUSTIVE_FREQUENCY` is evaluated per storage at each maintenance run: once the interval has elapsed since the last exhaustive success, that run's prune includes `-exhaustive`. An infrequent maintenance schedule simply fires it belatedly at the next opportunity. Force it any time with `archiver maintenance exhaustive`.

#### Multi-Repository Shared Storage

If multiple archiver deployments back up to the same storage target, **only ONE should maintain it** to avoid duplicate work and prune races:

1. Set `PRUNE_BACKUPS="false"` and `CHECK_BACKUPS="false"` in all but one deployment
2. The designated deployment maintains all snapshot IDs on the shared storage via the `-all` flag

See the [Duplicacy prune documentation](https://forum.duplicacy.com/t/prune-command-details/1005) for more details on the two-step fossil collection algorithm.

Maintenance runs from a repository of its own in `logs/.maintenance-repo/`, whose cache holds the pending fossil collections of the storages it prunes; mount the logs directory so they survive container restarts (otherwise their chunks wait for the next exhaustive prune). On its first run it takes over the collections earlier versions left in the service directories' repositories.

#### Secondary storages under copy workers

When copy workers run (a schedule and at least one secondary), maintenance keeps to the primary, and each worker maintains its own secondary once it has caught up:

- **Mirroring.** The primary's retention is the only one: after each catch-up, and whenever a maintenance run prunes the primary, the worker deletes on its secondary the revisions the primary has pruned, so an offsite never holds a revision the primary dropped or lacks one it kept. It touches only this deployment's snapshot IDs (`<hostname>-...`), never a snapshot ID's newest revision, nothing when the primary's listing fails or lacks the ID entirely, and refuses (with a notification) a pass that would delete more than half of an ID's revisions. After a deliberate retention change, `archiver mirror --allow-large` lets the next pass through. `archiver mirror --dry-run` shows what the next pass would delete. Every deletion is logged in `copies.log`.
- **Exhaustive prune** on `PRUNE_EXHAUSTIVE_FREQUENCY` (`archiver maintenance exhaustive` forces one on the workers' next pass too). Mirroring and the exhaustive prune run only with `PRUNE_BACKUPS="true"`, so the shared-storage rule above still applies.
- **Check** on each secondary's own interval, when its worker is otherwise idle (a backup interrupts it; it runs again later): `STORAGE_TARGET_N_CHECK_INTERVAL`, else `CHECK_INTERVAL`, else 1 day for local, B2 and S3 storages and 7 days for SFTP, where a check lists every chunk. Intervals take `d`, `h` or `m` (`7d`, `12h`). `archiver status` shows when each secondary was last checked and flags a check more than twice its interval overdue. Checks run only with `CHECK_BACKUPS="true"`.

Each worker keeps a small repository in `logs/.copy-repos/` whose cache holds Duplicacy's pending fossil collections; mount the logs directory so they survive container restarts (otherwise their chunks wait for the next exhaustive prune).

### Checking a deployment (`archiver doctor`)

`docker exec archiver archiver doctor` checks the whole deployment in one pass and prints a line per check, `OK`, `WARN` or `FAIL`, saying what to do about anything wrong (exit 1 on any failure). It never creates a storage or writes data to one (it reads one small file to prove the RSA key decrypts it):

- **Configuration and secrets:** every setting and required secret, the RSA private key decrypting with `rsa_passphrase` and matching `public.pem`, the recovery kit password, and the notification destinations (`--notify` sends each a test).
- **Storages:** each reachable with its credentials and holding a Duplicacy storage that opens with the storage password, whose data decrypts with the mounted RSA key (checked on the smallest file up to 64 MB). A storage that does not exist yet is reported, never created.
- **Container:** the capabilities, the logs directory being a mounted volume (its state survives a recreate only then), free space for logs and the drill directory, the service directories, and the hooks (executable, and safe to run).
- **Freshness:** each service's newest revision on every storage (flagged when older than twice `BACKUP_SCHEDULE`'s interval), the copy workers, the recovery kit on every storage, the envelope, and the last restore drill.

Run it after setting up or changing a deployment, and after restoring one.

### Restore drills

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

### Performance

```bash
DUPLICACY_THREADS="10"
BACKUP_PARALLELISM="2"
```

`DUPLICACY_THREADS` is the number of parallel upload/download threads for each duplicacy operation. (Default: 4)

`BACKUP_PARALLELISM` is how many services a backup processes at once, each with its pre-backup hook, backup and post-backup hook. (Default: 2.) Set `1` to back them up one after another, in order, as before v1, for example when one service's hook depends on another's having finished. Each running service uses `DUPLICACY_THREADS` threads, so peak load grows with both.


### Backup Health

`archiver healthcheck` is the container's liveness check: it fails only for what a restart or a person must fix (a dead scheduler, a crashed run, no space for logs), so a failed backup never makes the container restart. Whether the backups are good is a separate answer, `archiver health --backups`:

| State | Meaning | Exit |
|---|---|---|
| `OK` | Every service has a good primary backup, nothing is wrong | 0 |
| `DEGRADED` | Backups are good, but a secondary is behind or down or a copy to it failed, a check or mirror failed, maintenance failed, or a service has not been backed up yet | 1 |
| `FAILING` | Two scheduled backups (`BACKUP_SCHEDULE`) have come since a service's last good primary backup, or its last backup failed, a backup was refused, the recovery kit or a restore drill is failing, or a log cannot be written | 2 |

It lists why, and `archiver status` shows it first. For a monitor such as Uptime Kuma, run `docker exec archiver archiver health --backups` and alert on a non-zero exit, or read `backup_health` from `archiver status --json`, which also carries each service's last backup, the copy workers, drills, storage maintenance and the open incidents. Each service's last backup is kept in `logs/.backup-state.json`.

### Status Page

With `WEB_PORT` set (for example `8470`), Archiver serves a read-only page: backup health and why, each service's last backup, the copy workers, storage maintenance, restore drills, open incidents and the last 200 lines of a log (backup, maintenance, copies or drill). It refreshes every 30 seconds, and `/status.json` serves the same as `archiver status --json`. Nothing on it changes anything (anything but GET is refused), and no secret appears on it.

**It has no login of its own.** Put it behind a reverse proxy that authenticates (Caddy, Traefik or nginx with basic auth or forward auth such as Authelia), or publish the port only on a trusted network, for example `127.0.0.1:8470:8470` for a proxy on the same host. A startup line warns while it is on. Without `WEB_PORT` nothing listens.

### Check-ins

A check-in is a dead man's switch: an outside monitor (an Uptime Kuma push monitor, healthchecks.io) expects to hear from Archiver on a schedule and alerts when it does not, so a container that is down or a backup that never ran is noticed as surely as one that failed.

- `CHECKIN_URL` is called after each backup run that succeeds. After one that fails, its fail variant is called, so the alert comes at once rather than at the monitor's timeout: an Uptime Kuma push URL (`/api/push/…`) gets `status=down`, any other gets `/fail` added to its path (healthchecks.io and its kind).
- `STORAGE_TARGET_N_CHECKIN_URL` is called when that secondary is caught up (a copy done or a check passed), and its fail variant when copies to it or its check fail.

Set the monitor's expected interval a little longer than the backup schedule's. A check-in that cannot be delivered is logged as a warning; the monitor alerts on the silence anyway. The URLs are settings, not secrets: whoever has one can only send check-ins.

### Metrics

Archiver keeps Prometheus metrics in `logs/archiver.prom`, rewritten after every backup and every minute while the container runs. Point node-exporter's textfile collector at the logs volume (`--collector.textfile.directory`) and nothing else is needed. With `METRICS_PORT` set (for example `9469`, published in `compose.yaml`), the daemon also serves the same on `http://<host>:<port>/metrics` for Prometheus to scrape; without it nothing listens.

| Metric | Labels | Meaning |
|---|---|---|
| `archiver_backup_health` | | 0 OK, 1 DEGRADED, 2 FAILING ([Backup Health](#backup-health)) |
| `archiver_running` | `run` | Whether a backup, maintenance or drill is running |
| `archiver_service_last_success_timestamp_seconds` | `service`, `directory` | When the service last backed up to the primary |
| `archiver_service_last_attempt_timestamp_seconds`, `_last_duration_seconds`, `_last_ok` | `service`, `directory` | Its last backup: when it ended, how long it took, whether it succeeded |
| `archiver_service_last_revision`, `_last_uploaded_bytes` | `service`, `directory` | The revision and the bytes uploaded by its last successful backup |
| `archiver_copy_behind_revisions`, `_failing`, `_last_success_timestamp_seconds`, `_last_check_timestamp_seconds` | `target` | Each secondary's copy worker |
| `archiver_storage_last_check_timestamp_seconds`, `_last_prune_timestamp_seconds` | `storage` | Maintenance's last successful check and prune |
| `archiver_drill_last_ok`, `_last_timestamp_seconds` | `storage`, `service` | The last restore drill |
| `archiver_incidents_open`, `archiver_incident_open` | `key`, `title` | Open incidents |

For example, alert when `time() - archiver_service_last_success_timestamp_seconds > 2 * 86400` or `archiver_backup_health > 0`.

### Storage Outages

Before a backup, a copy, a check or a prune, archiver makes a quick read-only check that the storage can be reached with its credentials, so a storage that is down costs seconds rather than Duplicacy's long retries:

- **Primary down:** the backup fails at once. No service is started (no pre-backup hook stops anything for a backup that cannot happen), each is reported skipped, and one **PRIMARY DOWN** notification is sent at a higher priority (Pushover priority 1, ntfy 5); backup health shows FAILING. If the primary is lost mid-run, a failed service backup checks it again, and when it is gone no further service starts (post-backup hooks still run for services whose pre hook ran). The next backup that reaches it sends one recovery notice.
- **Secondary down:** its copy is skipped with one line. The copy workers retry on their schedule, each retry checking first; a storage still down after 30 minutes is one alert.
- **During maintenance:** a secondary that cannot be reached fails its own check and prune, and the others are still maintained. The secondaries are registered through the primary, so a primary that cannot be reached fails the maintenance run.

### Notifications

Archiver sends to any combination of Pushover, an [Apprise API](https://github.com/caronc/apprise-api) server and [ntfy](https://ntfy.sh). Each destination receives the kinds of notification its setting asks for:

| `NOTIFY_ON` | Sent |
|---|---|
| `failures` (default) | A backup, copy, restore or drill that failed; a refused backup; a storage down or a failed storage check, and its recovery |
| `problems` | Also warning signs, such as a mirror pass refused for deleting too much |
| `everything` | Also routine news: backup and maintenance completed, a run paused, resumed or stopped |

`NOTIFY_ON` sets every destination; `PUSHOVER_NOTIFY_ON`, `APPRISE_NOTIFY_ON` and `NTFY_NOTIFY_ON` override it for one (for example `failures` to your phone, `everything` to ntfy). Log levels (INFO, WARNING, ERROR) are about log lines and do not decide notifications.

A notification is one per incident, not one per error line: a failed backup sends one message listing its errors (the first ten; the log has them all) when it ends, a storage down is one alert, not one per retry. While an incident lasts it is notified again every `ALERT_REPEAT_INTERVAL` (default `24h`; `6h`, `2d`, or `0` for never), and when it clears (a clean backup, a storage caught up, a check or drill passing again) one recovery notice goes to the destinations the alert went to. The open incidents are kept in `logs/.incidents.json`. `archiver notify test` sends one message to every destination, saying which events it receives.

```bash
# Pushover
NOTIFICATION_SERVICE="Pushover"   # with secrets pushover_user_key and pushover_api_token

# Apprise API: the notify URL (with user:password@ if the server needs basic auth) is the
# secret apprise_url; APPRISE_TAGS optionally sends each kind with an Apprise tag
APPRISE_TAGS="failure=critical,problem=alerts,routine=quiet"

# ntfy: server and topic; an access token, if the topic needs one, is the secret ntfy_token
NTFY_URL="https://ntfy.sh/my-archiver"
```

Apprise receives `title`, `body`, a `type` (info, warning, failure) and the kind's tag when `APPRISE_TAGS` names one (otherwise `all`, so every URL of the configuration is notified, tagged or not); an HTTP 424 (some URL of the tag failed) is logged as a failed notification and not retried, so the URLs that did receive it are not sent it twice. ntfy gets priority 2 for routine news, 3 for problems and 4 for failures.

---

<details>
<summary><h2>Manual Commands</h2></summary>

With `BACKUP_SCHEDULE`/`MAINTENANCE_SCHEDULE` set, the pipelines run automatically. Without them, run commands manually.

### View logs

```bash
docker exec -it archiver archiver logs
docker logs --tail 20 -f archiver
```

### Check status

```bash
docker exec archiver archiver status
docker exec archiver archiver healthcheck
```

### Start backup

```bash
docker exec archiver archiver backup --detach   # run a backup in the background
docker exec archiver archiver logs              # follow it
docker exec archiver archiver maintenance       # run check + prune now
```

### Manage active backups

```bash
docker exec archiver archiver pause
docker exec archiver archiver resume
docker exec archiver archiver stop
```

### Full Command Reference

```bash
archiver backup            # Run the backup pipeline now (synchronous, exit code propagates)
archiver backup --detach   # Run it in the background (follow with 'archiver logs')
archiver maintenance       # Run per-storage check + prune now (synchronous)
archiver maintenance exhaustive  # Same, forcing the full-listing exhaustive prune
archiver drill [SERVICE] [STORAGE]  # Run a restore drill now (README "Restore drills")
archiver doctor [--notify]  # Check everything read-only (README "Checking a deployment"); --notify sends a test
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
archiver help              # Show help
```

</details>

<details>
<summary><h2>Scheduled Backups via External Schedulers</h2></summary>

### Running a Backup from an External Scheduler (`run backup`)

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

</details>

<details>
<summary><h2>Restoring Data</h2></summary>

### Interactive Restore with Existing Container

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

### Recovering a Lost Host (`archiver recover`)

When the host is gone, all you need is the recovery kit (on every storage, and fetched with the envelope's command) and its password. On the new host, start a one-off container with the service directories mounted where the old deployment had them, the kit, and an empty directory for the recovered configuration:

```bash
docker run -it --rm --hostname <old hostname> \
  -v ./archiver-recovered:/opt/archiver/recovered \
  -v ./archiver-recovery-kit-<old hostname>.tar.enc:/kit.tar.enc:ro \
  -v /srv:/srv \
  forgejo.bryantserver.com/sisyphusmd/archiver:1 recover /kit.tar.enc
```

It asks for the kit password (or reads a mounted `recovery_password` secret), then, checking each step before the next: decrypts the kit, writes its `archiver.env`, `secrets/` (owner-only) and `RECREATE.txt` to `/opt/archiver/recovered`, puts the keys in place, checks every storage, and, after you confirm (`--yes` skips the question), restores every service of the old host into its directory from `SERVICE_DIRECTORIES` (a pattern like `/srv/*/` places each service beside its siblings, even though the directories do not exist yet). `--hook` also runs each service's restore hook. It ends by saying how to recreate the deployment: `archiver.env` as its environment, `secrets/` as `/run/secrets`, and the old hostname, so new backups continue the same snapshots. Move the plaintext secrets into your secret store and delete the directory afterwards.

### One-Off Restore with Temporary Container

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

### Non-interactive Restore (CI / Kubernetes)

Snapshot IDs are `<hostname>-<service directory basename>` (e.g. `backup-server-nextcloud`). When restoring on a different machine, run the container with `hostname:` set to the ORIGINAL value or pass the full `SNAPSHOT_ID` explicitly.

For automated disaster recovery flows (e.g. Kubernetes init containers), Archiver exposes two non-interactive commands driven by environment variables. Exit codes are the machine-readable answer; stdout is informational.

#### `archiver snapshot-exists`

Probes every configured storage target for `SNAPSHOT_ID` and short-circuits on the first hit. Useful to gate a restore on whether a backup is actually available.

| Env Var | Required | Description |
|---------|----------|-------------|
| `SNAPSHOT_ID` | Yes | Snapshot ID to look up |

Exit codes:
- `0` — snapshot exists on at least one target (prints `EXISTS`)
- `1` — no target has the snapshot (prints `NOT FOUND`)
- `2` — all targets unreachable or invalid env (prints `UNDETERMINED`)
- `3` — an Archiver backup is in progress; check skipped

#### `archiver auto-restore`

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

#### Running Without a Long-Lived Container (`run` mode)

For Kubernetes Jobs, init containers, or one-shot `docker run` invocations, use the entrypoint's `run` mode (see [Container Modes](#container-modes)). The configuration is loaded, the subcommand runs, and the container's exit code equals the subcommand's exit code:

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

</details>

<details>
<summary><h2>Advanced Usage</h2></summary>

### Custom Service Scripts

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

A hook can be any program the container can run (most are shell scripts). It runs in the service directory, and its output goes to the Archiver log; a line starting `[ERROR] ` or `[WARNING] ` is logged at that level, and an `[ERROR]` line counts as an error of the run (with a notification) without skipping the service. A background process a hook starts must redirect its own output: Archiver stops reading a hook's output two seconds after the hook exits. It receives `ARCHIVER_SERVICE`, `ARCHIVER_SERVICE_DIR`, `ARCHIVER_SNAPSHOT_ID`, and `ARCHIVER_STATE_DIR`, a scratch directory shared by that run's `pre-backup` and `post-backup` (to pass a value from one to the other). `post-backup` also receives `ARCHIVER_BACKUP_RESULT`: `success`, `failed`, `skipped`, or `stopped`. Hooks never receive storage credentials or the RSA passphrase.

Exit codes count. If `pre-backup` exits non-zero (the dump above failing, say), that service is **not** backed up that run: its newest revision stays the last good one instead of one holding a broken dump, and the run reports an error while every other service still backs up. `post-backup` always runs once `pre-backup` has, even after a failed `pre-backup`, a failed backup, or a stop, so it can restart whatever `pre-backup` stopped; a non-zero exit from it is reported as an error. A hook file that exists but is not executable is an error, and that service is skipped.

**Who can change a hook.** Hooks run as root in the container, which can read every storage credential and the RSA key. So a hook runs only if nobody but its owner can change it: the hook file and every directory above it must not be writable by group or others, except a sticky directory such as `/tmp` (for a symlinked hook, its target's path is checked the same way). Otherwise the service is skipped with an error that names the fix (`chmod go-w <path>`). The owner is not checked, so a service's own user may keep its hooks; but anyone who can write a service directory can then make the backup run code as root. If an app (or a user) you trust less than Archiver owns a service directory, keep that service's hooks in `HOOKS_DIR` instead: a directory outside the backed-up data, mounted read-only, holding `<service>/pre-backup`, `<service>/post-backup` and `<service>/post-restore`, where `<service>` is the service directory's name. With `HOOKS_DIR` set, hooks come only from there, and any left in a service directory are ignored with a warning. Duplicacy's own `.duplicacy/scripts` never run.

**Upgrading from `service-backup-settings.sh`:** convert once with `archiver migrate hooks`. For every configured service it writes `pre-backup` and `post-backup` wrappers that call your existing functions, writes your `DUPLICACY_FILTERS_PATTERNS` to `filters` (a pattern naming the old file is rewritten to name the new ones), and keeps the old file as `service-backup-settings.legacy.sh`, which the wrappers source. It warns if a post-backup function reads a variable its pre-backup function sets, which no longer carries over between the two processes. A container whose services still hold `service-backup-settings.sh` refuses to start and says so; run the conversion as its command, with the same mounts and environment (`docker compose run --rm archiver migrate hooks`), then start it again.

### Custom Restore Scripts

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

</details>

<details>
<summary><h2>Documentation</h2></summary>

### Migration and Setup Guides

- [Legacy to Docker Migration](docs/guides/migration/legacy-to-docker.md) - Migrating from v0.3.2-v0.6.5 to Docker-only v0.7.0
- [Uninstalling Legacy Installation](docs/guides/maintenance/uninstall-legacy.md) - Removing legacy installation after migration

### Configuration Guides

- [Editing Configuration](docs/guides/configuration/editing-config.md) - How to edit config in Docker environment
- [Local Storage Setup](docs/guides/configuration/local-storage-setup.md) - Adding local disk as primary backup target
- [SSH Key Management](docs/guides/configuration/ssh-key-management.md) - Creating and managing SSH keys for SFTP

</details>

---

## Licensing

Archiver is free and open-source software licensed under [GNU AGPL-3.0](LICENSE).

Archiver uses the [Duplicacy CLI v3.2.5](https://github.com/gilbertchen/duplicacy/tree/v3.2.5) binary as an external tool. Duplicacy is licensed separately under [its own terms](https://github.com/gilbertchen/duplicacy/blob/v3.2.5/LICENSE.md):

- **Free for personal use** and **commercial trials**
- **Requires a CLI license** for non-trial commercial use ($50/computer/year from [duplicacy.com](https://duplicacy.com/buy.html))

**What counts as commercial use?** Backing up files related to employment or for-profit activities.

**Note:** Restore and management operations (restore, check, copy, prune) never require a license. Only the `backup` command requires a license for commercial use.

If you're using Archiver commercially, please purchase a Duplicacy CLI license to support the project that makes this tool possible

