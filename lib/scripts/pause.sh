#!/bin/bash

PAUSE_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOCKFILE_CORE}"
source_if_not_sourced "${NOTIFICATION_FEATURE}"

if ! is_lock_valid; then
  echo "No running backup found."
  exit 0
fi

LOCK_PID=$(get_lock_pid)

if is_paused; then
  echo "Backup is already paused. Use 'archiver resume' to resume."
  exit 0
fi

echo "Pausing backup..."
log_message "INFO" "Pausing backup process (PID: ${LOCK_PID})"

pkill -STOP -P "${LOCK_PID}"
record_state_change "paused"

ELAPSED_SECONDS=$(calculate_elapsed_time)
ELAPSED_TIME_READABLE=$(format_duration "${ELAPSED_SECONDS}")

echo "Backup paused. Active runtime: ${ELAPSED_TIME_READABLE}."
log_message "INFO" "Backup paused. Active runtime: ${ELAPSED_TIME_READABLE}"

notify "Backup Paused" "Paused after ${ELAPSED_TIME_READABLE} of active runtime."

exit 0
