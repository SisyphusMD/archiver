#!/bin/bash
# Loads and validates user configuration from config.sh

CONFIG_LOADER_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOGGING_CORE}"
source "${CONFIG_FILE}"
DUPLICACY_THREADS="${DUPLICACY_THREADS:-4}"

# Converts storage names to valid Bash variable format
# Replaces special chars with underscores, prepends _ if starts with digit
sanitize_storage_name() {
  local name="${1}"
  local sanitized
  sanitized="$(printf "%s" "${name}" | tr -c '[:alnum:]_' '_' | sed 's/^[0-9]/_&/')"

  # Still logged via log_message (it writes to the archiver log file internally), but
  # redirect log_message's STDOUT to stderr: this function returns the sanitized name
  # ON STDOUT (callers do `name="$(sanitize_storage_name …)"`), and auto-restore.sh
  # redefines log_message to echo to stdout — without this redirect that echo would be
  # captured and corrupt the returned name.
  if [[ "${name}" != "${sanitized}" ]]; then
    log_message "WARN" "Storage name ${name} was sanitized to ${sanitized} for use in Duplicacy commands and environment variables." >&2
  fi

  printf "%s" "${sanitized}"
}

# Echoes the Duplicacy storage URL for a storage-target numeric ID on STDOUT. Pure
# string construction from that target's STORAGE_TARGET_<id>_* config (no side effects,
# safe inside "$(...)"), so the per-type URL format lives in exactly one place. Returns
# non-zero and prints nothing for an unsupported type.
build_storage_url() {
  local storage_id="${1}"
  local storage_type_var="STORAGE_TARGET_${storage_id}_TYPE"
  local storage_type="${!storage_type_var}"

  case "${storage_type}" in
    local)
      local local_path_var="STORAGE_TARGET_${storage_id}_LOCAL_PATH"
      printf "%s" "${!local_path_var}"
      ;;
    sftp)
      local sftp_user_var="STORAGE_TARGET_${storage_id}_SFTP_USER"
      local sftp_url_var="STORAGE_TARGET_${storage_id}_SFTP_URL"
      local sftp_port_var="STORAGE_TARGET_${storage_id}_SFTP_PORT"
      local sftp_path_var="STORAGE_TARGET_${storage_id}_SFTP_PATH"
      printf "sftp://%s@%s:%s//%s" "${!sftp_user_var}" "${!sftp_url_var}" "${!sftp_port_var}" "${!sftp_path_var}"
      ;;
    b2)
      local b2_bucketname_var="STORAGE_TARGET_${storage_id}_B2_BUCKETNAME"
      printf "b2://%s" "${!b2_bucketname_var}"
      ;;
    s3)
      local s3_endpoint_var="STORAGE_TARGET_${storage_id}_S3_ENDPOINT"
      local s3_bucketname_var="STORAGE_TARGET_${storage_id}_S3_BUCKETNAME"
      local s3_region_var="STORAGE_TARGET_${storage_id}_S3_REGION"
      local s3_region
      s3_region="${!s3_region_var}"
      s3_region="${s3_region:-none}"
      printf "s3://%s@%s/%s" "${s3_region}" "${!s3_endpoint_var}" "${!s3_bucketname_var}"
      ;;
    *)
      return 1
      ;;
  esac
}

# Exports the DUPLICACY_<NAME>_* credential env vars the duplicacy binary reads for a
# storage-target numeric ID: the storage password (every type) plus the type-specific
# secrets. Centralizes the sanitize -> UPPER -> DUPLICACY_<NAME>_ mapping that the two
# do-spaces field incidents (0.8.10 hyphen, 0.8.11 log-leak) traced back to. Exports as
# a side effect, so call it as a plain statement — NOT inside "$(...)", whose subshell
# would discard the exports. Unknown types export only the password; the caller's own
# type dispatch reports the unsupported type.
export_duplicacy_storage_secrets() {
  local storage_id="${1}"
  local storage_name_var="STORAGE_TARGET_${storage_id}_NAME"
  local storage_type_var="STORAGE_TARGET_${storage_id}_TYPE"
  local storage_name
  local storage_name_upper
  local storage_type
  local password_var

  storage_name="$(sanitize_storage_name "${!storage_name_var}")"
  storage_name_upper="$(echo "${storage_name}" | tr '[:lower:]' '[:upper:]')"
  storage_type="${!storage_type_var}"

  password_var="DUPLICACY_${storage_name_upper}_PASSWORD"
  export "${password_var}"="${STORAGE_PASSWORD}"

  case "${storage_type}" in
    local)
      # local storage takes no credentials
      ;;
    sftp)
      local ssh_key_file_var="DUPLICACY_${storage_name_upper}_SSH_KEY_FILE"
      export "${ssh_key_file_var}"="${DUPLICACY_SSH_PRIVATE_KEY_FILE}"
      ;;
    b2)
      local config_b2_id_var="STORAGE_TARGET_${storage_id}_B2_ID"
      local config_b2_key_var="STORAGE_TARGET_${storage_id}_B2_KEY"
      local duplicacy_b2_id_var="DUPLICACY_${storage_name_upper}_B2_ID"
      local duplicacy_b2_key_var="DUPLICACY_${storage_name_upper}_B2_KEY"
      export "${duplicacy_b2_id_var}"="${!config_b2_id_var}"
      export "${duplicacy_b2_key_var}"="${!config_b2_key_var}"
      ;;
    s3)
      local config_s3_id_var="STORAGE_TARGET_${storage_id}_S3_ID"
      local config_s3_secret_var="STORAGE_TARGET_${storage_id}_S3_SECRET"
      local duplicacy_s3_id_var="DUPLICACY_${storage_name_upper}_S3_ID"
      local duplicacy_s3_secret_var="DUPLICACY_${storage_name_upper}_S3_SECRET"
      export "${duplicacy_s3_id_var}"="${!config_s3_id_var}"
      export "${duplicacy_s3_secret_var}"="${!config_s3_secret_var}"
      ;;
  esac
}

# Expands glob patterns in SERVICE_DIRECTORIES (e.g., /srv/*/ -> /srv/app1/ /srv/app2/)
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
    log_message "INFO" "${count} backup targets configured."
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
        handle_error "Missing LOCAL_PATH configuration for the ${storage_name} storage. Please check your 'STORAGE_TARGET_${storage_id}' configuration."
        exit 1
      fi

    elif [[ "${storage_type}" == "sftp" ]]; then
      local config_vars=("SFTP_URL" "SFTP_PORT" "SFTP_USER" "SFTP_PATH")
      for var in "${config_vars[@]}"; do
        local config_var="STORAGE_TARGET_${storage_id}_${var}"
        if [[ -z "${!config_var}" ]]; then
          handle_error "Missing SFTP configuration setting ${var} for the ${storage_name} storage. Please check your 'STORAGE_TARGET_${storage_id}' configuration."
          exit 1
        fi
      done

    elif [[ "${storage_type}" == "b2" ]]; then
      local config_vars=("B2_BUCKETNAME" "B2_ID" "B2_KEY")
      for var in "${config_vars[@]}"; do
        local config_var="STORAGE_TARGET_${storage_id}_${var}"
        if [[ -z "${!config_var}" ]]; then
          handle_error "Missing B2 configuration setting ${var} for the ${storage_name} storage. Please check your 'STORAGE_TARGET_${storage_id}' configuration."
          exit 1
        fi
      done

    elif [[ "${storage_type}" == "s3" ]]; then
      local config_vars=("S3_BUCKETNAME" "S3_ENDPOINT" "S3_ID" "S3_SECRET")
      for var in "${config_vars[@]}"; do
        local config_var="STORAGE_TARGET_${storage_id}_${var}"
        if [[ -z "${!config_var}" ]]; then
          handle_error "Missing S3 configuration setting ${var} for the ${storage_name} storage. Please check your 'STORAGE_TARGET_${storage_id}' configuration."
          exit 1
        fi
      done

    else
      handle_error "The storage type ${storage_type} is not supported. Please check your ${storage_type_var} configuration."
      exit 1
    fi
  done
}

check_required_secrets() {
  local secrets=("STORAGE_PASSWORD" "RSA_PASSPHRASE")

  for secret in "${secrets[@]}"; do
    if [[ -z "${!secret}" ]]; then
      handle_error "The required secret ${secret} is not set. Please edit config.sh and specify a value for ${secret}."
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
        handle_error "Notification service is set to ${NOTIFICATION_SERVICE}, but the necessary setting ${setting} is not set. Please edit config.sh and specify a value for ${setting}."
        exit 1
      fi
    done
    log_message "INFO" "All required ${NOTIFICATION_SERVICE} settings are set."
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

  log_message "INFO" "Backup rotation settings: ROTATE_BACKUPS=${ROTATE_BACKUPS}, PRUNE_KEEP=${PRUNE_KEEP}."
}

verify_config(){
  expand_service_directories
  count_storage_targets
  verify_target_settings
  check_required_secrets
  check_notification_config
  check_backup_rotation_settings
}
