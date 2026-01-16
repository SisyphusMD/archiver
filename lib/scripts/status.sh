#!/bin/bash

source "/opt/archiver/lib/core/common.sh"
source "/opt/archiver/lib/core/lockfile.sh"

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
