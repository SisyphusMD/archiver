#!/bin/bash
set -e

source "/opt/archiver/lib/core/common.sh"

LOG_FILE="${LOG_DIR}/archiver.log"

handle_shutdown() {
  echo "Received shutdown signal, attempting graceful stop..."

  archiver stop 2>&1 || true

  # During a service backup 'archiver stop' only sets the stop flag; the pipeline records
  # the stop and releases its lock itself. Exiting before that happens tears down the PID
  # namespace and SIGKILLs that cleanup mid-flight, so wait for both pipeline locks to
  # clear (bounded well under the documented stop_grace_period of 2m).
  for _ in $(seq 1 100); do
    [ ! -e "${LOCKFILE}" ] && [ ! -e "${MAINTENANCE_LOCKFILE}" ] && break
    sleep 1
  done

  for tailer_pid in "$LOG_TAILER_PID" "$MAINT_TAILER_PID" "${COPIES_TAILER_PID:-}"; do
    if [ -n "$tailer_pid" ] && kill -0 "$tailer_pid" 2>/dev/null; then
      kill "$tailer_pid" 2>/dev/null || true
    fi
  done

  # Stop the scheduler or the idle tail
  if [ -n "$MAIN_PID" ] && kill -0 "$MAIN_PID" 2>/dev/null; then
    kill "$MAIN_PID" 2>/dev/null || true
  fi

  exit 0
}

trap 'handle_shutdown' SIGTERM

# Copy a provided key file into its canonical KEYS_DIR path with the right mode. A missing
# source is a no-op (an unused optional SSH key).
place_key_file() {
    local src="$1" dst="$2" mode="$3"
    [ -f "$src" ] || return 0
    cp "$src" "$dst"
    chmod "$mode" "$dst"
}

# Place the mounted RSA/SSH key files in KEYS_DIR. Paths default under SECRETS_DIR; each is
# overridable via its <NAME>_FILE env var.
overlay_key_files() {
    mkdir -p "${KEYS_DIR}"
    place_key_file "${RSA_PRIVATE_KEY_FILE:-${SECRETS_DIR}/rsa_private_key}" "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" 600
    place_key_file "${RSA_PUBLIC_KEY_FILE:-${SECRETS_DIR}/rsa_public_key}"   "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" 644
    place_key_file "${SSH_PRIVATE_KEY_FILE:-${SECRETS_DIR}/ssh_private_key}" "${DUPLICACY_SSH_PRIVATE_KEY_FILE}" 600
    # The SFTP restore path requires the public half too (duplicacy-restore checks both).
    place_key_file "${SSH_PUBLIC_KEY_FILE:-${SECRETS_DIR}/ssh_public_key}"   "${DUPLICACY_SSH_PUBLIC_KEY_FILE}" 644
}

# A deployment still configured by a bundle must convert first (ADRs 4, 22): starting without
# it would back up nothing, or with half a configuration. Anything bundle-era refuses.
refuse_bundle() {
    local found=""
    [ -e "${BUNDLE_DIR}/bundle.tar.enc" ] && found="${BUNDLE_DIR}/bundle.tar.enc"
    [ -e "${CONFIG_FILE}" ] && found="${CONFIG_FILE}"
    [ -e "${BUNDLE_PASSWORD_FILE:-${SECRETS_DIR}/bundle_password}" ] && found="${BUNDLE_PASSWORD_FILE:-${SECRETS_DIR}/bundle_password}"
    [ -n "${BUNDLE_PASSWORD:-}" ] && found="BUNDLE_PASSWORD in the environment"
    [ -z "${found}" ] && return 0
    echo "ERROR: found ${found}." >&2
    print_bundle_migration_help
    exit 1
}

# Keys come from files; the rest of the configuration is validated at run time by
# config-loader. Only the RSA keypair must be present here.
prepare_config() {
    overlay_key_files
    if [ ! -f "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" ] || [ ! -f "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" ]; then
        echo "ERROR: no RSA key files found." >&2
        echo "Mount the RSA keypair at ${SECRETS_DIR}/rsa_private_key and ${SECRETS_DIR}/rsa_public_key" >&2
        echo "(or point RSA_PRIVATE_KEY_FILE / RSA_PUBLIC_KEY_FILE at them)." >&2
        exit 1
    fi
    echo "Configuration: keys loaded from files."
}

echo "==================================="
echo "Archiver Container Starting"
echo "==================================="

if [ "$1" = "init" ]; then
    echo "Running in INIT mode"
    echo ""

    if [ -e "${SETUP_DIR}/env-native" ]; then
        echo "WARNING: ${SETUP_DIR}/env-native already exists"
        echo "Continuing will overwrite it."
        echo ""
    fi

    mkdir -p "${SETUP_DIR}"

    cd "${ARCHIVER_DIR}"
    exec archiver init
fi

if [ "$1" = "run" ]; then
    shift
    if [ $# -eq 0 ]; then
        echo "ERROR: 'run' requires a subcommand (e.g., 'run snapshot-exists')" >&2
        exit 2
    fi

    case "$1" in
        auto-restore|auto-restore-all|snapshot-exists|healthcheck|backup|maintenance) ;;
        *)
            echo "ERROR: 'run' only supports: auto-restore, auto-restore-all, snapshot-exists, healthcheck, backup, maintenance" >&2
            echo "Received: $1" >&2
            exit 2
            ;;
    esac

    echo "Running in RUN mode: $*"
    echo ""
    refuse_bundle
    prepare_config
    cd "${ARCHIVER_DIR}"
    exec archiver "$@"
fi

refuse_bundle
prepare_config

# Clear any lock/stop-flag state left by a prior container. These live under /var/lock (not
# a volume/tmpfs), so they survive 'docker restart'; a fresh PID namespace cannot host a live
# prior-boot holder, so removing them is safe and prevents (a) a recycled PID faking a live
# lock — every scheduled backup then refusing "already running" — and (b) a leftover stop flag
# silently aborting the first backup. 2>/dev/null swallows the no-match glob.
rm -f "${LOCKFILE}" "${STOP_FLAG}" "${MAINTENANCE_LOCKFILE}" "${MAINTENANCE_STOP_FLAG}" \
      "${LOCKFILE}.tmp" "${MAINTENANCE_LOCKFILE}.tmp" 2>/dev/null || true

# Forward a pipeline's log file to stdout so 'docker logs -f' works. tail -F follows the
# symlink by name through rotations; the wait loop idles harmlessly if the pipeline never
# runs (e.g. maintenance not scheduled and never invoked).
start_log_tailer() {
    local file="$1" banner="$2"
    (
        while [ ! -f "${file}" ]; do
            sleep 1
        done
        echo "--- ${banner} ---"
        tail -F -n 0 "${file}" 2>/dev/null
    ) &
}

if [ -d "${LOG_DIR}" ]; then
    if [ -f "${LOGO_DIR}/logo.ascii" ]; then
        cat "${LOGO_DIR}/logo.ascii"
        echo ""
    fi
    start_log_tailer "${LOG_FILE}" "Archiver Logs"
    LOG_TAILER_PID=$!
    start_log_tailer "${LOG_DIR}/maintenance.log" "Maintenance Logs"
    MAINT_TAILER_PID=$!
    start_log_tailer "${LOG_DIR}/copies.log" "Copy Logs"
    COPIES_TAILER_PID=$!
fi

# CRON_SCHEDULE was renamed. Refusing to start beats silently ignoring it — an ignored
# schedule rename would mean no scheduled backups and nobody noticing.
if [ -n "${CRON_SCHEDULE:-}" ]; then
    echo "ERROR: CRON_SCHEDULE was renamed to BACKUP_SCHEDULE. Rename the environment variable and restart." >&2
    exit 1
fi
if [ -n "${ROTATE_BACKUPS:-}" ]; then
    echo "WARNING: ROTATE_BACKUPS is deprecated; rename it to PRUNE_BACKUPS (still honored for now)."
fi

# Check/prune only run on MAINTENANCE_SCHEDULE (or a manual 'archiver maintenance').
# A scheduled deployment without it would back up forever and never enforce retention
# or verify storages — say so once, loudly, at startup.
if [ -n "${BACKUP_SCHEDULE:-}" ] && [ -z "${MAINTENANCE_SCHEDULE:-}" ]; then
    echo "WARNING: MAINTENANCE_SCHEDULE is not set: storage check and prune will never run automatically."
    echo "         Set MAINTENANCE_SCHEDULE (e.g. \"0 13 * * *\") or run 'archiver maintenance' yourself."
fi

if [ -n "${BACKUP_SCHEDULE:-}" ] || [ -n "${MAINTENANCE_SCHEDULE:-}" ]; then
    if [ -n "${BACKUP_SCHEDULE:-}" ]; then
        echo "Backups scheduled: ${BACKUP_SCHEDULE}"
    fi
    if [ -n "${MAINTENANCE_SCHEDULE:-}" ]; then
        echo "Maintenance scheduled: ${MAINTENANCE_SCHEDULE}"
    fi

    # Fail fast on a malformed schedule instead of crash-looping the container.
    if ! archiver daemon --check; then
        echo "ERROR: BACKUP_SCHEDULE or MAINTENANCE_SCHEDULE is invalid."
        exit 1
    fi

    echo "Starting scheduler..."

    # Background (not exec) so the SIGTERM trap can still run 'archiver stop' and
    # tear down the log tailers. Schedules are evaluated in the container's TZ.
    archiver daemon &
    MAIN_PID=$!
    wait $MAIN_PID
else
    echo "No BACKUP_SCHEDULE set. Container will wait for manual commands."
    echo "Use 'docker exec <container> archiver backup' to run backups manually ('archiver backup --detach' to background)"
    echo ""
    echo "Container is ready and will stay running."

    # Keep container alive indefinitely
    # This allows users to exec in and run commands manually
    tail -f /dev/null &
    MAIN_PID=$!
    wait $MAIN_PID
fi
