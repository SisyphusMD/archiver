#!/bin/bash

STOP_SH_SOURCED=true

# Source common.sh (must use regular source for the first file)
if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOCKFILE_CORE}"
source_if_not_sourced "${NOTIFICATION_FEATURE}"

# Send stopped notification
send_stopped_notification() {
  local elapsed_seconds
  local elapsed_time_readable

  elapsed_seconds=$(calculate_elapsed_time)
  elapsed_time_readable=$(format_duration "${elapsed_seconds}")

  echo "Backup stopped. Active runtime: ${elapsed_time_readable}."
  log_message "INFO" "Backup stopped. Active runtime: ${elapsed_time_readable}"
  notify "Backup Stopped" "Interrupted after ${elapsed_time_readable} of active runtime."
}

# Terminate process using appropriate signal based on pause state
terminate_process() {
  local pid="${1}"

  record_state_change "stopped"

  if is_paused; then
    # SIGTERM doesn't work on paused processes
    pkill -KILL -P "${pid}" 2>/dev/null || true
    kill -KILL "${pid}" 2>/dev/null || true
  else
    pkill -TERM -P "${pid}" 2>/dev/null || true
    kill -TERM "${pid}" 2>/dev/null || true
  fi
}

# Parse arguments
IMMEDIATE_MODE=false
[ "$1" = "--immediate" ] && IMMEDIATE_MODE=true

# Check if backup is running
if ! is_lock_valid; then
  echo "No running backup found."
  exit 0
fi

LOCK_PID=$(get_lock_pid)
LOCK_CONTEXT=$(get_lock_context)
LOCK_STAGE=$(get_lock_stage)

echo "Stopping backup (PID: ${LOCK_PID}, context: ${LOCK_CONTEXT}, stage: ${LOCK_STAGE})..."
log_message "INFO" "Stop requested (PID: ${LOCK_PID}, context: ${LOCK_CONTEXT}, stage: ${LOCK_STAGE})"

# Immediate mode or duplicacy stage: terminate immediately
if [ "$IMMEDIATE_MODE" = true ] || [[ "${LOCK_CONTEXT}" == "duplicacy" ]]; then
  terminate_process "${LOCK_PID}"
  send_stopped_notification
  exit 0
# Service stage: set flag, resume, and wait for cleanup
elif [[ "${LOCK_CONTEXT}" =~ ^service: ]]; then
  log_message "INFO" "Setting stop flag for service cleanup"
  request_stop
  "${RESUME_SCRIPT}"
  echo "Stop flag set. Service will complete cleanup and terminate."
  exit 0
fi
