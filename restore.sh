#!/bin/bash

set -e # Exit immediately if a command exits with a non-zero status

# Ensure the script is run as root
if [ "$(id -u)" -ne 0 ]; then
  echo "This script must be run as root. Please use sudo or log in as the root user." 1>&2
  exit 1
fi

handle_error() {
  echo "Error: ${1}"
  exit 1
}

# Configuration Section
# ---------------------

# Determine the full path of the script
RESTORE_SCRIPT="$(readlink -f "${0}" 2>/dev/null)"
# Determine the full path of the containing dir of the script
ARCHIVER_DIR="$(cd "$(dirname "${RESTORE_SCRIPT}")" && pwd)"
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
source "${ARCHIVER_DIR}/utils/set-config.sh"
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
    local storage_type_var="STORAGE_TARGET_${storage_id}_TYPE"
    local storage_type="${!storage_type_var}"
    
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
    read -p "Enter the number of your choice: " choice
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
  mkdir -p -m 0755 "${LOCAL_DIR}"
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

  if [[ "${SELECTED_STORAGE_TARGET_TYPE}" == "sftp" ]]; then
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

restore_repository() {
  duplicacy restore -r "${REVISION}" -key "${DUPLICACY_RSA_PRIVATE_KEY_FILE}"
  echo "Repository restored."
}

service_specific_restore_script() {
  if [ -f restore-service.sh ]; then
    echo "Now running restore-service.sh..."
    bash restore-service.sh
  fi
}

main() {
  select_storage_target
  local_restore_selections
  initialize_duplicacy
  choose_revision
  restore_repository
  service_specific_restore_script
}

main
