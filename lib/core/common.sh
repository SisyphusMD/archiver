#!/bin/bash
# Common paths and constants

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

# Script paths
MAIN_SCRIPT="${SCRIPTS_DIR}/main.sh"
LOGS_SCRIPT="${SCRIPTS_DIR}/logs.sh"
STOP_SCRIPT="${SCRIPTS_DIR}/stop.sh"
PAUSE_SCRIPT="${SCRIPTS_DIR}/pause.sh"
RESUME_SCRIPT="${SCRIPTS_DIR}/resume.sh"
STATUS_SCRIPT="${SCRIPTS_DIR}/status.sh"
RESTORE_SCRIPT="${SCRIPTS_DIR}/restore.sh"
BUNDLE_EXPORT_SCRIPT="${SCRIPTS_DIR}/bundle-export.sh"
BUNDLE_IMPORT_SCRIPT="${SCRIPTS_DIR}/bundle-import.sh"
HEALTHCHECK_SCRIPT="${SCRIPTS_DIR}/healthcheck.sh"
INIT_SCRIPT="${SCRIPTS_DIR}/init.sh"
