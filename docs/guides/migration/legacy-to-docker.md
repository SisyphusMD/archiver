# Migrating Legacy Installation to Docker

## ⚠️ BREAKING CHANGES

**Direct installation on host systems is no longer supported in v0.7.0.**

All deployments must now use Docker. If you're currently running Archiver directly on your host system, you'll need to migrate to the Docker-based deployment model.

---

## Pre-Migration Steps

### 1. Identify Your Current Version

Check which version you're running:

```bash
cat /path/to/archiver/CHANGELOG.md | head -20
```

Or check your git repository:

```bash
cd /path/to/archiver
git describe --tags
```

### 2. Locate Your Configuration Bundle

Your bundle location depends on which version you're migrating from:

#### If migrating from v0.5.0 - v0.6.5:
- Directory: `bundle/`
- Filename: `bundle.tar.enc`
- Full path example: `/path/to/archiver/bundle/bundle.tar.enc`

#### If migrating from v0.3.2 - v0.4.x:
- Directory: `exports/`
- Filename: `export-YYYYMMDD-HHMMSS.tar.enc` (timestamped)
- Full path example: `/path/to/archiver/exports/export-20240606-153022.tar.enc`

**Action:** Find and note the full path to your configuration file.

### 3. Note Your Cron Schedule

Your current backup schedule is in the root crontab:

```bash
sudo crontab -l | grep archiver
```

Example output:
```
0 3 * * * archiver start
```

**Action:** Note your cron schedule to configure it in Docker Compose later.

### 4. Check Your Hostname

Your current hostname is used in Duplicacy snapshot IDs:

```bash
hostname
```

**Action:** Note your hostname - you'll need to match this in Docker Compose to continue your existing backups seamlessly.

### 5. Review Your Service-Specific Scripts

If you have `service-backup-settings.sh` files in your service directories, review them for:

- **Docker commands**: If your scripts use `docker exec`, `docker stop`, etc., you'll need to mount the Docker socket
- **System commands**: Verify all commands used are available in the container (see available tools list below)
- **File paths**: Ensure paths will be accessible when mounted into the container

**Available tools in the container:**
- Backup: duplicacy, docker, openssh-client, openssl
- Databases: sqlite3
- Host management: systemctl, zfs
- Network: curl, wget, iputils-ping
- Editors: nano, vim
- Utilities: expect, procps, ca-certificates, gnupg
- Standard Unix utilities: tar, gzip, grep, awk, sed, find, etc.

---

## Migration Steps

### Step 1: Prepare Your Bundle Directory

Create a dedicated directory on your host for the bundle file. This directory will be mounted into the Docker container.

#### If migrating from v0.5.0 - v0.6.5:

```bash
# Create a new directory for Docker mounting
mkdir -p ~/archiver-bundle

# Copy your bundle file
cp /path/to/archiver/bundle/bundle.tar.enc ~/archiver-bundle/
```

#### If migrating from v0.3.2 - v0.4.x:

```bash
# Create a new directory for Docker mounting
mkdir -p ~/archiver-bundle

# Copy and rename your most recent export file
# Replace the timestamp with your actual filename
cp /path/to/archiver/exports/export-YYYYMMDD-HHMMSS.tar.enc ~/archiver-bundle/bundle.tar.enc
```

**Important:** If you have multiple export files, use the most recent one.

### Step 2: Create Docker Compose Configuration

Create a `compose.yaml` file in a new directory:

```yaml
services:
  archiver:
    container_name: archiver
    image: forgejo.bryantserver.com/sisyphusmd/archiver:0.7.0
    restart: unless-stopped
    stop_grace_period: 2m  # Allow time for graceful shutdown and cleanup

    # IMPORTANT: Match this to your current host's hostname
    # This ensures Duplicacy continues your existing backups
    hostname: YOUR_CURRENT_HOSTNAME_HERE

    environment:
      # Your bundle password (same password you used to create the bundle)
      # Note: If password contains $, escape it as $$ (e.g., my$password → my$$password)
      BUNDLE_PASSWORD: "your-bundle-password-here"

      # Your cron schedule from step 3 (or omit for manual backups)
      CRON_SCHEDULE: "0 3 * * *"

      # Timezone for cron scheduling and timestamps
      TZ: "America/New_York"

    volumes:
      # Bundle directory (required) - mounts to /opt/archiver/bundle in container
      - /home/user/archiver-bundle:/opt/archiver/bundle

      # Persistent logs (optional but recommended)
      - /home/user/archiver-logs:/opt/archiver/logs

      # Data directories to backup
      # IMPORTANT: Container paths MUST match SERVICE_DIRECTORIES in your config.sh
      # Example: if config.sh has /srv/*, mount as:
      - /srv:/srv

      # For restores: mount restore destination (add when needed)
      # - /path/to/restore:/mnt/restore

      # Docker socket (only if your scripts use docker commands)
      # - /var/run/docker.sock:/var/run/docker.sock  # SECURITY WARNING: See below
```

**Critical Configuration Notes:**

1. **Hostname**: Must match your current host's hostname (from pre-migration step 4)
2. **Bundle directory**: The host path (left side) is where you placed bundle.tar.enc in Step 1. The container path (right side) must be `/opt/archiver/bundle`
3. **Volume paths for data**: Container paths (right side) must exactly match `SERVICE_DIRECTORIES` in your `config.sh`
4. **Bundle password**: Same password you used when creating the export/bundle file

**Docker Socket Security Warning:**

Only mount the Docker socket if your backup scripts use `docker exec` or similar commands. This grants the container root-level access to Docker, meaning it can:
- Start, stop, or delete any container
- Access any data on the system
- Potentially escape the container

Only enable if absolutely necessary.

### Step 3: Start the Container

```bash
# Navigate to the directory with your compose.yaml
cd /path/to/compose-directory

# Start the container
docker compose up -d

# Watch the container startup logs
docker compose logs -f
```

The container will automatically:
1. Import your bundle file
2. Load your configuration
3. Set up the cron schedule (if configured)
4. Start running backups on schedule

**Note for SFTP users:** As of v0.7.0, the `STORAGE_TARGET_X_SFTP_KEY_FILE` configuration variable has been removed. The SSH key path is now hardcoded to `/opt/archiver/keys/id_ed25519`. If your config.sh contains this variable, it will be ignored (no action needed on your part).

### Step 4: Verify the Migration

Run a test backup manually:

```bash
docker exec archiver archiver start
```

Monitor the backup:

```bash
docker exec -it archiver archiver logs
```

Check that:
- Backup starts successfully
- All service directories are accessible
- Pre/post-backup scripts run correctly
- Backup completes without errors
- Notifications are sent (if configured)

### Step 5: Test Restore

Before uninstalling your legacy installation, verify you can restore from the Docker-based setup.

First, ensure you have a restore destination mounted in your `compose.yaml`. If not, add it and restart:

```yaml
volumes:
  # ... other volumes ...
  - /path/to/restore/destination:/mnt/restore
```

```bash
# Restart container to pick up new volume
docker compose restart

# Run interactive restore
docker exec -it archiver archiver restore
```

Follow the prompts to:
1. Select a storage backend
2. Choose a snapshot
3. Select a revision
4. Enter restore path: `/mnt/restore` (the container path)

Verify the restored files appear on your host at `/path/to/restore/destination` and are correct and complete.

### Step 6: Disable Old Cron Job

Once you've verified backups work in Docker, disable the old cron job. The cron job may be in either root's crontab or your user's crontab.

**Check root crontab:**
```bash
# View root crontab
sudo crontab -l

# If archiver job is present, edit root crontab
sudo crontab -e

# Comment out or delete the archiver-related lines:
# PATH=/usr/local/bin:/usr/bin:/bin
# 0 3 * * * archiver start
```

**Check user crontab:**
```bash
# View your user crontab
crontab -l

# If archiver job is present, edit your user crontab
crontab -e

# Comment out or delete the archiver-related lines
```

**Important:** Don't uninstall the legacy installation yet - keep it as a backup until you're confident.

### Step 7: Monitor for Several Backup Cycles

Run the Docker-based setup for at least **2-3 successful scheduled backup cycles** while keeping your legacy installation intact. This ensures:
- Scheduled backups run correctly
- All service-specific scripts work
- Notifications function properly
- No unexpected issues arise

### Step 8: Uninstall Traditional Installation

After successfully running Docker-based backups for several cycles, you can uninstall the legacy installation.

See: [Uninstalling Legacy Archiver Installation](../maintenance/uninstall-legacy.md)

---

## Recommended: Transition to Local Primary Backup

**We strongly recommend using local disk storage as your primary backup target** with v0.7.0. This provides:

- **Significantly faster backups** (local disk I/O vs network transfer)
- **Lower bandwidth usage** (only final copy goes over network)
- **Better reliability** (not dependent on network connectivity)
- **Incremental off-site protection** (copies to remote storage after local backup completes)

**Storage Requirements:**

Local storage will require additional disk space equal to your backup size. However, with Duplicacy's deduplication, this is typically smaller than your source data.

**Migration Path:**

See: [Local Storage Setup Guide](../configuration/local-storage-setup.md) for detailed instructions on adding local storage to your existing configuration.

---

## Post-Migration Guides

### Editing Configuration

Now that you're running in Docker, editing configuration requires a slightly different workflow.

See: [Editing Configuration in Docker](../configuration/editing-config.md)

### Managing SSH Keys

If you need to create new SSH keys or update existing ones for SFTP storage:

See: [SSH Key Management Guide](../configuration/ssh-key-management.md)
