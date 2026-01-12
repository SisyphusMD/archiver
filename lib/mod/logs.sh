#!/bin/bash

# Initialize variables
START_TIME=0

# Determine archiver repo directory path by traversing up the directory tree until we find 'archiver.sh' or reach the root
LOGS_SCRIPT_PATH="$(realpath "$0")"
CURRENT_DIR="$(dirname "${LOGS_SCRIPT_PATH}")"
ARCHIVER_DIR=""
while [ "${CURRENT_DIR}" != "/" ]; do
  if [ -f "${CURRENT_DIR}/archiver.sh" ]; then
    ARCHIVER_DIR="${CURRENT_DIR}"
    break
  fi
  CURRENT_DIR="$(dirname "${CURRENT_DIR}")"
done

# Check if we found the file
if [ -z "${ARCHIVER_DIR}" ]; then
  echo "Error: archiver.sh not found in any parent directory."
  exit 1
fi

# Define lib, src, mod, log, logo directories
LOG_DIR="${ARCHIVER_DIR}/logs"
LIB_DIR="${ARCHIVER_DIR}/lib"
MOD_DIR="${LIB_DIR}/mod"
LOGO_DIR="${LIB_DIR}/logos"

# Define unique identifier for the main script (e.g., main script's full path)
MAIN_SCRIPT_PATH="${MOD_DIR}/main.sh"
LOCKFILE="/var/lock/archiver-$(echo "${MAIN_SCRIPT_PATH}" | md5sum | cut -d' ' -f1).lock"

# Check if the script is run with sudo
if [ "$(id -u)" -ne 0 ]; then
  # Escalate privileges if not sudo
  exec sudo "$0" "$@"
fi

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
  case "${1}" in
    time)
      if [[ -n "${2}" ]]; then
        START_TIME="${2}"
        shift 2
      else
        START_TIME=0
      fi
      ;;
  esac
done

tail_logs() {
  local log_file
  local last_inode

  log_file="${LOG_DIR}/archiver.log"
  last_inode=$(stat -c %i "${log_file}")

  tail -f "${LOGO_DIR}/logo.ascii" "${log_file}" &
  tail_pid=$!

  while sleep 0.1; do
    # Check if lockfile still exists (backup still running)
    if [ ! -e "${LOCKFILE}" ]; then
      # Lockfile removed - backup completed
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
  # Wait for the log directory to be created
  while [ ! -d "${LOG_DIR}" ]; do
    sleep 0.1
  done

  # Wait for the log file symlink to be present and updated after the specified start time
  while true; do
    if [ -f "${LOG_DIR}/archiver.log" ]; then
      file_time="$(stat -c %Y "${LOG_DIR}/archiver.log")"
      if [ "${file_time}" -ge "${START_TIME}" ]; then
        break
      fi
    fi
    sleep 0.1
  done
}

# Retry mechanism
retry=20
while [ ${retry} -gt 0 ]; do
  if [ -e "${LOCKFILE}" ]; then
    LOCK_INFO="$(cat "${LOCKFILE}")"
    LOCK_PID="$(echo "${LOCK_INFO}" | cut -d' ' -f1)"
    LOCK_SCRIPT="$(echo "${LOCK_INFO}" | cut -d' ' -f2)"

    if [ -n "${LOCK_PID}" ] && [ "${LOCK_SCRIPT}" = "${MAIN_SCRIPT_PATH}" ] && kill -0 "${LOCK_PID}" 2>/dev/null; then
      # Other instance of main script is running; view logs directly
      tail_logs
      exit 0
    else
      # Lock file exists but main script is not running; wait for logs to be created/updated
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
