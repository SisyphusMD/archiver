#!/bin/bash

# Record the start time before calling main.sh
START_TIME="$(date +%s)"

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
  echo "  --view-logs  View the logs after starting Archiver"
  echo "  --help       Display this help message"
  exit 1
}

# Parse command-line arguments
view_logs=false
main_script_args=("--start-time" "${START_TIME}")
while [[ $# -gt 0 ]]; do
  case "${1}" in
    --view-logs)
      view_logs=true
      args+=("--no-view-logs-error")
      shift
      ;;
    --help)
      usage  # Call usage when --help is provided
      ;;
    # --pause) # Example for the future for passing specific arguments to the main script
    #   args+=("${1}")
    #   shift
    #   ;;
    *)
      echo "Unknown option: ${1}"
      usage  # Call usage for unknown options
      ;;
  esac
done

# Define the path to the Archiver scripts
ARCHIVER_SCRIPT="$(realpath "${0}")"
ARCHIVER_DIR="$(dirname "${ARCHIVER_SCRIPT}")"
MAIN_SCRIPT="${ARCHIVER_DIR}/main.sh"
VIEW_LOG_SCRIPT="${ARCHIVER_DIR}/view-logs.sh"

# Start Archiver in the background using nohup and pass appropriate arguments
nohup "${MAIN_SCRIPT}" "${main_script_args[@]}" &>/dev/null &
echo "Archiver main script started in the background."

# Optionally view logs
if [ "${view_logs}" = true ]; then
  "${VIEW_LOG_SCRIPT}" --start-time "${START_TIME}"
fi

exit 0
