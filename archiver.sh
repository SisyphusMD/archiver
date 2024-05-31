#!/bin/bash

# Record the start time before calling main.sh
STARTTIME="$(date +%s)"

# Function to print usage information
usage() {
  echo "Usage: $0 [--view-logs]"
  echo
  echo "Options:"
  echo "  --view-logs  View the logs after starting Archiver"
  echo "  --help      Display this help message"
  exit 1
}

# Parse command-line arguments
view_logs=false
while [[ $# -gt 0 ]]; do
  case "${1}" in
    --view-logs)
      view_logs=true
      shift
      ;;
    --help)
      usage
      ;;
    *)
      echo "Unknown option: ${1}"
      usage
      ;;
  esac
done

# Define the path to the Archiver scripts
ARCHIVER_SCRIPT="$(readlink -f "${0}" 2>/dev/null)"
ARCHIVER_DIR="$(cd "$(dirname "${ARCHIVER_SCRIPT}")" && pwd)"
MAIN_SCRIPT="${ARCHIVER_DIR}/main.sh"
VIEW_LOG_SCRIPT="${ARCHIVER_DIR}/view-logs.sh"

# Check if Archiver is already running
if pgrep -f "${MAIN_SCRIPT}" > /dev/null; then
  echo "Archiver is already running."
  if [ "${view_logs}" = true ]; then
    sudo "${VIEW_LOG_SCRIPT}"
  fi
else
  # Start Archiver in the background using nohup and pass all arguments
  nohup "${MAIN_SCRIPT}" "$@" &>/dev/null &
  echo "Archiver started in the background."
  if [ "${view_logs}" = true ]; then
    sudo "${VIEW_LOG_SCRIPT}" --start-time "${STARTTIME}"
  fi
fi

exit 0
