# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.6.1] - 2026-01-13

### Added
- **Docker CLI Support**: Docker CLI now included in container image
  - Enables service-specific backup scripts to control other Docker containers
  - Version-locked to Docker CLI 29.1.4-1 for reproducible builds
  - Required for backup scripts that need to exec commands in other containers or manage container state

### Documentation
- Added requirement to mount Docker socket (`/var/run/docker.sock`) for service scripts that need Docker access
- Added security warning about Docker socket access implications

## [0.6.0] - 2026-01-13

### Added
- **Local Disk Storage Backend**: Support for local disk as a storage target alongside SFTP, B2, and S3
  - Ideal as primary backup target with fast local backups
  - Can be combined with remote storage for off-site redundancy
- **Performance Optimization**: Configurable thread count for duplicacy operations
  - New `DUPLICACY_THREADS` configuration variable (default: 4)
  - Parallel upload/download threads for faster backups
  - Applied to backup, copy, restore, check, and prune operations
- **Interactive Restore Options**: Advanced restore configuration wizard
  - Hash-based file detection option
  - Overwrite existing files option
  - Delete files not in snapshot option
  - Ignore ownership option
  - Continue on errors option
  - Customizable thread count for restore operations
- **Graceful Container Shutdown**: Docker containers handle SIGTERM properly
  - Running backups are stopped gracefully on `docker stop`
  - Prevents data corruption from forced termination

### Improved
- Restore operations now have configurable performance and behavior options
- Backup operations complete faster with parallel threading
- Docker container lifecycle management with proper signal handling

## [0.5.1] - 2026-01-13

### Added
- SSH public key display after bundle export for easy SFTP server configuration

### Fixed
- Symlink for `archiver.log` now uses relative path, resolving correctly when logs directory is mounted outside container
- Bundle backup (`.old` file) handling improved to remove existing backup before creating new one

## [0.5.0] - 2026-01-11

### Added
- **Docker Support**: Run Archiver in containers on any platform
  - Interactive setup mode generates bundle file without Linux installation
  - Automated backups with cron scheduling or manual execution
  - Multi-architecture images (linux/amd64, linux/arm64) published to GHCR
- **Bundle Commands**: `archiver bundle export` and `archiver bundle import` for managing configuration
- **Health Check**: `archiver healthcheck` command for monitoring system health

### Changed
- **BREAKING**: Renamed export/import to bundle terminology
  - `archiver export` → `archiver bundle export`
  - `archiver import` → `archiver bundle import`
  - Directory: `exports/` → `bundle/`
  - Environment: `EXPORT_PASSWORD` → `BUNDLE_PASSWORD`
- Bundle file is now always `bundle.tar.enc` (previous version saved as `.old`, no timestamps)
- Cron jobs now scheduled for invoking user instead of root user
- Logs now show storage target name (e.g., "Service: Synology") instead of generic "Archiver" during copy/wrap-up operations

### Fixed
- Setup script now handles special characters (like `$`) in passwords and API keys correctly
- Setup script now creates bundle file even if one wasn't imported
- Setup script improved S3 region prompt with better examples and optional handling
- Setup script and bundle-export script now work correctly in Docker (no SUDO_USER dependency)
- Cron schedule now properly created from setup script for the correct user
- Log viewer now properly exits when backup completes instead of hanging
- Restored files now have correct ownership matching the invoking user
- Directory ownership fixed when restoring to non-existent directory
- Removed obsolete `PRUNE_KEEP_ARRAY` from log output
- Better error messages when running setup on unsupported platforms

### Improved
- Setup script auto-imports existing bundle files
- Version pinning for reliable installations
- Platform detection provides helpful Docker guidance for non-Linux systems
- Cron instructions now show correct commands for user crontab

### Migration Notes

#### Migrating from v0.4.x to v0.5.0 (Bundle Terminology)

The export/import functionality has been renamed to use "bundle" terminology:

1. **Export file location changed**: `exports/` directory is now `bundle/`
2. **Commands renamed**:
   - `archiver export` → `archiver bundle export`
   - `archiver import` → `archiver bundle import`
3. **Bundle file naming**: Files are now always named `bundle.tar.enc` (previous versions saved as `.old`, no timestamps)
4. **Environment variable**: For Docker, `EXPORT_PASSWORD` is now `BUNDLE_PASSWORD`

**Action Required for existing export files**:
- Rename your `exports/` directory to `bundle/`
- Rename your export file to `bundle.tar.enc`
- Use the new `archiver bundle export` and `archiver bundle import` commands going forward

#### Migrating from Traditional Installation to Docker

Docker installation is now the recommended deployment method. Traditional installation will be **deprecated in v0.7.0**.

**Steps to migrate**:

1. **Export your configuration** (from your current traditional install):
   ```bash
   archiver bundle export
   ```
   This creates `bundle/bundle.tar.enc` with your config and keys encrypted.

2. **Save your bundle password**: Remember the password you used to encrypt the bundle - you'll need it for Docker.

3. **Set up Docker Compose**:
   ```yaml
   services:
     archiver:
       container_name: archiver
       image: ghcr.io/sisyphusmd/archiver:0.5.0
       restart: unless-stopped
       environment:
         TZ: "UTC" # Examples: "America/New_York", "Europe/London", "Asia/Tokyo"
         BUNDLE_PASSWORD: "your-bundle-password-here"
         CRON_SCHEDULE: "0 3 * * *"  # Optional - backup will wait for manual invocation if not set
       volumes:
         - /path/to/bundle.tar.enc:/opt/archiver/bundle/bundle.tar.enc
         - /path/to/logs:/opt/archiver/logs # Optional - for log persistence
         - /path/to/service_directories:/path/to/service_directories # Container path must match SERVICE_DIRECTORIES in your config.sh
       hostname: backup-server # Optional - used in backup snapshot IDs
   ```

4. **Important**: Volume paths inside the container must match the `SERVICE_DIRECTORIES` paths in your `config.sh`. If your config has `/home/user/data`, mount it to the same path: `-/home/user/data:/home/user/data`

5. **Start the container**:
   ```bash
   docker compose up -d
   ```

6. **Verify**: Check logs with `docker logs -f archiver`

**Note**: You can run both traditional and Docker installations side-by-side during migration for testing.

## [0.4.1] - 2025-06-07
### Fixed
- Added missing restore function for S3 storage backend.
- Fixed ownership of new local directory when restoring.

## [0.4.0] - 2025-06-06
### Added
- S3 storage backend support added alongside existing SFTP and B2 backends.

### Improved
- Setup script explicitly checks for export file existence, skipping import gracefully if no exports exist.

## [0.3.2] - 2024-06-06
### Improved
- Added import and export functions to backup your config.sh and keys files
- Setup script will auto-import any export file placed in the archiver repo directory
- Setup script also ensures you have an export file, and places it in an 'exports' directory

## [0.3.1] - 2024-06-06
### Fixed
- Fixed pruning (backup rotations)

## [0.3.0] - 2024-06-05
## **!!!BREAKING CHANGE!!!**
- Calling the script with no argument will no longer initiate a backup
- Must use the full command with argument: 'archiver start'
- If you have cronjobs scheduled without the 'start' argument, they will no longer initiate a backup without editing to include 'start'
  - You can edit your cronjobs with the following command: 'sudo crontab -e'
  - i.e. '0 3 * * * archiver start'

### Improved
- Massive argument improvements:
  - Arguments are now single words, not prefaced by '--'
    - archiver start|stop|pause|resume|restart|logs|status|setup|uninstall|restore|help
  - 'archiver' command with no argument (or with 'help' argument) prints a guide to available arguments
  - 'start':
    - 'archiver start' is now required to initiate a backup
      - may need to edit 'sudo crontab -e' if it previously did not include the 'start' argument
    - 'archiver start logs' to initiate a backup and view logs
    - 'archiver start prune|retain' prune and retain will override the behavior to prune or retain backups for this run only
    - logs and prune|retain can be combined
  - 'stop|pause|resume':
    - 'archiver stop|pause|resume' manually stops, pauses, or resumes a running backup
    - 'archiver resume' can be combined with 'logs'
  - 'logs'
    - 'archiver logs' will display the logs of a running backup (but no longer starts a new backup)
  - 'status'
    - 'archiver status' prints whether or not there is a currently running backup process
  - 'restart'
    - 'archiver restart' will stop any running backup and start a new one from the beginning
    - similar to 'archiver start' can be used with logs|prune|retain
  - 'restore'
    - 'archiver restore' will run the restore script
  - 'help'
    - 'archiver help' will display information about available commands and arguments
  - 'setup|uninstall'
    - 'archiver setup|uninstall' will run the setup or uninstall scripts
    - although, on first run, setup will require './archiver.sh setup' from the archiver dir, given archiver will not be in the PATH yet
    - uninstall function coming soon

## [0.2.3] - 2024-06-04
### Fixed
- Fixed LOCKFILE being left by main.sh.

### Improved
- Setup script places archiver in PATH.
  - Please run './setup.sh' again from the archiver repo directory to make this change.
    - You can skip all sections of the setup script by typing 'n' when prompted. The script will make this change regardless.
  - You should now run Archiver backups with the command 'archiver'.
  - This is global, no more need to change to your archiver directory.
  - It accepts arguments, such as 'archiver --view-logs' and 'archiver --stop'. More arguments to come soon.
  - Cron can also call 'archiver' directly: (e.g. '0 3 * * * archiver').
    - To edit your prior cronjob, run 'sudo crontab -e', and replace the path to the archiver script with simply 'archiver'.

## [0.2.2] - 2024-06-03
### Improved
- Scripts will auto-escalate to sudo now. So README no longer recommends to run with sudo.
- Logs all go to a single file now for easier viewing.
- Reorganized directory structure.
- Stop is now an argument './archiver.sh --stop'.

## [0.2.1] - 2024-06-02
### Improved
- The LOCKFILE mechanism is much more robust now.
- Setting the stage for all commands to be through "sudo archiver --argument" rather than through calling various scripts.

## [0.2.0] - 2024-06-01
### Improved
- Major improvements to speed. Backup to primary storage for all repositories completes first, then each secondary storage copies sequentially.

## [0.1.1] - 2024-06-01
### Fixed
- Fixed the Duplicacy Prune function. Backup rotations work now.

## [0.1.0] - 2024-05-31
### Added
- Initial release of the project.
