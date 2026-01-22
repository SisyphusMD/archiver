# Local Storage Setup Guide

This guide explains how to configure local disk storage as your primary backup target in Archiver. Using local storage provides faster backups, lower bandwidth usage, and better reliability compared to remote-only backups.

## Why Use Local Storage as Primary?

**Benefits:**
- **Significantly faster backups**: Local disk I/O vs network transfer speeds
- **Lower bandwidth usage**: Only secondary copies go over network
- **Better reliability**: Not dependent on network connectivity during primary backup
- **Incremental off-site protection**: Remote copies happen after local backup completes

**Trade-offs:**
- Requires additional disk space on your backup server
- Local storage failures could affect backups (mitigated by secondary remote storage)

---

## Prerequisites

- Archiver v0.7.0 or later running in Docker
- Additional disk space for local backups

---

## Scenario 1: Adding Local Storage to Existing Configuration

If you already have remote storage configured (e.g., B2, S3, SFTP), you can add local storage and make it primary.

### Important: Storage Must Be Populated First

You cannot directly add local storage as `STORAGE_TARGET_1` (primary) when you already have backups. The local storage must first be populated as a secondary storage before it can become primary.

**The correct approach:**
1. Add local storage as `STORAGE_TARGET_2` (secondary)
2. Run a backup to populate the local storage from your existing primary
3. Swap the storage numbers so local becomes `STORAGE_TARGET_1` (primary)
4. Future backups will go to local first, then copy to remote

### Step 1: Prepare the Local Storage Directory

On your Docker host, create a directory for backups:

```bash
# Create backup storage directory
mkdir -p /path/to/local-backups

# Set ownership to match Docker user
sudo chown -R 1000:1000 /path/to/local-backups
```

### Step 2: Mount the Directory in Docker Compose

Edit your [compose.yaml](compose.yaml) to add the local storage volume:

```yaml
services:
  archiver:
    # ... other configuration ...
    volumes:
      # ... existing volumes ...

      # Local backup storage
      - /path/to/local-backups:/mnt/backups
```

Apply the changes:

```bash
docker compose down
docker compose up -d
```

### Step 3: Add Local Storage as Secondary

Exec into the container and edit your configuration:

```bash
docker exec -it archiver bash
nano /opt/archiver/config.sh
```

Add the local storage configuration as `STORAGE_TARGET_2`:

```bash
# Existing primary storage (e.g., B2)
STORAGE_TARGET_1_NAME="b2-primary"
STORAGE_TARGET_1_TYPE="b2"
STORAGE_TARGET_1_B2_BUCKETNAME="your-bucket"
STORAGE_TARGET_1_B2_ID="your-key-id"
STORAGE_TARGET_1_B2_KEY="your-app-key"

# New local storage (as secondary)
STORAGE_TARGET_2_NAME="local-backups"
STORAGE_TARGET_2_TYPE="local"
STORAGE_TARGET_2_LOCAL_PATH="/mnt/backups"
```

Save and exit, then export:

```bash
archiver bundle export
exit
```

**IMPORTANT: Backup Your Bundle File**

After exporting, copy your updated bundle file to a safe location outside the Docker host:

```bash
# Copy from host (example path from compose.yaml mount)
cp ~/archiver-bundle/bundle.tar.enc /path/to/safe/location/
```

Keep both the bundle file and your bundle password in a secure location. Without both, you cannot recover your configuration.

### Step 4: Populate the Local Storage

Run a backup to initialize and populate the local storage:

```bash
docker exec archiver archiver start
```

Monitor the backup:

```bash
docker exec -it archiver archiver logs
```

You should see the local storage being initialized and data being copied from the primary storage.

### Step 5: Promote Local Storage to Primary

After the first successful backup, swap the storage numbers:

```bash
docker exec -it archiver bash
nano /opt/archiver/config.sh
```

Renumber the storage targets:

```bash
# Local storage is now primary
STORAGE_TARGET_1_NAME="local-backups"
STORAGE_TARGET_1_TYPE="local"
STORAGE_TARGET_1_LOCAL_PATH="/mnt/backups"

# Remote storage is now secondary
STORAGE_TARGET_2_NAME="b2-backup"
STORAGE_TARGET_2_TYPE="b2"
STORAGE_TARGET_2_B2_BUCKETNAME="your-bucket"
STORAGE_TARGET_2_B2_ID="your-key-id"
STORAGE_TARGET_2_B2_KEY="your-app-key"
```

Export the configuration:

```bash
archiver bundle export
exit
```

**IMPORTANT: Backup Your Bundle File Again**

After this export, backup the updated bundle file again:

```bash
cp ~/archiver-bundle/bundle.tar.enc /path/to/safe/location/
```

### Step 6: Verify the New Configuration

Run a test backup:

```bash
docker exec archiver archiver start
```

Verify that:
- Backup completes to local storage first
- Copy operation sends data to remote storage
- No errors in the logs

---

## Scenario 2: New Installation with Local Primary Storage

If you're setting up Archiver for the first time, configure local storage as primary from the start.

### Step 1: Prepare Local Storage Directory

Same as Scenario 1, Step 1.

### Step 2: Initialize Configuration with Local Storage

Start the container in init mode:

```bash
docker run -it --rm \
  -v /path/to/local-backups:/mnt/backups \
  -v /path/to/bundle:/opt/archiver/bundle \
  ghcr.io/sisyphusmd/archiver:v0.7.0 init
```

Follow the prompts and configure:

**Primary Storage (STORAGE_TARGET_1):**
- Name: `local-backups`
- Type: `local`
- Local Path: `/mnt/backups`

**Secondary Storage (STORAGE_TARGET_2, optional):**
- Configure your remote storage (B2, S3, SFTP, etc.)

### Step 3: Complete Setup

Follow the remaining init prompts to configure directories, scheduling, and export your bundle.

**IMPORTANT: Backup Your Bundle File**

After initialization completes, copy your bundle file to a safe location outside the Docker host:

```bash
cp ~/archiver-bundle/bundle.tar.enc /path/to/safe/location/
```

Keep both the bundle file and your bundle password in a secure location.

---

## Monitoring Disk Space

Check available space on your backup volume:

```bash
docker exec archiver df -h /mnt/backups
```

---

## Getting Help

If you encounter issues:
- View logs: `docker exec -it archiver archiver logs`
- Report issues: https://github.com/sisyphusmd/archiver/issues
