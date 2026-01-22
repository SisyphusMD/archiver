#!/bin/bash

LOGGING_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${ERROR_CORE}"
source_if_not_sourced "${NOTIFICATION_FEATURE}"

log_message() {
  local log_level="${1}"
  local message="${2}"
  local timestamp
  local service_name
  local target_log_file

  timestamp="$(date +'%Y-%m-%d %H:%M:%S')"
  service_name="${SERVICE:-archiver}"
  target_log_file="${LOG_DIR}/archiver.log"

  echo "[${timestamp}] [${log_level}] [Service: ${service_name}] ${message}" >> "${target_log_file}" || \
    handle_error "Failed to log message for ${service_name} service to ${target_log_file}. Check if the log file is writable and disk space is available."

  if [[ "${log_level}" == "WARNING" || "${log_level}" == "ERROR" ]]; then
    echo "[${timestamp}] [${log_level}] [Service: ${service_name}] ${message}"
  fi

  if [[ "${log_level}" == "ERROR" ]]; then
    notify "Backup Error" "[${service_name}] ${message}"
  fi
}

# Pipes stdin to log_message line by line
log_output() {
  local log_level="${1:-INFO}"

  while IFS= read -r line; do
    log_message "${log_level}" "${line}"
  done
}

# Creates timestamped log file, symlinks archiver.log to it, deletes logs >7 days old
rotate_logs() {
  local new_log_file
  local datetime
  local relative_log_path

  mkdir -p "${OLD_LOG_DIR}" || handle_error "Unable to create log directory ${OLD_LOG_DIR}."

  datetime="$(date +'%Y-%m-%d_%H%M%S')"
  new_log_file="${OLD_LOG_DIR}/archiver-${datetime}.log"

  touch "${new_log_file}" || handle_error "Could not create log file ${new_log_file}."
  log_message "INFO" "Log file created: ${new_log_file}."

  relative_log_path="$(basename "${OLD_LOG_DIR}")/$(basename "${new_log_file}")"
  ln -sf "${relative_log_path}" "${LOG_DIR}/archiver.log" || \
    handle_error "Could not update/create symlink for 'archiver.log' to ${new_log_file}."
  log_message "INFO" "Symlink 'archiver.log' updated to point to ${new_log_file}."

  find "${OLD_LOG_DIR}" -name "*.log" -type f -mtime +7 -exec rm -f {} \; || \
    handle_error "Failed to delete old 'archiver' log files."
  log_message "INFO" "Old log files deleted (>7 days)."
}

# Converts unix timestamp to human-readable format
format_timestamp() {
  local timestamp="${1}"
  date -d "@${timestamp}" +'%Y-%m-%d %H:%M:%S'
}

# Converts seconds to readable duration (e.g., "2 days, 3 hours, and 15 minutes")
format_duration() {
  local seconds="${1}"
  local days=$((seconds / 86400))
  local hours=$((seconds % 86400 / 3600))
  local minutes=$((seconds % 3600 / 60))
  local secs=$((seconds % 60))
  local parts=()

  if (( days > 0 )); then
    parts+=("${days} day$( (( days != 1 )) && echo "s" )")
  fi

  if (( hours > 0 )); then
    parts+=("${hours} hour$( (( hours != 1 )) && echo "s" )")
  fi

  if (( minutes > 0 )); then
    parts+=("${minutes} minute$( (( minutes != 1 )) && echo "s" )")
  fi

  if (( secs > 0 )); then
    parts+=("${secs} second$( (( secs != 1 )) && echo "s" )")
  fi

  local count=${#parts[@]}
  if (( count == 0 )); then
    echo "0 seconds"
  elif (( count == 1 )); then
    echo "${parts[0]}"
  elif (( count == 2 )); then
    echo "${parts[0]} and ${parts[1]}"
  else
    local result=""
    for (( i=0; i<count-1; i++ )); do
      result+="${parts[i]}, "
    done
    result+="and ${parts[count-1]}"
    echo "${result}"
  fi
}
