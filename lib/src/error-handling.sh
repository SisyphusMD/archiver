#!/bin/bash

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