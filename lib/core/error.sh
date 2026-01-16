#!/bin/bash

source "/opt/archiver/lib/core/common.sh"

ERROR_COUNT=0

handle_error() {
  local message="${1}"
  local code="${2:-1}"
  local timestamp
  local service_name

  ((ERROR_COUNT++))

  timestamp="$(date +'%Y-%m-%d %H:%M:%S')"
  service_name="${SERVICE:-archiver}"

  # Write to log file if available
  if [ -n "${LOG_DIR}" ]; then
    echo "[${timestamp}] [ERROR] [Service: ${service_name}] ${message}" >> "${LOG_DIR}/archiver.log" 2>/dev/null
  fi

  # Always write to stderr
  echo "[${timestamp}] [ERROR] ${message}" >&2

  return "${code}"
}
