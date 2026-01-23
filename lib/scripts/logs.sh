#!/bin/bash

LOGS_SH_SOURCED=true

if [[ -z "${COMMON_SH_SOURCED}" ]]; then
  source "/opt/archiver/lib/core/common.sh"
fi
source_if_not_sourced "${LOCKFILE_CORE}"

tail_logs() {
  local log_file
  local last_inode

  log_file="${LOG_DIR}/archiver.log"
  last_inode=$(stat -c %i "${log_file}")

  tail -f "${LOGO_DIR}/logo.ascii" "${log_file}" &
  tail_pid=$!

  while sleep 0.1; do
    if [ ! -e "${LOCKFILE}" ]; then
      # Give tail a moment to catch up with final log lines
      sleep 1
      kill "${tail_pid}" 2>/dev/null
      echo ""
      echo "Backup completed. Exiting log viewer."
      exit 0
    fi

    current_inode=$(stat -c %i "${log_file}")
    if [[ "${current_inode}" != "${last_inode}" ]]; then
      echo "Log file has changed. Following the new log file..."
      kill "${tail_pid}"
      last_inode="${current_inode}"
      tail -f "${LOGO_DIR}/logo.ascii" "${log_file}" &
      tail_pid=$!
    fi
  done
}

wait_for_logs() {
  local start_time
  local file_time

  start_time=$(get_backup_start_time)
  [ -z "${start_time}" ] && start_time=0

  while [ ! -d "${LOG_DIR}" ]; do
    sleep 0.1
  done

  while true; do
    if [ -f "${LOG_DIR}/archiver.log" ]; then
      file_time="$(stat -c %Y "${LOG_DIR}/archiver.log")"
      if [ "${file_time}" -ge "${start_time}" ]; then
        break
      fi
    fi
    sleep 0.1
  done
}

retry=20
while [ ${retry} -gt 0 ]; do
  if [ -e "${LOCKFILE}" ]; then
    lock_pid=$(get_lock_pid)

    if [ -n "${lock_pid}" ] && kill -0 "${lock_pid}" 2>/dev/null; then
      tail_logs
      exit 0
    else
      # Lockfile exists but process not running; wait for logs to be created
      wait_for_logs
      tail_logs
      exit 0
    fi
  else
    sleep 0.1
    retry=$((retry - 1))
  fi
done

echo "Archiver is not running."
exit 1
