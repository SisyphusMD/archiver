#!/bin/bash
# Common paths, constants, and utility functions

COMMON_SH_SOURCED=true

# Prevents double-sourcing of scripts using guard variables
source_if_not_sourced() {
  local script_path="${1}"
  local script_name
  local guard_var_name

  script_name="$(basename "${script_path}")"
  guard_var_name="$(echo "${script_name}" | tr '[:lower:].-' '[:upper:]__')_SOURCED"

  if [[ -z "${!guard_var_name}" ]]; then
    source "${script_path}"
  fi
}

# Paths
ARCHIVER_DIR="/opt/archiver"
LIB_DIR="${ARCHIVER_DIR}/lib"
CORE_DIR="${LIB_DIR}/core"
FEATURES_DIR="${LIB_DIR}/features"
SCRIPTS_DIR="${LIB_DIR}/scripts"
LOGO_DIR="${LIB_DIR}/logos"
LOG_DIR="${ARCHIVER_DIR}/logs"
OLD_LOG_DIR="${LOG_DIR}/prior_logs"
KEYS_DIR="${ARCHIVER_DIR}/keys"
BUNDLE_DIR="${ARCHIVER_DIR}/bundle"
# init writes its generated materials (env-native/ + the bundle) here — a neutral
# output path, so the primary flow is not branded by the transitional bundle mount.
SETUP_DIR="${ARCHIVER_DIR}/setup"
# Conventional mount point for the user's deployment manifests (compose.yaml, .nix files,
# k8s YAML — whatever the runtime uses). Anything mounted here is captured verbatim into
# the recovery kit, so the kit can recreate the deployment byte-exact.
DEPLOYMENT_DIR="${ARCHIVER_DIR}/deployment"

CONFIG_FILE="${ARCHIVER_DIR}/config.sh"
# Directory holding file-based secrets (Docker/k8s convention). Each secret is one file;
# a per-secret <NAME>_FILE env var can point elsewhere. Overridable for tests.
SECRETS_DIR="${SECRETS_DIR:-/run/secrets}"

# Backup pipeline lock (path kept from the single-pipeline era for compatibility).
LOCKFILE="/var/lock/archiver-main.lock"
STOP_FLAG="/var/lock/archiver-stop-requested"
# Maintenance pipeline lock: check+prune runs independently of backups.
MAINTENANCE_LOCKFILE="/var/lock/archiver-maintenance.lock"
MAINTENANCE_STOP_FLAG="/var/lock/archiver-maintenance-stop-requested"
# Per-storage locks serialize the two pipelines on one storage (copy vs check/prune).
STORAGE_LOCK_PREFIX="/var/lock/archiver-storage-"

DUPLICACY_RSA_PUBLIC_KEY_FILE="${KEYS_DIR}/public.pem"
DUPLICACY_RSA_PRIVATE_KEY_FILE="${KEYS_DIR}/private.pem"
DUPLICACY_SSH_PUBLIC_KEY_FILE="${KEYS_DIR}/id_ed25519.pub"
DUPLICACY_SSH_PRIVATE_KEY_FILE="${KEYS_DIR}/id_ed25519"

CONFIG_LOADER_CORE="${CORE_DIR}/config-loader.sh"
CONFIG_SERIALIZE_CORE="${CORE_DIR}/config-serialize.sh"
ERROR_CORE="${CORE_DIR}/error.sh"
LOCKFILE_CORE="${CORE_DIR}/lockfile.sh"
LOGGING_CORE="${CORE_DIR}/logging.sh"
REQUIRE_CONTAINER_CORE="${CORE_DIR}/require-container.sh"

DUPLICACY_BACKUP_FEATURE="${FEATURES_DIR}/duplicacy-backup.sh"
DUPLICACY_MAINTENANCE_FEATURE="${FEATURES_DIR}/duplicacy-maintenance.sh"
DUPLICACY_RESTORE_FEATURE="${FEATURES_DIR}/duplicacy-restore.sh"
NOTIFICATION_FEATURE="${FEATURES_DIR}/notification.sh"
RECOVERY_KIT_FEATURE="${FEATURES_DIR}/recovery-kit.sh"

AUTO_RESTORE_SCRIPT="${SCRIPTS_DIR}/auto-restore.sh"
AUTO_RESTORE_ALL_SCRIPT="${SCRIPTS_DIR}/auto-restore-all.sh"
BUNDLE_EXPORT_SCRIPT="${SCRIPTS_DIR}/bundle-export.sh"
BUNDLE_IMPORT_SCRIPT="${SCRIPTS_DIR}/bundle-import.sh"
RECOVERY_KIT_SCRIPT="${SCRIPTS_DIR}/recovery-kit.sh"
HEALTHCHECK_SCRIPT="${SCRIPTS_DIR}/healthcheck.sh"
INIT_SCRIPT="${SCRIPTS_DIR}/init.sh"
LOGS_SCRIPT="${SCRIPTS_DIR}/logs.sh"
MAIN_SCRIPT="${SCRIPTS_DIR}/main.sh"
MAINTENANCE_SCRIPT="${SCRIPTS_DIR}/maintenance.sh"
MIGRATE_SCRIPT="${SCRIPTS_DIR}/migrate.sh"
PAUSE_SCRIPT="${SCRIPTS_DIR}/pause.sh"
RESTORE_SCRIPT="${SCRIPTS_DIR}/restore.sh"
RESUME_SCRIPT="${SCRIPTS_DIR}/resume.sh"
SNAPSHOT_EXISTS_SCRIPT="${SCRIPTS_DIR}/snapshot-exists.sh"
STATUS_SCRIPT="${SCRIPTS_DIR}/status.sh"
STOP_SCRIPT="${SCRIPTS_DIR}/stop.sh"

