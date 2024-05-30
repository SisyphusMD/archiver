#!/bin/bash

# Function to print usage information
usage() {
  echo "Usage: $0 --starttime STARTTIME"
  echo
  echo "Options:"
  echo "  --starttime STARTTIME  Specify the start time"
  echo "  --help                 Display this help message"
  exit 1
}

# Initialize variables
start_time=""

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --starttime)
      if [ -n "$2" ] && [[ "$2" != --* ]]; then
        start_time="$2"
        shift 2
      else
        echo "Error: --starttime requires a non-empty option argument."
        usage  # Call usage if --starttime is missing a value
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

# Exit if start_time is not provided
if [ -z "$start_time" ]; then
  echo "Error: --starttime is required."
  usage  # Call usage if --starttime is not provided
fi

# Exit if not run as root
if [ "$(id -u)" -ne 0 ]; then
 echo "This script must be run as root. Please use sudo or log in as the root user."
 exit 1
fi

# Record the current time to compare file modification times
start_time=$(date +%s)

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

# Record the current time to compare file modification times
start_time=$(date +%s)

# Wait until all specified log files are present and updated after the script started
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
tail_cmd="tail -f"
for log_prefix in "${LOG_PREFIXES[@]}"; do
  tail_cmd+=" ${LOG_DIR}/${log_prefix}.log"
done

# Execute the tail command
eval "${tail_cmd}"
