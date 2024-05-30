# Logs a message to the archiver's log file with a timestamp.
# Parameters:
#   1. Log Level: The severity level of the log message (e.g., INFO, WARNING, ERROR).
#   2. Message: The log message to be recorded.
# Output:
#   Writes the log message to the archiver's log file. No console output except for WARNING or ERROR.

# Define log file paths. Add new log files to the below array to ensure they're included in rotation.
LOG_DIR="${ARCHIVER_DIR}/logs" # Path to Archiver logs directory
ARCHIVER_LOG_FILE="${LOG_DIR}/archiver.log" # Log file for Archiver logs
DUPLICACY_LOG_FILE="${LOG_DIR}/duplicacy-output.log" # Log file for Duplicacy output
DOCKER_LOG_FILE="${LOG_DIR}/docker-output.log" # Log file for Docker output
CURL_LOG_FILE="${LOG_DIR}/curl-output.log" # Log file for Curl output
# Array of log file variables, make sure to add more log file variables to this array if adding to the list above
ALL_LOG_FILES=(
  "${ARCHIVER_LOG_FILE}"
  "${DUPLICACY_LOG_FILE}"
  "${DOCKER_LOG_FILE}"
  "${CURL_LOG_FILE}"
)

log_message() {
  local log_level
  local message
  local timestamp
  local target_log_file
  local service_name

  log_level="${1}"
  message="${2}"
  timestamp="$(date +'%Y-%m-%d %H:%M:%S')"
  target_log_file="${3:-${ARCHIVER_LOG_FILE}}" # Use ARCHIVER_LOG_FILE by default if no log file is specified
  # Use "Archiver" if SERVICE is unset or empty
  local service_name="${SERVICE:-Archiver}"

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
    mapfile -t log_files < <(find "${LOG_DIR}" -name "${log_prefix}*.log" -type f -printf "%T@ %p\n" | sort -nr | cut -d' ' -f2-)
    num_files="${#log_files[@]}"

    # Check if the number of log files exceeds the maximum allowed
    if [ "${num_files}" -gt "${max_versions}" ]; then
      # Iterate over log files starting from the (max_versions - 1) index
      # and remove any excess log files beyond the maximum allowed versions
      for (( i = max_versions; i < num_files; i++ )); do
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
  [ -d "${LOG_DIR}" ] || mkdir -p "${LOG_DIR}"

  # Rotate the log file if needed
  if [ -f "${log_file}" ]; then
    local creation_date

    # Extract the creation date of the log file
    creation_date="$(date -r "${log_file}" +'%Y-%m-%d')"

    # Check if the log file was created on a different date than today
    if [ "${creation_date}" != "${DATE}" ]; then
      local new_log_file

      # Generate a new log file name based on the log type and current date
      new_log_file="${LOG_DIR}/${log_type}-${creation_date}.log"

      # Rename the existing log file to the new name
      mv "${log_file}" "${new_log_file}" || \
        handle_error "Could not rename the log file from '${log_file}' to '${new_log_file}' for the ${SERVICE} service. Check file permissions and path validity."
      log_message "INFO" "Rotated log file: ${log_file} -> ${new_log_file}"

      # Keep a maximum of max_versions log files after rotation
      keep_max_versions "${log_type}" "${max_versions}"
    fi
  fi
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
