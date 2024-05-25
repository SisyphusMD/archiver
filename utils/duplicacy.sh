# Function to verify if a given storage is already initialized
# Parameters:
#   1. Duplicacy Storage Name
# Output:
#   Returns exit code 0 if initialized, and non-0 if not initialized
set_duplicacy_variables() {
  DUPLICACY_REPO_DIR="${SERVICE_DIR}/.duplicacy" # Directory for various Duplicacy repos
  DUPLICACY_FILTERS_FILE="${DUPLICACY_REPO_DIR}/filters" # Location for Duplicacy filters file
  DUPLICACY_SNAPSHOT_ID="${HOSTNAME}-${SERVICE}" # Snapshot ID for Duplicacy
}

verify_duplicacy() {
  local exit_status

  # Verify Duplicacy Storage existance
  "${DUPLICACY_BIN}" list -storage "${1}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
  exit_status="${PIPESTATUS[0]}"
  return "${exit_status}"
}

filters_duplicacy() {
  # Prepare Duplicacy filters file
  # Remove the filters file if it already exists
  rm -f "${DUPLICACY_FILTERS_FILE}" || handle_error "Error removing filters file."
  touch "${DUPLICACY_FILTERS_FILE}" || handle_error "Unable to create the Duplicacy filters file for the ${SERVICE} service."

  # Add Duplicacy filters patterns to the filters file
  for line in "${DUPLICACY_FILTERS_PATTERNS[@]}"; do
    echo "${line}" >> "${DUPLICACY_FILTERS_FILE}" || handle_error "Unable to modify the Duplicacy filters file for the ${SERVICE} service."
  done

  # Log success message
  log_message "INFO" "Preparation for Duplicacy filter for ${SERVICE} service completed successfully."
}

# Initializes Duplicacy for the service's backup directory if not already done.
# Parameters:
#   None. Operates within the context of the current service's backup directory.
# Output:
#   Initializes the Duplicacy repository in the backup directory. No direct output.
initialize_duplicacy() {
  local exit_status

  # Move to backup directory or exit if failed
  cd "${SERVICE_DIR}" || handle_error "Failed to change to backup directory ${SERVICE_DIR} for backup operations of ${SERVICE}."

  if ! verify_duplicacy "${DUPLICACY_OMV_STORAGE_NAME}"; then
    export DUPLICACY_OMV_SSH_KEY_FILE # Export SSH key file for omv storage so Duplicacy binary can see variable
    export DUPLICACY_OMV_PASSWORD # Export password for omv storage so Duplicacy binary can see variable

    # Initialize OMV Duplicacy Storage
    log_message "INFO" "Initializing OMV Duplicacy Storage for ${SERVICE} service."
    "${DUPLICACY_BIN}" init -e -key "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" \
      -storage-name "${DUPLICACY_OMV_STORAGE_NAME}" \
      "${DUPLICACY_SNAPSHOT_ID}" "${DUPLICACY_OMV_STORAGE_URL}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "OMV Duplicacy Storage initialization for the ${SERVICE} service failed. Ensure Duplicacy is installed and the repository path is correct."
    fi

    # Set Password for OMV Duplicacy Storage
    "${DUPLICACY_BIN}" set -key password -value "${DUPLICACY_OMV_PASSWORD}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the OMV Duplicacy Storage password for the ${SERVICE} service failed. Ensure the storage password is correctly specified in the service's backup settings or environment variables."
    fi

    # Set RSA Passphrase for OMV Duplicacy Storage
    "${DUPLICACY_BIN}" set -storage "${DUPLICACY_OMV_STORAGE_NAME}" -key rsa_passphrase \
      -value "${DUPLICACY_OMV_RSA_PASSPHRASE}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the OMV Duplicacy Storage RSA Passphrase for the ${SERVICE} service failed. Ensure the storage RSA Passphrase is correctly specified in the service's backup settings or environment variables."
    fi

    # Set SSH key file for OMV Duplicacy Storage
    "${DUPLICACY_BIN}" set -key ssh_key_file -value "${DUPLICACY_OMV_SSH_KEY_FILE}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the OMV Duplicacy Storage SSH key file for the ${SERVICE} service failed. Verify the SSH key file path and permissions."
    fi

    # Verify OMV Duplicacy Storage initiation
    if ! verify_duplicacy "${DUPLICACY_OMV_STORAGE_NAME}"; then
      handle_error "Verification of the OMV Duplicacy Storage initialization for the ${SERVICE} service failed. Ensure Duplicacy has been properly initialized."
    fi
    log_message "INFO" "OMV Duplicacy Storage initialization verified for ${SERVICE} service."

    # Prepare Duplicacy filters file
    filters_duplicacy || handle_error "Preparing Duplicacy filters file for the ${SERVICE} service failed."

  else
    log_message "INFO" "Duplicacy OMV storage already initialized for ${SERVICE} service."
  fi
}

# Adds BackBlaze Duplicacy storage if not already added
add_storage_duplicacy() {
  local exit_status

  # Move to backup directory or exit if failed
  cd "${SERVICE_DIR}" || handle_error "Failed to change to backup directory ${SERVICE_DIR} for backup operations of ${SERVICE}."

  if ! verify_duplicacy "${DUPLICACY_BACKBLAZE_STORAGE_NAME}"; then
    export DUPLICACY_BACKBLAZE_B2_ID # Export BackBlaze Account ID for backblaze storage so Duplicacy binary can see variable
    export DUPLICACY_BACKBLAZE_B2_KEY # Export BackBlaze Application Key for backblaze storage so Duplicacy binary can see variable
    export DUPLICACY_BACKBLAZE_PASSWORD # Export password for backblaze storage so Duplicacy binary can see variable

    # Add BackBlaze Duplicacy Storage
    log_message "INFO" "Adding BackBlaze Duplicacy Storage for ${SERVICE} service."
    "${DUPLICACY_BIN}" add -e -copy "${DUPLICACY_OMV_STORAGE_NAME}" -bit-identical -key \
      "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" "${DUPLICACY_BACKBLAZE_STORAGE_NAME}" \
      "${DUPLICACY_SNAPSHOT_ID}" "${DUPLICACY_BACKBLAZE_STORAGE_URL}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Adding BackBlaze Duplicacy Storage for the ${SERVICE} service failed. Ensure the BackBlaze variables are correctly specified in the secrets file."
    fi

    # Set Password for BackBlaze Duplicacy Storage
    "${DUPLICACY_BIN}" set -storage "${DUPLICACY_BACKBLAZE_STORAGE_NAME}" -key password \
      -value "${DUPLICACY_BACKBLAZE_PASSWORD}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the BackBlaze Duplicacy Storage password for the ${SERVICE} service failed. Ensure the storage password is correctly specified in the service's backup settings or environment variables."
    fi

    # Set RSA Passphrase for BackBlaze Duplicacy Storage
    "${DUPLICACY_BIN}" set -storage "${DUPLICACY_BACKBLAZE_STORAGE_NAME}" -key rsa_passphrase \
      -value "${DUPLICACY_BACKBLAZE_RSA_PASSPHRASE}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the BackBlaze Duplicacy Storage RSA Passphrase for the ${SERVICE} service failed. Ensure the storage RSA Passphrase is correctly specified in the service's backup settings or environment variables."
    fi

    # Set Key ID for BackBlaze Duplicacy Storage
    "${DUPLICACY_BIN}" set -storage "${DUPLICACY_BACKBLAZE_STORAGE_NAME}" -key b2_id \
      -value "${DUPLICACY_BACKBLAZE_B2_ID}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the BackBlaze Duplicacy Storage Key ID for the ${SERVICE} service failed. Ensure the Key ID is correctly specified in the service's backup settings or environment variables."
    fi

    # Set Application Key for BackBlaze Duplicacy Storage
    "${DUPLICACY_BIN}" set -storage "${DUPLICACY_BACKBLAZE_STORAGE_NAME}" -key b2_key \
      -value "${DUPLICACY_BACKBLAZE_B2_KEY}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the BackBlaze Duplicacy Storage Application Key for the ${SERVICE} service failed. Ensure the Application Key is correctly specified in the service's backup settings or environment variables."
    fi

    # Verify BackBlaze Duplicacy Storage initialization
    if ! verify_duplicacy "${DUPLICACY_BACKBLAZE_STORAGE_NAME}"; then
      handle_error "Verification of the BackBlaze Duplicacy Storage addition for the ${SERVICE} service failed. Ensure Duplicacy BackBlaze storage has been properly added."
    fi
    log_message "INFO" "BackBlaze Duplicacy Storage addition verified for ${SERVICE} service."

    # Prepare Duplicacy filters file
    filters_duplicacy || handle_error "Preparing Duplicacy filters file for the ${SERVICE} service failed."

  else
    log_message "INFO" "Duplicacy BackBlaze storage already initialized for ${SERVICE} service."
  fi
}

# Runs the Duplicacy backup operation for the current service's data.
# Parameters:
#   None. Uses configured Duplicacy settings and operates within the service's backup context.
# Output:
#   Performs a Duplicacy backup. Output is logged to the Duplicacy log file.
backup_duplicacy() {
  local exit_status

  # Initialize OMV Duplicacy Storage if not already initialized
  initialize_duplicacy || handle_error "Duplicacy initialization failed for ${SERVICE}."

  # Prepare Duplicacy filters file
  filters_duplicacy || handle_error "Preparing Duplicacy filters file for the ${SERVICE} service failed."

  # Run the Duplicacy backup to the OMV Storage
  log_message "INFO" "Running OMV Duplicacy Storage backup for ${SERVICE} service."
  "${DUPLICACY_BIN}" backup -storage "${DUPLICACY_OMV_STORAGE_NAME}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Running the OMV Duplicacy Storage backup for the ${SERVICE} service failed. Review the Duplicacy logs for details."
  fi
  log_message "INFO" "The OMV Duplicacy Storage backup completed successfully for ${SERVICE} service."

#  # Verify OMV Duplicacy Storage backup completion
#  if ! verify_duplicacy "${DUPLICACY_OMV_STORAGE_NAME}"; then
#    handle_error "Verification of the OMV Duplicacy Storage backup for the ${SERVICE} service failed. Check the backup integrity and storage accessibility."
#  fi
#  log_message "INFO" "OMV Duplicacy Storage backup verified for ${SERVICE} service."
}

# Runs the Duplicacy copy backup operation for the current service's data.
# Parameters:
#   None. Uses configured Duplicacy settings and operates within the service's backup context.
# Output:
#   Performs a Duplicacy copy backup. Output is logged to the Duplicacy log file.
copy_backup_duplicacy() {
  local exit_status

  # Add BackBlaze Duplicacy Storage if not already added
  add_storage_duplicacy || handle_error "Duplicacy initialization failed for ${SERVICE}."

  # Run the Duplicacy backup to the BackBlaze Storage
  log_message "INFO" "Running BackBlaze Duplicacy Storage backup for ${SERVICE} service."
  "${DUPLICACY_BIN}" copy -id "${DUPLICACY_SNAPSHOT_ID}" \
    -from "${DUPLICACY_OMV_STORAGE_NAME}" -to "${DUPLICACY_BACKBLAZE_STORAGE_NAME}" \
    -key "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Running the BackBlaze Duplicacy Storage backup for the ${SERVICE} service failed. Review the Duplicacy logs for details."
  fi
  log_message "INFO" "The BackBlaze Duplicacy Storage backup completed successfully for ${SERVICE} service."

#  # Verify BackBlaze Duplicacy Storage backup completion
#  if ! verify_duplicacy "${DUPLICACY_BACKBLAZE_STORAGE_NAME}"; then
#    handle_error "Verification of the BackBlaze Duplicacy Storage backup for the ${SERVICE} service failed. Check the backup integrity and storage accessibility."
#  fi
#  log_message "INFO" "BackBlaze Duplicacy Storage backup verified for ${SERVICE} service."
}

full_check_duplicacy() {
  # Full Check Duplicacy
  "${DUPLICACY_BIN}" check -all -storage "${1}" -fossils -resurrect 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
  exit_status="${PIPESTATUS[0]}"
  return "${exit_status}"
}

# Runs the Duplicacy prune operation for all repositories.
# Parameters:
#   None. Uses configured Duplicacy settings and operates within the final service's backup context.
# Output:
#   Performs a Duplicacy prune. Output is logged to the Duplicacy log file.
prune_duplicacy() {
  local exit_status

  # First Duplicacy Check the OMV storage
  full_check_duplicacy "${DUPLICACY_OMV_STORAGE_NAME}"

  # Prune the OMV Duplicacy Storage
  log_message "INFO" "Running OMV Duplicacy Storage prune for all repositories."
  "${DUPLICACY_BIN}" prune -all -storage "${DUPLICACY_OMV_STORAGE_NAME}" \
    -keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Running OMV Duplicacy Storage prune failed. Review the Duplicacy logs for details."
  fi
  log_message "INFO" "OMV Duplicacy Storage prune completed successfully."

  # First Duplicacy Check the Backblaze storage
  full_check_duplicacy "${DUPLICACY_BACKBLAZE_STORAGE_NAME}"

  # Prune the BackBlaze Duplicacy Storage
  log_message "INFO" "Running BackBlaze Duplicacy Storage prune for all repositories."
  "${DUPLICACY_BIN}" prune -all -storage "${DUPLICACY_BACKBLAZE_STORAGE_NAME}" \
    -keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1 2>&1 | log_output "${DUPLICACY_LOG_FILE}"
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Running BackBlaze Duplicacy Storage prune failed. Review the Duplicacy logs for details."
  fi
  log_message "INFO" "BackBlaze Duplicacy Storage prune completed successfully."
}
