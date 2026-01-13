# Docker Implementation Summary

This document provides an overview of the Docker implementation for Archiver.

## Files Created

### Core Docker Files
1. **[Dockerfile](Dockerfile)** - Multi-architecture build supporting linux/amd64 and linux/arm64
2. **[docker-entrypoint.sh](docker-entrypoint.sh)** - Container startup script that handles import and cron setup
3. **[.dockerignore](.dockerignore)** - Excludes unnecessary files from Docker build context
4. **[compose.yaml](compose.yaml)** - Example Docker Compose configuration for users

### CI/CD
5. **[.github/workflows/docker-publish.yml](.github/workflows/docker-publish.yml)** - GitHub Actions workflow for automated builds and publishing to GHCR

### Documentation
6. **README.md** - Updated with comprehensive Docker installation and usage instructions
7. **DOCKER.md** (this file) - Implementation summary

### Modified Files
8. **[lib/mod/bundle-import.sh](lib/mod/bundle-import.sh)** - Enhanced to support non-interactive mode for Docker
9. **[archiver.sh](archiver.sh)** - Updated to support `bundle` command with `export`/`import` subcommands

## How It Works

### Setup Mode (Interactive)
Run the container with `setup` argument to configure Archiver:
1. Container starts with `setup` command
2. Runs interactive `./archiver.sh setup` script
3. User configures storage backends, directories, and options
4. Generates RSA and SSH keys
5. Creates `bundle/bundle.tar.enc` file in mounted volume
6. Container exits after setup completes

### Runtime Mode (Automated)
Normal operation with existing bundle file:
1. Container starts and runs `docker-entrypoint.sh`
2. Script validates required environment variables (`BUNDLE_PASSWORD`)
3. Script checks for mounted bundle file at `/opt/archiver/bundle/bundle.tar.enc`
4. Script runs `bundle-import.sh` in non-interactive mode to decrypt and extract config/keys
5. Script verifies critical files exist (config.sh, RSA keys)
6. Script starts log tailer in background to forward logs to stdout
7. **If CRON_SCHEDULE is set**: Sets up cron job and starts cron daemon
8. **If CRON_SCHEDULE is empty**: Keeps container running, waits for manual commands

### Environment Variables
- `BUNDLE_PASSWORD` (required in runtime mode) - Password to decrypt the bundle.tar.enc file
- `CRON_SCHEDULE` (optional) - Cron expression for automatic backups

### Volume Mounts
- `/opt/archiver/bundle/bundle.tar.enc` (required in runtime mode, read-only) - Encrypted bundle file
- `/opt/archiver/bundle/` (required in setup mode, read-write) - Directory to save generated bundle
- `/opt/archiver/logs` (optional) - Persistent logs directory
- User-defined service directories (required) - Must match paths in config.sh

## Docker Usage

For comprehensive Docker usage instructions including manual commands, backup operations, log viewing, and restore procedures, see the **Docker Manual Commands** section in the [README.md](README.md#docker-manual-commands).

## GitHub Container Registry Setup

To publish images to GHCR, follow these steps:

### 1. Enable GitHub Actions
The workflow is already in `.github/workflows/docker-publish.yml` and will trigger on:
- Version tags (e.g., `v0.5.0`) - Builds and pushes semantic version tags
- Pull requests (build only, no push)
- Manual workflow dispatch

### 2. Enable GitHub Packages
1. Go to your repository on GitHub
2. Click **Settings** > **Actions** > **General**
3. Under "Workflow permissions", ensure "Read and write permissions" is selected
4. Click **Save**

### 3. Make Repository Public (for public images)
If you want public images at ghcr.io:
1. Go to **Settings** > **General**
2. Scroll to "Danger Zone"
3. Click "Change visibility" > "Make public"

Or keep private and authenticate with GitHub token when pulling images.

### 4. Create a Release
To trigger the first build:

```bash
# Commit all changes
git add .
git commit -m "Add Docker support"
git push origin main

# Create and push a version tag
git tag v0.5.0
git push origin v0.5.0
```

### 5. Monitor Build
1. Go to **Actions** tab in GitHub
2. Watch the "Build and Publish Docker Image" workflow
3. Once complete, image will be available at `ghcr.io/sisyphusmd/archiver:0.5.0`

### 6. Verify Published Images
After successful build, images will be tagged with:
- Full version: `ghcr.io/sisyphusmd/archiver:0.5.0` (recommended - most specific)
- Minor version: `ghcr.io/sisyphusmd/archiver:0.5` (gets latest patch updates)
- Major version: `ghcr.io/sisyphusmd/archiver:0` (gets all updates in v0.x.x)

**Note**: This project does not use a `latest` tag. Always specify a version tag to ensure reproducible deployments.

### 7. Make Package Public (optional)
By default, GHCR packages inherit repository visibility. To make public:
1. Go to your GitHub profile > **Packages**
2. Click on "archiver" package
3. Click **Package settings**
4. Scroll to "Danger Zone"
5. Click "Change visibility" > "Public"

## Testing Locally

Before pushing, you can test the Docker build locally:

```bash
# Build for your local architecture
docker build -t archiver:test .

# Or build multi-arch (requires buildx)
docker buildx create --use
docker buildx build --platform linux/amd64,linux/arm64 -t archiver:test .

# Test setup mode
docker run --rm -it \
  -v ./test-bundle:/opt/archiver/bundle \
  archiver:test setup

# Test runtime mode
docker run --rm -it \
  -e BUNDLE_PASSWORD="your-password" \
  -e CRON_SCHEDULE="" \
  -v ./test-bundle/bundle.tar.enc:/opt/archiver/bundle/bundle.tar.enc:ro \
  archiver:test
```

## Key Design Decisions

1. **Two modes** - Setup mode for interactive configuration, runtime mode for automated backups
2. **Ephemeral config/keys** - Extracted from bundle file on every container start, never persisted in volumes
3. **Single bundle file** - Always `bundle.tar.enc`, old versions saved as `.old` on export
4. **Debian base** - Maintains compatibility with existing scripts and packages
5. **Root user** - Required for current script architecture (can be improved in future)
6. **Cron in container** - Simple approach for v1, external schedulers can be used in future
7. **Log forwarding** - Background tail process pipes logs to stdout for `docker logs`
8. **Non-interactive import** - New environment variables allow automated extraction without prompts

## Future Improvements

Potential enhancements for future versions:
- Run as non-root user with proper permissions
- Support external schedulers (Kubernetes CronJobs)
- Add health checks
- Volume for Duplicacy cache to improve performance
- Environment variable overrides for all config.sh secrets
- Alpine Linux base image for smaller size
- Init system (s6-overlay) for better process management
