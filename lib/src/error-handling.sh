# Handles error scenarios by logging an error message and optionally exiting the script with a failure status.
# Parameters:
#   1. Error Message: The error message to log.
#   2. Exit Code (optional): The exit code to terminate the script with. If not provided, the script does not exit.
# Output:
#   Logs the error message to the standard error stream and exits the script if an exit code is provided.

# Initialize starting error count.
ERROR_COUNT=0

handle_error() {
  local message
  local code

  message="${1}" # Error message to display
  code="${2:-1}" # Exit status (default: 1)

  # Increment ERROR_COUNT by 1
  ((ERROR_COUNT++))

  if [[ -z "${RECURSIVE_CALL}" ]]; then
    export RECURSIVE_CALL=true
    log_message "ERROR" "${message}"
    unset RECURSIVE_CALL
  else
    # Direct fallback logging to stderr
    echo "${message}" >&2
  fi

  return "${code}" # Return with specified code, exiting the loop for the current service.
}