# Archiver

<p>
  <img src="lib/logos/72x72.png" alt="Logo" align="left" style="margin-right: 10px;">
  Automated encrypted backups with deduplication to local disk, SFTP, BackBlaze B2, and S3 storage. Leverages <a href="https://github.com/gilbertchen/duplicacy">Duplicacy</a> to follow the <a href="https://www.backblaze.com/blog/the-3-2-1-backup-strategy/">3-2-1 Backup Strategy</a> while removing the complexity of manual configuration.
</p>

## What is Archiver?

Archiver automates backing up directories to multiple remote storage locations with encryption and deduplication. Configure once, then backups run automatically on a schedule.

Each directory gets backed up independently, with optional pre/post-backup scripts for service-specific needs (like database dumps). Backups are encrypted at rest and deduplicated across all your directories to save storage space.

Supports local disk, SFTP (Synology NAS, etc.), BackBlaze B2, and S3-compatible storage.

## Quick Start

Run interactive initialization in a container to generate your bundle file:
```bash
docker run -it --rm \
  -v ./archiver-bundle:/opt/archiver/bundle \
  ghcr.io/sisyphusmd/archiver:v0.7.0 init
```
Then use the generated `bundle.tar.enc` file with Docker Compose. See [Installation](#installation) for details.

## Features

- **Encrypted & Deduplicated**: Block-level deduplication minimizes storage, RSA encryption secures data
- **Multiple Backends**: Local disk, SFTP, BackBlaze B2, S3-compatible storage
- **Automated Rotation**: Configurable retention policies (keep daily, weekly, monthly snapshots)
- **Service Integration**: Pre/post-backup scripts, custom restore procedures
- **Notifications**: Pushover alerts for successes and failures
- **Easy Restoration**: Interactive restore script to recover specific revisions

---


## Installation

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) installed
- [Docker Compose](https://docs.docker.com/compose/install/) (optional, for easier management)

### Step 1: Generate Bundle File

Run initialization interactively to generate your configuration bundle:

```bash
docker run -it --rm \
  -v ./archiver-bundle:/opt/archiver/bundle \
  ghcr.io/sisyphusmd/archiver:v0.7.0 init
```

This creates `archiver-bundle/bundle.tar.enc` with your configuration and keys.

### Step 2: Configure

Create `compose.yaml`:

```yaml
services:

  archiver:

    container_name: archiver
    image: ghcr.io/sisyphusmd/archiver:v0.7.0
    restart: unless-stopped

    hostname: backup-server       # used for backup service label (optional)

    environment:
      BUNDLE_PASSWORD: "your-bundle-password-here"
      CRON_SCHEDULE: "0 3 * * *"  # Ex: daily at 3am, or omit for manual mode
      TZ: "UTC"                   # Timezone for cron scheduling

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
docker exec -it archiver archiver logs
```

### Docker Socket (Advanced)

If your backup scripts need to control other containers (e.g., `docker exec` for database dumps), mount the socket:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
```

**Security Warning**: This grants root-level access to the Docker daemon. The container can start/stop/delete any container or access any data. Only use if necessary.

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `BUNDLE_PASSWORD` | Yes | Password for decrypting bundle.tar.enc |
| `CRON_SCHEDULE` | No | Cron expression for automatic backups (empty = manual mode) |
| `TZ` | No | Timezone for cron scheduling (default: UTC) |

### Manual Commands

With `CRON_SCHEDULE` set, backups run automatically. Without it, run commands manually.

View logs:
```bash
docker exec -it archiver archiver logs
```

Check status:
```bash
docker exec archiver archiver status
docker exec archiver archiver healthcheck
```

Start backup:
```bash
docker exec archiver archiver start
docker exec archiver archiver start logs    # with log viewing
docker exec archiver archiver start prune   # force rotation
docker exec archiver archiver start retain  # force retention
```

Manage active backups:
```bash
docker exec archiver archiver pause
docker exec archiver archiver resume
docker exec archiver archiver stop
```

Export/import bundle:
```bash
docker exec -it archiver archiver bundle export
docker exec -it archiver archiver bundle import
```

#### Restoring Data

##### Interactive Restore with Existing Container

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

##### One-Off Restore with Temporary Container

For a one-time restore without modifying your running container, use a temporary container:

```bash
# One-off restore (container exits after completion)
docker run --rm -it \
  -e BUNDLE_PASSWORD="your-bundle-password-here" \
  -v /path/to/bundle/dir:/opt/archiver/bundle \
  -v /path/to/restore/destination:/mnt/restore \
  ghcr.io/sisyphusmd/archiver:v0.7.0 \
  archiver restore
```

When prompted for the local directory path during restore, enter the container path (e.g., `/mnt/restore`). The restored files will appear on your host at `/path/to/restore/destination`.

### Image Tags

- `v0.7.0` - Specific version (recommended)
- `v0.7` - Minor version (receives patches automatically)
- `v0` - Major version (receives minor/patch updates)

---

## Storage Backend Setup

Prepare at least one storage location before running init.

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

## Configuration

The `config.sh` file defines what to backup and where:

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
STORAGE_TARGET_2_SFTP_KEY_FILE="/path/to/keys/id_ed25519"

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
ROTATE_BACKUPS="true"
PRUNE_KEEP="-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1"
```

This keeps:
- All backups from last 1 day (1:1)
- Daily backups for 7 days (7:7)
- Weekly backups for 30 days (30:30)
- Monthly backups for 180 days (0:180)

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

## Advanced Usage

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

### Command Reference

```bash
archiver start          # Run backup now
archiver start logs     # Run backup and follow logs
archiver start prune    # Run backup and force prune (ignore config)
archiver start retain   # Run backup without pruning (ignore config)
archiver stop           # Stop running backup
archiver restart        # Stop then start backup
archiver pause          # Pause backup (experimental)
archiver resume         # Resume paused backup (experimental)
archiver logs           # Follow backup logs
archiver status         # Check if backup is running
archiver bundle export  # Create encrypted config/keys bundle
archiver bundle import  # Import from encrypted bundle
archiver restore        # Restore data from backup
archiver healthcheck    # Check system health
archiver help           # Show help
```

---

## Licensing

Archiver is licensed under [GNU AGPL-3.0](LICENSE).

Archiver uses the [Duplicacy CLI v3.2.5](https://github.com/gilbertchen/duplicacy/tree/v3.2.5) binary as an external tool. Duplicacy is licensed separately under [its own terms](https://github.com/gilbertchen/duplicacy/blob/v3.2.5/LICENSE.md):

- **Free for personal use** and **commercial trials**
- **Requires a CLI license** for non-trial commercial use ($50/computer/year from [duplicacy.com](https://duplicacy.com/buy.html))

**What counts as commercial use?** Backing up files related to employment or for-profit activities.

**Note:** Restore and management operations (restore, check, copy, prune) never require a license. Only the `backup` command requires a license for commercial use.

If you're using Archiver commercially, please purchase a Duplicacy CLI license to support the project that makes this tool possible

