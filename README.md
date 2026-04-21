# Archiver

<p>
  <img src="lib/logos/72x72.png" alt="Logo" align="left" style="margin-right: 10px;">
  Automated encrypted backups with deduplication to local disk, SFTP, BackBlaze B2, and S3 storage. Leverages <a href="https://github.com/gilbertchen/duplicacy/tree/v3.2.5">Duplicacy CLI v3.2.5</a> to follow the <a href="https://www.backblaze.com/blog/the-3-2-1-backup-strategy/">3-2-1 Backup Strategy</a> while removing the complexity of manual configuration.
</p>

## What is Archiver?

Archiver automates backing up directories to multiple remote storage locations with encryption and deduplication. Configure once, then backups run automatically on a schedule.

Each directory gets backed up independently, with optional pre/post-backup scripts for service-specific needs (like database dumps). Backups are encrypted at rest and deduplicated across all your directories to save storage space.

Supports local disk, SFTP (Synology NAS, etc.), BackBlaze B2, and S3-compatible storage.

## ⚠️ Breaking Changes in v0.7.0

**Direct installation on host systems is no longer supported.** All deployments must now use Docker.

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

- [Docker](https://docs.docker.com/get-docker/) installed
- [Docker Compose](https://docs.docker.com/compose/install/) (optional, for easier management)

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

### Step 1: Generate Bundle File

**Skip this step if you already have a bundle file** (e.g., `bundle.tar.enc` or `export-*.tar.enc` from a previous installation).

For new installations, run initialization interactively to generate your configuration bundle:

```bash
docker run -it --rm \
  -v ./archiver-bundle:/opt/archiver/bundle \
  ghcr.io/sisyphusmd/archiver:0.8.0 init
```

This creates `archiver-bundle/bundle.tar.enc` with your configuration and keys.

### Step 2: Configure Docker Compose

Create `compose.yaml`:

```yaml
services:

  archiver:

    container_name: archiver
    image: ghcr.io/sisyphusmd/archiver:0.8.0
    restart: unless-stopped
    stop_grace_period: 2m         # Allow time for graceful shutdown and cleanup

    hostname: backup-server       # used for backup service label (optional)

    cap_drop:
      - ALL
    cap_add:
      - DAC_OVERRIDE              # Write to dirs owned by other UIDs
      - SETGID                    # Required for cron (remove if not using CRON_SCHEDULE)
    security_opt:
      - no-new-privileges:true

    environment:
      BUNDLE_PASSWORD: "your-bundle-password-here"  # Escape $ as $$ (e.g., my$pass → my$$pass)
      CRON_SCHEDULE: "0 3 * * *"  # Ex: daily at 3am, or omit for manual mode
      TZ: "America/New_York"      # Timezone for cron and timestamps (default: UTC)

    volumes:
      - ./archiver-bundle:/opt/archiver/bundle       # Bundle file (required)
      - ./archiver-logs:/opt/archiver/logs           # Persistent logs (optional)
      - /path/to/host/backup-dir:/mnt/backup-dir     # Data to backup (must match config.sh)
      # - /var/run/docker.sock:/var/run/docker.sock  # For docker exec in scripts (optional)
      # - /path/to/host/restore-dir:/mnt/restore-dir # Restore location (will be prompted)
```

Update paths and password, then start:

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
      - DAC_OVERRIDE # Required: write to directories owned by other UIDs
      - SETGID       # Required if using CRON_SCHEDULE: cron needs setgid to execute jobs
    security_opt:
      - no-new-privileges:true  # Prevent privilege escalation
```

| Capability | When Required | Why |
|-----------|---------------|-----|
| `DAC_OVERRIDE` | Always (with `cap_drop: ALL`) | Archiver runs as root but writes to service data directories that may be owned by other users (e.g., UID 1000) |
| `SETGID` | When using `CRON_SCHEDULE` | Debian's cron daemon requires `setgid` to execute scheduled jobs. Without it, cron starts but jobs silently fail to run |

**Note**: `no-new-privileges` is a kernel security option, not a Linux capability. It is compatible with both `DAC_OVERRIDE` and `SETGID` and is recommended for defense in depth.

### Graceful Shutdown

The `stop_grace_period: 2m` setting allows the container time to complete cleanup when stopped. When `docker compose down` or `docker stop` is called, Archiver will:
- Complete any running pre-backup hooks
- Run post-backup hooks to restore services (e.g., restart databases, remove snapshots)
- Terminate gracefully

If your post-backup hooks take longer than 2 minutes, increase this value accordingly.

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `BUNDLE_PASSWORD` | Yes | Password for decrypting bundle.tar.enc. **Note:** If your password contains `$`, you must escape it as `$$` (e.g., `my$password` → `my$$password`) |
| `CRON_SCHEDULE` | No | Cron expression for automatic backups (empty = manual mode) |
| `TZ` | No | Timezone for cron scheduling (default: UTC) |
| `SYSTEMCTL_FORCE_BUS` | No | Set to `1` to enable systemctl access to host services via D-Bus socket (requires socket mounts, see above) |

### Image Tags

- `0.8.0` - Specific version (recommended)
- `0.8` - Minor version (receives patches automatically)
- `0` - Major version (receives minor/patch updates)

---

## Configuration

The `config.sh` file defines what to backup and where. See [Editing Configuration](docs/guides/configuration/editing-config.md) for how to modify it in Docker.

### Service Directories

Directories to backup. Use `*` for subdirectories:

```bash
SERVICE_DIRECTORIES=(
  "/srv/*/"          # Each subdirectory as separate repository
  "/home/user/data/" # Single repository
)
```

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
archiver auto-restore      # Restore data from backup (non-interactive, env-driven)
archiver snapshot-exists   # Check if a snapshot exists on any storage target
archiver healthcheck       # Check system health
archiver help              # Show help
```

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

For a one-time restore without modifying your running container, use a temporary container:

```bash
# One-off restore (container exits after completion)
docker run --rm -it \
  -e BUNDLE_PASSWORD='your-bundle-password-here' \
  -v /path/to/bundle/dir:/opt/archiver/bundle \
  -v /path/to/restore/destination:/mnt/restore \
  ghcr.io/sisyphusmd/archiver:0.8.0 \
  archiver restore
```

When prompted for the local directory path during restore, enter the container path (e.g., `/mnt/restore`). The restored files will appear on your host at `/path/to/restore/destination`.

### Non-interactive Restore (CI / Kubernetes)

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
| `RESTORE_THREADS` | No | Override download thread count (default matches `DUPLICACY_THREADS`) |

Exit codes:
- `0` — snapshot restored
- `1` — snapshot not found on any reachable target, or the restore itself failed
- `2` — all targets unreachable, or invalid env
- `3` — an Archiver backup is in progress; restore skipped

Example (gate-and-restore):

```bash
docker exec \
  -e SNAPSHOT_ID=myservice \
  archiver archiver snapshot-exists \
  && docker exec \
       -e SNAPSHOT_ID=myservice \
       -e LOCAL_DIR=/mnt/restore \
       archiver archiver auto-restore
```

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

