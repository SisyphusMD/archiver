#!/bin/bash

STATUS_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOCKFILE_CORE}"

if ! is_lock_valid; then
  echo "Archiver backup is not running."
  exit 0
fi

LOCK_PID=$(get_lock_pid)

if is_paused; then
  echo "An Archiver backup is paused (PID: ${LOCK_PID})."
else
  echo "An Archiver backup is running (PID: ${LOCK_PID})."
fi

exit 0
