#!/bin/bash

source "/opt/archiver/lib/core/common.sh"
source "/opt/archiver/lib/core/lockfile.sh"
source "/opt/archiver/lib/features/notification.sh"

# Check if backup is running
if ! is_lock_valid; then
  echo "No paused backup found."
  exit 0
fi

LOCK_PID=$(get_lock_pid)

# Check if actually paused
if ! is_paused; then
  echo "Backup is not paused. Nothing to resume."
  exit 0
fi

# Calculate pause duration
PAUSE_START=$(tail -n 1 "${LOCKFILE}" | cut -d' ' -f1)
PAUSE_DURATION=$(($(date +%s) - ${PAUSE_START}))
PAUSE_TIME_READABLE=$(format_duration "${PAUSE_DURATION}")

# Resume the backup
echo "Resuming backup..."
log_message "INFO" "Resuming backup process (PID: ${LOCK_PID})"

pkill -CONT -P "${LOCK_PID}"
record_state_change "running"

echo "Backup resumed. Was paused for: ${PAUSE_TIME_READABLE}."
log_message "INFO" "Backup resumed. Was paused for: ${PAUSE_TIME_READABLE}"

# Send notification
notify "Backup Resumed" "Backup resumed. Was paused for: ${PAUSE_TIME_READABLE}."

exit 0
