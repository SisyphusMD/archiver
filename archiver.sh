#!/bin/bash

# Record the start time before calling main.sh
STARTTIME=$(date +%s)

# Function to print usage information
usage() {
  echo "Usage: $0 [--viewlogs]"
  echo
  echo "Options:"
  echo "  --viewlogs  View the logs after starting Archiver"
  echo "  --help      Display this help message"
  exit 1
}

# Parse command-line arguments
view_logs=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --viewlogs)
      view_logs=true
      shift
      ;;
    --help)
      usage
      ;;
    *)
      echo "Unknown option: $1"
      usage
      ;;
  esac
done

# Define the path to the Archiver main script
ARCHIVER_SCRIPT="$(readlink -f "${0}" 2>/dev/null)"
ARCHIVER_DIR="$(cd "$(dirname "${ARCHIVER_SCRIPT}")" && pwd)"
MAIN_SCRIPT="${ARCHIVER_DIR}/main.sh"
VIEW_LOG_SCRIPT="${ARCHIVER_DIR}/view_logs.sh"

# Check if Archiver is already running
if pgrep -f "${MAIN_SCRIPT}" > /dev/null; then
  echo "Archiver is already running."
else
  # Start Archiver in the background using nohup and pass all arguments
  nohup "${MAIN_SCRIPT}" "$@" &>/dev/null &
  echo "Archiver started in the background."
fi

# Call the view_logs.sh script and pass the start time as an argument if --viewlogs is set
if [ "${view_logs}" = true ]; then
  sudo "${VIEW_LOG_SCRIPT}" --starttime "$STARTTIME"
fi

exit 0
