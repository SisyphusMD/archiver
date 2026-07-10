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

## ⚠️ Breaking Change in v0.9.0

**`BUNDLE_PASSWORD` is no longer read from the environment.** The bundle password is now a file-based secret: mount it at `/run/secrets/bundle_password` (a Compose or Swarm `secrets:` entry named `bundle_password`, as in the [compose template](compose.yaml)), or point `BUNDLE_PASSWORD_FILE` at another path. A container that still sets `BUNDLE_PASSWORD` via `environment:` or `env_file` fails fast at startup with a migration message.

To upgrade an existing deployment: write the password to a file (for example `./secrets/bundle_password`), add the `secrets:` entries from the compose template, and remove `BUNDLE_PASSWORD` from the environment. This password decrypts the entire bundle, including the RSA private key, and an environment variable leaks through `docker inspect` and `/proc` — a file does not. See [Configuration Sources](#configuration-sources-env-native-or-bundle) for the full file-based secrets model, and [Migrating a bundle to env-native](#migrating-a-bundle-to-env-native) to move the rest of the configuration out of the bundle too (optional).

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

**Skip this step if you already have configuration** — env-native materials (`archiver.env` + secret files) or a bundle (`bundle.tar.enc` / `export-*.tar.enc`) from a previous installation.

For new installations, run initialization interactively (the mount is just an output directory for the generated materials):

```bash
docker run -it --rm \
  -v ./archiver-setup:/opt/archiver/setup \
  forgejo.bryantserver.com/sisyphusmd/archiver:0.9.0 init
```

This writes your configuration into `archiver-setup/`:

- `env-native/` — `archiver.env` (non-secret settings) plus `secrets/` (one file per secret, including the keys). **This is what you deploy with**: a Compose `environment:` block + `secrets:`, or a Kubernetes ConfigMap + Secret. The files are plaintext — move them into your secret store and delete `env-native/` afterwards.
- `bundle.tar.enc` — an encrypted, self-contained copy of the same configuration. Store it (and its password) somewhere safe as your disaster-recovery escrow. (It can also drive a deployment directly — transitional: move it into a directory mounted at `/opt/archiver/bundle` and see the commented alternative in [compose.yaml](compose.yaml).)

### Step 2: Configure Docker Compose

Create `compose.yaml` (env-native, the primary mode — fill the `environment:` values from `env-native/archiver.env` and point the `secrets:` files at where you moved `env-native/secrets/`):

```yaml
services:

  archiver:

    container_name: archiver
    image: forgejo.bryantserver.com/sisyphusmd/archiver:0.9.0
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
      CRON_SCHEDULE: "0 3 * * *"  # Ex: daily at 3am, or omit for manual mode
      TZ: "America/New_York"      # Timezone for scheduled backups and timestamps (default: UTC)
      # Non-secret settings from env-native/archiver.env:
      SERVICE_DIRECTORIES: "/mnt/backup-dir/"       # colon-delimited list
      STORAGE_TARGET_1_NAME: "local"
      STORAGE_TARGET_1_TYPE: "local"
      STORAGE_TARGET_1_LOCAL_PATH: "/mnt/backup/storage"
      ROTATE_BACKUPS: "true"

    secrets:                      # each lands at /run/secrets/<name>
      - storage_password
      - rsa_passphrase
      - rsa_private_key
      - rsa_public_key
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
      # - /var/run/docker.sock:/var/run/docker.sock  # For docker exec in scripts (optional)
      # - /path/to/host/restore-dir:/mnt/restore-dir # Restore location (will be prompted)

secrets:
  storage_password: { file: ./secrets/storage_password }
  rsa_passphrase:   { file: ./secrets/rsa_passphrase }
  rsa_private_key:  { file: ./secrets/rsa_private_key }
  rsa_public_key:   { file: ./secrets/rsa_public_key }
  # ssh_private_key: { file: ./secrets/ssh_private_key }
  # ssh_public_key:  { file: ./secrets/ssh_public_key }
```

To deploy from the encrypted bundle instead (transitional / cold-restore path), use the commented bundle-mode service in [compose.yaml](compose.yaml): mount `./archiver-bundle:/opt/archiver/bundle` and provide the bundle password as the `bundle_password` secret file.

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

**Note**: `no-new-privileges` is a kernel security option, not a Linux capability. It is compatible with `DAC_OVERRIDE`, `CHOWN`, and `FOWNER`, and is recommended for defense in depth. Scheduled backups (`CRON_SCHEDULE`) no longer need `SETGID` — Archiver uses [supercronic](https://github.com/aptible/supercronic), which runs jobs as the container user rather than forking with `setgid` like Debian's cron.

### Graceful Shutdown

The `stop_grace_period: 2m` setting allows the container time to complete cleanup when stopped. When `docker compose down` or `docker stop` is called, Archiver will:
- Complete any running pre-backup hooks
- Run post-backup hooks to restore services (e.g., restart databases, remove snapshots)
- Terminate gracefully

If your post-backup hooks take longer than 2 minutes, increase this value accordingly.

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `CRON_SCHEDULE` | No | Standard 5-field cron expression for automatic backups (empty = manual mode) |
| `TZ` | No | Timezone for scheduled backups and log timestamps (default: UTC) |
| `SYSTEMCTL_FORCE_BUS` | No | Set to `1` to enable systemctl access to host services via D-Bus socket (requires socket mounts, see above) |

The bundle password is **not** an environment variable. It is a file-based secret, read from `/run/secrets/bundle_password` (or the path in `BUNDLE_PASSWORD_FILE`); a container that still sets `BUNDLE_PASSWORD` in its environment fails fast at startup. Existing deployments that set it via `environment:` or `env_file` must move the value to that file and remove the env var. The rest of Archiver's configuration (service directories, storage targets, secrets, rotation) can also be supplied as environment variables and file-based secrets instead of, or on top of, the bundle. See [Configuration Sources](#configuration-sources-env-native-or-bundle).

### Container Modes

The entrypoint selects one of three modes based on the first container argument:

| Mode | How it's invoked | Behavior |
|------|------------------|----------|
| `init` | `docker run ... archiver:<tag> init` | Interactive setup: generates env-native materials + an encrypted escrow bundle. Exits when done. |
| _default_ (daemon) | `docker run ... archiver:<tag>` (no args) | Loads configuration (env-native and/or bundle), then either runs `supercronic` (if `CRON_SCHEDULE` is set) or idles on `tail -f /dev/null` so you can `docker exec` in. |
| `run` | `docker run ... archiver:<tag> run <subcommand>` | Loads configuration (env-native and/or bundle), then `exec`s a single non-interactive subcommand and exits with that subcommand's exit code. Designed for Kubernetes Jobs / init containers and other CI flows. |

`run` mode only accepts subcommands whose exit codes form a meaningful contract: `auto-restore`, `auto-restore-all`, `snapshot-exists`, `healthcheck`, and `backup` (a synchronous backup path intended for external schedulers — see [Running a Backup from an External Scheduler](#running-a-backup-from-an-external-scheduler-run-backup)). Any other subcommand is rejected with exit code `2`. The user-facing `archiver start` command remains async and is intentionally not supported here.

### Image Tags

- `forgejo.bryantserver.com/sisyphusmd/archiver:0.9.0` - exact version (recommended; this line always names the current release)
- `MAJOR.MINOR` (e.g. `0.9`) - receives patch updates automatically
- `MAJOR` (e.g. `0`) - receives minor and patch updates automatically

---

## Configuration

The settings below define what to backup and where. Supply them as environment variables plus file-based secrets (the primary, env-native mode), through the encrypted bundle's `config.sh` (transitional), or a mix of the two (see [Configuration Sources](#configuration-sources-env-native-or-bundle) below). Env-native settings are edited wherever they live — your compose file, ConfigMap, or secret store; for editing a bundle's `config.sh`, see [Editing Configuration](docs/guides/configuration/editing-config.md).

### Configuration Sources: Env-Native or Bundle

The primary mode is **env-native**: environment variables carry the non-secret settings and files under `/run/secrets` carry the secrets — config stays under version control (compose file / ConfigMap) and secrets stay in a secret store. Underneath, an optional, transitional baseline can exist. In increasing precedence:

1. **The encrypted bundle** (`config.sh` + keys, decrypted from `bundle.tar.enc`) — optional baseline and the cold-restore escrow: mount it and provide its password at `/run/secrets/bundle_password` and it becomes the baseline again.
2. **Environment variables** for non-secret settings, which override the bundle.
3. **Files** for secrets, which override the bundle.

With no bundle at all, configuration is fully env-native — this is the recommended deployment. With a bundle and no overrides, behavior is exactly as it was pre-0.9.0. Because the layers stack, an existing bundle deployment can migrate one value at a time: set an env var or mount a secret file, confirm the backup still runs, and repeat until nothing depends on the bundle.

**Non-secret settings (plain env vars).** These override the bundle when set: `SERVICE_DIRECTORIES`, the non-secret `STORAGE_TARGET_N_*` fields (`NAME`, `TYPE`, `LOCAL_PATH`, `SFTP_URL`, `SFTP_PORT`, `SFTP_USER`, `SFTP_PATH`, `B2_BUCKETNAME`, `S3_BUCKETNAME`, `S3_ENDPOINT`, `S3_REGION`), `ROTATE_BACKUPS`, `PRUNE_KEEP`, `DUPLICACY_THREADS`, and `NOTIFICATION_SERVICE`. As an env var, `SERVICE_DIRECTORIES` is a colon-delimited list rather than a bash array, for example `SERVICE_DIRECTORIES=/srv/*/:/home/user/data/` (newlines also work, so a YAML block scalar is fine). The bundle's bash-array form is still read.

**Secrets (files only).** Secrets are never read from a plain env var (one would leak through `/proc` and `docker inspect`, and Archiver purges any it finds). Each secret is read from a file: `<NAME>_FILE` if set, otherwise `/run/secrets/<lowercased name>`. The secrets are `BUNDLE_PASSWORD` (the bundle decryption password, read from `/run/secrets/bundle_password` or `BUNDLE_PASSWORD_FILE`), `STORAGE_PASSWORD`, `RSA_PASSPHRASE`, `PUSHOVER_USER_KEY`, `PUSHOVER_API_TOKEN`, and each target's `B2_ID`, `B2_KEY`, `S3_ID`, and `S3_SECRET`. For example, `STORAGE_PASSWORD` reads `/run/secrets/storage_password` and `STORAGE_TARGET_1_B2_KEY` reads `/run/secrets/storage_target_1_b2_key`. `STORAGE_PASSWORD` must be at least 8 characters (a Duplicacy requirement). Because `/run/secrets` is the native mount path for Docker and Kubernetes secrets, a Compose or Swarm `secrets:` entry named to match (for example `bundle_password`) is picked up with no extra configuration.

**Keys (files).** Keys are always files under `/opt/archiver/keys`. In env-native mode (no bundle) the RSA keypair must be provided as files at `/run/secrets/rsa_private_key` and `/run/secrets/rsa_public_key` (override the paths with `RSA_PRIVATE_KEY_FILE` / `RSA_PUBLIC_KEY_FILE`). The SFTP keypair is optional, for sftp targets, at `/run/secrets/ssh_private_key` and `/run/secrets/ssh_public_key` (override with `SSH_PRIVATE_KEY_FILE` / `SSH_PUBLIC_KEY_FILE`; restore needs both halves). When a bundle is also present, mounted key files override the bundle's keys.

Starting env-native from scratch (no bundle, no `archiver init`)? Generate the RSA keypair yourself — Duplicacy needs the traditional PKCS#1 PEM format, and the passphrase must match your `rsa_passphrase` secret:

```bash
openssl genrsa -aes256 -passout pass:YOUR_RSA_PASSPHRASE -traditional -out rsa_private_key 2048
openssl rsa -in rsa_private_key -passin pass:YOUR_RSA_PASSPHRASE -pubout -out rsa_public_key
```

(For sftp targets, also `ssh-keygen -t ed25519 -N "" -f ssh_private_key`, which writes `ssh_private_key` and `ssh_private_key.pub` — supply the latter as `ssh_public_key`.)

**What to escrow for disaster recovery (env-native).** To restore after losing the host you need, stored somewhere that does not burn down with it: `STORAGE_PASSWORD` (unlocks the Duplicacy storage), `RSA_PASSPHRASE` + `rsa_private_key` (decrypt the file data), your storage-target settings (`archiver.env` or equivalents), and for sftp targets the SSH keypair. Missing any of the first three means the backups are permanently undecryptable. The simplest escrow is still a bundle: run `archiver bundle export` and keep `bundle.tar.enc` + its password — that one artifact carries everything above.

### Migrating a bundle to env-native

To move an existing bundle deployment to env-native without hand-transcribing anything, run `archiver migrate` inside the container and copy the result out (the default output directory `/opt/archiver/migrate` is inside the container, not on the host):

```bash
docker exec archiver archiver migrate
docker cp archiver:/opt/archiver/migrate ./archiver-migrate
docker exec archiver rm -rf /opt/archiver/migrate
```

The copied files hold your secrets in **plaintext** — move them into your secret store, then delete the plain copies.

It writes the effective configuration as ready-to-use materials:

- `archiver.env`: the non-secret settings as `KEY=value`, for a Compose `environment:` block or a Kubernetes ConfigMap.
- `secrets/`: one file per secret plus the RSA/SSH keys, to load as Docker secrets, a Kubernetes Secret, or openbao entries mounted under `/run/secrets`.

Load those, start the container without the bundle, and you are fully env-native. The move is reversible: `bundle export` is mode-agnostic, so from an env-native deployment you can regenerate a portable encrypted bundle at any time for cold restore.

### Service Directories

Directories to backup, colon-delimited. Use `*` for subdirectories:

```bash
SERVICE_DIRECTORIES=/srv/*/:/home/user/data/
# /srv/*/           -> each subdirectory becomes its own repository
# /home/user/data/  -> a single repository
```

(Newlines work as separators too, so a YAML block scalar is fine. A legacy bundle `config.sh` may still declare it as a bash array — both forms are read.)

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

### Secrets

```bash
STORAGE_PASSWORD="encryption-password-for-duplicacy-storage"
RSA_PASSPHRASE="passphrase-for-rsa-private-key"
```

### Backup Rotation

```bash
ROTATE_BACKUPS="true"  # Enable automatic pruning after backups
PRUNE_KEEP="-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1"
```

**Default retention policy** keeps:
- All backups younger than 1 day old
- 1 backup every 1 day for backups older than 1 day (`-keep 1:1`)
- 1 backup every 7 days for backups older than 7 days (`-keep 7:7`)
- 1 backup every 30 days for backups older than 30 days (`-keep 30:30`)
- Delete all backups older than 180 days (`-keep 0:180`)

**Format:** `-keep n:m` means keep 1 snapshot every `n` days if the snapshot is at least `m` days old.

#### Per-Run Override

You can override `ROTATE_BACKUPS` for individual backup runs:

```bash
docker exec archiver archiver start prune   # Force pruning this run
docker exec archiver archiver start retain  # Skip pruning this run
```

#### Multi-Repository Shared Storage

If multiple repositories backup to the same storage target, **only ONE should run prune** to avoid race conditions:

1. Set `ROTATE_BACKUPS="false"` in all but one repository's config
2. The designated repository will prune for all snapshot IDs using the `-all` flag
3. Prune uses `-exhaustive` flag to remove ALL unreferenced chunks, including:
   - Orphaned chunks from manually deleted snapshots
   - Incomplete backup chunks from interrupted operations
   - Unreferenced chunks from any source

See the [Duplicacy prune documentation](https://forum.duplicacy.com/t/prune-command-details/1005) for more details on the two-step fossil collection algorithm.

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

With `CRON_SCHEDULE` set, backups run automatically. Without it, run commands manually.

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
docker exec archiver archiver start
docker exec archiver archiver start logs    # with log viewing
docker exec archiver archiver start prune   # force rotation
docker exec archiver archiver start retain  # force retention
```

### Manage active backups

```bash
docker exec archiver archiver pause
docker exec archiver archiver resume
docker exec archiver archiver stop
```

### Export/import bundle

```bash
docker exec -it archiver archiver bundle export
docker exec -it archiver archiver bundle import
```

### Full Command Reference

```bash
archiver start             # Run backup now
archiver start logs        # Run backup and follow logs
archiver start prune       # Run backup and force prune (ignore config)
archiver start retain      # Run backup without pruning (ignore config)
archiver stop              # Stop backup gracefully (completes cleanup)
archiver stop --immediate  # Stop backup immediately (skip cleanup)
archiver restart           # Stop then start backup
archiver pause             # Pause backup (experimental)
archiver resume            # Resume paused backup (experimental)
archiver logs              # Follow backup logs
archiver status            # Check if backup is running
archiver bundle export     # Create encrypted config/keys bundle
archiver bundle import     # Import from encrypted bundle
archiver restore           # Restore data from backup (interactive)
archiver auto-restore      # Restore one snapshot from backup (non-interactive, env-driven)
archiver auto-restore-all  # Restore every service in one pass (non-interactive)
archiver snapshot-exists   # Check if a snapshot exists on any storage target
archiver migrate [DIR]     # Write the effective config as env-native materials (env + secret files)
archiver backup [prune|retain]  # Synchronous backup, exit code propagates (external schedulers/Jobs)
archiver healthcheck       # Check system health (Docker HEALTHCHECK uses this; on Kubernetes wire it as an exec probe)
archiver help              # Show help
```

</details>

<details>
<summary><h2>Scheduled Backups via External Schedulers</h2></summary>

### Running a Backup from an External Scheduler (`run backup`)

For most users, a long-lived container with `CRON_SCHEDULE` set is the simplest way to get scheduled backups — the in-container scheduler fires `archiver start` on schedule, and you don't have to manage anything. Skip this section unless you specifically need to drive scheduling from *outside* the container.

If your environment already owns scheduling — e.g., a Kubernetes `CronJob`, a GitHub Actions scheduled workflow, a systemd timer on the host, or any other platform that spawns a short-lived container per run and expects a meaningful exit code — use the entrypoint's `run backup` mode instead. It loads the configuration (env-native or bundle), runs a backup **synchronously**, and exits with the backup's result code. The container terminates when the backup finishes; your scheduler then reports success or failure based on the exit code.

Exit codes:
- `0` — backup completed
- `1` — lock contention (another backup already in progress) or catastrophic startup failure
- non-zero — see stderr / logs for details

**Example: one-shot `docker run`** (env-native: `archiver.env` + `secrets/` as emitted by `init` or `migrate`)

```bash
docker run --rm \
  --env-file /path/to/archiver.env \
  -v /path/to/secrets:/run/secrets:ro \
  -v /path/to/host/backup-dir:/mnt/backup-dir \
  forgejo.bryantserver.com/sisyphusmd/archiver:0.9.0 run backup
```

(Bundle mode instead: replace the first two lines with `-v /path/to/bundle/dir:/opt/archiver/bundle` and `-v /path/to/bundle_password:/run/secrets/bundle_password:ro`.) Accepts the same optional flags as `archiver start`: `run backup prune` forces rotation, `run backup retain` forces retention (overriding `ROTATE_BACKUPS`).

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
          restartPolicy: OnFailure
          containers:
            - name: archiver
              image: forgejo.bryantserver.com/sisyphusmd/archiver:0.9.0
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

Create the ConfigMap and Secret straight from what `init` or `migrate` emitted: `kubectl create configmap archiver-config --from-env-file=archiver.env` and `kubectl create secret generic archiver-secrets --from-file=secrets/`. (Bundle mode also works: mount the bundle tar as a Secret at `/opt/archiver/bundle` plus a `bundle_password` key under `/run/secrets`.)

The Pod lives for the duration of one backup and exits. If the backup fails, the Pod exits non-zero and Kubernetes marks the Job failed — the usual CronJob semantics apply.

> **Why `run backup` instead of `run start`?** `archiver start` is asynchronous — it backgrounds the backup and returns exit `0` immediately, before the backup has done any real work. That's the right behavior for the in-container scheduler (which fires and forgets), but it would cause an external scheduler to always report "success" regardless of what actually happened. `run backup` blocks until the backup finishes so the exit code is meaningful. For this reason `start` is deliberately not whitelisted in `run` mode.

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
  forgejo.bryantserver.com/sisyphusmd/archiver:0.9.0

docker exec -it archiver-restore archiver restore

docker rm -f archiver-restore
```

Restoring from a bundle instead (the cold-restore path): replace the first two option lines with `-v /path/to/bundle/dir:/opt/archiver/bundle` and `-v /path/to/bundle_password:/run/secrets/bundle_password:ro`.

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

For Kubernetes Jobs, init containers, or one-shot `docker run` invocations, use the entrypoint's `run` mode (see [Container Modes](#container-modes)). The configuration is loaded (env-native or bundle), the subcommand runs, and the container's exit code equals the subcommand's exit code:

```bash
# Probe whether a backup is available
docker run --rm \
  --env-file /path/to/archiver.env \
  -v /path/to/secrets:/run/secrets:ro \
  -e SNAPSHOT_ID=myservice \
  forgejo.bryantserver.com/sisyphusmd/archiver:0.9.0 run snapshot-exists

# Restore a snapshot into a mounted destination
docker run --rm \
  --env-file /path/to/archiver.env \
  -v /path/to/secrets:/run/secrets:ro \
  -e SNAPSHOT_ID=myservice \
  -e LOCAL_DIR=/mnt/restore \
  -e OVERWRITE=1 \
  -v /path/to/restore/destination:/mnt/restore \
  forgejo.bryantserver.com/sisyphusmd/archiver:0.9.0 run auto-restore
```

(Bundle mode: swap the `--env-file` + secrets mount for `-v /path/to/bundle/dir:/opt/archiver/bundle` and `-v /path/to/bundle_password:/run/secrets/bundle_password:ro`.)

In Kubernetes this is typically an init container on the workload pod: probe with `run snapshot-exists`, and if a backup exists, run `run auto-restore` to seed the data volume before the main container starts. The exit-code contract means the pod's `restartPolicy` and init-container failure handling behave as expected.

</details>

<details>
<summary><h2>Advanced Usage</h2></summary>

### Custom Service Scripts

Create `service-backup-settings.sh` in any service directory:

```bash
#!/bin/bash

# Custom file filters
DUPLICACY_FILTERS_PATTERNS=(
  "+*.txt"
  "-*.tmp"
  "+*"
)

# Run before backup
service_specific_pre_backup_function() {
  echo "Dumping database..."
  docker exec postgres-container pg_dump -U user dbname > backup.sql
}

# Run after backup
service_specific_post_backup_function() {
  echo "Cleaning up..."
  rm -f backup.sql
}
```

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

