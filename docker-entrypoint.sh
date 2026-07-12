#!/bin/bash
set -e

source "/opt/archiver/lib/core/common.sh"

BUNDLE_FILE="${BUNDLE_DIR}/bundle.tar.enc"
LOG_FILE="${LOG_DIR}/archiver.log"

handle_shutdown() {
  echo "Received shutdown signal, attempting graceful stop..."

  "${ARCHIVER_DIR}/archiver.sh" stop 2>&1 || true

  # During a service backup 'archiver stop' only sets the stop flag; the pipeline records
  # the stop and releases its lock itself. Exiting before that happens tears down the PID
  # namespace and SIGKILLs that cleanup mid-flight, so wait for both pipeline locks to
  # clear (bounded well under the documented stop_grace_period of 2m).
  for _ in $(seq 1 100); do
    [ ! -e "${LOCKFILE}" ] && [ ! -e "${MAINTENANCE_LOCKFILE}" ] && break
    sleep 1
  done

  for tailer_pid in "$LOG_TAILER_PID" "$MAINT_TAILER_PID"; do
    if [ -n "$tailer_pid" ] && kill -0 "$tailer_pid" 2>/dev/null; then
      kill "$tailer_pid" 2>/dev/null || true
    fi
  done

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
    # The SFTP restore path requires the public half too (duplicacy-restore checks both).
    place_key_file "${SSH_PUBLIC_KEY_FILE:-${SECRETS_DIR}/ssh_public_key}"   "${DUPLICACY_SSH_PUBLIC_KEY_FILE}" 644
}

# BUNDLE_PASSWORD decrypts the whole bundle (including the RSA private key), so it is the most
# sensitive secret and is read from a file, never the environment. A raw-env value is rejected
# with a migration message rather than silently ignored, which would otherwise drop a bundle
# deployment into env-native mode and fail later with a confusing "no bundle" error.
resolve_bundle_password() {
    if [ -n "${BUNDLE_PASSWORD:-}" ]; then
        echo "ERROR: BUNDLE_PASSWORD is no longer read from the environment (an env var leaks via 'docker inspect' and /proc)." >&2
        echo "Write the password to a file, mount it at ${SECRETS_DIR}/bundle_password (or set BUNDLE_PASSWORD_FILE to its path)," >&2
        echo "and remove BUNDLE_PASSWORD from the container environment." >&2
        exit 1
    fi
    # An explicitly set BUNDLE_PASSWORD_FILE pointing nowhere is a config error, not a
    # fallback — silently ignoring it would drop a bundle deployment into env-native mode.
    if [ -n "${BUNDLE_PASSWORD_FILE:-}" ] && [ ! -f "${BUNDLE_PASSWORD_FILE}" ]; then
        echo "ERROR: BUNDLE_PASSWORD_FILE is set to '${BUNDLE_PASSWORD_FILE}', but no such file exists." >&2
        exit 1
    fi
    local path="${BUNDLE_PASSWORD_FILE:-${SECRETS_DIR}/bundle_password}"
    if [ -f "${path}" ]; then
        BUNDLE_PASSWORD="$(<"${path}")"
        BUNDLE_PASSWORD="${BUNDLE_PASSWORD%$'\r'}"   # CRLF-edited file would fail as "wrong password"
    fi
}

import_bundle() {
    echo "Bundle file found: $BUNDLE_FILE"
    echo "Decrypting and importing configuration..."
    export ARCHIVER_BUNDLE_PASSWORD="$BUNDLE_PASSWORD"
    export ARCHIVER_BUNDLE_FILE="$BUNDLE_FILE"

    cd "${ARCHIVER_DIR}"
    if ! "${BUNDLE_IMPORT_SCRIPT}"; then
        echo "ERROR: Failed to import configuration"
        echo "Please verify the bundle password (at ${SECRETS_DIR}/bundle_password or BUNDLE_PASSWORD_FILE) is correct"
        exit 1
    fi

    # The password's job is done; keeping it exported would hand it to every child process
    # (supercronic, backups, user hooks) via /proc — the leak the file-only rule exists for.
    unset ARCHIVER_BUNDLE_PASSWORD ARCHIVER_BUNDLE_FILE BUNDLE_PASSWORD

    if [ ! -f "${CONFIG_FILE}" ]; then
        echo "ERROR: config.sh not found after import"
        exit 1
    fi
}

# Prepare configuration + keys from whichever source is present. An encrypted bundle
# (a mounted bundle.tar.enc + its password file) is the optional baseline; env vars plus
# file-based secrets are the override layer, resolved later by config-loader. Keys are
# files, so they are materialized into KEYS_DIR here regardless of mode.
prepare_config() {
    local have_bundle=0
    if [ -f "$BUNDLE_FILE" ] && [ -z "$BUNDLE_PASSWORD" ]; then
        # A mounted bundle with no resolvable password is the upgrade path every pre-0.9.0
        # deployment walks; falling through to env-native here would either start a container
        # that never backs up (keys mounted) or blame a missing bundle that plainly exists.
        echo "ERROR: bundle found at $BUNDLE_FILE, but no bundle password." >&2
        echo "Provide it at ${SECRETS_DIR}/bundle_password (or point BUNDLE_PASSWORD_FILE at it)." >&2
        exit 1
    fi
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
        echo "Provide an encrypted bundle (mount it at $BUNDLE_FILE with its password at ${SECRETS_DIR}/bundle_password)," >&2
        echo "or mount the RSA keypair at ${SECRETS_DIR}/rsa_private_key and ${SECRETS_DIR}/rsa_public_key" >&2
        echo "(or point RSA_PRIVATE_KEY_FILE / RSA_PUBLIC_KEY_FILE at them)." >&2
        exit 1
    fi
    echo "Env-native configuration (no bundle): keys loaded from files."
}

echo "==================================="
echo "Archiver Container Starting"
echo "==================================="

resolve_bundle_password

if [ "$1" = "init" ]; then
    echo "Running in INIT mode"
    echo ""

    if [ -f "${SETUP_DIR}/bundle.tar.enc" ]; then
        echo "WARNING: ${SETUP_DIR}/bundle.tar.enc already exists"
        echo "Continuing will overwrite it."
        echo ""
    fi

    mkdir -p "${SETUP_DIR}"

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
        auto-restore|auto-restore-all|snapshot-exists|healthcheck|backup|maintenance) ;;
        *)
            echo "ERROR: 'run' only supports: auto-restore, auto-restore-all, snapshot-exists, healthcheck, backup, maintenance" >&2
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
fi

# CRON_SCHEDULE was renamed. Refusing to start beats silently ignoring it — an ignored
# schedule rename would mean no scheduled backups and nobody noticing.
if [ -n "${CRON_SCHEDULE:-}" ]; then
    echo "ERROR: CRON_SCHEDULE was renamed to BACKUP_SCHEDULE. Rename the environment variable and restart." >&2
    exit 1
fi
if [ -n "${ROTATE_BACKUPS:-}" ]; then
    echo "WARNING: ROTATE_BACKUPS is deprecated; rename it to PRUNE_BACKUPS (still honored for old bundle configs)."
fi

# Check/prune only run on MAINTENANCE_SCHEDULE (or a manual 'archiver maintenance').
# A scheduled deployment without it would back up forever and never enforce retention
# or verify storages — say so once, loudly, at startup.
if [ -n "${BACKUP_SCHEDULE:-}" ] && [ -z "${MAINTENANCE_SCHEDULE:-}" ]; then
    echo "WARNING: MAINTENANCE_SCHEDULE is not set: storage check and prune will never run automatically."
    echo "         Set MAINTENANCE_SCHEDULE (e.g. \"0 13 * * *\") or run 'archiver maintenance' yourself."
fi

if [ -n "${BACKUP_SCHEDULE:-}" ] || [ -n "${MAINTENANCE_SCHEDULE:-}" ]; then
    CRONTAB_FILE=/tmp/archiver.crontab
    : > "${CRONTAB_FILE}"
    # Synchronous verbs: supercronic then knows each job's real duration and adds its own
    # skip-if-still-running protection on top of the pipeline locks.
    if [ -n "${BACKUP_SCHEDULE:-}" ]; then
        echo "${BACKUP_SCHEDULE} ${ARCHIVER_DIR}/archiver.sh backup" >> "${CRONTAB_FILE}"
        echo "Backups scheduled: ${BACKUP_SCHEDULE}"
    fi
    if [ -n "${MAINTENANCE_SCHEDULE:-}" ]; then
        echo "${MAINTENANCE_SCHEDULE} ${ARCHIVER_DIR}/archiver.sh maintenance" >> "${CRONTAB_FILE}"
        echo "Maintenance scheduled: ${MAINTENANCE_SCHEDULE}"
    fi

    # Fail fast on a malformed schedule instead of crash-looping the container.
    if ! supercronic -test "${CRONTAB_FILE}"; then
        echo "ERROR: BACKUP_SCHEDULE or MAINTENANCE_SCHEDULE is invalid."
        exit 1
    fi

    echo "Starting supercronic..."

    # Background (not exec) so the SIGTERM trap can still run 'archiver stop' and
    # tear down the log tailers. supercronic passes TZ through for schedule evaluation.
    supercronic -passthrough-logs "${CRONTAB_FILE}" &
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
