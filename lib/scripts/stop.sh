#!/bin/bash

STOP_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOCKFILE_CORE}"
source_if_not_sourced "${NOTIFICATION_FEATURE}"

terminate_process() {
  local pid="${1}"
  local start_time
  local end_time
  local elapsed_time
  local total_time_taken
  local message

  # Record stopped state
  record_state_change "stopped"

  # Calculate elapsed time and build message
  start_time=$(get_backup_start_time)
  end_time=$(date +%s)
  elapsed_time=$((end_time - start_time))
  total_time_taken=$(format_duration "${elapsed_time}")

  if [ "${ERROR_COUNT:-0}" -eq 0 ]; then
    message="Stopped after ${total_time_taken} with no errors."
  elif [ "${ERROR_COUNT}" -eq 1 ]; then
    message="Stopped after ${total_time_taken} with 1 error."
  else
    message="Stopped after ${total_time_taken} with ${ERROR_COUNT} errors."
  fi

  # Send notification before killing process
  echo "Backup stopped. ${message}"
  log_message "INFO" "Backup stopped. ${message}"
  notify "Backup Stopped" "${message}"

  # Kill the process
  if is_paused; then
    # SIGTERM doesn't work on paused processes
    pkill -KILL -P "${pid}" 2>/dev/null || true
    kill -KILL "${pid}" 2>/dev/null || true
  else
    pkill -TERM -P "${pid}" 2>/dev/null || true
    kill -TERM "${pid}" 2>/dev/null || true
  fi

  exit 0
}

IMMEDIATE_MODE=false
[ "$1" = "--immediate" ] && IMMEDIATE_MODE=true

if ! is_lock_valid; then
  echo "No running backup found."
  exit 0
fi

LOCK_PID=$(get_lock_pid)
LOCK_CONTEXT=$(get_lock_context)
LOCK_STAGE=$(get_lock_stage)

echo "Stopping backup (PID: ${LOCK_PID}, context: ${LOCK_CONTEXT}, stage: ${LOCK_STAGE})..."
log_message "INFO" "Stop requested (PID: ${LOCK_PID}, context: ${LOCK_CONTEXT}, stage: ${LOCK_STAGE})."

if [ "$IMMEDIATE_MODE" = true ] || [[ "${LOCK_CONTEXT}" == "duplicacy" ]]; then
  terminate_process "${LOCK_PID}"
elif [[ "${LOCK_CONTEXT}" =~ ^service: ]]; then
  log_message "INFO" "Setting stop flag for service cleanup."
  request_stop

  # Only resume if backup is actually paused
  if is_paused; then
    "${RESUME_SCRIPT}"
  fi

  echo "Stop flag set. Service will complete cleanup and terminate."
  exit 0
fi
