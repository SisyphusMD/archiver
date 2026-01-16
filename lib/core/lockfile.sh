#!/bin/bash

# Source common.sh (must use regular source for the first file)
if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOGGING_CORE}"

LOCKFILE_SH_SOURCED=true
LOCKFILE="/var/lock/archiver-main.lock"

get_lock_pid() {
  head -n 1 "${LOCKFILE}" 2>/dev/null
}

get_current_state() {
  tail -n 1 "${LOCKFILE}" 2>/dev/null | cut -d' ' -f2
}

is_paused() {
  local current_state
  current_state=$(get_current_state)
  [ "${current_state}" = "paused" ]
}

record_state_change() {
  local new_state="${1}"
  local timestamp
  timestamp=$(date +%s)
  echo "${timestamp} ${new_state}" >> "${LOCKFILE}"
}

get_backup_start_time() {
  sed -n '2p' "${LOCKFILE}" 2>/dev/null | cut -d' ' -f1
}

calculate_total_pause_time() {
  local lines
  local pause_start=""
  local total_pause=0

  lines=$(tail -n +2 "${LOCKFILE}" 2>/dev/null)

  while IFS=' ' read -r timestamp state; do
    if [ "${state}" = "paused" ]; then
      pause_start="${timestamp}"
    elif [ "${state}" = "running" ] && [ -n "${pause_start}" ]; then
      local pause_duration=$((timestamp - pause_start))
      total_pause=$((total_pause + pause_duration))
      pause_start=""
    fi
  done <<< "${lines}"

  if [ -n "${pause_start}" ]; then
    local now
    now=$(date +%s)
    local current_pause=$((now - pause_start))
    total_pause=$((total_pause + current_pause))
  fi

  echo "${total_pause}"
}

calculate_elapsed_time() {
  local start_time
  local current_time
  local total_pause
  local elapsed

  start_time=$(get_backup_start_time)
  current_time=$(date +%s)
  total_pause=$(calculate_total_pause_time)

  elapsed=$((current_time - start_time - total_pause))
  echo "${elapsed}"
}

is_lock_valid() {
  local lock_pid

  [ -e "${LOCKFILE}" ] || return 1

  lock_pid=$(get_lock_pid)

  [ -n "${lock_pid}" ] && kill -0 "${lock_pid}" 2>/dev/null
}

acquire_lock() {
  local lock_pid

  if [ -e "${LOCKFILE}" ]; then
    lock_pid=$(get_lock_pid)

    if [ -n "${lock_pid}" ] && kill -0 "${lock_pid}" 2>/dev/null; then
      return 1
    else
      rm -f "${LOCKFILE}"
      return 2
    fi
  fi

  echo "$$" > "${LOCKFILE}"
  record_state_change "running"
  return 0
}

log_lockfile_summary() {
  local start_time
  local end_time
  local end_state
  local total_time
  local total_pause
  local active_time
  local status_message

  start_time=$(get_backup_start_time)
  end_time=$(tail -n 1 "${LOCKFILE}" 2>/dev/null | cut -d' ' -f1)
  end_state=$(tail -n 1 "${LOCKFILE}" 2>/dev/null | cut -d' ' -f2)

  total_time=$((end_time - start_time))
  total_pause=$(calculate_total_pause_time)
  active_time=$((total_time - total_pause))

  if [ "${end_state}" = "completed" ]; then
    status_message="Backup completed successfully"
  elif [ "${end_state}" = "stopped" ]; then
    status_message="Backup stopped before completion"
  else
    status_message="Backup ended with state: ${end_state}"
  fi

  log_message "INFO" "Backup session summary: ${status_message}"
  log_message "INFO" "  Start time: $(format_timestamp "${start_time}")"
  log_message "INFO" "  End time: $(format_timestamp "${end_time}")"
  log_message "INFO" "  Total time: $(format_duration "${total_time}")"
  log_message "INFO" "  Pause time: $(format_duration "${total_pause}")"
  log_message "INFO" "  Active time: $(format_duration "${active_time}")"
}

release_lock() {
  rm -f "${LOCKFILE}"
}
