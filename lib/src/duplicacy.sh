#!/bin/bash

# Define primary Duplicacy-related configuration variables.
DUPLICACY_BIN="duplicacy" # Path to Duplicacy binary. Maybe a bad idea, but assuming duplicacy is in PATH. Previously used "/usr/local/bin/duplicacy".
DUPLICACY_KEY_DIR="${ARCHIVER_DIR}/keys" # Path to Duplicacy key directory
DUPLICACY_RSA_PUBLIC_KEY_FILE="${DUPLICACY_KEY_DIR}/public.pem" # Path to RSA public key file for Duplicacy
DUPLICACY_RSA_PRIVATE_KEY_FILE="${DUPLICACY_KEY_DIR}/private.pem" # Path to RSA private key file for Duplicacy

set_duplicacy_variables() {
  DUPLICACY_REPO_DIR="${SERVICE_DIR}/.duplicacy" # Directory for various Duplicacy repos
  DUPLICACY_FILTERS_FILE="${DUPLICACY_REPO_DIR}/filters" # Location for Duplicacy filters file
  DUPLICACY_PREFERENCES_FILE="${DUPLICACY_REPO_DIR}/preferences" # Location for Duplicacy preferences file
  DUPLICACY_SNAPSHOT_ID="${HOSTNAME}-${SERVICE}" # Snapshot ID for Duplicacy
}

duplicacy_binary_check() {
  if ! command -v "${DUPLICACY_BIN}" &> /dev/null; then
    handle_error "Duplicacy binary not installed. Please install Duplicacy binary before running main script."
    exit 1
  else
    log_message "INFO" "Duplicacy binary is installed. Proceeding with backup script." 
  fi
}

duplicacy_verify() {
  local exit_status
  storage_name="${1}"

  # Verify Duplicacy Storage existance with a duplicacy list command
  "${DUPLICACY_BIN}" list -storage "${storage_name}" 2>&1 | log_output
  exit_status="${PIPESTATUS[0]}"
  return "${exit_status}"
}

duplicacy_filters() {
  # Prepare Duplicacy filters file
  # Remove the filters file if it already exists
  rm -f "${DUPLICACY_FILTERS_FILE}" || handle_error "Error removing filters file for the '${SERVICE}' service."
  touch "${DUPLICACY_FILTERS_FILE}" || handle_error "Unable to create the Duplicacy filters file for the '${SERVICE}' service."

  # Add Duplicacy filters patterns to the filters file
  for line in "${DUPLICACY_FILTERS_PATTERNS[@]}"; do
    echo "${line}" >> "${DUPLICACY_FILTERS_FILE}" || handle_error "Unable to modify the Duplicacy filters file for the '${SERVICE}' service."
  done

  # Log success message
  log_message "INFO" "Preparation for Duplicacy filter for '${SERVICE}' service completed successfully."
}

duplicacy_primary_backup() {
  local exit_status
  local storage_name
  local storage_name_upper
  local duplicacy_storage_password_var
  local backup_type

  storage_name="${STORAGE_TARGET_1_NAME}"
  storage_name_upper="$(echo "${storage_name}" | tr '[:lower:]' '[:upper:]')"
  duplicacy_storage_password_var="DUPLICACY_${storage_name_upper}_PASSWORD"
  backup_type="${STORAGE_TARGET_1_TYPE}"

  export "${duplicacy_storage_password_var}"="${STORAGE_PASSWORD}" # Export password for primary duplicacy storage so Duplicacy binary can see variable

  # Move to backup directory or exit if failed
  cd "${SERVICE_DIR}" || handle_error "Failed to change to directory '${SERVICE_DIR}'."

  # Remove the duplicacy preferences file if it already exists to start from scratch every time
  rm -f "${DUPLICACY_PREFERENCES_FILE}" || handle_error "Error removing preferences file for the '${SERVICE}' service."

  # Initialize Duplicacy primary storage
  if [[ "${backup_type}" == "sftp" ]]; then
    local duplicacy_ssh_key_file_var

    duplicacy_ssh_key_file_var="DUPLICACY_${storage_name_upper}_SSH_KEY_FILE"

    export "${duplicacy_ssh_key_file_var}"="${STORAGE_TARGET_1_SFTP_KEY_FILE}" # Export SSH key file for primary duplicacy storage so Duplicacy binary can see variable

    # Initialize SFTP storage
    log_message "INFO" "Initializing Primary Duplicacy Storage for ${SERVICE} service."

    "${DUPLICACY_BIN}" init -e -key "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" \
      -storage-name "${storage_name}" "${DUPLICACY_SNAPSHOT_ID}" \
      "sftp://${STORAGE_TARGET_1_SFTP_USER}@${STORAGE_TARGET_1_SFTP_URL}:${STORAGE_TARGET_1_SFTP_PORT}//${STORAGE_TARGET_1_SFTP_PATH}" 2>&1 | \
      log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Primary Duplicacy Storage initialization for the ${SERVICE} service failed."
    fi

    # Set SSH key file for Primary Duplicacy Storage
    "${DUPLICACY_BIN}" set -storage "${storage_name}" -key ssh_key_file -value "${STORAGE_TARGET_1_SFTP_KEY_FILE}" 2>&1 | log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the Primary Duplicacy Storage SSH key file for the ${SERVICE} service failed. Verify the SSH key file path and permissions."
    fi

  elif [[ "${backup_type}" == "b2" ]]; then
    local duplicacy_b2_id_var
    local duplicacy_b2_key_var

    duplicacy_b2_id_var="DUPLICACY_${storage_name_upper}_B2_ID"
    duplicacy_b2_key_var="DUPLICACY_${storage_name_upper}_B2_KEY"

    export "${duplicacy_b2_id_var}"="${STORAGE_TARGET_1_B2_ID}" # Export BackBlaze Key ID for backblaze storage so Duplicacy binary can see variable
    export "${duplicacy_b2_key_var}"="${STORAGE_TARGET_1_B2_KEY}" # Export BackBlaze Application Key for backblaze storage so Duplicacy binary can see variable

    # Initialize B2 storage
    log_message "INFO" "Initializing Primary Duplicacy Storage for '${SERVICE}' service."

    "${DUPLICACY_BIN}" init -e -key "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" \
      -storage-name "${storage_name}" "${DUPLICACY_SNAPSHOT_ID}" \
      "b2://${STORAGE_TARGET_1_B2_BUCKETNAME}" 2>&1 | \
      log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Primary Duplicacy Storage initialization for the '${SERVICE}' service failed."
    fi

    # Set Key ID for BackBlaze Duplicacy Storage
    "${DUPLICACY_BIN}" set -storage "${storage_name}" -key b2_id \
      -value "${STORAGE_TARGET_1_B2_ID}" 2>&1 | log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the BackBlaze Duplicacy Storage Key ID for the '${SERVICE}' service failed."
    fi

    # Set Application Key for BackBlaze Duplicacy Storage
    "${DUPLICACY_BIN}" set -storage "${storage_name}" -key b2_key \
      -value "${STORAGE_TARGET_1_B2_KEY}" 2>&1 | log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the BackBlaze Duplicacy Storage Application Key for the '${SERVICE}' service failed."
    fi

  elif [[ "${backup_type}" == "s3" ]]; then
    local duplicacy_s3_id_var
    local duplicacy_s3_secret_var
    local s3_region

    duplicacy_s3_id_var="DUPLICACY_${storage_name_upper}_S3_ID"
    duplicacy_s3_secret_var="DUPLICACY_${storage_name_upper}_S3_SECRET"
    s3_region="${STORAGE_TARGET_1_S3_REGION:-none}"

    export "${duplicacy_s3_id_var}"="${STORAGE_TARGET_1_S3_ID}" # Export S3 Access Key so Duplicacy binary can see variable
    export "${duplicacy_s3_secret_var}"="${STORAGE_TARGET_1_S3_SECRET}" # Export S3 Secret Key so Duplicacy binary can see variable

    # Initialize S3 storage
    log_message "INFO" "Initializing Primary Duplicacy Storage for '${SERVICE}' service."

    "${DUPLICACY_BIN}" init -e -key "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" \
      -storage-name "${storage_name}" "${DUPLICACY_SNAPSHOT_ID}" \
      "s3://${s3_region}@${STORAGE_TARGET_1_S3_ENDPOINT}/${STORAGE_TARGET_1_S3_BUCKETNAME}" 2>&1 | \
      log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Primary Duplicacy Storage initialization for the '${SERVICE}' service failed."
    fi

    # Set ID for S3 Duplicacy Storage
    "${DUPLICACY_BIN}" set -storage "${storage_name}" -key s3_id \
      -value "${STORAGE_TARGET_1_S3_ID}" 2>&1 | log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the S3 Duplicacy ID for the '${SERVICE}' service failed."
    fi

    # Set Secret for S3 Duplicacy Storage
    "${DUPLICACY_BIN}" set -storage "${storage_name}" -key s3_secret \
      -value "${STORAGE_TARGET_1_S3_SECRET}" 2>&1 | log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the S3 Duplicacy Secret for the '${SERVICE}' service failed."
    fi

  else
    handle_error "'${backup_type}' is not a supported backup type. Please edit config.sh to fix."
  fi

  # Set Password for Primary Duplicacy Storage
  "${DUPLICACY_BIN}" set -storage "${storage_name}" -key password -value "${STORAGE_PASSWORD}" 2>&1 | log_output
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Setting the Primary Duplicacy Storage password for the '${SERVICE}' service failed."
  fi

  # Set RSA Passphrase for Primary Duplicacy Storage
  "${DUPLICACY_BIN}" set -storage "${storage_name}" -key rsa_passphrase \
    -value "${RSA_PASSPHRASE}" 2>&1 | log_output
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Setting the Primary Duplicacy Storage RSA Passphrase for the '${SERVICE}' service failed."
  fi

  # Verify Primary Duplicacy Storage initiation
  if ! duplicacy_verify "${storage_name}"; then
    handle_error "Verification of the Primary Duplicacy Storage initialization for the '${SERVICE}' service failed."
  fi
  log_message "INFO" "Primary Duplicacy Storage initialization verified for the '${SERVICE}' service."

  # Prepare Duplicacy filters file
  log_message "INFO" "Preparing the Duplicacy filters file for the '${SERVICE}' service."
  # Remove the filters file if it already exists
  rm -f "${DUPLICACY_FILTERS_FILE}" || handle_error "Error removing filters file for the '${SERVICE}' service."
  # Build the Duplicacy filters file
  touch "${DUPLICACY_FILTERS_FILE}" || handle_error "Unable to create the Duplicacy filters file for the '${SERVICE}' service."
  for line in "${DUPLICACY_FILTERS_PATTERNS[@]}"; do
    echo "${line}" >> "${DUPLICACY_FILTERS_FILE}" || handle_error "Unable to modify the Duplicacy filters file for the '${SERVICE}' service."
  done

  # Run the Duplicacy backup to the Primary Storage
  log_message "INFO" "Running Duplicacy primary storage backup to '${STORAGE_TARGET_1_NAME}' storage for the '${SERVICE}' service."
  
  "${DUPLICACY_BIN}" backup -storage "${STORAGE_TARGET_1_NAME}" 2>&1 | log_output
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Duplicacy primary storage backup to '${STORAGE_TARGET_1_NAME}' storage for the '${SERVICE}' service failed."
  fi

  log_message "INFO" "Duplicacy primary storage backup to '${STORAGE_TARGET_1_NAME}' storage for the '${SERVICE}' service completed successfully."
}

duplicacy_add_backup() {
  if [[ "${STORAGE_TARGET_COUNT}" -gt 1 ]]; then
    for i in $(seq 2 "${STORAGE_TARGET_COUNT}"); do
      local exit_status
      local storage_id
      local backup_type_var
      local backup_type
      local storage_name_var
      local storage_name
      local storage_name_upper
      local duplicacy_storage_password_var

      storage_id="${i}"
      backup_type_var="STORAGE_TARGET_${storage_id}_TYPE"
      backup_type="${!backup_type_var}"
      storage_name_var="STORAGE_TARGET_${storage_id}_NAME"
      storage_name="${!storage_name_var}"
      storage_name_upper="$(echo "${storage_name}" | tr '[:lower:]' '[:upper:]')"
      duplicacy_storage_password_var="DUPLICACY_${storage_name_upper}_PASSWORD"

      export "${duplicacy_storage_password_var}"="${STORAGE_PASSWORD}" # Export password for duplicacy storage so Duplicacy binary can see variable

      # Move to backup directory or exit if failed
      cd "${SERVICE_DIR}" || handle_error "Failed to change to directory ${SERVICE_DIR}."

      # Initialize Duplicacy secondary storage
      if [[ "${backup_type}" == "sftp" ]]; then
        # Add SFTP Duplicacy Storage if not already added
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

        export "${duplicacy_ssh_key_file_var}"="${!config_sftp_key_file_var}" # Export SSH key file for sftp duplicacy storage so Duplicacy binary can see variable

        # Add SFTP Duplicacy Storage
        log_message "INFO" "Adding SFTP Duplicacy Storage '${storage_name} for the '${SERVICE}' service."
        "${DUPLICACY_BIN}" add -e -copy "${STORAGE_TARGET_1_NAME}" -bit-identical -key \
          "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" "${storage_name}" "${DUPLICACY_SNAPSHOT_ID}" \
          "sftp://${!config_sftp_user_var}@${!config_sftp_url_var}:${!config_sftp_port_var}//${!config_sftp_path_var}" 2>&1 | \
          log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Adding SFTP Duplicacy Storage '${storage_name}' for the '${SERVICE}' service failed."
        fi

        # Set SSH key file for SFTP Duplicacy Storage
        "${DUPLICACY_BIN}" set -storage "${storage_name}" -key ssh_key_file -value "${!config_sftp_key_file_var}" 2>&1 | log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Setting the SFTP Duplicacy storage '${storage_name}' SSH key file for the '${SERVICE}'service failed. Verify the SSH key file path and permissions."
        fi

      elif [[ "${backup_type}" == "b2" ]]; then
        # Add BackBlaze Duplicacy Storage if not already added
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

        # Add BackBlaze Duplicacy Storage
        log_message "INFO" "Adding BackBlaze Duplicacy Storage '${storage_name}' for the '${SERVICE}' service."
        "${DUPLICACY_BIN}" add -e -copy "${STORAGE_TARGET_1_NAME}" -bit-identical -key \
          "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" "${storage_name}" "${DUPLICACY_SNAPSHOT_ID}" \
          "b2://${!config_b2_bucketname_var}" 2>&1 | \
          log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Adding BackBlaze Duplicacy Storage '${storage_name}' for the '${SERVICE}' service failed."
        fi

        # Set Key ID for BackBlaze Duplicacy Storage
        "${DUPLICACY_BIN}" set -storage "${storage_name}" -key b2_id \
          -value "${!config_b2_id_var}" 2>&1 | log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Setting the BackBlaze Duplicacy Storage '${storage_name}' Key ID for the '${SERVICE}' service failed."
        fi

        # Set Application Key for BackBlaze Duplicacy Storage
        "${DUPLICACY_BIN}" set -storage "${storage_name}" -key b2_key \
          -value "${!config_b2_key_var}" 2>&1 | log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Setting the BackBlaze Duplicacy Storage '${storage_name}' Application Key for the '${SERVICE}' service failed."
        fi

      elif [[ "${backup_type}" == "s3" ]]; then
        # Add S3 Duplicacy Storage if not already added
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

        # Add S3 Duplicacy Storage
        log_message "INFO" "Adding S3 Duplicacy Storage '${storage_name}' for the '${SERVICE}' service."
        "${DUPLICACY_BIN}" add -e -copy "${STORAGE_TARGET_1_NAME}" -bit-identical -key \
          "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" "${storage_name}" "${DUPLICACY_SNAPSHOT_ID}" \
          "s3://${s3_region}@${!config_s3_endpoint_var}/${!config_s3_bucketname_var}" 2>&1 | \
          log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Adding S3 Duplicacy Storage '${storage_name}' for the '${SERVICE}' service failed."
        fi

        # Set ID for S3 Duplicacy Storage
        "${DUPLICACY_BIN}" set -storage "${storage_name}" -key s3_id \
          -value "${!config_s3_id_var}" 2>&1 | log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Setting the S3 Duplicacy Storage '${storage_name}' ID for the '${SERVICE}' service failed."
        fi

        # Set Secret for S3 Duplicacy Storage
        "${DUPLICACY_BIN}" set -storage "${storage_name}" -key s3_secret \
          -value "${!config_s3_secret_var}" 2>&1 | log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Setting the S3 Duplicacy Storage '${storage_name}' Secret for the '${SERVICE}' service failed."
        fi

      else
        handle_error "'${backup_type}' is not a supported backup type. Please edit config.sh to only reference supported backup types."
      fi

      # Set Password for the additional storage
      "${DUPLICACY_BIN}" set -storage "${storage_name}" -key password \
        -value "${STORAGE_PASSWORD}" 2>&1 | log_output
      exit_status="${PIPESTATUS[0]}"
      if [ "${exit_status}" -ne 0 ]; then
        handle_error "Setting the Duplicacy storage password for the '${SERVICE}' service failed."
      fi

      # Set RSA Passphrase for the additional storage
      "${DUPLICACY_BIN}" set -storage "${storage_name}" -key rsa_passphrase \
        -value "${RSA_PASSPHRASE}" 2>&1 | log_output
      exit_status="${PIPESTATUS[0]}"
      if [ "${exit_status}" -ne 0 ]; then
        handle_error "Setting the Duplicacy storage RSA Passphrase for the '${SERVICE}' service failed."
      fi
    done
  fi
}

duplicacy_copy_backup() {
  if [[ "${STORAGE_TARGET_COUNT}" -gt 1 ]]; then
    for i in $(seq 2 "${STORAGE_TARGET_COUNT}"); do
      local exit_status
      local storage_id
      local storage_name_var
      local storage_name

      storage_id="${i}"
      storage_name_var="STORAGE_TARGET_${storage_id}_NAME"
      storage_name="${!storage_name_var}"

      # Set SERVICE to storage name for logging context
      SERVICE="${storage_name}"

      # Run the Duplicacy copy backup
      log_message "INFO" "Running Duplicacy copy backup to '${storage_name}' storage."

      "${DUPLICACY_BIN}" copy -from "${STORAGE_TARGET_1_NAME}" -to "${storage_name}" \
        -key "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" 2>&1 | log_output
      exit_status="${PIPESTATUS[0]}"

      if [ "${exit_status}" -ne 0 ]; then
        handle_error "Running the Duplicacy copy backup to '${storage_name}' storage failed."
      else
        log_message "INFO" "The Duplicacy copy backup to '${storage_name}' storage completed successfully."
      fi

      # duplicacy_wrap_up will set and unset SERVICE for its operations
      duplicacy_wrap_up "${storage_name}"
    done
  fi
}

duplicacy_wrap_up() {
  local exit_status
  local storage_name

  storage_name="${1}"

  # Set SERVICE to storage name for logging context
  SERVICE="${storage_name}"

  # Full Check the Duplicacy storage
  "${DUPLICACY_BIN}" check -all -storage "${storage_name}" -fossils -resurrect 2>&1 | log_output
  exit_status="${PIPESTATUS[0]}"
  if [[ "${exit_status}" -ne 0 ]]; then
    handle_error "Running the Duplicacy full '${storage_name}' storage check failed."
  else
    log_message "INFO" "The Duplicacy full '${storage_name}' storage check completed successfully."
  fi

  if [[ "$(echo "${ROTATE_BACKUPS}" | tr '[:upper:]' '[:lower:]')" == "true" ]]; then
    # Build the keep options array
    declare -a PRUNE_KEEP_ARRAY
    PRUNE_KEEP_ARRAY=()
    read -r -a PRUNE_KEEP_ARRAY <<< "${PRUNE_KEEP}"

    "${DUPLICACY_BIN}" prune -all -storage "${storage_name}" "${PRUNE_KEEP_ARRAY[@]}" 2>&1 | log_output
    exit_status="${PIPESTATUS[0]}"
    if [[ "${exit_status}" -ne 0 ]]; then
      handle_error "Running Duplicacy storage '${storage_name}' prune failed. Review the Duplicacy logs for details."
    else
      log_message "INFO" "Duplicacy storage '${storage_name}' prune completed successfully."
    fi
  fi

  # Unset SERVICE after wrap-up operations complete
  unset SERVICE
}
