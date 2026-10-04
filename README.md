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
- **Notifications**: Pushover alerts for successes and failures
- **Easy Restoration**: Interactive restore script to recover specific revisions

---

## Prerequisites

- A container runtime — [Docker](https://docs.docker.com/get-docker/), [Podman](https://podman.io/), or Kubernetes. The rest of this README uses Docker commands as the default; translate them to your runtime as needed.
- [Docker Compose](https://docs.docker.com/compose/install/) (optional, for easier management of a long-lived container)

---

## Storage Backend Setup

Prepare at least one storage location before running init. Expand the sections below for setup instructions.

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
| `SYSTEMCTL_FORCE_BUS` | No | Set to `1` to enable systemctl access to host services via D-Bus socket (requires socket mounts, see above) |

Archiver's configuration itself (service directories, storage targets, secrets) is likewise environment variables plus file-based secrets. See [Configuration Sources](#configuration-sources).

### Container Modes

The entrypoint selects one of three modes based on the first container argument:

| Mode | How it's invoked | Behavior |
|------|------------------|----------|
| `init` | `docker run ... archiver:<tag> init` | Interactive setup: generates env-native materials and the recovery password. Exits when done. |
| _default_ (daemon) | `docker run ... archiver:<tag>` (no args) | Loads the configuration, then either runs the scheduler, `archiver daemon` (if `BACKUP_SCHEDULE` and/or `MAINTENANCE_SCHEDULE` is set) or idles on `tail -f /dev/null` so you can `docker exec` in. |
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

**Non-secret settings (plain env vars).** `SERVICE_DIRECTORIES`, the non-secret `STORAGE_TARGET_N_*` fields (`NAME`, `TYPE`, `LOCAL_PATH`, `SFTP_URL`, `SFTP_PORT`, `SFTP_USER`, `SFTP_PATH`, `B2_BUCKETNAME`, `S3_BUCKETNAME`, `S3_ENDPOINT`, `S3_REGION`), `CHECK_BACKUPS`, `CHECK_INTERVAL`, `STORAGE_TARGET_N_CHECK_INTERVAL`, `PRUNE_BACKUPS`, `PRUNE_KEEP`, `PRUNE_EXHAUSTIVE_FREQUENCY`, `DUPLICACY_THREADS`, `NOTIFICATION_SERVICE`, and `RECOVERY_KIT_EXTRA_PATHS`. As an env var, `SERVICE_DIRECTORIES` is a colon-delimited list rather than a bash array, for example `SERVICE_DIRECTORIES=/srv/*/:/home/user/data/` (newlines also work, so a YAML block scalar is fine).

**Secrets (files only).** Secrets are never read from a plain env var (one would leak through `/proc` and `docker inspect`, and Archiver purges any it finds). Each secret is read from a file: `<NAME>_FILE` if set, otherwise `/run/secrets/<lowercased name>`. The secrets are `STORAGE_PASSWORD`, `RSA_PASSPHRASE`, `PUSHOVER_USER_KEY`, `PUSHOVER_API_TOKEN`, and each target's `B2_ID`, `B2_KEY`, `S3_ID`, `S3_SECRET` and optional [break-glass credentials](#break-glass-envelope). For example, `STORAGE_PASSWORD` reads `/run/secrets/storage_password` and `STORAGE_TARGET_1_B2_KEY` reads `/run/secrets/storage_target_1_b2_key`. `STORAGE_PASSWORD` must be at least 8 characters (a Duplicacy requirement). Because `/run/secrets` is the native mount path for Docker and Kubernetes secrets, a Compose or Swarm `secrets:` entry named to match (for example `storage_password`) is picked up with no extra configuration.

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

Define multiple storage locations (local disk, SFTP, B2, S3):

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

The page holds the recovery password, the decrypt command, and for each storage target where the kit sits (address, user, bucket, path) with a credential that can read it, as text and as QR codes, plus space to write account-recovery codes by hand. The files are plaintext, owner-only, and never sent anywhere: print one and delete them.

**Break-glass credentials.** By default each storage's block carries its backup credential, marked **FULL ACCESS**, because whoever holds the page could also delete those backups. Where the provider allows a narrower credential, create one that can only read the bucket and give it to archiver as a secret file; the page then carries it instead (archiver never uses it for anything else):

| Storage type | Secret files (`/run/secrets/...`) |
|---|---|
| B2 | `storage_target_N_breakglass_b2_id`, `storage_target_N_breakglass_b2_key` (a key with `listBuckets`, `listFiles`, `readFiles` on the bucket) |
| S3 | `storage_target_N_breakglass_s3_id`, `storage_target_N_breakglass_s3_secret` |
| SFTP | `storage_target_N_breakglass_ssh_key` (a private key for a read-only account, whose name goes in the `STORAGE_TARGET_N_BREAKGLASS_SFTP_USER` env var; otherwise the page carries the backup key, since the kit holding it is itself on that server) |

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

#### Secondary storages under copy workers

When copy workers run (a schedule and at least one secondary), maintenance keeps to the primary, and each worker maintains its own secondary once it has caught up:

- **Mirroring.** The primary's retention is the only one: after each catch-up, and whenever a maintenance run prunes the primary, the worker deletes on its secondary the revisions the primary has pruned, so an offsite never holds a revision the primary dropped or lacks one it kept. It touches only this deployment's snapshot IDs (`<hostname>-...`), never a snapshot ID's newest revision, nothing when the primary's listing fails or lacks the ID entirely, and refuses (with a notification) a pass that would delete more than half of an ID's revisions. After a deliberate retention change, `archiver mirror --allow-large` lets the next pass through. `archiver mirror --dry-run` shows what the next pass would delete. Every deletion is logged in `copies.log`.
- **Exhaustive prune** on `PRUNE_EXHAUSTIVE_FREQUENCY` (`archiver maintenance exhaustive` forces one on the workers' next pass too). Mirroring and the exhaustive prune run only with `PRUNE_BACKUPS="true"`, so the shared-storage rule above still applies.
- **Check** on each secondary's own interval, when its worker is otherwise idle (a backup interrupts it; it runs again later): `STORAGE_TARGET_N_CHECK_INTERVAL`, else `CHECK_INTERVAL`, else 1 day for local, B2 and S3 storages and 7 days for SFTP, where a check lists every chunk. Intervals take `d`, `h` or `m` (`7d`, `12h`). `archiver status` shows when each secondary was last checked and flags a check more than twice its interval overdue. Checks run only with `CHECK_BACKUPS="true"`.

Each worker keeps a small repository in `logs/.copy-repos/` whose cache holds Duplicacy's pending fossil collections; mount the logs directory so they survive container restarts (otherwise their chunks wait for the next exhaustive prune).

### Performance

```bash
DUPLICACY_THREADS="10"
```

Number of parallel upload/download threads for duplicacy operations. (Default: 4)


### Notifications

```bash
NOTIFICATION_SERVICE="Pushover"
PUSHOVER_USER_KEY="userKey"
PUSHOVER_API_TOKEN="apiToken"
```

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
archiver stop [backup|maintenance|all]  # Stop a pipeline gracefully (default all: both pipelines)
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
archiver healthcheck       # Check system health (Docker HEALTHCHECK uses this; on Kubernetes wire it as an exec probe)
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
| `RUN_RESTORE_SERVICE` | No | Non-empty runs `./restore-service.sh` after a successful file restore (DB reload, stack restart); its exit code propagates |
| `RESTORE_THREADS` | No | Override download thread count (default matches `DUPLICACY_THREADS`) |

Exit codes:
- `0` — snapshot restored (and, if `RUN_RESTORE_SERVICE` set, `restore-service.sh` succeeded)
- `1` — snapshot not found on any reachable target, the restore itself failed, or `restore-service.sh` failed
- `2` — all targets unreachable, or invalid env
- `3` — an Archiver backup is in progress; restore skipped

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

**Upgrading from `service-backup-settings.sh`:** run `archiver migrate hooks` once. For every configured service it writes `pre-backup` and `post-backup` wrappers that call your existing functions, writes your `DUPLICACY_FILTERS_PATTERNS` to `filters` (a pattern naming the old file is rewritten to name the new ones), and keeps the old file as `service-backup-settings.legacy.sh`, which the wrappers source. It warns if a post-backup function reads a variable its pre-backup function sets, which no longer carries over between the two processes. Until a deployment is migrated it keeps running on the previous (bash) backup pipeline.

### Custom Restore Scripts

Create `restore-service.sh` in any service directory to run post-restore tasks:

```bash
#!/bin/bash
# Runs after restoration completes

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

