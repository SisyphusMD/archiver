#!/bin/bash
set -e

# Docker entrypoint script for Archiver
# Handles import of encrypted config/keys and optional cron setup

BUNDLE_FILE="/opt/archiver/bundle/bundle.tar.enc"
BUNDLE_OUTPUT_DIR="/opt/archiver/bundle"
LOG_FILE="/opt/archiver/logs/archiver.log"

# Signal handler for graceful shutdown
handle_shutdown() {
  echo "Received shutdown signal, attempting graceful stop..."

  # Stop any running backup (handles all cases gracefully)
  /opt/archiver/archiver.sh stop 2>&1 || true

  # Kill the log tailer if it exists
  if [ -n "$LOG_TAILER_PID" ] && kill -0 "$LOG_TAILER_PID" 2>/dev/null; then
    kill "$LOG_TAILER_PID" 2>/dev/null || true
  fi

  exit 0
}

# Set up signal trap for SIGTERM (sent by docker stop)
trap 'handle_shutdown' SIGTERM

echo "==================================="
echo "Archiver Container Starting"
echo "==================================="

# Check if running in setup mode
if [ "$1" = "setup" ]; then
    echo "Running in SETUP mode"
    echo ""
    echo "This will guide you through the initial configuration of Archiver."
    echo "Make sure you have a volume mounted at /opt/archiver/bundle to save the generated bundle file."
    echo ""

    # Ensure bundle output directory exists and is writable
    mkdir -p "$BUNDLE_OUTPUT_DIR"

    # Run setup script interactively
    cd /opt/archiver
    exec ./archiver.sh setup
fi

# Normal runtime mode - validate required environment variables
if [ -z "$BUNDLE_PASSWORD" ]; then
    echo "ERROR: BUNDLE_PASSWORD environment variable is required"
    echo "Please set it to the password used to encrypt your bundle.tar.enc file"
    exit 1
fi

# Check if bundle file exists
if [ ! -f "$BUNDLE_FILE" ]; then
    echo "ERROR: Bundle file not found at $BUNDLE_FILE"
    echo "Please mount your bundle directory to /opt/archiver/bundle"
    echo "Example: docker run -v /path/to/bundle/dir:/opt/archiver/bundle ..."
    exit 1
fi

echo "Bundle file found: $BUNDLE_FILE"

# Import the encrypted config and keys non-interactively
echo "Decrypting and importing configuration..."
export ARCHIVER_NON_INTERACTIVE=1
export ARCHIVER_BUNDLE_PASSWORD="$BUNDLE_PASSWORD"
export ARCHIVER_BUNDLE_FILE="$BUNDLE_FILE"

cd /opt/archiver
if ! /opt/archiver/lib/mod/bundle-import.sh; then
    echo "ERROR: Failed to import configuration"
    echo "Please verify your BUNDLE_PASSWORD is correct"
    exit 1
fi

echo "Configuration imported successfully"

# Verify critical files exist after import
if [ ! -f "/opt/archiver/config.sh" ]; then
    echo "ERROR: config.sh not found after import"
    exit 1
fi

if [ ! -f "/opt/archiver/keys/private.pem" ]; then
    echo "ERROR: RSA keys not found after import"
    exit 1
fi

echo "All required files present"

# Start log tailer in background to forward logs to stdout
# This allows 'docker logs -f' to work
# Use tail -F to follow the symlink through log rotations
if [ -d "/opt/archiver/logs" ]; then
    (
        # Wait for log file to be created (may take a moment if cron is used)
        while [ ! -f "$LOG_FILE" ]; do
            sleep 1
        done

        # Display logo
        if [ -f "/opt/archiver/lib/logos/logo.ascii" ]; then
            cat /opt/archiver/lib/logos/logo.ascii
            echo ""
        fi
        echo "--- Archiver Logs ---"
        # -F follows by name (handles log rotation), -n 0 shows only new lines from now
        tail -F -n 0 "$LOG_FILE" 2>/dev/null
    ) &
    LOG_TAILER_PID=$!
fi

# Setup cron if CRON_SCHEDULE is provided
if [ -n "$CRON_SCHEDULE" ]; then
    echo "Setting up cron with schedule: $CRON_SCHEDULE"

    # Create cron job with PATH set
    cat > /etc/cron.d/archiver << EOF
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
$CRON_SCHEDULE /usr/local/bin/archiver start >> /proc/1/fd/1 2>&1
EOF
    chmod 0644 /etc/cron.d/archiver

    # Apply cron job
    crontab /etc/cron.d/archiver

    echo "Cron configured. Backups will run on schedule: $CRON_SCHEDULE"
    echo "Starting cron daemon..."

    # Start cron in foreground
    cron -f
else
    echo "No CRON_SCHEDULE set. Container will wait for manual commands."
    echo "Use 'docker exec <container> archiver start' to run backups manually"
    echo ""
    echo "Container is ready and will stay running."

    # Keep container alive indefinitely
    # This allows users to exec in and run commands manually
    tail -f /dev/null
fi
