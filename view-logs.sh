#!/bin/bash

# Function to print usage information
usage() {
  echo "Usage: $0 [--start-time STARTTIME]"
  echo
  echo "Options:"
  echo "  --start-time STARTTIME  Specify the start time (optional, defaults to 0)"
  echo "  --help                 Display this help message"
  exit 1
}

# Initialize variables
start_time=0

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --start-time)
      if [ -n "$2" ] && [[ "$2" != --* ]]; then
        start_time="$2"
        shift 2
      else
        echo "Error: --start-time requires a non-empty option argument."
        usage  # Call usage if --start-time is missing a value
      fi
      ;;
    --help)
      usage  # Call usage when --help is provided
      ;;
    *)
      echo "Unknown option: $1"
      usage  # Call usage for unknown options
      ;;
  esac
done

# Exit if not run as root
if [ "$(id -u)" -ne 0 ]; then
 echo "This script must be run as root. Please use sudo or log in as the root user."
 exit 1
fi

LOCKFILE="/var/lock/archiver.lock"

# Check if the lock file exists and contains a valid PID
if [ -e "${LOCKFILE}" ]; then
  # Determine the full path of the script
  VIEW_LOG_SCRIPT="$(readlink -f "${0}" 2>/dev/null)"
  # Determine the full path of the containing dir of the script
  ARCHIVER_DIR="$(cd "$(dirname "${VIEW_LOG_SCRIPT}")" && pwd)"
  # Define the log directory and log files
  LOG_DIR="${ARCHIVER_DIR}/logs"
  LOG_PREFIXES=(
      "archiver"
      "duplicacy"
      "docker"
      "curl"
  )

  # Check if the log directory exists, if not wait until it does
  while [ ! -d "${LOG_DIR}" ]; do
    echo "Waiting for log directory ${LOG_DIR} to be created..."
    sleep 1
  done

  # Wait until all specified log files are present and updated after the specified start time
  for log_prefix in "${LOG_PREFIXES[@]}"; do
    while true; do
      if [ -L "${LOG_DIR}/${log_prefix}.log" ]; then
        file_time=$(stat -c %Y "${LOG_DIR}/${log_prefix}.log")
        if [ "${file_time}" -ge "${start_time}" ]; then
          echo "${log_prefix}.log is present and has been updated."
          break
        else
          echo "Waiting for ${log_prefix}.log to be updated..."
        fi
      else
        echo "Waiting for ${log_prefix}.log to be created..."
      fi
      sleep 1
    done
  done

  # Follow the specified log files
  # Construct the tail command dynamically
  tail_cmd="tail -f ${ARCHIVER_DIR}/logos/logo.ascii"
  for log_prefix in "${LOG_PREFIXES[@]}"; do
    tail_cmd+=" ${LOG_DIR}/${log_prefix}.log"
  done

  # Execute the tail command
  eval "${tail_cmd}"
else
  echo "Archiver is not running."
  exit 1
fi