# Archiver

<p>
  <img src="lib/logos/72x72.png" alt="Logo" align="left" style="margin-right: 10px;">
  Automated encrypted backups with deduplication to local disk, SFTP, BackBlaze B2, and S3 storage. Leverages <a href="https://github.com/gilbertchen/duplicacy">Duplicacy</a> to follow the <a href="https://www.backblaze.com/blog/the-3-2-1-backup-strategy/">3-2-1 Backup Strategy</a> while removing the complexity of manual configuration.
</p>

## What is Archiver?

Archiver automates backing up directories to multiple remote storage locations with encryption and deduplication. Configure once, then backups run automatically via cron or Docker scheduling.

Each directory gets backed up independently, with optional pre/post-backup scripts for service-specific needs (like database dumps). Backups are encrypted at rest and deduplicated across all your directories to save storage space.

Supports local disk, SFTP (Synology NAS, etc.), BackBlaze B2, and S3-compatible storage. Run it natively on Linux or via Docker on any platform.

## Quick Start

### Docker (any platform)
Run interactive setup in a container to generate your bundle file:
```bash
docker run -it --rm \
  -v ./archiver-bundle:/opt/archiver/bundle \
  ghcr.io/sisyphusmd/archiver:0.6.3 setup
```
Then use the generated `bundle.tar.enc` file with Docker Compose. See [Docker Installation](#docker-installation) for details.

### Direct Installation (Linux only)
1. Prepare storage backend ([Local Disk](#local-disk), [SFTP](#sftp---synology-nas), [B2](#b2---backblaze), or [S3](#s3-compatible-storage))
2. Clone repository and run `./archiver.sh setup`
3. See [Traditional Installation](#traditional-installation) for details

## Features

- **Encrypted & Deduplicated**: Block-level deduplication minimizes storage, RSA encryption secures data
- **Multiple Backends**: Local disk, SFTP, BackBlaze B2, S3-compatible storage
- **Automated Rotation**: Configurable retention policies (keep daily, weekly, monthly snapshots)
- **Service Integration**: Pre/post-backup scripts, custom restore procedures
- **Notifications**: Pushover alerts for successes and failures
- **Easy Restoration**: Interactive restore script to recover specific revisions

---

## Traditional Installation

For direct installation on Linux systems.

### Prerequisites

- Debian-based Linux (Ubuntu, Debian, Raspberry Pi OS)
- Architecture: ARM64 or AMD64
- Git installed
- At least one storage backend prepared (local disk, SFTP, B2, or S3)

### Installation Steps

1. Navigate to desired parent directory:
   ```bash
   cd ~
   ```

2. Clone repository:
   ```bash
   git clone --branch v0.6.3 https://github.com/SisyphusMD/archiver.git
   cd archiver
   ```

3. Run setup script:
   ```bash
   ./archiver.sh setup
   ```

4. Follow prompts to:
   - Install dependencies (expect, openssh-client, openssl, wget, cron)
   - Download Duplicacy binary
   - Generate RSA encryption keys
   - Generate SSH keys (for SFTP backends)
   - Configure directories to backup
   - Configure storage targets
   - Set up notifications (optional)
   - Schedule via cron (optional)

5. **IMPORTANT**: Back up your bundle file
   ```bash
   archiver bundle export
   ```
   Save the generated `bundle/bundle.tar.enc` file and remember the password. You need this to restore configuration or migrate to Docker.

### Basic Usage

- **Start backup**: `archiver start`
- **View logs**: `archiver logs`
- **Check status**: `archiver status`
- **Stop backup**: `archiver stop`
- **Create bundle**: `archiver bundle export`
- **Import bundle**: `archiver bundle import`
- **Restore data**: `archiver restore`

---

## Docker Installation

Run Archiver in a container on any platform.

### Prerequisites

- Docker or Docker Compose installed

### Step 1: Generate Bundle File

Run setup interactively in a container to generate your configuration bundle:

```bash
docker run -it --rm \
  -v ./archiver-bundle:/opt/archiver/bundle \
  ghcr.io/sisyphusmd/archiver:0.6.3 setup
```

This creates `archiver-bundle/bundle.tar.enc` with your configuration and keys.

### Step 2: Configure Docker Compose

Create `compose.yaml`:

```yaml
services:
  archiver:
    image: ghcr.io/sisyphusmd/archiver:0.6.3
    container_name: archiver
    restart: unless-stopped
    hostname: backup-server

    environment:
      BUNDLE_PASSWORD: "your-bundle-password-here"
      CRON_SCHEDULE: "0 3 * * *"  # Daily at 3am, or omit for manual mode

    volumes:
      # Encrypted config/keys bundle
      - ./archiver-bundle/bundle.tar.enc:/opt/archiver/bundle/bundle.tar.enc:ro

      # Optional: persistent logs
      - ./archiver-logs:/opt/archiver/logs

      # Required: directories to backup (must match config.sh paths)
      - /path/to/host/services:/data/services:ro

      # Optional: Docker socket (required if service scripts need to control other containers)
      # Security note: Grants container access to Docker daemon - use only if needed
      # - /var/run/docker.sock:/var/run/docker.sock
```

Update paths and password, then start:

```bash
docker compose up -d
docker logs -f archiver
```

### Docker Socket Access (Advanced)

If your service-specific backup scripts need to control other Docker containers (e.g., putting a database in maintenance mode, creating database dumps via `docker exec`), you must mount the Docker socket:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
```

**Security Warning**: Mounting the Docker socket grants the archiver container root-level access to the Docker daemon. This means the container can:
- Start, stop, or delete any container on the host
- Access data from any container
- Potentially compromise the host system

**Only mount the Docker socket if:**
- Your backup scripts genuinely need to control other containers
- You trust the archiver code and your service-specific backup scripts
- You understand and accept the security implications

**Examples of when you need it:**
- Running `docker exec` to create database dumps before backup
- Putting services in maintenance mode during backups
- Stopping/starting containers as part of the backup process

**Alternative**: If possible, design your backup strategy to avoid needing Docker socket access (e.g., use volume mounts to access data directly, use database backup tools inside the archiver container).

### Docker Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `BUNDLE_PASSWORD` | Yes | Password for decrypting bundle.tar.enc |
| `CRON_SCHEDULE` | No | Cron expression for automatic backups (empty = manual mode) |

### Docker Manual Commands

#### Automated vs Manual Operation

The container can run in two modes:

1. **Automated mode** (with `CRON_SCHEDULE` set): Backups run automatically on schedule
2. **Manual mode** (without `CRON_SCHEDULE` or set to empty string): Container stays idle, waiting for manual commands

#### Viewing Logs

The **recommended method** to view logs is via Docker's native logging:

```bash
# View live container logs (recommended)
docker logs -f archiver

# View recent logs
docker logs --tail 100 archiver

# View logs since a specific time
docker logs --since 1h archiver
```

Alternatively, view logs from within the container or from mounted volumes:

```bash
# Use archiver's built-in log viewer
docker exec -it archiver archiver logs

# Or tail log files directly
docker exec archiver tail -f /opt/archiver/logs/archiver.log

# From mounted volume on host (if logs volume is mounted)
tail -f /path/to/host/logs/archiver.log
```

#### Checking Status Before Manual Operations

**Best practice**: Check if a backup is already running before starting manual commands:

```bash
# Check current status (recommended first step)
docker exec archiver archiver status

# Check system health
docker exec archiver archiver healthcheck
```

#### Running Manual Backups

**Note**: Manual backups are only needed if `CRON_SCHEDULE` is not set. With automated mode, backups run on schedule.

```bash
# Start a backup immediately
docker exec archiver archiver start

# Start a backup and view logs
docker exec -it archiver archiver start logs

# Force pruning/rotation on next backup
docker exec archiver archiver start prune

# Skip pruning/rotation on next backup
docker exec archiver archiver start retain
```

#### Managing Running Backups

```bash
# Pause a running backup
docker exec archiver archiver pause

# Resume a paused backup
docker exec archiver archiver resume

# Stop a running backup
docker exec archiver archiver stop

# Restart a backup
docker exec archiver archiver restart
```

#### Bundle Management

```bash
# Export bundle (create new bundle from current config)
docker exec -it archiver archiver bundle export

# Import bundle (typically done automatically at startup)
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
  -v /path/to/bundle.tar.enc:/opt/archiver/bundle/bundle.tar.enc:ro \
  -v /path/to/restore/destination:/mnt/restore \
  ghcr.io/sisyphusmd/archiver:0.6.3 \
  archiver restore
```

When prompted for the local directory path during restore, enter the container path (e.g., `/mnt/restore`). The restored files will appear on your host at `/path/to/restore/destination`.

### Image Tags

- `0.5.0` - Specific version (recommended)
- `0.5` - Minor version (receives patches automatically)
- `0` - Major version (receives minor/patch updates)

---

## Storage Backend Setup

Prepare at least one storage location before running setup.

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

Generate SSH key first (setup script can do this), then:

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

You'll enter the **User Key** and **API Token** during setup.

</details>

---

## Restoration

### Restoring Archiver Configuration

If you need to set up Archiver on a new machine:

1. Clone repository:
   ```bash
   cd ~
   git clone --branch v0.6.3 https://github.com/SisyphusMD/archiver.git
   cd archiver
   ```

2. Place your `bundle.tar.enc` file in the archiver directory

3. Run setup:
   ```bash
   ./archiver.sh setup
   ```
   The setup script will detect and import your bundle file.

### Restoring Backed Up Data

To restore files from a backup:

```bash
archiver restore
```

Follow the interactive prompts to:
1. Select storage backend
2. Choose snapshot ID
3. Specify local directory for restoration
4. Select revision to restore

If a `restore-service.sh` script exists in the restored directory, you'll be prompted to run it.

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
STORAGE_TARGET_1_NAME="backblaze"
STORAGE_TARGET_1_TYPE="b2"
STORAGE_TARGET_1_B2_BUCKETNAME="my-bucket"
STORAGE_TARGET_1_B2_ID="keyID"
STORAGE_TARGET_1_B2_KEY="applicationKey"

# Secondary storage (optional)
STORAGE_TARGET_2_NAME="nas"
STORAGE_TARGET_2_TYPE="sftp"
STORAGE_TARGET_2_SFTP_URL="192.168.1.100"
STORAGE_TARGET_2_SFTP_PORT="22"
STORAGE_TARGET_2_SFTP_USER="backup"
STORAGE_TARGET_2_SFTP_PATH="/volume1/backups"
STORAGE_TARGET_2_SFTP_KEY_FILE="/path/to/keys/id_ed25519"
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
  echo "Stopping service..."
  systemctl stop myservice
}

# Run after backup
service_specific_post_backup_function() {
  echo "Starting service..."
  systemctl start myservice
}
```

### Custom Restore Scripts

Create `restore-service.sh` in any service directory:

```bash
#!/bin/bash
# Runs after restoration completes

echo "Restoring database..."
mysql -u root < backup.sql

echo "Setting permissions..."
chown -R www-data:www-data /var/www

echo "Starting service..."
systemctl start myservice
```

### Cron Scheduling

During setup or manually:

```bash
# Daily at 3am
(crontab -l 2>/dev/null; echo "0 3 * * * archiver start") | crontab -

# Every 6 hours
(crontab -l 2>/dev/null; echo "0 */6 * * * archiver start") | crontab -

# Weekly on Sunday at 2am
(crontab -l 2>/dev/null; echo "0 2 * * 0 archiver start") | crontab -
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
archiver setup          # Run initial setup
archiver help           # Show help
```

