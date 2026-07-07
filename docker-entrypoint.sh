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

  # Kill supercronic or tail process if running
  if [ -n "$MAIN_PID" ] && kill -0 "$MAIN_PID" 2>/dev/null; then
    kill "$MAIN_PID" 2>/dev/null || true
  fi

  exit 0
}

trap 'handle_shutdown' SIGTERM

# Copy a provided key file into its canonical KEYS_DIR path with the right mode. A missing
# source is a no-op (bundle mode with no mounted keys, or an unused optional SSH key).
place_key_file() {
    local src="$1" dst="$2" mode="$3"
    [ -f "$src" ] || return 0
    cp "$src" "$dst"
    chmod "$mode" "$dst"
}

# Overlay any mounted RSA/SSH key files onto KEYS_DIR. Mounted files win over bundle-extracted
# keys, so a deployment can move just its keys to secrets while the rest still comes from a
# bundle. Paths default under SECRETS_DIR; each is overridable via its <NAME>_FILE env var.
overlay_key_files() {
    mkdir -p "${KEYS_DIR}"
    place_key_file "${RSA_PRIVATE_KEY_FILE:-${SECRETS_DIR}/rsa_private_key}" "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" 600
    place_key_file "${RSA_PUBLIC_KEY_FILE:-${SECRETS_DIR}/rsa_public_key}"   "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" 644
    place_key_file "${SSH_PRIVATE_KEY_FILE:-${SECRETS_DIR}/ssh_private_key}" "${DUPLICACY_SSH_PRIVATE_KEY_FILE}" 600
}

import_bundle() {
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
}

# Prepare configuration + keys from whichever source is present. An encrypted bundle
# (BUNDLE_PASSWORD + a mounted bundle.tar.enc) is the optional baseline; env vars plus
# file-based secrets are the override layer, resolved later by config-loader. Keys are
# files, so they are materialized into KEYS_DIR here regardless of mode.
prepare_config() {
    local have_bundle=0
    if [ -n "$BUNDLE_PASSWORD" ] && [ -f "$BUNDLE_FILE" ]; then
        have_bundle=1
        import_bundle
    fi

    overlay_key_files

    if [ "$have_bundle" -eq 1 ]; then
        if [ ! -f "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" ]; then
            echo "ERROR: RSA keys not found after import"
            exit 1
        fi
        echo "Configuration imported successfully"
        return 0
    fi

    # Env-native mode: config comes from env + ${SECRETS_DIR} (validated at run time by
    # config-loader). Only the RSA keypair must already be present here, as files.
    if [ ! -f "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" ] || [ ! -f "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" ]; then
        echo "ERROR: no bundle and no RSA key files found." >&2
        echo "Provide an encrypted bundle (set BUNDLE_PASSWORD and mount it at $BUNDLE_FILE)," >&2
        echo "or mount the RSA keypair at ${SECRETS_DIR}/rsa_private_key and ${SECRETS_DIR}/rsa_public_key" >&2
        echo "(or point RSA_PRIVATE_KEY_FILE / RSA_PUBLIC_KEY_FILE at them)." >&2
        exit 1
    fi
    echo "Env-native configuration (no bundle): keys loaded from files."
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
        auto-restore|auto-restore-all|snapshot-exists|healthcheck|backup) ;;
        *)
            echo "ERROR: 'run' only supports: auto-restore, auto-restore-all, snapshot-exists, healthcheck, backup" >&2
            echo "Received: $1" >&2
            exit 2
            ;;
    esac

    echo "Running in RUN mode: $*"
    echo ""
    prepare_config
    cd "${ARCHIVER_DIR}"
    exec "${ARCHIVER_DIR}/archiver.sh" "$@"
fi

prepare_config

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
    echo "Setting up scheduler (supercronic) with schedule: $CRON_SCHEDULE"

    CRONTAB_FILE=/tmp/archiver.crontab
    echo "${CRON_SCHEDULE} ${ARCHIVER_DIR}/archiver.sh start" > "${CRONTAB_FILE}"

    # Fail fast on a malformed schedule instead of crash-looping the container.
    if ! supercronic -test "${CRONTAB_FILE}"; then
        echo "ERROR: CRON_SCHEDULE is invalid: '$CRON_SCHEDULE'"
        exit 1
    fi

    echo "Scheduler configured. Backups will run on schedule: $CRON_SCHEDULE"
    echo "Starting supercronic..."

    # Background (not exec) so the SIGTERM trap can still run 'archiver stop' and
    # tear down the log tailer. supercronic passes TZ through for schedule evaluation.
    supercronic -passthrough-logs "${CRONTAB_FILE}" &
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
