#!/bin/bash
set -e

source "/opt/archiver/lib/core/common.sh"

BUNDLE_FILE="${BUNDLE_DIR}/bundle.tar.enc"
LOG_FILE="${LOG_DIR}/archiver.log"

handle_shutdown() {
  echo "Received shutdown signal, attempting graceful stop..."

  "${ARCHIVER_DIR}/archiver.sh" stop 2>&1 || true

  if [ -n "$LOG_TAILER_PID" ] && kill -0 "$LOG_TAILER_PID" 2>/dev/null; then
    kill "$LOG_TAILER_PID" 2>/dev/null || true
  fi

  # Kill cron or tail process if running
  if [ -n "$MAIN_PID" ] && kill -0 "$MAIN_PID" 2>/dev/null; then
    kill "$MAIN_PID" 2>/dev/null || true
  fi

  exit 0
}

trap 'handle_shutdown' SIGTERM

prepare_bundle() {
    if [ -z "$BUNDLE_PASSWORD" ]; then
        echo "ERROR: BUNDLE_PASSWORD environment variable is required"
        echo "Please set it to the password used to encrypt your bundle.tar.enc file"
        exit 1
    fi

    if [ ! -f "$BUNDLE_FILE" ]; then
        echo "ERROR: Bundle file not found at $BUNDLE_FILE"
        echo "Please mount your bundle directory to ${BUNDLE_DIR}"
        echo "Example: docker run -v /path/to/bundle:${BUNDLE_DIR} ..."
        exit 1
    fi

    echo "Bundle file found: $BUNDLE_FILE"

    echo "Decrypting and importing configuration..."
    export ARCHIVER_BUNDLE_PASSWORD="$BUNDLE_PASSWORD"
    export ARCHIVER_BUNDLE_FILE="$BUNDLE_FILE"

    cd "${ARCHIVER_DIR}"
    if ! "${BUNDLE_IMPORT_SCRIPT}"; then
        echo "ERROR: Failed to import configuration"
        echo "Please verify your BUNDLE_PASSWORD is correct"
        exit 1
    fi

    if [ ! -f "${CONFIG_FILE}" ]; then
        echo "ERROR: config.sh not found after import"
        exit 1
    fi

    if [ ! -f "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" ]; then
        echo "ERROR: RSA keys not found after import"
        exit 1
    fi

    echo "Configuration imported successfully"
}

echo "==================================="
echo "Archiver Container Starting"
echo "==================================="

if [ "$1" = "init" ]; then
    echo "Running in INIT mode"
    echo ""

    if [ -f "$BUNDLE_FILE" ]; then
        echo "WARNING: Bundle file already exists at $BUNDLE_FILE"
        echo "Continuing will overwrite it with a new bundle."
        echo ""
    fi

    mkdir -p "${BUNDLE_DIR}"

    cd "${ARCHIVER_DIR}"
    exec "${SCRIPTS_DIR}/init.sh"
fi

if [ "$1" = "run" ]; then
    shift
    if [ $# -eq 0 ]; then
        echo "ERROR: 'run' requires a subcommand (e.g., 'run snapshot-exists')" >&2
        exit 2
    fi

    case "$1" in
        auto-restore|snapshot-exists|healthcheck) ;;
        *)
            echo "ERROR: 'run' only supports: auto-restore, snapshot-exists, healthcheck" >&2
            echo "Received: $1" >&2
            exit 2
            ;;
    esac

    echo "Running in RUN mode: $*"
    echo ""
    prepare_bundle
    cd "${ARCHIVER_DIR}"
    exec "${ARCHIVER_DIR}/archiver.sh" "$@"
fi

prepare_bundle

# Start log tailer in background to forward logs to stdout
# This allows 'docker logs -f' to work
# Use tail -F to follow the symlink through log rotations
if [ -d "${LOG_DIR}" ]; then
    (
        while [ ! -f "${LOG_FILE}" ]; do
            sleep 1
        done

        if [ -f "${LOGO_DIR}/logo.ascii" ]; then
            cat "${LOGO_DIR}/logo.ascii"
            echo ""
        fi
        echo "--- Archiver Logs ---"
        # -F follows by name (handles log rotation), -n 0 shows only new lines from now
        tail -F -n 0 "${LOG_FILE}" 2>/dev/null
    ) &
    LOG_TAILER_PID=$!
fi

if [ -n "$CRON_SCHEDULE" ]; then
    echo "Setting up cron with schedule: $CRON_SCHEDULE"

    cat > /etc/cron.d/archiver << EOF
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
TZ=${TZ:-UTC}
$CRON_SCHEDULE ${ARCHIVER_DIR}/archiver.sh start >> /proc/1/fd/1 2>&1
EOF
    chmod 0644 /etc/cron.d/archiver

    crontab /etc/cron.d/archiver

    echo "Cron configured. Backups will run on schedule: $CRON_SCHEDULE"
    echo "Starting cron daemon..."

    cron -f &
    MAIN_PID=$!
    wait $MAIN_PID
else
    echo "No CRON_SCHEDULE set. Container will wait for manual commands."
    echo "Use 'docker exec <container> archiver start' to run backups manually"
    echo ""
    echo "Container is ready and will stay running."

    # Keep container alive indefinitely
    # This allows users to exec in and run commands manually
    tail -f /dev/null &
    MAIN_PID=$!
    wait $MAIN_PID
fi
