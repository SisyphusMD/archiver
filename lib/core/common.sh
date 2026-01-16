#!/bin/bash
# Common paths and constants

# Guard to prevent circular sourcing
COMMON_SH_SOURCED=true

# Function to safely source scripts only once
source_if_not_sourced() {
  local script_path="${1}"
  local script_name
  local guard_var_name

  # Extract script name and convert to guard variable name
  # e.g., "/opt/archiver/lib/core/logging.sh" -> "LOGGING_SH_SOURCED"
  script_name="$(basename "${script_path}")"
  guard_var_name="$(echo "${script_name}" | tr '[:lower:].-' '[:upper:]__')_SOURCED"

  # Check if already sourced
  if [[ -z "${!guard_var_name}" ]]; then
    source "${script_path}"
  fi
}

# Directory paths
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

# Configuration file
CONFIG_FILE="${ARCHIVER_DIR}/config.sh"

# Key file paths
DUPLICACY_RSA_PUBLIC_KEY_FILE="${KEYS_DIR}/public.pem"
DUPLICACY_RSA_PRIVATE_KEY_FILE="${KEYS_DIR}/private.pem"
DUPLICACY_SSH_PUBLIC_KEY_FILE="${KEYS_DIR}/id_ed25519.pub"
DUPLICACY_SSH_PRIVATE_KEY_FILE="${KEYS_DIR}/id_ed25519"

# Core paths
CONFIG_LOADER_CORE="${CORE_DIR}/config-loader.sh"
ERROR_CORE="${CORE_DIR}/error.sh"
LOCKFILE_CORE="${CORE_DIR}/lockfile.sh"
LOGGING_CORE="${CORE_DIR}/logging.sh"
REQUIRE_DOCKER_CORE="${CORE_DIR}/require-docker.sh"

# Feature paths
DUPLICACY_FEATURE="${FEATURES_DIR}/duplicacy.sh"
NOTIFICATION_FEATURE="${FEATURES_DIR}/notification.sh"

# Script paths
BUNDLE_EXPORT_SCRIPT="${SCRIPTS_DIR}/bundle-export.sh"
BUNDLE_IMPORT_SCRIPT="${SCRIPTS_DIR}/bundle-import.sh"
HEALTHCHECK_SCRIPT="${SCRIPTS_DIR}/healthcheck.sh"
INIT_SCRIPT="${SCRIPTS_DIR}/init.sh"
LOGS_SCRIPT="${SCRIPTS_DIR}/logs.sh"
MAIN_SCRIPT="${SCRIPTS_DIR}/main.sh"
PAUSE_SCRIPT="${SCRIPTS_DIR}/pause.sh"
RESTORE_SCRIPT="${SCRIPTS_DIR}/restore.sh"
RESUME_SCRIPT="${SCRIPTS_DIR}/resume.sh"
STATUS_SCRIPT="${SCRIPTS_DIR}/status.sh"
STOP_SCRIPT="${SCRIPTS_DIR}/stop.sh"
