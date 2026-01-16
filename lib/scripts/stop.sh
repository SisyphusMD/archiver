#!/bin/bash

source "/opt/archiver/lib/core/common.sh"
source "/opt/archiver/lib/core/lockfile.sh"
source "/opt/archiver/lib/features/notification.sh"

# Send stopped notification
send_stopped_notification() {
  local elapsed_seconds
  local elapsed_time_readable

  elapsed_seconds=$(calculate_elapsed_time)
  elapsed_time_readable=$(format_duration "${elapsed_seconds}")

  echo "Backup stopped. Active runtime: ${elapsed_time_readable}."
  log_message "INFO" "Backup stopped. Active runtime: ${elapsed_time_readable}"
  notify "Backup Stopped" "Backup stopped. Active runtime: ${elapsed_time_readable}."
}

# Check if backup is running
if ! is_lock_valid; then
  echo "No running backup found."
  exit 0
fi

LOCK_PID=$(get_lock_pid)

# Stop the backup
echo "Stopping backup..."
log_message "INFO" "Stopping backup process (PID: ${LOCK_PID})"

# Check if paused before modifying lockfile
if is_paused; then
  # If paused, need to force kill since SIGTERM won't work on stopped processes
  echo "Backup is paused. Force terminating process tree."
  record_state_change "stopped"
  pkill -KILL -P "${LOCK_PID}"
  kill -KILL "${LOCK_PID}" 2>/dev/null
else
  # Normal graceful shutdown
  record_state_change "stopped"
  pkill -TERM -P "${LOCK_PID}"
  kill -TERM "${LOCK_PID}" 2>/dev/null
fi

send_stopped_notification

exit 0
