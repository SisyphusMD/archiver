# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- Consolidated the per-storage-type credential and URL construction — previously duplicated across the primary-backup, add-copy, and restore code paths — into two shared helpers in `config-loader.sh`: `build_storage_url` and `export_duplicacy_storage_secrets`. No behavior change. Adds bats coverage of the `DUPLICACY_<NAME>_*` credential mapping across all four storage types (the surface the 0.8.10/0.8.11 do-spaces incidents traced to), so a single definition is now the source of truth for both backup and restore.

### Fixed
- Restore silently lost original file ownership under the recommended hardened cap set: the example `compose.yaml` / `README.md` did `cap_drop: ALL` and added back only `DAC_OVERRIDE` + `SETGID`, but restore preserves ownership by default (`-ignore-owner` is opt-in) and Duplicacy recreates the original UID/GID via `chown()`, which requires `CAP_CHOWN` (and `CAP_FOWNER` to set mode/timestamps on other-UID files). `DAC_OVERRIDE` bypasses permission checks but not `chown`, so a restore completed with every file owned by root. Added `CHOWN` + `FOWNER` to the documented cap set. (Default Docker caps already include `CHOWN`; this only affected the hardened `cap_drop: ALL` template.)

## [0.8.11] - 2026-06-05

### Fixed
- The 0.8.10 do-spaces restore fix was still broken in `auto-restore`: `sanitize_storage_name` logs a "name was sanitized" WARN via `log_message`, and `auto-restore.sh` redefines `log_message` to also echo to **stdout** — so callers doing `name="$(sanitize_storage_name …)"` captured that log line into the name, re-corrupting `DUPLICACY_<NAME>_*`. The function now redirects its `log_message` to stderr, so only the sanitized name reaches stdout. Kept as one shared function (called by both backup and restore) so the sanitization is guaranteed identical on both paths.

## [0.8.10] - 2026-06-05

### Fixed
- Restore from a storage whose name contains a character invalid in a shell identifier (e.g. a hyphen, like `do-spaces`) failed: `duplicacy-restore.sh` built `DUPLICACY_<NAME>_S3_ID`/`_SECRET`/`_PASSWORD` from the **raw** name, so `export` rejected it ("not a valid identifier"), the credentials never reached duplicacy, and it fell through to an interactive prompt → EOF → that target was skipped. Now sanitizes the storage name (matching `duplicacy-backup.sh`) before building the env-var names and `-storage-name`.

## [0.8.9] - 2026-06-04

### Fixed
- `auto-restore-all.sh` shipped without its execute bit (mode 0644 in the repo, preserved into the image by `COPY`), so `archiver auto-restore-all` failed with "Permission denied" when the dispatcher `exec`'d it — breaking the tier-3 full-host restore path. Restored the script's exec bit and hardened the Dockerfile to `chmod +x lib/scripts/*.sh` so a missing source bit can't recur.

## [0.8.8] - 2026-06-03

### Added
- `RUN_RESTORE_SERVICE` env toggle on `auto-restore` (and, via pass-through, `auto-restore-all`): when non-empty, runs the service's `./restore-service.sh` after a successful file restore — non-interactively reloading databases and restarting the stack, mirroring what the interactive `restore` command prompts for. Its exit code propagates, so a failed reload surfaces as a failed restore (and a failed service in the `auto-restore-all` summary). Required for full-host DR where databases are backed up as dumps (not raw data dirs) and so must be reloaded post-restore. The interactive `restore` command also honors the toggle to skip its y/N prompt.

## [0.8.7] - 2026-06-03

### Added
- `auto-restore-all` command: non-interactive restore of every service in `SERVICE_DIRECTORIES` in one pass. Iterates the service registry and runs `auto-restore` at the latest revision (trying all storage targets) for each, then aggregates — exits 0 only if every service restored, 1 if any failed, 3 if the backup lock is held. Optional env (`REVISION`, `STORAGE_TARGET`, `OVERWRITE`, `DELETE_EXTRA`, `HASH_COMPARE`, `IGNORE_OWNERSHIP`) passes through to each per-service restore. Available as `archiver auto-restore-all` and entrypoint `run auto-restore-all`. Built for full-host disaster recovery onto a blank box.

### Dependencies

- chore(deps): update actions/checkout action to v6.0.3
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to c7d3c51
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 159006f
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 2bfc4bc
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 8f55827
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 5a74dfa
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 8c42892
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to fd4978c
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 61e442c
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 684d92c
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 59ebcd2
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 49e7bdf
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 4d6a1f3
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to bc173ba
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 96db676
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 9d905e2
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to b63ff3b
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 179b9ff
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 4139c95
- chore(deps): update quay.io/skopeo/stable:v1.22.2 docker digest to 4d60d6c

## [0.8.6] - 2026-05-05 - CI and Dockerfile modernization

No behavioral changes to the archiver application itself — only the CI pipeline, release flow, and image build process. Aligns archiver with the same externalized-repo conventions used by sibling projects on `forgejo.bryantserver.com`.

### Added
- `release.yml` workflow for cutting releases via `workflow_dispatch` with `bump=patch|minor|major`. Promotes `[Unreleased]` to a versioned section, bumps image refs in `compose.yaml` and `README.md`, runs a smoke test, then tags + pushes — replaces the prior manual `git tag` flow.
- `.renovaterc.json` introduced. Tracks the Dockerfile base image, ENV-pinned upstream versions (`DUPLICACY_VERSION` via github-releases against `gilbertchen/duplicacy`, `DOCKER_CLI_VERSION` via github-releases against `moby/moby`), Forgejo Action versions, and the inline skopeo image referenced by `docker-publish.yml`.

### Changed
- CI: `docker-publish.yml` split from a monolithic `build-and-push` job into separate jobs (`shellcheck`, `validate-changelog`, `pr-validate`, `build-and-push`, `create-release`, `summary`). Each concern has its own gate; failures surface to the new aggregator job rather than silently passing.
- CI: cluster push now builds to a local OCI archive and uses `skopeo copy` per tag rather than `docker buildx build --push`. Forgejo 15.0.1 nil-derefs in `EndUploadBlob` when buildx's parallel multi-arch push hits the `UQE_package_blob_blake2b`/`sha512` unique constraint — byte-identical blobs across amd64/arm64 (e.g. arch-independent `COPY` layers) race two PUTs of the same digest. Skopeo walks manifests serially with `TryReusingBlob` HEAD-checks, so byte-identical blobs across arches dedupe before PUT.
- CI: NAS Forgejo mirror now uses `skopeo sync` (full repo, self-healing) instead of `docker buildx imagetools create`. Same Forgejo 15.0.1 bug class as the cluster push.
- CI: GitHub Container Registry mirror now uses `skopeo copy` per tag instead of `docker buildx imagetools create`. Different registry (no Forgejo nil-deref bug there), but skopeo's HEAD-first dedup is safer against any concurrent-PUT race in the GHCR backend.
- CI: build cache moved from `type=registry,ref=...:buildcache` (separate cache push that hits the same Forgejo bug) to `type=inline` (cache annotations embedded on the published image manifest; `cache-from: ...:0.8` reads the previous release's inline cache on subsequent builds).
- CI: skopeo invocation stages the OCI archive into a docker-managed named volume via `docker cp` rather than bind-mounting from `${{ github.workspace }}` — the runner-container's view of the workspace path differs from the docker host's, so a child container's `-v <archive>:<target>` silently creates an empty dir at the host path instead of finding the archive.
- CI: release creation now reconciles every `v*.*.*` tag on every release run across all three registries (cluster Forgejo, NAS Forgejo, GitHub). A release that failed to publish on any registry (transient 5xx, NAS down, mirror lag) is picked up by the next successful release run rather than staying absent forever. Includes per-tag `target_commitish` from `git rev-parse "${tag}^{}"` for metadata normalization and defense against mirror-lag races where a reconciled tag isn't yet on the destination.
- CI: cluster Forgejo release is now attributed to a real-user PAT (`CLUSTER_REPO_WRITE_PAT`) instead of the auto-injected `GITHUB_TOKEN`, so release commits and tags carry a stable author identity rather than appearing as "ghost".
- CI: secret naming aligned with the bryantserver externalized-repo convention. `BRYANTSERVER_REGISTRY_PAT` → `CLUSTER_REGISTRY_PUSH_PAT`, `NAS_FORGEJO_PAT` → `NAS_FORGEJO_WRITE_PAT`. New: `CLUSTER_REPO_WRITE_PAT` for `release.yml`'s commit/tag/push and create-release attribution. `GHCR_PUSH_PAT` and `GH_RELEASE_PAT` unchanged (already convention-shaped).
- CI: tag patterns reduced to SemVer-only (was also `:main`/`:pr-N` from `type=ref,event=branch`/`event=pr`). PR builds now validate via the `pr-validate` job (build + smoke + arm64 dry-run) without polluting the registry with branch-name and PR-number tags.
- CI: Forgejo Action versions digest-pinned (`@v4`/`@v5` → `@<digest>`); Renovate tracks both major version and digest.
- Dockerfile: base image digest-pinned (`debian:trixie-20260112-slim` → `...@sha256:...`).
- Dockerfile: removed `wget`, `gnupg`, and `lsb-release` from the apt install list. No longer needed once curl replaces wget for the duplicacy download and the docker-ce-cli apt repo dance is gone.
- Dockerfile: Docker CLI install switched from the `docker-ce-cli` debian package (with `epoch:upstream-revision` versioning that has no clean Renovate datasource) to the static binary archive at `download.docker.com/linux/static/stable/<arch>/docker-<version>.tgz`. ENV pin becomes clean SemVer (`29.4.2` instead of `5:29.1.4-1`), Renovate-tracked via `extractVersion=^docker-v(?<version>.+)$` against `moby/moby` GitHub releases. Trade-off: lose apt's auto security-patch flow within a release stream; gain explicit Renovate-managed bumps. Acceptable for a CLI talking to a host-mounted socket.
- Dockerfile: `DUPLICACY_VERSION` Renovate-tracked via `extractVersion=^v(?<version>.+)$` against `gilbertchen/duplicacy` GitHub releases. Renovate auto-opens PRs for new duplicacy releases.
- Dockerfile: switched `wget` → `curl` for the duplicacy binary download, since curl is already installed and wget is no longer needed.

### Notes
- All 27 prior releases (v0.1.0 through v0.8.5) will be retroactively reconciled on the cluster + NAS Forgejos and GitHub on the next release run to set per-tag `target_commitish`. They previously had blank or `"main"` values; cosmetic backfill, no functional change.

## [0.8.5] - 2026-04-30 — Forgejo as primary registry

### Changed
- **Distribution**: Container images are now published to both `forgejo.bryantserver.com/sisyphusmd/archiver` and `ghcr.io/sisyphusmd/archiver`. Forgejo is the primary registry; `ghcr.io` remains available as a mirror. Existing image tags through 0.8.4 are unchanged on both registries.
- **Repository**: Project source-of-truth has moved to [forgejo.bryantserver.com/SisyphusMD/archiver](https://forgejo.bryantserver.com/SisyphusMD/archiver). The GitHub repository at [github.com/SisyphusMD/archiver](https://github.com/SisyphusMD/archiver) is now a read-only push-mirror.
- **Documentation**: README and configuration guides updated to reference the Forgejo image path as primary.

## [0.8.4] - 2026-04-24

### Changed
- **`archiver run backup` CLI now exits `1` on per-service errors.** Previously exited 0 regardless, surfacing failures only via log output and the optional Pushover notification. This makes the CLI suitable for external schedulers that key off the exit code — e.g., Kubernetes `Job` / `CronJob` (where the `Complete` / `Failed` status condition is derived from container exit code) or CI pipelines. The long-lived container deployment (`archiver start`, with the internal cron) is unaffected: per-service iteration and notification behavior are unchanged, and the backgrounded backup cycle's exit code remains unobserved by the daemon.

### Migration
- Shell wrappers around `archiver run backup` that relied on exit 0 and chained subsequent commands: wrap with `|| true` if the old continue-on-error behavior is desired.
- No action needed for containerized `archiver start` deployments.

## [0.8.3] - 2026-04-23

### Fixed
- **Container Detection in Kubernetes**: The container-deployment guard now recognizes Kubernetes Pods running on containerd or CRI-O, which create neither `/.dockerenv` nor `/run/.containerenv`. Detection now also checks `$KUBERNETES_SERVICE_HOST` (set automatically in every Pod) and `/proc/self/cgroup` (catches standalone containerd, CRI-O, LXC, rkt). Previously, running `archiver:0.8.2 run backup` from a Kubernetes `CronJob` or `Job` failed at startup with "Only container deployment is supported" unless the image was rebuilt with a `touch /.dockerenv` kludge.

### Changed
- **Internal**: Renamed `lib/core/require-docker.sh` → `lib/core/require-container.sh` and the associated `REQUIRE_DOCKER_CORE` / `REQUIRE_DOCKER_SH_SOURCED` constants. No user-visible change; the script is sourced internally.

## [0.8.2] - 2026-04-21

### Added
- **`run backup` Mode**: New synchronous backup path for external schedulers (Kubernetes `CronJob`, GitHub Actions scheduled workflows, systemd timers, etc.). `docker run ... archiver:0.8.2 run backup` decrypts the bundle, runs a backup synchronously, and exits with the backup's result code so the scheduler can report success/failure accurately. Accepts the same optional `prune` / `retain` flags as `archiver start`. This complements — and does not replace — `archiver start`, which remains async and is the right choice for the in-container cron daemon and for fire-and-forget use from a long-lived container.

## [0.8.1] - 2026-04-21

### Added
- **`run` Entrypoint Mode**: New container mode for one-shot non-interactive command invocation, designed for Kubernetes Jobs / init containers and other CI flows. `docker run ... archiver:0.8.1 run <subcommand>` decrypts the bundle, execs the subcommand, and the container's exit code equals the subcommand's exit code. Whitelisted subcommands: `auto-restore`, `snapshot-exists`, `healthcheck`. Long-running or async commands (`start`, `stop`, `pause`, etc.) are intentionally rejected with exit code `2`.

### Changed
- **Internal**: Renamed `lib/features/duplicacy.sh` to `lib/features/duplicacy-backup.sh` for symmetry with `lib/features/duplicacy-restore.sh` (introduced in 0.8.0). No user-visible change.
- **Internal**: Extracted bundle decrypt + import logic in `docker-entrypoint.sh` into a `prepare_bundle()` helper, reused by both the default daemon path and the new `run` mode. Behavior of existing `init` and daemon modes is unchanged.

## [0.8.0] - 2026-04-21

### Added
- **Non-interactive Restore Commands**: Two new subcommands for automated disaster recovery flows (e.g. Kubernetes init containers)
  - `archiver snapshot-exists` — probes every configured storage target for `SNAPSHOT_ID` and short-circuits on the first hit; exit codes `0=exists`, `1=not found`, `2=undetermined`, `3=lock held`
  - `archiver auto-restore` — iterates storage targets in order and restores from the first target that has the requested snapshot; env-driven (`SNAPSHOT_ID`, `LOCAL_DIR` required; `REVISION`, `STORAGE_TARGET`, `OVERWRITE`, `DELETE_EXTRA`, `HASH_COMPARE`, `IGNORE_OWNERSHIP`, `RESTORE_THREADS` optional)
- **Shared Restore Library**: Extracted duplicacy restore plumbing (storage target resolution, repo init, revision listing, restore) into `lib/features/duplicacy-restore.sh`, shared by interactive `restore`, `snapshot-exists`, and `auto-restore`

### Fixed
- **`archiver healthcheck` Exit Code**: The dispatcher previously clobbered the healthcheck's exit status to `0`, causing Docker healthchecks based on `archiver healthcheck` to always report healthy. It now propagates the underlying exit code

## [0.7.1] - 2026-04-14

### Added
- **Podman Support**: Container detection now recognizes Podman (`/run/.containerenv`) in addition to Docker (`/.dockerenv`), removing the need to mount a fake `.dockerenv` file
- **Host Management Tools**: Added `systemd` and `zfsutils-linux` packages to the container image
  - `systemctl` — manage host services from restore scripts (start, stop, mask, unmask, etc.)
  - `zfs` — take ZFS snapshots before restore operations (requires `/dev/zfs` mount)
  - Requires `SYSTEMCTL_FORCE_BUS=1` env var and D-Bus socket + systemd unit directory mounts
- **Security Hardening**: Added `cap_drop: ALL` with explicit `cap_add` to compose.yaml and README examples
  - `DAC_OVERRIDE` — required for writing to directories owned by other UIDs
  - `SETGID` — required for cron to execute scheduled jobs
  - `no-new-privileges:true` — recommended security option
- **Socket Documentation**: Documented mounting options for Podman socket, systemd D-Bus socket, systemd unit directory, and ZFS device node with per-socket security warnings

### Changed
- Updated available tools list in example scripts and migration guide to include `systemctl` and `zfs`

## [0.7.0] - 2026-01-22

### ⚠️ BREAKING CHANGES
- **Docker-Only Deployment**: Direct installation on host systems is no longer supported. See [migration guide](docs/guides/migration/legacy-to-docker.md).

### Added
- **Graceful Stop**: Stop command respects backup stage, completing service cleanup before termination
  - New `archiver stop --immediate` flag for emergency termination
  - Sends summary notification with runtime and error count
- **Docker Compose Graceful Shutdown**: Added `stop_grace_period: 2m` to ensure proper service cleanup when stopping containers
- **Migration Documentation**: Comprehensive guides for migrating legacy installations (v0.3.2+) to Docker v0.7.0
  - Legacy to Docker migration guide
  - Configuration editing in Docker
  - Local storage setup
  - SSH key management

### Improved
- **Prune Operation**: Now uses `-exhaustive` flag to remove orphaned chunks from manually deleted snapshots and incomplete backups
- **Notifications**: Include hostname and timestamp; respect TZ environment variable
- **Bundle Export**: Offers to reuse BUNDLE_PASSWORD environment variable
- **Storage Names**: Automatically sanitize storage names with hyphens for Bash compatibility
- **Code Organization**: Restructured lib directory into core/features/scripts

### Changed
- Streamlined documentation for Docker-only workflow
- **SFTP SSH Key Path**: Hardcoded to `/opt/archiver/keys/id_ed25519` for Docker consistency
  - Removed `STORAGE_TARGET_X_SFTP_KEY_FILE` configuration variable
  - Migrating users: Old variable in config.sh will be safely ignored

### Removed
- Legacy/direct installation support and related commands

## [0.6.5] - 2026-01-14

### Added
- **Container Tools**: Added text editors and network utilities to Docker image
  - `nano` - Simple text editor for quick file edits
  - `vim` - Full-featured text editor
  - `iputils-ping` - Network connectivity testing

### Changed
- **Documentation**: Updated README to prioritize `archiver logs` command over `docker logs`
- **Examples**: Updated service script examples with complete list of available tools

## [0.6.4] - 2026-01-14

### Fixed
- **Docker Container Issues**:
  - Added `procps` package to Docker image to provide `pkill` and `pgrep` commands
  - Fixes "command not found" errors when stopping backup processes
  - Resolves container shutdown hangs caused by orphaned processes
  - Improved graceful shutdown by explicitly terminating log tailer process
- **Bundle Export**: Fixed bundle backup verification to prevent data loss
  - Now verifies the backup file exists before continuing with export
  - Exits with error if backup operation fails instead of proceeding
- **Docker Logs**: Changed log tailer to show only new logs (`tail -n 0`)
  - Prevents `docker logs` from hanging with large log files (100k+ lines)
  - Improves performance for long-running backups with extensive logging

### Changed
- **Docker Volume Mount**: Updated documentation to mount bundle directory instead of single file
  - Resolves "Device or resource busy" errors during bundle export
  - Allows proper file operations within mounted directory

## [0.6.3] - 2026-01-13

### Changed
- **Duplicacy Update**: Updated Duplicacy to v3.2.5 (from v3.2.3)

## [0.6.2] - 2026-01-13

### Added
- **SQLite3 Support**: sqlite3 package now included in container image
  - Enables database operations and backups directly from service scripts
  - Useful for services using SQLite databases

### Documentation
- Added comprehensive list of available tools and packages in example service scripts
- Documents all available tools: duplicacy, docker, sqlite3, curl, wget, ssh, openssl, etc.
- Clarifies Docker socket requirement for docker commands

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

Docker installation is now the recommended deployment method. Legacy installation will be **deprecated in v0.7.0**.

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
