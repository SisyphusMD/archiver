#!/bin/bash
# Duplicacy backup operations: init, backup, add storage, copy, prune

DUPLICACY_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOGGING_CORE}"
DUPLICACY_BIN="duplicacy"

set_duplicacy_variables() {
  DUPLICACY_REPO_DIR="${SERVICE_DIR}/.duplicacy"
  DUPLICACY_FILTERS_FILE="${DUPLICACY_REPO_DIR}/filters"
  DUPLICACY_PREFERENCES_FILE="${DUPLICACY_REPO_DIR}/preferences"
  DUPLICACY_SNAPSHOT_ID="${HOSTNAME}-${SERVICE}"
}

duplicacy_binary_check() {
  log_message "INFO" "Proceeding with backup script."
}

duplicacy_verify() {
  local exit_status
  storage_name="${1}"

  "${DUPLICACY_BIN}" list -storage "${storage_name}" 2>&1 | log_output
  exit_status="${PIPESTATUS[0]}"
  return "${exit_status}"
}

duplicacy_filters() {
  rm -f "${DUPLICACY_FILTERS_FILE}" || handle_error "Error removing filters file for the '${SERVICE}' service."
  touch "${DUPLICACY_FILTERS_FILE}" || handle_error "Unable to create the Duplicacy filters file for the '${SERVICE}' service."

  for line in "${DUPLICACY_FILTERS_PATTERNS[@]}"; do
    echo "${line}" >> "${DUPLICACY_FILTERS_FILE}" || handle_error "Unable to modify the Duplicacy filters file for the '${SERVICE}' service."
  done

  log_message "INFO" "Preparation for Duplicacy filter for '${SERVICE}' service completed successfully."
}

duplicacy_primary_backup() {
  local exit_status
  local storage_name
  local storage_name_upper
  local duplicacy_storage_password_var
  local backup_type

  storage_name="$(sanitize_storage_name "${STORAGE_TARGET_1_NAME}")"
  storage_name_upper="$(echo "${storage_name}" | tr '[:lower:]' '[:upper:]')"
  duplicacy_storage_password_var="DUPLICACY_${storage_name_upper}_PASSWORD"
  backup_type="${STORAGE_TARGET_1_TYPE}"

  # Export password so Duplicacy binary can see variable
  export "${duplicacy_storage_password_var}"="${STORAGE_PASSWORD}"

  cd "${SERVICE_DIR}" || handle_error "Failed to change to directory '${SERVICE_DIR}'."

  rm -f "${DUPLICACY_PREFERENCES_FILE}" || handle_error "Error removing preferences file for the '${SERVICE}' service."

  if [[ "${backup_type}" == "local" ]]; then
    log_message "INFO" "Initializing Primary Duplicacy Storage for '${SERVICE}' service."

    "${DUPLICACY_BIN}" init -e -key "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" \
      -storage-name "${storage_name}" "${DUPLICACY_SNAPSHOT_ID}" \
      "${STORAGE_TARGET_1_LOCAL_PATH}" 2>&1 | \
      log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Primary Duplicacy Storage initialization for the '${SERVICE}' service failed."
    fi

  elif [[ "${backup_type}" == "sftp" ]]; then
    local duplicacy_ssh_key_file_var

    duplicacy_ssh_key_file_var="DUPLICACY_${storage_name_upper}_SSH_KEY_FILE"

    # Export SSH key file so Duplicacy binary can see variable
    export "${duplicacy_ssh_key_file_var}"="${STORAGE_TARGET_1_SFTP_KEY_FILE}"
    log_message "INFO" "Initializing Primary Duplicacy Storage for ${SERVICE} service."

    "${DUPLICACY_BIN}" init -e -key "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" \
      -storage-name "${storage_name}" "${DUPLICACY_SNAPSHOT_ID}" \
      "sftp://${STORAGE_TARGET_1_SFTP_USER}@${STORAGE_TARGET_1_SFTP_URL}:${STORAGE_TARGET_1_SFTP_PORT}//${STORAGE_TARGET_1_SFTP_PATH}" 2>&1 | \
      log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Primary Duplicacy Storage initialization for the ${SERVICE} service failed."
    fi

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

    # Export BackBlaze credentials so Duplicacy binary can see variables
    export "${duplicacy_b2_id_var}"="${STORAGE_TARGET_1_B2_ID}"
    export "${duplicacy_b2_key_var}"="${STORAGE_TARGET_1_B2_KEY}"
    log_message "INFO" "Initializing Primary Duplicacy Storage for '${SERVICE}' service."

    "${DUPLICACY_BIN}" init -e -key "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" \
      -storage-name "${storage_name}" "${DUPLICACY_SNAPSHOT_ID}" \
      "b2://${STORAGE_TARGET_1_B2_BUCKETNAME}" 2>&1 | \
      log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Primary Duplicacy Storage initialization for the '${SERVICE}' service failed."
    fi

    "${DUPLICACY_BIN}" set -storage "${storage_name}" -key b2_id \
      -value "${STORAGE_TARGET_1_B2_ID}" 2>&1 | log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the BackBlaze Duplicacy Storage Key ID for the '${SERVICE}' service failed."
    fi

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

    # Export S3 credentials so Duplicacy binary can see variables
    export "${duplicacy_s3_id_var}"="${STORAGE_TARGET_1_S3_ID}"
    export "${duplicacy_s3_secret_var}"="${STORAGE_TARGET_1_S3_SECRET}"
    log_message "INFO" "Initializing Primary Duplicacy Storage for '${SERVICE}' service."

    "${DUPLICACY_BIN}" init -e -key "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" \
      -storage-name "${storage_name}" "${DUPLICACY_SNAPSHOT_ID}" \
      "s3://${s3_region}@${STORAGE_TARGET_1_S3_ENDPOINT}/${STORAGE_TARGET_1_S3_BUCKETNAME}" 2>&1 | \
      log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Primary Duplicacy Storage initialization for the '${SERVICE}' service failed."
    fi

    "${DUPLICACY_BIN}" set -storage "${storage_name}" -key s3_id \
      -value "${STORAGE_TARGET_1_S3_ID}" 2>&1 | log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the S3 Duplicacy ID for the '${SERVICE}' service failed."
    fi

    "${DUPLICACY_BIN}" set -storage "${storage_name}" -key s3_secret \
      -value "${STORAGE_TARGET_1_S3_SECRET}" 2>&1 | log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Setting the S3 Duplicacy Secret for the '${SERVICE}' service failed."
    fi

  else
    handle_error "'${backup_type}' is not a supported backup type. Please edit config.sh to fix."
  fi

  "${DUPLICACY_BIN}" set -storage "${storage_name}" -key password -value "${STORAGE_PASSWORD}" 2>&1 | log_output
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Setting the Primary Duplicacy Storage password for the '${SERVICE}' service failed."
  fi

  "${DUPLICACY_BIN}" set -storage "${storage_name}" -key rsa_passphrase \
    -value "${RSA_PASSPHRASE}" 2>&1 | log_output
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Setting the Primary Duplicacy Storage RSA Passphrase for the '${SERVICE}' service failed."
  fi

  if ! duplicacy_verify "${storage_name}"; then
    handle_error "Verification of the Primary Duplicacy Storage initialization for the '${SERVICE}' service failed."
  fi
  log_message "INFO" "Primary Duplicacy Storage initialization verified for the '${SERVICE}' service."

  log_message "INFO" "Preparing the Duplicacy filters file for the '${SERVICE}' service."
  rm -f "${DUPLICACY_FILTERS_FILE}" || handle_error "Error removing filters file for the '${SERVICE}' service."
  touch "${DUPLICACY_FILTERS_FILE}" || handle_error "Unable to create the Duplicacy filters file for the '${SERVICE}' service."
  for line in "${DUPLICACY_FILTERS_PATTERNS[@]}"; do
    echo "${line}" >> "${DUPLICACY_FILTERS_FILE}" || handle_error "Unable to modify the Duplicacy filters file for the '${SERVICE}' service."
  done

  log_message "INFO" "Running Duplicacy primary storage backup to '${STORAGE_TARGET_1_NAME}' storage for the '${SERVICE}' service."

  "${DUPLICACY_BIN}" backup -storage "${storage_name}" -stats -threads "${DUPLICACY_THREADS}" 2>&1 | log_output
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Duplicacy primary storage backup to '${STORAGE_TARGET_1_NAME}' storage for the '${SERVICE}' service failed."
  fi

  log_message "INFO" "Duplicacy primary storage backup to '${STORAGE_TARGET_1_NAME}' storage for the '${SERVICE}' service completed successfully."
}

duplicacy_add_backup() {
  if [[ "${STORAGE_TARGET_COUNT}" -gt 1 ]]; then
    local primary_storage_name
    primary_storage_name="$(sanitize_storage_name "${STORAGE_TARGET_1_NAME}")"

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
      storage_name="$(sanitize_storage_name "${!storage_name_var}")"
      storage_name_upper="$(echo "${storage_name}" | tr '[:lower:]' '[:upper:]')"
      duplicacy_storage_password_var="DUPLICACY_${storage_name_upper}_PASSWORD"

      # Export password so Duplicacy binary can see variable
      export "${duplicacy_storage_password_var}"="${STORAGE_PASSWORD}"

      cd "${SERVICE_DIR}" || handle_error "Failed to change to directory ${SERVICE_DIR}."

      if [[ "${backup_type}" == "local" ]]; then
        local config_local_path_var

        config_local_path_var="STORAGE_TARGET_${storage_id}_LOCAL_PATH"

        log_message "INFO" "Adding local disk Duplicacy Storage '${storage_name}' for the '${SERVICE}' service."
        "${DUPLICACY_BIN}" add -e -copy "${primary_storage_name}" -bit-identical -key \
          "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" "${storage_name}" "${DUPLICACY_SNAPSHOT_ID}" \
          "${!config_local_path_var}" 2>&1 | \
          log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Adding local disk Duplicacy Storage '${storage_name}' for the '${SERVICE}' service failed."
        fi

      elif [[ "${backup_type}" == "sftp" ]]; then
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

        # Export SSH key file so Duplicacy binary can see variable
        export "${duplicacy_ssh_key_file_var}"="${!config_sftp_key_file_var}"

        log_message "INFO" "Adding SFTP Duplicacy Storage '${storage_name}' for the '${SERVICE}' service."
        "${DUPLICACY_BIN}" add -e -copy "${primary_storage_name}" -bit-identical -key \
          "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" "${storage_name}" "${DUPLICACY_SNAPSHOT_ID}" \
          "sftp://${!config_sftp_user_var}@${!config_sftp_url_var}:${!config_sftp_port_var}//${!config_sftp_path_var}" 2>&1 | \
          log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Adding SFTP Duplicacy Storage '${storage_name}' for the '${SERVICE}' service failed."
        fi

        "${DUPLICACY_BIN}" set -storage "${storage_name}" -key ssh_key_file -value "${!config_sftp_key_file_var}" 2>&1 | log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Setting the SFTP Duplicacy storage '${storage_name}' SSH key file for the '${SERVICE}'service failed. Verify the SSH key file path and permissions."
        fi

      elif [[ "${backup_type}" == "b2" ]]; then
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

        # Export BackBlaze credentials so Duplicacy binary can see variables
        export "${duplicacy_b2_id_var}"="${!config_b2_id_var}"
        export "${duplicacy_b2_key_var}"="${!config_b2_key_var}"

        log_message "INFO" "Adding BackBlaze Duplicacy Storage '${storage_name}' for the '${SERVICE}' service."
        "${DUPLICACY_BIN}" add -e -copy "${primary_storage_name}" -bit-identical -key \
          "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" "${storage_name}" "${DUPLICACY_SNAPSHOT_ID}" \
          "b2://${!config_b2_bucketname_var}" 2>&1 | \
          log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Adding BackBlaze Duplicacy Storage '${storage_name}' for the '${SERVICE}' service failed."
        fi

        "${DUPLICACY_BIN}" set -storage "${storage_name}" -key b2_id \
          -value "${!config_b2_id_var}" 2>&1 | log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Setting the BackBlaze Duplicacy Storage '${storage_name}' Key ID for the '${SERVICE}' service failed."
        fi

        "${DUPLICACY_BIN}" set -storage "${storage_name}" -key b2_key \
          -value "${!config_b2_key_var}" 2>&1 | log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Setting the BackBlaze Duplicacy Storage '${storage_name}' Application Key for the '${SERVICE}' service failed."
        fi

      elif [[ "${backup_type}" == "s3" ]]; then
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

        # Export S3 credentials so Duplicacy binary can see variables
        export "${duplicacy_s3_id_var}"="${!config_s3_id_var}"
        export "${duplicacy_s3_secret_var}"="${!config_s3_secret_var}"

        log_message "INFO" "Adding S3 Duplicacy Storage '${storage_name}' for the '${SERVICE}' service."
        "${DUPLICACY_BIN}" add -e -copy "${primary_storage_name}" -bit-identical -key \
          "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" "${storage_name}" "${DUPLICACY_SNAPSHOT_ID}" \
          "s3://${s3_region}@${!config_s3_endpoint_var}/${!config_s3_bucketname_var}" 2>&1 | \
          log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Adding S3 Duplicacy Storage '${storage_name}' for the '${SERVICE}' service failed."
        fi

        "${DUPLICACY_BIN}" set -storage "${storage_name}" -key s3_id \
          -value "${!config_s3_id_var}" 2>&1 | log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Setting the S3 Duplicacy Storage '${storage_name}' ID for the '${SERVICE}' service failed."
        fi

        "${DUPLICACY_BIN}" set -storage "${storage_name}" -key s3_secret \
          -value "${!config_s3_secret_var}" 2>&1 | log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Setting the S3 Duplicacy Storage '${storage_name}' Secret for the '${SERVICE}' service failed."
        fi

      else
        handle_error "'${backup_type}' is not a supported backup type. Please edit config.sh to only reference supported backup types."
      fi

      "${DUPLICACY_BIN}" set -storage "${storage_name}" -key password \
        -value "${STORAGE_PASSWORD}" 2>&1 | log_output
      exit_status="${PIPESTATUS[0]}"
      if [ "${exit_status}" -ne 0 ]; then
        handle_error "Setting the Duplicacy storage password for the '${SERVICE}' service failed."
      fi

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
    local primary_storage_name
    primary_storage_name="$(sanitize_storage_name "${STORAGE_TARGET_1_NAME}")"

    for i in $(seq 2 "${STORAGE_TARGET_COUNT}"); do
      local exit_status
      local storage_id
      local storage_name_var
      local storage_name

      storage_id="${i}"
      storage_name_var="STORAGE_TARGET_${storage_id}_NAME"
      storage_name="$(sanitize_storage_name "${!storage_name_var}")"

      # Set SERVICE to storage name for logging context
      SERVICE="${storage_name}"

      log_message "INFO" "Running Duplicacy copy backup to '${storage_name}' storage."

      "${DUPLICACY_BIN}" copy -from "${primary_storage_name}" -to "${storage_name}" \
        -key "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" -threads "${DUPLICACY_THREADS}" -download-threads "${DUPLICACY_THREADS}" 2>&1 | log_output
      exit_status="${PIPESTATUS[0]}"

      if [ "${exit_status}" -ne 0 ]; then
        handle_error "Running the Duplicacy copy backup to '${storage_name}' storage failed."
      else
        log_message "INFO" "The Duplicacy copy backup to '${storage_name}' storage completed successfully."
      fi

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

  "${DUPLICACY_BIN}" check -all -storage "${storage_name}" -fossils -resurrect -stats -threads "${DUPLICACY_THREADS}" 2>&1 | log_output
  exit_status="${PIPESTATUS[0]}"
  if [[ "${exit_status}" -ne 0 ]]; then
    handle_error "Running the Duplicacy full '${storage_name}' storage check failed."
  else
    log_message "INFO" "The Duplicacy full '${storage_name}' storage check completed successfully."
  fi

  if [[ "$(echo "${ROTATE_BACKUPS}" | tr '[:upper:]' '[:lower:]')" == "true" ]]; then
    declare -a PRUNE_KEEP_ARRAY
    PRUNE_KEEP_ARRAY=()
    read -r -a PRUNE_KEEP_ARRAY <<< "${PRUNE_KEEP}"

    "${DUPLICACY_BIN}" prune -all -storage "${storage_name}" "${PRUNE_KEEP_ARRAY[@]}" -threads "${DUPLICACY_THREADS}" 2>&1 | log_output
    exit_status="${PIPESTATUS[0]}"
    if [[ "${exit_status}" -ne 0 ]]; then
      handle_error "Running Duplicacy storage '${storage_name}' prune failed. Review the Duplicacy logs for details."
    else
      log_message "INFO" "Duplicacy storage '${storage_name}' prune completed successfully."
    fi
  fi

  unset SERVICE
}
