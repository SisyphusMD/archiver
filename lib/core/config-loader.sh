#!/bin/bash
# Loads and validates user configuration from config.sh

CONFIG_LOADER_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOGGING_CORE}"

# ------------------------------------------------------------------------------
# Layered configuration load
#
# Config is resolved from three sources, in increasing precedence:
#   1. the decrypted bundle's config.sh              (optional baseline; the cold-restore path)
#   2. plain environment variables                   (non-secret config only)
#   3. files: <NAME>_FILE, else ${SECRETS_DIR}/<name>  (secrets only — never raw env)
#
# This lets a deployment migrate off the bundle one value at a time: set an env var or
# mount a secret file and it shadows the bundle; once every value is shadowed the bundle
# can be dropped (pure env-native, e.g. a k8s ConfigMap + Secret). With no env/secret
# overrides and a bundle present, the result is byte-identical to sourcing config.sh alone.
# ------------------------------------------------------------------------------

SECRETS_DIR="${SECRETS_DIR:-/run/secrets}"

# Secrets must come from the bundle or a file, never a plain env var (which would leak via
# /proc and `docker inspect`). Drop any passed in the environment before loading, so the
# only remaining sources are config.sh and the secret files.
purge_raw_env_secrets() {
  local v
  unset STORAGE_PASSWORD RSA_PASSPHRASE PUSHOVER_USER_KEY PUSHOVER_API_TOKEN
  while IFS= read -r v; do
    unset "${v}"
  done < <(compgen -v | grep -E '^STORAGE_TARGET_[0-9]+_(B2_ID|B2_KEY|S3_ID|S3_SECRET)$')
}

# Non-secret config passed via the environment is the override layer that must win over the
# bundle. Capture it now, because sourcing config.sh below would otherwise clobber it.
declare -gA ENV_CONFIG_OVERRIDES=()
snapshot_env_overrides() {
  local v
  while IFS= read -r v; do
    ENV_CONFIG_OVERRIDES["${v}"]="${!v}"
  done < <(compgen -v | grep -E '^(SERVICE_DIRECTORIES|ROTATE_BACKUPS|PRUNE_KEEP|DUPLICACY_THREADS|NOTIFICATION_SERVICE|STORAGE_TARGET_[0-9]+_(NAME|TYPE|LOCAL_PATH|SFTP_URL|SFTP_PORT|SFTP_USER|SFTP_PATH|B2_BUCKETNAME|S3_BUCKETNAME|S3_ENDPOINT|S3_REGION))$')
}

apply_env_overrides() {
  local v
  for v in "${!ENV_CONFIG_OVERRIDES[@]}"; do
    unset "${v}"                                   # drop a possible array baseline before re-setting as scalar
    printf -v "${v}" '%s' "${ENV_CONFIG_OVERRIDES[${v}]}"
  done
}

# Populate one secret from a file only: ${VAR}_FILE if set, else ${SECRETS_DIR}/<var lower>.
# Overrides the bundle value only when such a file exists, so a bundle-provided secret
# survives when nothing is mounted. $(<file) trims the trailing newline editors/tools add.
resolve_secret() {
  local var="${1}"
  local file_var="${var}_FILE"
  local path="${!file_var:-${SECRETS_DIR}/$(printf '%s' "${var}" | tr '[:upper:]' '[:lower:]')}"
  if [[ -f "${path}" ]]; then
    printf -v "${var}" '%s' "$(<"${path}")"
  fi
}

resolve_secret_files() {
  resolve_secret STORAGE_PASSWORD
  resolve_secret RSA_PASSPHRASE
  resolve_secret PUSHOVER_USER_KEY
  resolve_secret PUSHOVER_API_TOKEN

  local n=1 name_var type_var type
  while :; do
    name_var="STORAGE_TARGET_${n}_NAME"
    [[ -z "${!name_var}" ]] && break
    type_var="STORAGE_TARGET_${n}_TYPE"
    type="${!type_var}"
    case "${type}" in
      b2) resolve_secret "STORAGE_TARGET_${n}_B2_ID";  resolve_secret "STORAGE_TARGET_${n}_B2_KEY" ;;
      s3) resolve_secret "STORAGE_TARGET_${n}_S3_ID";  resolve_secret "STORAGE_TARGET_${n}_S3_SECRET" ;;
    esac
    n=$((n + 1))
  done
}

# SERVICE_DIRECTORIES may be a bash array (legacy bundle config.sh) or a colon/newline
# delimited scalar (env-native and new config.sh). Normalize the scalar into the array
# that expand_service_directories() consumes; leave a real array untouched.
normalize_service_directories() {
  case "${SERVICE_DIRECTORIES@a}" in
    *a*) return 0 ;;                               # already an array
  esac
  local raw="${SERVICE_DIRECTORIES:-}"
  [[ -z "${raw}" ]] && return 0
  raw="${raw//$'\n'/:}"                            # accept newline as a separator too (YAML block scalars)
  local parts=() out=() p
  IFS=':' read -ra parts <<<"${raw}"
  for p in "${parts[@]}"; do
    [[ -n "${p}" ]] && out+=("${p}")
  done
  unset SERVICE_DIRECTORIES
  SERVICE_DIRECTORIES=("${out[@]}")
}

purge_raw_env_secrets
snapshot_env_overrides
[[ -f "${CONFIG_FILE}" ]] && source "${CONFIG_FILE}"
apply_env_overrides
resolve_secret_files
normalize_service_directories
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
    handle_error "SERVICE_DIRECTORIES is not set. Provide it via config.sh or the SERVICE_DIRECTORIES environment variable (colon-delimited)."
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
    handle_error "No storage targets specified. Provide at least one via config.sh or the STORAGE_TARGET_N_* environment variables."
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
      handle_error "The required secret ${secret} is not set. Provide it via the bundle, ${secret}_FILE, or ${SECRETS_DIR}/$(echo "${secret}" | tr '[:upper:]' '[:lower:]')."
      exit 1
    fi
  done

  # Duplicacy rejects a storage password shorter than 8 characters, but only at init time
  # with an opaque failure. Catch it here with a clear message (env-native users set this).
  if (( ${#STORAGE_PASSWORD} < 8 )); then
    handle_error "STORAGE_PASSWORD must be at least 8 characters (a Duplicacy requirement); got ${#STORAGE_PASSWORD}."
    exit 1
  fi

  log_message "INFO" "All required secrets are set."
}

check_notification_config() {
  local notification_service_lower
  notification_service_lower=$(echo "${NOTIFICATION_SERVICE}" | tr '[:upper:]' '[:lower:]')

  if [[ "$notification_service_lower" = "pushover" ]]; then
    local settings=("PUSHOVER_USER_KEY" "PUSHOVER_API_TOKEN")
    for setting in "${settings[@]}"; do
      if [[ -z "${!setting}" ]]; then
        handle_error "Notification service is set to ${NOTIFICATION_SERVICE}, but ${setting} is not set. Provide it via config.sh or your env-native configuration."
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
