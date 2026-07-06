#!/bin/bash
# Duplicacy backup operations: init, backup, add storage, copy, prune

DUPLICACY_BACKUP_SH_SOURCED=true

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
  rm -f "${DUPLICACY_FILTERS_FILE}" || handle_error "Error removing filters file for the ${SERVICE} service."
  touch "${DUPLICACY_FILTERS_FILE}" || handle_error "Unable to create the Duplicacy filters file for the ${SERVICE} service."

  for line in "${DUPLICACY_FILTERS_PATTERNS[@]}"; do
    echo "${line}" >> "${DUPLICACY_FILTERS_FILE}" || handle_error "Unable to modify the Duplicacy filters file for the ${SERVICE} service."
  done

  log_message "INFO" "Duplicacy filters configured for ${SERVICE} service."
}

duplicacy_primary_backup() {
  local exit_status
  local storage_name
  local backup_type
  local storage_url

  storage_name="$(sanitize_storage_name "${STORAGE_TARGET_1_NAME}")"
  backup_type="${STORAGE_TARGET_1_TYPE}"

  # Export the DUPLICACY_<NAME>_* credentials the duplicacy binary reads.
  export_duplicacy_storage_secrets "1"

  storage_url="$(build_storage_url "1")"
  if [[ -z "${storage_url}" ]]; then
    handle_error "${backup_type} is not a supported backup type. Please edit config.sh to fix."
    return 1
  fi

  cd "${SERVICE_DIR}" || handle_error "Failed to change to directory ${SERVICE_DIR}."

  rm -f "${DUPLICACY_PREFERENCES_FILE}" || handle_error "Error removing preferences file for the ${SERVICE} service."

  log_message "INFO" "Initializing primary storage for ${SERVICE} service."
  "${DUPLICACY_BIN}" init -e -key "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" \
    -storage-name "${storage_name}" "${DUPLICACY_SNAPSHOT_ID}" \
    "${storage_url}" 2>&1 | \
    log_output
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Primary storage initialization failed for ${SERVICE} service."
  fi

  # Persist type-specific credentials to the storage's preferences file.
  if [[ "${backup_type}" == "sftp" ]]; then
    "${DUPLICACY_BIN}" set -storage "${storage_name}" -key ssh_key_file -value "${DUPLICACY_SSH_PRIVATE_KEY_FILE}" 2>&1 | log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Failed to set SSH key file for ${SERVICE} service. Verify the SSH key file path and permissions."
    fi

  elif [[ "${backup_type}" == "b2" ]]; then
    "${DUPLICACY_BIN}" set -storage "${storage_name}" -key b2_id \
      -value "${STORAGE_TARGET_1_B2_ID}" 2>&1 | log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Failed to set BackBlaze key ID for ${SERVICE} service."
    fi

    "${DUPLICACY_BIN}" set -storage "${storage_name}" -key b2_key \
      -value "${STORAGE_TARGET_1_B2_KEY}" 2>&1 | log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Failed to set BackBlaze application key for ${SERVICE} service."
    fi

  elif [[ "${backup_type}" == "s3" ]]; then
    "${DUPLICACY_BIN}" set -storage "${storage_name}" -key s3_id \
      -value "${STORAGE_TARGET_1_S3_ID}" 2>&1 | log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Failed to set S3 ID for ${SERVICE} service."
    fi

    "${DUPLICACY_BIN}" set -storage "${storage_name}" -key s3_secret \
      -value "${STORAGE_TARGET_1_S3_SECRET}" 2>&1 | log_output
    exit_status="${PIPESTATUS[0]}"
    if [ "${exit_status}" -ne 0 ]; then
      handle_error "Failed to set S3 secret for ${SERVICE} service."
    fi
  fi

  "${DUPLICACY_BIN}" set -storage "${storage_name}" -key password -value "${STORAGE_PASSWORD}" 2>&1 | log_output
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Failed to set storage password for ${SERVICE} service."
  fi

  "${DUPLICACY_BIN}" set -storage "${storage_name}" -key rsa_passphrase \
    -value "${RSA_PASSPHRASE}" 2>&1 | log_output
  exit_status="${PIPESTATUS[0]}"
  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Failed to set RSA passphrase for ${SERVICE} service."
  fi

  if ! duplicacy_verify "${storage_name}"; then
    handle_error "Primary storage verification failed for ${SERVICE} service."
  fi
  log_message "INFO" "Primary storage verified for ${SERVICE} service."

  duplicacy_filters

  log_message "INFO" "Starting backup to ${STORAGE_TARGET_1_NAME} for ${SERVICE} service."

  # Run backup in background to allow stop monitoring
  "${DUPLICACY_BIN}" backup -storage "${storage_name}" -stats -threads "${DUPLICACY_THREADS}" 2>&1 | log_output &
  local backup_pid=$!

  # Monitor for stop requests while backup runs
  while kill -0 "${backup_pid}" 2>/dev/null; do
    if is_stop_requested; then
      log_message "INFO" "Stop requested during duplicacy backup for ${SERVICE} service."
      # Kill the duplicacy backup process
      pkill -TERM -P "${backup_pid}" 2>/dev/null || true
      kill -TERM "${backup_pid}" 2>/dev/null || true
      # Wait briefly for graceful termination
      sleep 2
      # Force kill if still running
      if kill -0 "${backup_pid}" 2>/dev/null; then
        pkill -KILL -P "${backup_pid}" 2>/dev/null || true
        kill -KILL "${backup_pid}" 2>/dev/null || true
      fi
      wait "${backup_pid}" 2>/dev/null || true
      log_message "INFO" "Duplicacy backup stopped for ${SERVICE} service."
      return 1
    fi
    sleep 1
  done

  # Get the exit status
  wait "${backup_pid}"
  exit_status=$?

  if [ "${exit_status}" -ne 0 ]; then
    handle_error "Backup to ${STORAGE_TARGET_1_NAME} failed for ${SERVICE} service."
  fi

  log_message "INFO" "Backup to ${STORAGE_TARGET_1_NAME} completed for ${SERVICE} service."
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
      local storage_url

      storage_id="${i}"
      backup_type_var="STORAGE_TARGET_${storage_id}_TYPE"
      backup_type="${!backup_type_var}"
      storage_name_var="STORAGE_TARGET_${storage_id}_NAME"
      storage_name="$(sanitize_storage_name "${!storage_name_var}")"

      # Export the DUPLICACY_<NAME>_* credentials the duplicacy binary reads.
      export_duplicacy_storage_secrets "${storage_id}"

      storage_url="$(build_storage_url "${storage_id}")"
      if [[ -z "${storage_url}" ]]; then
        handle_error "${backup_type} is not a supported backup type. Please edit config.sh to only reference supported backup types."
        continue
      fi

      cd "${SERVICE_DIR}" || handle_error "Failed to change to directory ${SERVICE_DIR}."

      log_message "INFO" "Adding ${backup_type} storage ${storage_name} for ${SERVICE} service."
      "${DUPLICACY_BIN}" add -e -copy "${primary_storage_name}" -bit-identical -key \
        "${DUPLICACY_RSA_PUBLIC_KEY_FILE}" "${storage_name}" "${DUPLICACY_SNAPSHOT_ID}" \
        "${storage_url}" 2>&1 | \
        log_output
      exit_status="${PIPESTATUS[0]}"
      if [ "${exit_status}" -ne 0 ]; then
        handle_error "Failed to add ${backup_type} storage ${storage_name} for ${SERVICE} service."
      fi

      # Persist type-specific credentials to the storage's preferences file.
      if [[ "${backup_type}" == "sftp" ]]; then
        "${DUPLICACY_BIN}" set -storage "${storage_name}" -key ssh_key_file -value "${DUPLICACY_SSH_PRIVATE_KEY_FILE}" 2>&1 | log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Failed to set SSH key file for SFTP storage ${storage_name} for ${SERVICE} service. Verify the SSH key file path and permissions."
        fi

      elif [[ "${backup_type}" == "b2" ]]; then
        local config_b2_id_var="STORAGE_TARGET_${storage_id}_B2_ID"
        local config_b2_key_var="STORAGE_TARGET_${storage_id}_B2_KEY"

        "${DUPLICACY_BIN}" set -storage "${storage_name}" -key b2_id \
          -value "${!config_b2_id_var}" 2>&1 | log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Failed to set BackBlaze key ID for storage ${storage_name} for ${SERVICE} service."
        fi

        "${DUPLICACY_BIN}" set -storage "${storage_name}" -key b2_key \
          -value "${!config_b2_key_var}" 2>&1 | log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Failed to set BackBlaze application key for storage ${storage_name} for ${SERVICE} service."
        fi

      elif [[ "${backup_type}" == "s3" ]]; then
        local config_s3_id_var="STORAGE_TARGET_${storage_id}_S3_ID"
        local config_s3_secret_var="STORAGE_TARGET_${storage_id}_S3_SECRET"

        "${DUPLICACY_BIN}" set -storage "${storage_name}" -key s3_id \
          -value "${!config_s3_id_var}" 2>&1 | log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Failed to set S3 ID for storage ${storage_name} for ${SERVICE} service."
        fi

        "${DUPLICACY_BIN}" set -storage "${storage_name}" -key s3_secret \
          -value "${!config_s3_secret_var}" 2>&1 | log_output
        exit_status="${PIPESTATUS[0]}"
        if [ "${exit_status}" -ne 0 ]; then
          handle_error "Failed to set S3 secret for storage ${storage_name} for ${SERVICE} service."
        fi
      fi

      "${DUPLICACY_BIN}" set -storage "${storage_name}" -key password \
        -value "${STORAGE_PASSWORD}" 2>&1 | log_output
      exit_status="${PIPESTATUS[0]}"
      if [ "${exit_status}" -ne 0 ]; then
        handle_error "Failed to set storage password for ${SERVICE} service."
      fi

      "${DUPLICACY_BIN}" set -storage "${storage_name}" -key rsa_passphrase \
        -value "${RSA_PASSPHRASE}" 2>&1 | log_output
      exit_status="${PIPESTATUS[0]}"
      if [ "${exit_status}" -ne 0 ]; then
        handle_error "Failed to set RSA passphrase for ${SERVICE} service."
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

      log_message "INFO" "Copying backup to ${storage_name} storage."

      "${DUPLICACY_BIN}" copy -from "${primary_storage_name}" -to "${storage_name}" \
        -key "${DUPLICACY_RSA_PRIVATE_KEY_FILE}" -threads "${DUPLICACY_THREADS}" -download-threads "${DUPLICACY_THREADS}" 2>&1 | log_output
      exit_status="${PIPESTATUS[0]}"

      if [ "${exit_status}" -ne 0 ]; then
        handle_error "Copy to ${storage_name} storage failed."
      else
        log_message "INFO" "Copy to ${storage_name} storage completed."
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
    handle_error "Storage check failed for ${storage_name}."
  else
    log_message "INFO" "Storage check completed for ${storage_name}."
  fi

  if [[ "$(echo "${ROTATE_BACKUPS}" | tr '[:upper:]' '[:lower:]')" == "true" ]]; then
    declare -a PRUNE_KEEP_ARRAY
    PRUNE_KEEP_ARRAY=()
    read -r -a PRUNE_KEEP_ARRAY <<< "${PRUNE_KEEP}"

    "${DUPLICACY_BIN}" prune -all -storage "${storage_name}" "${PRUNE_KEEP_ARRAY[@]}" -exhaustive -threads "${DUPLICACY_THREADS}" 2>&1 | log_output
    exit_status="${PIPESTATUS[0]}"
    if [[ "${exit_status}" -ne 0 ]]; then
      handle_error "Prune failed for ${storage_name} storage. Review the Duplicacy logs for details."
    else
      log_message "INFO" "Prune completed for ${storage_name} storage."
    fi
  fi

  unset SERVICE
}
