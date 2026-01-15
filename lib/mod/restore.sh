#!/bin/bash

set -e # Exit immediately if a command exits with a non-zero status

# Check if the script is run with sudo
if [ "$(id -u)" -ne 0 ]; then
  # Escalate privileges if not sudo
  export INVOKING_UID="$(id -u)"
  export INVOKING_GID="$(id -g)"
  exec sudo -E "$0" "$@"
fi

# Creating this function for requirements of sourced functions
log_message() {
  echo "$@"
}

handle_error() {
  log_message "Error: ${1}"
  exit 1
}

# Configuration Section
# ---------------------
# Archiver directory
ARCHIVER_DIR="/opt/archiver"

KEYS_DIR="${ARCHIVER_DIR}/keys"
DUPLICACY_RSA_PUBLIC_KEY_FILE="${KEYS_DIR}/public.pem" # Path to RSA public key file for Duplicacy
DUPLICACY_RSA_PRIVATE_KEY_FILE="${KEYS_DIR}/private.pem" # Path to RSA private key file for Duplicacy
DUPLICACY_SSH_PUBLIC_KEY_FILE="${KEYS_DIR}/id_ed25519.pub" # Path to SSH public key file for Duplicacy
DUPLICACY_SSH_PRIVATE_KEY_FILE="${KEYS_DIR}/id_ed25519" # Path to SSH private key file for Duplicacy

# Check if duplicacy is available and exit if not
if ! command -v duplicacy &> /dev/null; then
  handle_error "Unable to find the Duplicacy binary in the PATH. This script requires the Duplicacy binary to function. Please install it before running this script."
fi

# Check if config.sh file is available, and exit if not
if [ ! -f "${ARCHIVER_DIR}/config.sh" ]; then
  handle_error "Unable to find your config.sh file. This script requires your backed up config.sh file from the archiver directory. Please restore it before running this script."
fi

if [ ! -f "${KEYS_DIR}/private.pem" ] || [ ! -f "${KEYS_DIR}/public.pem" ]; then
  handle_error "Unable to find your RSA key files. This script requires your backed up RSA private.pem and public.pem files from the keys directory. Please restore those before running this script."
fi

# ---------------------
# Configuration Check
# ---------------------
source "${ARCHIVER_DIR}/lib/src/set-config.sh"
# imports functions:
#   - verify_config
#   - expand_service_directories
#   - count_storage_targets
#   - verify_target_settings
#   - check_required_secrets
#   - check_notification_config
#   - check_backup_rotation_settings
# exports variables:
#   - STORAGE_TARGET_COUNT
#   - others I haven't documented yet
count_storage_targets
verify_target_settings
check_required_secrets

# Global variables to store the selected storage target info
SELECTED_STORAGE_TARGET_ID=""
SELECTED_STORAGE_TARGET_NAME=""
SELECTED_STORAGE_TARGET_TYPE=""
SNAPSHOT_ID=""
LOCAL_DIR=""
REVISION=""

# Function to list and prompt user for storage target selection
select_storage_target() {
  local storage_targets=()
    
  # Read all storage targets
  for i in $(seq 1 "${STORAGE_TARGET_COUNT}"); do
    local storage_id="${i}"
    local storage_name_var="STORAGE_TARGET_${storage_id}_NAME"
    local storage_name="${!storage_name_var}"
    
    storage_targets+=("${storage_id}) ${storage_name}")
  done

  # Display the storage targets to the user
  echo "Which of the following storage targets would you like to restore from?"
  for target in "${storage_targets[@]}"; do
    echo " - ${target}"
  done

  # Prompt user for selection
  local choice
  while true; do
    read -rp "Enter the number of your choice: " choice
    if [[ "${choice}" =~ ^[0-9]+$ ]] && [ "${choice}" -ge 1 ] && [ "${choice}" -le "${STORAGE_TARGET_COUNT}" ]; then
      break
    else
      echo "Invalid choice. Please enter a valid number between 1 and ${STORAGE_TARGET_COUNT}."
    fi
  done
    
  # Store the selected storage target ID in a global variable
  SELECTED_STORAGE_TARGET_ID="${choice}"
  local selected_storage_name_var="STORAGE_TARGET_${choice}_NAME"
  SELECTED_STORAGE_TARGET_NAME="${!selected_storage_name_var}"
  local selected_storage_type_var="STORAGE_TARGET_${choice}_TYPE"
  SELECTED_STORAGE_TARGET_TYPE="${!selected_storage_type_var}"

  if [[ "${SELECTED_STORAGE_TARGET_TYPE}" == "sftp" ]]; then
    if [ ! -f "${DUPLICACY_SSH_PRIVATE_KEY_FILE}" ] || [ ! -f "${DUPLICACY_SSH_PUBLIC_KEY_FILE}" ]; then
      handle_error "Unable to find your SSH key files. This script requires your backed up SSH id_ed25519 and id_ed25519.pub files from the keys directory to restore from an SFTP storage target. Please restore those before running this script."
    fi
  fi
}

local_restore_selections() {
  echo    # Move to a new line
  while [ -z "${SNAPSHOT_ID}" ]; do
    echo    # Move to a new line
    read -rp "Snapshot ID to restore (required, list of available snapshot IDs can be found in the target storage 'snapshots' directory): " SNAPSHOT_ID
    if [ -z "${SNAPSHOT_ID}" ]; then
      echo "Error: Snapshot ID is required."
    fi
  done
  echo "Chosen Snapshot ID: ${SNAPSHOT_ID}"

  while [ -z "${LOCAL_DIR}" ]; do
    echo    # Move to a new line
    read -rp "Local directory path to restore to (required): " LOCAL_DIR
    if [ -z "${LOCAL_DIR}" ]; then
      echo "Error: Local directory path is required."
    fi
  done
  echo "Chosen local directory path: ${LOCAL_DIR}"
}

initialize_duplicacy() {
  if [ ! -d "${LOCAL_DIR}" ]; then
    mkdir -p -m 0755 "${LOCAL_DIR}"
    # If variables available, use them for ownership
    if [ -n "${INVOKING_UID}" ] && [ -n "${INVOKING_GID}" ]; then
      chown "${INVOKING_UID}":"${INVOKING_GID}" "${LOCAL_DIR}"
    fi
  fi
  
  cd "${LOCAL_DIR}" || handle_error "Failed to change directory to '${LOCAL_DIR}'."

  local storage_id
  local storage_name
  local storage_name_upper
  local duplicacy_storage_password_var

  storage_id="${SELECTED_STORAGE_TARGET_ID}"
  storage_name="${SELECTED_STORAGE_TARGET_NAME}"
  storage_name_upper="$(echo "${storage_name}" | tr '[:lower:]' '[:upper:]')"
  duplicacy_storage_password_var="DUPLICACY_${storage_name_upper}_PASSWORD"

  export "${duplicacy_storage_password_var}"="${STORAGE_PASSWORD}" # Export Duplicacy storage password so Duplicacy binary can see variable

  if [[ "${SELECTED_STORAGE_TARGET_TYPE}" == "local" ]]; then
    local config_local_path_var

    config_local_path_var="STORAGE_TARGET_${storage_id}_LOCAL_PATH"

    duplicacy init -e -key "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" \
      -storage-name "${storage_name}" "${SNAPSHOT_ID}" \
      "${!config_local_path_var}" || \
      handle_error "Duplicacy Local Storage initialization failed."

  elif [[ "${SELECTED_STORAGE_TARGET_TYPE}" == "sftp" ]]; then
    local config_sftp_url_var
    local config_sftp_port_var
    local config_sftp_user_var
    local config_sftp_path_var
    local config_sftp_key_file_var
    local duplicacy_ssh_key_file_var

    config_sftp_url_var="STORAGE_TARGET_${storage_id}_SFTP_URL"
    config_sftp_port_var="STORAGE_TARGET_${storage_id}_SFTP_PORT"
    config_sftp_user_var="STORAGE_TARGET_${storage_id}_SFTP_USER"
    config_sftp_path_var="STORAGE_TARGET_${storage_id}_SFTP_PATH"
    config_sftp_key_file_var="STORAGE_TARGET_${storage_id}_SFTP_KEY_FILE"
    duplicacy_ssh_key_file_var="DUPLICACY_${storage_name_upper}_SSH_KEY_FILE"

    export "${duplicacy_ssh_key_file_var}"="${!config_sftp_key_file_var}" # Export Duplicacy storage SSH key file so Duplicacy binary can see variable

    duplicacy init -e -key "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" \
      -storage-name "${storage_name}" "${SNAPSHOT_ID}" \
      "sftp://${!config_sftp_user_var}@${!config_sftp_url_var}:${!config_sftp_port_var}//${!config_sftp_path_var}" || \
      handle_error "Duplicacy SFTP Storage initialization failed."
    
    duplicacy set -storage "${storage_name}" -key ssh_key_file -value "${!config_sftp_key_file_var}" || \
      handle_error "Setting the Duplicacy SFTP key file failed."

  elif [[ "${SELECTED_STORAGE_TARGET_TYPE}" == "b2" ]]; then
    local config_b2_bucketname_var
    local config_b2_id_var
    local config_b2_key_var
    local duplicacy_b2_id_var
    local duplicacy_b2_key_var

    config_b2_bucketname_var="STORAGE_TARGET_${storage_id}_B2_BUCKETNAME"
    config_b2_id_var="STORAGE_TARGET_${storage_id}_B2_ID"
    config_b2_key_var="STORAGE_TARGET_${storage_id}_B2_KEY"
    duplicacy_b2_id_var="DUPLICACY_${storage_name_upper}_B2_ID"
    duplicacy_b2_key_var="DUPLICACY_${storage_name_upper}_B2_KEY"

    export "${duplicacy_b2_id_var}"="${!config_b2_id_var}" # Export BackBlaze Key ID for backblaze storage so Duplicacy binary can see variable
    export "${duplicacy_b2_key_var}"="${!config_b2_key_var}" # Export BackBlaze Application Key for backblaze storage so Duplicacy binary can see variable

    duplicacy init -e -key "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" \
      -storage-name "${storage_name}" "${SNAPSHOT_ID}" \
      "b2://${!config_b2_bucketname_var}" || \
      handle_error "Duplicacy B2 Storage initialization failed."

    duplicacy set -storage "${storage_name}" -key b2_id -value "${!config_b2_id_var}" || \
      handle_error "Setting the Duplicacy B2 keyID failed."

    duplicacy set -storage "${storage_name}" -key b2_key -value "${!config_b2_key_var}" || \
      handle_error "Setting the Duplicacy B2 applicationKey failed."

  elif [[ "${SELECTED_STORAGE_TARGET_TYPE}" == "s3" ]]; then
    local config_s3_bucketname_var
    local config_s3_endpoint_var
    local config_s3_region_var
    local config_s3_id_var
    local config_s3_secret_var
    local duplicacy_s3_id_var
    local duplicacy_s3_secret_var
    local s3_region

    config_s3_bucketname_var="STORAGE_TARGET_${storage_id}_S3_BUCKETNAME"
    config_s3_endpoint_var="STORAGE_TARGET_${storage_id}_S3_ENDPOINT"
    config_s3_region_var="STORAGE_TARGET_${storage_id}_S3_REGION"
    config_s3_id_var="STORAGE_TARGET_${storage_id}_S3_ID"
    config_s3_secret_var="STORAGE_TARGET_${storage_id}_S3_SECRET"
    duplicacy_s3_id_var="DUPLICACY_${storage_name_upper}_S3_ID"
    duplicacy_s3_secret_var="DUPLICACY_${storage_name_upper}_S3_SECRET"

    s3_region="${!config_s3_region_var}"
    s3_region="${s3_region:-none}"

    export "${duplicacy_s3_id_var}"="${!config_s3_id_var}" # Export S3 ID so Duplicacy binary can see variable
    export "${duplicacy_s3_secret_var}"="${!config_s3_secret_var}" # Export S3 Secret so Duplicacy binary can see variable

    duplicacy init -e -key "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" \
      -storage-name "${storage_name}" "${SNAPSHOT_ID}" \
      "s3://${s3_region}@${!config_s3_endpoint_var}/${!config_s3_bucketname_var}" || \
      handle_error "Duplicacy S3 Storage initialization failed."

    duplicacy set -storage "${storage_name}" -key s3_id -value "${!config_s3_id_var}" || \
      handle_error "Setting the Duplicacy S3 ID failed."

    duplicacy set -storage "${storage_name}" -key s3_secret -value "${!config_s3_secret_var}" || \
      handle_error "Setting the Duplicacy S3 Secret failed."

  else
    handle_error "'${SELECTED_STORAGE_TARGET_TYPE}' is not a supported backup type. Please edit config.sh to only reference supported backup types."

  fi

  duplicacy set -storage "${storage_name}" -key password -value "${STORAGE_PASSWORD}" || \
    handle_error "Setting the Duplicacy storage password failed."

  duplicacy set -storage "${storage_name}" -key rsa_passphrase -value "${RSA_PASSPHRASE}" || \
    handle_error "Setting the Duplicacy storage RSA Passphrase failed."
}

choose_revision() {
  echo
  duplicacy list #this should give you the info for revision number, needed below
  echo

  while [ -z "${REVISION}" ]; do
    echo    # Move to a new line
    read -rp "Choose a revision number to restore (required): " REVISION
    if [ -z "${REVISION}" ]; then
      echo "Error: Revision number is required."
    fi
  done
  echo "Chosen revision: ${REVISION}"
}

configure_restore_options() {
  # Ask if user wants to customize restore options
  echo    # Move to a new line
  read -p "Customize restore options (advanced)? (y/N): " -n 1 -r
  echo    # Move to a new line

  RESTORE_FLAGS=""
  RESTORE_THREADS="${DUPLICACY_THREADS}"

  if [[ $REPLY =~ ^[Yy]$ ]]; then
    # Ask about hash-based detection
    read -p "Detect file differences by hash (slower but more thorough)? (y/N): " -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      RESTORE_FLAGS="${RESTORE_FLAGS} -hash"
    fi

    # Ask about overwriting files
    read -p "Overwrite existing files in the restore directory? (y/N): " -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      RESTORE_FLAGS="${RESTORE_FLAGS} -overwrite"
    fi

    # Ask about deleting extra files
    read -p "Delete files not in the snapshot? (WARNING: Removes extra files) (y/N): " -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      RESTORE_FLAGS="${RESTORE_FLAGS} -delete"
    fi

    # Ask about ignoring ownership
    read -p "Ignore original file ownership (useful when restoring to different machine)? (y/N): " -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      RESTORE_FLAGS="${RESTORE_FLAGS} -ignore-owner"
    fi

    # Ask about persistence on errors
    read -p "Continue even if errors occur? (y/N): " -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      RESTORE_FLAGS="${RESTORE_FLAGS} -persist"
    fi

    # Ask about thread count
    read -p "Override download thread count (current: ${DUPLICACY_THREADS})? (y/N): " -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      read -rp "Enter thread count: " RESTORE_THREADS
      if [ -z "${RESTORE_THREADS}" ]; then
        RESTORE_THREADS="${DUPLICACY_THREADS}"
      fi
    fi
  fi

  if [ -n "${RESTORE_FLAGS}" ]; then
    echo "Additional restore flags:${RESTORE_FLAGS}"
  fi
  if [ "${RESTORE_THREADS}" != "${DUPLICACY_THREADS}" ]; then
    echo "Download threads: ${RESTORE_THREADS}"
  fi
}

restore_repository() {
  duplicacy restore -r "${REVISION}" -key "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" -stats -threads "${RESTORE_THREADS}" "${RESTORE_FLAGS}"
  # Fix ownership of restored files to match the invoking user
  if [ -n "${INVOKING_UID}" ] && [ -n "${INVOKING_GID}" ]; then
    chown -R "${INVOKING_UID}":"${INVOKING_GID}" "${LOCAL_DIR}"
  fi
  echo "Repository restored."
}

service_specific_restore_script() {
  if [ -f restore-service.sh ]; then
    echo    # Move to a new line
    read -p "Found a restore-service.sh file. Would you like to run it now? (y|N):" -n 1 -r
    echo    # Move to a new line
    if [[ $REPLY =~ ^[Yy]$ ]]; then
      echo "Now running restore-service.sh..."
      bash restore-service.sh
    else
      echo "Did not run restore-service.sh script."
    fi
  fi
}

main() {
  select_storage_target
  local_restore_selections
  initialize_duplicacy
  choose_revision
  configure_restore_options
  restore_repository
  service_specific_restore_script
}

main
