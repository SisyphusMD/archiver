#!/bin/bash

ERROR_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOGGING_CORE}"
ERROR_COUNT=0

handle_error() {
  local message="${1}"
  local code="${2:-1}"

  ((ERROR_COUNT++))

  # Prevent recursive calls with RECURSIVE_CALL guard
  if [[ -z "${RECURSIVE_CALL}" ]]; then
    export RECURSIVE_CALL=true
    log_message "ERROR" "${message}"
    unset RECURSIVE_CALL
  else
    local timestamp="$(date +'%Y-%m-%d %H:%M:%S')"
    echo "[${timestamp}] [ERROR] ${message}" >&2
  fi

  return "${code}"
}
