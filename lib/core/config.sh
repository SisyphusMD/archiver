#!/bin/bash

# Source common.sh (must use regular source for the first file)
if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOGGING_CORE}"
source "${CONFIG_FILE}"

CONFIG_SH_SOURCED=true
DUPLICACY_THREADS="${DUPLICACY_THREADS:-4}"

expand_service_directories() {
  local expanded_service_directories=()

  if [[ -z "${SERVICE_DIRECTORIES[*]}" ]]; then
    handle_error "SERVICE_DIRECTORIES is not defined or is empty. Please set the SERVICE_DIRECTORIES array."
    exit 1
  fi

  for pattern in "${SERVICE_DIRECTORIES[@]}"; do
    for dir in ${pattern}; do
      if [[ -d "${dir}" ]]; then
        expanded_service_directories+=("${dir%/}")
      fi
    done
  done

  export EXPANDED_SERVICE_DIRECTORIES=("${expanded_service_directories[@]}")
}

count_storage_targets() {
  local count=0

  while true; do
    count=$((count + 1))
    local var_name="STORAGE_TARGET_${count}_NAME"

    if [[ -z "${!var_name}" ]]; then
      count=$((count - 1))
      break
    fi
  done

  if [[ $count -eq 0 ]]; then
    handle_error "No Storage Targets specified. Please edit config.sh and specify at least one storage target."
    exit 1
  else
    log_message "INFO" "$count backup targets configured."
    export STORAGE_TARGET_COUNT=$count
  fi
}

verify_target_settings() {
  for i in $(seq 1 "${STORAGE_TARGET_COUNT}"); do
    local storage_id="${i}"
    local storage_name_var="STORAGE_TARGET_${storage_id}_NAME"
    local storage_name="${!storage_name_var}"
    local storage_type_var="STORAGE_TARGET_${storage_id}_TYPE"
    local storage_type="${!storage_type_var}"

    if [[ -z "${storage_name}" || -z "${storage_type}" ]]; then
      handle_error "Missing storage name or type for storage target ${storage_id}. Please check your configuration."
      exit 1
    fi

    if [[ "${storage_type}" == "local" ]]; then
      local config_var="STORAGE_TARGET_${storage_id}_LOCAL_PATH"
      if [[ -z "${!config_var}" ]]; then
        handle_error "Missing LOCAL_PATH configuration for the '${storage_name}' storage. Please check your 'STORAGE_TARGET_${storage_id}' configuration."
        exit 1
      fi

    elif [[ "${storage_type}" == "sftp" ]]; then
      local config_vars=("SFTP_URL" "SFTP_PORT" "SFTP_USER" "SFTP_PATH" "SFTP_KEY_FILE")
      for var in "${config_vars[@]}"; do
        local config_var="STORAGE_TARGET_${storage_id}_${var}"
        if [[ -z "${!config_var}" ]]; then
          handle_error "Missing SFTP configuration setting '${var}' for the '${storage_name}' storage. Please check your 'STORAGE_TARGET_${storage_id}' configuration."
          exit 1
        fi
      done

    elif [[ "${storage_type}" == "b2" ]]; then
      local config_vars=("B2_BUCKETNAME" "B2_ID" "B2_KEY")
      for var in "${config_vars[@]}"; do
        local config_var="STORAGE_TARGET_${storage_id}_${var}"
        if [[ -z "${!config_var}" ]]; then
          handle_error "Missing B2 configuration setting '${var}' for the '${storage_name}' storage. Please check your 'STORAGE_TARGET_${storage_id}' configuration."
          exit 1
        fi
      done

    elif [[ "${storage_type}" == "s3" ]]; then
      local config_vars=("S3_BUCKETNAME" "S3_ENDPOINT" "S3_ID" "S3_SECRET")
      for var in "${config_vars[@]}"; do
        local config_var="STORAGE_TARGET_${storage_id}_${var}"
        if [[ -z "${!config_var}" ]]; then
          handle_error "Missing S3 configuration setting '${var}' for the '${storage_name}' storage. Please check your 'STORAGE_TARGET_${storage_id}' configuration."
          exit 1
        fi
      done

    else
      handle_error "The storage type '${storage_type}' is not supported. Please check your '${storage_type_var}' configuration."
      exit 1
    fi
  done
}

check_required_secrets() {
  local secrets=("STORAGE_PASSWORD" "RSA_PASSPHRASE")

  for secret in "${secrets[@]}"; do
    if [[ -z "${!secret}" ]]; then
      handle_error "The required secret '${secret}' is not set. Please edit config.sh and specify a value for '${secret}'."
      exit 1
    fi
  done

  log_message "INFO" "All required secrets are set."
}

check_notification_config() {
  local notification_service_lower
  notification_service_lower=$(echo "${NOTIFICATION_SERVICE}" | tr '[:upper:]' '[:lower:]')

  if [[ "$notification_service_lower" = "pushover" ]]; then
    local settings=("PUSHOVER_USER_KEY" "PUSHOVER_API_TOKEN")
    for setting in "${settings[@]}"; do
      if [[ -z "${!setting}" ]]; then
        handle_error "Notification service is set to '${NOTIFICATION_SERVICE}', but the necessary setting '${setting}' is not set. Please edit config.sh and specify a value for '${setting}'."
        exit 1
      fi
    done
    log_message "INFO" "All required '${NOTIFICATION_SERVICE}' settings are set."
  else
    NOTIFICATION_SERVICE="None"
    PUSHOVER_USER_KEY=""
    PUSHOVER_API_TOKEN=""
  fi

  export NOTIFICATION_SERVICE
  export PUSHOVER_USER_KEY
  export PUSHOVER_API_TOKEN
}

check_backup_rotation_settings() {
  if [[ -z "${ROTATE_BACKUPS}" ]]; then
    ROTATE_BACKUPS="true"
  fi

  if [ -z "${PRUNE_KEEP}" ]; then
    PRUNE_KEEP="-keep 0:180 -keep 30:30 -keep 7:7 -keep 1:1"
  fi

  if [[ "${ROTATION_OVERRIDE}" == "prune" ]]; then
    log_message "INFO" "Prune flag set, will perform backup rotation on this run."
    ROTATE_BACKUPS="true"
  elif [[ "${ROTATION_OVERRIDE}" == "retain" ]]; then
    log_message "INFO" "Retain flag set, will not perform backup rotation on this run."
    ROTATE_BACKUPS="false"
  fi

  export ROTATE_BACKUPS
  export PRUNE_KEEP

  log_message "INFO" "Backup rotation settings: ROTATE_BACKUPS=${ROTATE_BACKUPS}, PRUNE_KEEP=${PRUNE_KEEP}"
}

verify_config(){
  expand_service_directories
  count_storage_targets
  verify_target_settings
  check_required_secrets
  check_notification_config
  check_backup_rotation_settings
}
