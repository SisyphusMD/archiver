#!/bin/bash

ERROR_SH_SOURCED=true

# Source common.sh (must use regular source for the first file)
if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOGGING_CORE}"
ERROR_COUNT=0

handle_error() {
  local message="${1}"
  local code="${2:-1}"

  ((ERROR_COUNT++))

  # Use log_message to log errors (which triggers notifications)
  # Avoid recursive calls by checking RECURSIVE_CALL flag
  if [[ -z "${RECURSIVE_CALL}" ]]; then
    export RECURSIVE_CALL=true
    log_message "ERROR" "${message}"
    unset RECURSIVE_CALL
  else
    # Fallback: write directly if in recursive call to prevent infinite loop
    local timestamp="$(date +'%Y-%m-%d %H:%M:%S')"
    echo "[${timestamp}] [ERROR] ${message}" >&2
  fi

  return "${code}"
}
