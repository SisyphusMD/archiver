# Editing Configuration in Docker

This guide covers how to edit your Archiver configuration when running in Docker.

## Overview

Your configuration is stored in the encrypted bundle file. To edit it:

1. Exec into the container
2. Edit the config file
3. Export a new bundle
4. Keep the bundle backed up externally

---

## Editing Workflow

### Step 1: Exec into the Container

```bash
docker exec -it archiver bash
```

### Step 2: Edit the Configuration

Use nano or vim to edit:

```bash
# Using nano (simpler)
nano /opt/archiver/config.sh

# Or using vim
vim /opt/archiver/config.sh
```

Make your changes and save:
- **nano**: `Ctrl+O` to save, `Ctrl+X` to exit
- **vim**: `Esc`, then `:wq` and `Enter`

For configuration options and examples, see: `/opt/archiver/examples/config.sh.example`

### Step 3: Export the New Bundle

```bash
archiver bundle export
```

This will:
- Prompt for your bundle password (or press Enter to reuse the current password)
- Create a new `bundle.tar.enc` file
- Back up the old bundle as `bundle.tar.enc.old` in the same directory

### Step 4: Exit the Container

```bash
exit
```

### Step 5: Backup the Bundle Externally

**IMPORTANT:** Keep a copy of your bundle file and password in a safe location outside the Docker host.

The bundle file is accessible on your host at the location you mounted in `compose.yaml`. For example, if you mounted:

```yaml
volumes:
  - ~/archiver-bundle:/opt/archiver/bundle
```

Then the bundle is at `~/archiver-bundle/bundle.tar.enc` on your host.

**Backup both:**
- The bundle file: `bundle.tar.enc`
- Your bundle password

Without both, you cannot recover your configuration.

### Step 6: Test Your Changes

Run a test backup to verify your configuration changes:

```bash
docker exec archiver archiver start
```

---

## Notes

- **No restart needed**: Configuration changes take effect immediately on the next backup run
- **Backup retention**: The `.old` bundle is kept in the same directory, but only the most recent bundle is retained
- **Schedule changes**: The backup schedule (`CRON_SCHEDULE`) is set in `compose.yaml`, not `config.sh`. Changing it requires `docker compose restart`

---

## Getting Help

If you encounter issues:

- View logs: `docker exec -it archiver archiver logs`
- Check syntax: `bash -n /opt/archiver/config.sh` (inside container)
- Report issues: https://github.com/sisyphusmd/archiver/issues
