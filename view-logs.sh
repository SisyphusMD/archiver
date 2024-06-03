#!/bin/bash

# Initialize variables
START_TIME=0

# Define unique identifier for the main script (e.g., main script's full path)
SCRIPT_PATH="$(realpath "$0")"
ARCHIVER_DIR="$(dirname "${SCRIPT_PATH}")"
MAIN_SCRIPT_PATH="${ARCHIVER_DIR}/main.sh"
LOCKFILE="/var/lock/archiver-$(echo "${MAIN_SCRIPT_PATH}" | md5sum | cut -d' ' -f1).lock"

# Define the log directory and log files
LOG_DIR="${ARCHIVER_DIR}/logs"
LOG_PREFIXES=(
    "archiver"
    "duplicacy"
    "docker"
    "curl"
)

# Check if the script is run with sudo
if [ "$(id -u)" -ne 0 ]; then
  echo "This script must be run as root. Please use sudo or log in as the root user."
  exit 1
fi

# Function to print usage information
usage() {
  echo "Usage: ${0}"
  echo
  echo "Options:"
  echo "  --start-time START_TIME  Specify the start time (optional, defaults to 0)"
  echo "  --help                   Display this help message"
  exit 1
}

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
  case "${1}" in
    --start-time)
      if [[ -n "${2}" && "${2}" != --* ]]; then
        START_TIME="${2}"
        shift 2
      else
        echo "Error: --start-time requires a value."
        usage
      fi
      ;;
    --help)
      usage  # Call usage when --help is provided
      ;;
    *)
      echo "Unknown option: ${1}"
      usage  # Call usage for unknown options
      ;;
  esac
done

tail_logs() {
  # Construct the tail command dynamically
  tail_cmd="tail -f ${ARCHIVER_DIR}/logos/logo.ascii"
  for log_prefix in "${LOG_PREFIXES[@]}"; do
    tail_cmd+=" ${LOG_DIR}/${log_prefix}.log"
  done

  # Execute the tail command
  eval "${tail_cmd}"
}

wait_for_logs() {
  # Wait for the log directory to be created
  while [ ! -d "${LOG_DIR}" ]; do
    echo "Waiting for log directory ${LOG_DIR} to be created..."
    sleep 0.1
  done

  # Wait for all specified log files to be present and updated after the specified start time
  for log_prefix in "${LOG_PREFIXES[@]}"; do
    while true; do
      if [ -f "${LOG_DIR}/${log_prefix}.log" ]; then
        file_time="$(stat -c %Y "${LOG_DIR}/${log_prefix}.log")"
        if [ "${file_time}" -ge "${START_TIME}" ]; then
          echo "${log_prefix}.log is present and has been updated."
          break
        else
          echo "Waiting for ${log_prefix}.log to be updated..."
        fi
      else
        echo "Waiting for ${log_prefix}.log to be created..."
      fi
      sleep 0.1
    done
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
    echo "Archiver is not running. Retrying..."
    sleep 0.1
    retry=$((retry - 1))
  fi
done

echo "Archiver is not running."
exit 1
