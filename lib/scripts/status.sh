#!/bin/bash

STATUS_SH_SOURCED=true

# Source common.sh (must use regular source for the first file)
if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOCKFILE_CORE}"

# Check if backup is running
if ! is_lock_valid; then
  echo "Archiver backup is not running."
  exit 0
fi

LOCK_PID=$(get_lock_pid)

# Check if paused
if is_paused; then
  echo "An Archiver backup is paused (PID: ${LOCK_PID})."
else
  echo "An Archiver backup is running (PID: ${LOCK_PID})."
fi

exit 0
