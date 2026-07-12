#!/bin/bash

LOCKFILE_SH_SOURCED=true

# Deliberately sources nothing beyond common.sh. The dispatcher (archiver.sh) sources this
# file before backgrounding main.sh, and pulling in logging drags the notification ->
# config-loader chain along, whose load mutates the dispatcher's environment (e.g.
# SERVICE_DIRECTORIES becomes a bash array, which does not export) — gutting the
# environment the backgrounded child inherits. Only log_lockfile_summary needs logging;
# its callers provide it (see the note on the function).
if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi

# Every function below operates on ${ARCHIVER_LOCKFILE}/${ARCHIVER_STOP_FLAG_FILE}, which
# default to the backup pipeline's files. The maintenance pipeline (and any caller that
# needs to inspect the OTHER pipeline) points these at its own files — set them in a
# subshell for a one-off query: (ARCHIVER_LOCKFILE="${MAINTENANCE_LOCKFILE}"; is_lock_valid).
ARCHIVER_LOCKFILE="${ARCHIVER_LOCKFILE:-${LOCKFILE}}"
ARCHIVER_STOP_FLAG_FILE="${ARCHIVER_STOP_FLAG_FILE:-${STOP_FLAG}}"

get_lock_pid() {
  head -n 1 "${ARCHIVER_LOCKFILE}" 2>/dev/null | cut -d' ' -f1
}

get_lock_context() {
  head -n 1 "${ARCHIVER_LOCKFILE}" 2>/dev/null | cut -d' ' -f2
}

get_lock_stage() {
  head -n 1 "${ARCHIVER_LOCKFILE}" 2>/dev/null | cut -d' ' -f3
}

update_lock_stage() {
  local context="${1}"
  local stage="${2}"
  local pid
  local temp_file

  pid=$(get_lock_pid)
  temp_file="${ARCHIVER_LOCKFILE}.tmp"

  echo "${pid} ${context} ${stage}" > "${temp_file}"
  tail -n +2 "${ARCHIVER_LOCKFILE}" 2>/dev/null >> "${temp_file}"
  mv "${temp_file}" "${ARCHIVER_LOCKFILE}"
}

get_current_state() {
  tail -n 1 "${ARCHIVER_LOCKFILE}" 2>/dev/null | cut -d' ' -f2
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
  echo "${timestamp} ${new_state}" >> "${ARCHIVER_LOCKFILE}"
}

get_backup_start_time() {
  sed -n '2p' "${ARCHIVER_LOCKFILE}" 2>/dev/null | cut -d' ' -f1
}

calculate_total_pause_time() {
  local lines
  local pause_start=""
  local total_pause=0

  lines=$(tail -n +2 "${ARCHIVER_LOCKFILE}" 2>/dev/null)

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

  [ -e "${ARCHIVER_LOCKFILE}" ] || return 1

  lock_pid=$(get_lock_pid)

  [ -n "${lock_pid}" ] && kill -0 "${lock_pid}" 2>/dev/null
}

acquire_lock() {
  local lock_pid
  local stale=false

  if [ -e "${ARCHIVER_LOCKFILE}" ]; then
    lock_pid=$(get_lock_pid)

    if [ -n "${lock_pid}" ] && kill -0 "${lock_pid}" 2>/dev/null; then
      return 1
    fi
    # Stale lock (dead or missing PID): clean it up and still take the lock below —
    # returning without acquiring would leave the whole run lockless (stop/status/pause
    # blind to it, and a concurrent backup would be admitted).
    rm -f "${ARCHIVER_LOCKFILE}"
    stale=true
  fi

  echo "$$ duplicacy pre-backup" > "${ARCHIVER_LOCKFILE}"
  record_state_change "running"

  if [ "${stale}" = true ]; then
    return 2
  fi
  return 0
}

# Requires log_message/format_timestamp/format_duration from logging.sh — the caller must
# have it loaded (main.sh does, via config-loader). Everything else in this file is pure.
# $1 labels the session in the summary ("Backup" default; maintenance passes "Maintenance").
log_lockfile_summary() {
  local label="${1:-Backup}"
  local start_time
  local end_time
  local end_state
  local total_time
  local total_pause
  local active_time
  local status_message

  start_time=$(get_backup_start_time)
  end_time=$(tail -n 1 "${ARCHIVER_LOCKFILE}" 2>/dev/null | cut -d' ' -f1)
  end_state=$(tail -n 1 "${ARCHIVER_LOCKFILE}" 2>/dev/null | cut -d' ' -f2)

  total_time=$((end_time - start_time))
  total_pause=$(calculate_total_pause_time)
  active_time=$((total_time - total_pause))

  if [ "${end_state}" = "completed" ]; then
    status_message="${label} completed successfully"
  elif [ "${end_state}" = "stopped" ]; then
    status_message="${label} stopped before completion"
  else
    status_message="${label} ended with state: ${end_state}"
  fi

  log_message "INFO" "${label} session summary: ${status_message}"
  log_message "INFO" "  Start time: $(format_timestamp "${start_time}")"
  log_message "INFO" "  End time: $(format_timestamp "${end_time}")"
  log_message "INFO" "  Total time: $(format_duration "${total_time}")"
  log_message "INFO" "  Pause time: $(format_duration "${total_pause}")"
  log_message "INFO" "  Active time: $(format_duration "${active_time}")"
}

release_lock() {
  rm -f "${ARCHIVER_LOCKFILE}"
  rm -f "${ARCHIVER_STOP_FLAG_FILE}"
}

is_stop_requested() {
  [ -f "${ARCHIVER_STOP_FLAG_FILE}" ]
}

request_stop() {
  touch "${ARCHIVER_STOP_FLAG_FILE}"
}
