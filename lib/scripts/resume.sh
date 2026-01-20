#!/bin/bash

RESUME_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOCKFILE_CORE}"
source_if_not_sourced "${NOTIFICATION_FEATURE}"

if ! is_lock_valid; then
  echo "No paused backup found."
  exit 0
fi

LOCK_PID=$(get_lock_pid)

if ! is_paused; then
  echo "Backup is not paused. Nothing to resume."
  exit 0
fi

PAUSE_START=$(tail -n 1 "${LOCKFILE}" | cut -d' ' -f1)
PAUSE_DURATION=$(($(date +%s) - ${PAUSE_START}))
PAUSE_TIME_READABLE=$(format_duration "${PAUSE_DURATION}")

echo "Resuming backup..."
log_message "INFO" "Resuming backup process (PID: '${LOCK_PID}')."

pkill -CONT -P "${LOCK_PID}"
record_state_change "running"

echo "Backup resumed. Was paused for: ${PAUSE_TIME_READABLE}."
log_message "INFO" "Backup resumed. Was paused for: '${PAUSE_TIME_READABLE}'."

notify "Backup Resumed" "Continuing after ${PAUSE_TIME_READABLE} pause."

exit 0
