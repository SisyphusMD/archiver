# Logs a message to the archiver's log file with a timestamp.
# Parameters:
#   1. Log Level: The severity level of the log message (e.g., INFO, WARNING, ERROR).
#   2. Message: The log message to be recorded.
# Output:
#   Writes the log message to the archiver's log file. No console output except for WARNING or ERROR.
log_message() {
  local log_level
  local message
  local timestamp
  local target_log_file

  log_level="${1}"
  message="${2}"
  timestamp="$(date +'%Y-%m-%d %H:%M:%S')"
  target_log_file="${3:-${ARCHIVER_LOG_FILE}}" # Use ARCHIVER_LOG_FILE by default if no log file is specified

  echo "[${timestamp}] [${log_level}] [Service: ${SERVICE}] ${message}" >> "${target_log_file}" || \
    handle_error "Failed to log message for ${SERVICE} service to ${target_log_file}. Check if the log file is writable and disk space is available."

  # Print WARNING and ERROR messages to the terminal
  if [[ "${log_level}" == "WARNING" || "${log_level}" == "ERROR" ]]; then
    echo "[${timestamp}] [${log_level}] [Service: ${SERVICE}] ${message}"
  fi

  # Send ERROR messages as a Pushover notification
  if [[ "${log_level}" == "ERROR" ]]; then
    send_pushover_notification "Archiver Error" "[${timestamp}] [${log_level}] [Service: ${SERVICE}] ${message}"
  fi
}

# Logs output from backup operations, capturing both stdout and stderr streams.
# Parameters:
#   1. Output Message: The output message from the backup operation to be logged.
#   2. Log File (optional): Specifies the log file to write to. Defaults to the main archiver log file if not provided.
# Output:
#   Writes the provided output message to the specified log file or the default archiver log file. No console output.
log_output() {
  local target_log_file
  local log_level

  target_log_file="${1}"
  log_level="${2:-INFO}" # Use INFO log level by default if no log level is specified

  while IFS= read -r line; do
    log_message "${log_level}" "${line}" "${target_log_file}"
  done
}

# Rotates log files for the Archiver, ensuring log management adheres to a retention policy.
# Parameters:
#   1. Log File Path: The path to the log file to rotate.
# Output:
#   Old log files are archived or deleted according to the retention policy. No direct output.
rotate_logs() {
  # Enforces a retention policy by keeping only the specified maximum number of backup versions.
  # Parameters:
  #   1. Max Versions: The maximum number of backup versions to retain.
  #   2. Backup Directory: The directory containing the backup files to apply the retention policy to.
  # Output:
  #   Deletes older backup files exceeding the specified maximum count, preserving only the most recent versions.
  keep_max_versions() {
    local log_prefix
    local max_versions
    local log_files
    local num_files

    log_prefix="${1}"
    max_versions="${2}"

    # Find all log files matching the prefix and sort them in reverse order
    mapfile -t log_files < <(find "${ARCHIVER_LOG_DIR}" -name "${log_prefix}*.log" -type f -print0 | sort -rz)
    num_files="${#log_files[@]}"

    # Check if the number of log files exceeds the maximum allowed
    if [ "${num_files}" -gt "${max_versions}" ]; then
      # Iterate over log files starting from the (max_versions - 1) index
      # and remove any excess log files beyond the maximum allowed versions
      for (( i = max_versions - 1; i < num_files; i++ )); do
        rm -f "${log_files[i]}" || handle_error "Failed to remove file ${log_files[i]} for ${SERVICE} service. Verify file permissions and that the file is not in use."
        log_message "INFO" "Removed old log file: ${log_files[i]}"
      done
    fi
  }

  local log_file
  local log_name
  local log_type
  local max_versions

  log_file="${1}"
  log_name="$(basename "${log_file}")"
  log_type="${log_name%.*}"
  max_versions=7

  # Check if the log directory exists, and create it if it doesn't
  [ -d "${ARCHIVER_LOG_DIR}" ] || mkdir -p "${ARCHIVER_LOG_DIR}"

  # Rotate the log file if needed
  if [ -f "${log_file}" ]; then
    local creation_date

    # Extract the creation date of the log file
    creation_date="$(date -r "${log_file}" +'%Y-%m-%d')"

    # Check if the log file was created on a different date than today
    if [ "${creation_date}" != "${DATE}" ]; then
      local new_log_file

      # Generate a new log file name based on the log type and current date
      new_log_file="${ARCHIVER_LOG_DIR}/${log_type}-${creation_date}.log"

      # Rename the existing log file to the new name
      mv "${log_file}" "${new_log_file}" || \
        handle_error "Could not rename the log file from '${log_file}' to '${new_log_file}' for the ${SERVICE} service. Check file permissions and path validity."
      log_message "INFO" "Rotated log file: ${log_file} -> ${new_log_file}"

      # Keep a maximum of max_versions log files after rotation
      keep_max_versions "${log_type}" "${max_versions}"
    fi
  fi
}
