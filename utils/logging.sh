# Logs a message to the archiver's log file with a timestamp.
# Parameters:
#   1. Log Level: The severity level of the log message (e.g., INFO, WARNING, ERROR).
#   2. Message: The log message to be recorded.
# Output:
#   Writes the log message to the archiver's log file. No console output except for WARNING or ERROR.

LOG_DIR="${ARCHIVER_DIR}/logs" # Path to Archiver logs directory
LOG_PREFIXES=(
  "archiver"
  "duplicacy"
  "docker"
  "curl"
)

log_message() {
  local log_level
  local message
  local timestamp
  local log_prefix
  local service_name
  local target_log_file

  log_level="${1}"
  message="${2}"
  timestamp="$(date +'%Y-%m-%d %H:%M:%S')"
  log_prefix="${3:-archiver}" # Use ARCHIVER_LOG_FILE by default if no log file is specified
  target_log_file="${LOG_DIR}/${log_prefix}.log"
  # Use "archiver" if SERVICE is unset or empty
  local service_name="${SERVICE:-archiver}"

  echo "[${timestamp}] [${log_level}] [Service: ${service_name}] ${message}" >> "${target_log_file}" || \
    handle_error "Failed to log message for ${service_name} service to ${target_log_file}. Check if the log file is writable and disk space is available."

  # Print WARNING and ERROR messages to the terminal
  if [[ "${log_level}" == "WARNING" || "${log_level}" == "ERROR" ]]; then
    echo "[${timestamp}] [${log_level}] [Service: ${service_name}] ${message}"
  fi

  # Send ERROR messages as notification
  if [[ "${log_level}" == "ERROR" ]]; then
    notify "Archiver Error" "[${timestamp}] [${log_level}] [Service: ${service_name}] ${message}"
  fi
}

# Logs output from backup operations, capturing both stdout and stderr streams.
# Parameters:
#   1. Output Message: The output message from the backup operation to be logged.
#   2. Log File (optional): Specifies the log file to write to. Defaults to the main archiver log file if not provided.
# Output:
#   Writes the provided output message to the specified log file or the default archiver log file. No console output.
log_output() {
  local log_prefix
  local log_level

  log_prefix="${1}"
  log_level="${2:-INFO}" # Use INFO log level by default if no log level is specified

  while IFS= read -r line; do
    log_message "${log_level}" "${line}" "${log_prefix}"
  done
}

rotate_logs() {
  mkdir -p "${LOG_DIR}" || handle_error "Unable to create log directory '${LOG_DIR}'."

  for log_prefix in "${LOG_PREFIXES[@]}"; do
    local new_log_file

    # Generate a new log file name based on the log prefix and script variable: DATETIME
    new_log_file="${LOG_DIR}/${log_prefix}-${DATETIME}.log"

    # Create a new empty log file
    touch "${new_log_file}"  || \
      handle_error "Could not create log file '${new_log_file}'."
    log_message "INFO" "Created log file '${new_log_file}'."

    # Update or create the symlink to point to the new log file
    ln -sf "${new_log_file}" "${LOG_DIR}/${log_prefix}.log" || \
      handle_error "Could not update/create symlink for '${log_prefix}.log' to '${new_log_file}'."
    log_message "INFO" "Updated/created symlink for '${log_prefix}.log' to '${new_log_file}'."

    # Delete log files older than 7 days
    find "${LOG_DIR}" -name "${log_prefix}-*.log" -type f -mtime +7 -exec rm -f {} \; || \
      handle_error "Failed to delete old '${log_prefix}' log files."
    log_message "INFO" "Deleted '${log_prefix}' log files older than 7 days."
  done
}


# Function to convert seconds to human-readable format
function elapsed_time {
  local T=$1
  local D=$((T/60/60/24))
  local H=$((T/60/60%24))
  local M=$((T/60%60))
  local S=$((T%60))
  local result=""
  (( $D > 0 )) && result+="$D days "
  (( $H > 0 )) && result+="$H hours "
  (( $M > 0 )) && result+="$M minutes "
  (( $D > 0 || $H > 0 || $M > 0 )) && result+="and "
  result+="$S seconds"
  echo "$result"
}
