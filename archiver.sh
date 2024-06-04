#!/bin/bash

# Record the start time before calling main.sh
START_TIME="$(date +%s)"

# Check if the script is run with sudo
if [ "$(id -u)" -ne 0 ]; then
  # Escalate privileges if not sudo
  exec sudo "$0" "$@"
fi

# Function to print usage information
usage() {
  echo "Usage: ${0}"
  echo
  echo "Options:"
  echo "  logs    View the logs after starting Archiver"
  echo "  stop    Stop a running Archiver backup."
  echo "  help    Display this help message"
  exit 1
}

# Parse command-line arguments
run=true
logs=false
stop=false
main_script_args=("--start-time" "${START_TIME}")
while [[ $# -gt 0 ]]; do
  case "${1}" in
    logs)
      logs=true
      main_script_args+=("--no-view-logs-error")
      shift
      ;;
    stop)
      stop=true
      run=false
      shift
      ;;
    help)
      usage  # Call usage when --help is provided
      ;;
    # --pause) # Example for the future for passing specific arguments to the main script
    #   main_script_args+=("${1}")
    #   shift
    #   ;;
    *)
      echo "Unknown option: ${1}"
      usage  # Call usage for unknown options
      ;;
  esac
done

# Define the path to the Archiver script
ARCHIVER_SCRIPT="$(realpath "${0}")"
ARCHIVER_DIR="$(dirname "${ARCHIVER_SCRIPT}")"
# Define mod directory
MOD_DIR="${ARCHIVER_DIR}/lib/mod"
# Define paths to various scripts
MAIN_SCRIPT="${MOD_DIR}/main.sh"
LOGS_SCRIPT="${MOD_DIR}/logs.sh"
STOP_SCRIPT="${MOD_DIR}/stop.sh"

if [[ "${stop}" == "true" ]]; then
  "${STOP_SCRIPT}"
fi

if [[ "${run}" == "true" ]]; then
  # Start Archiver in a new session using setsid
  setsid nohup "${MAIN_SCRIPT}" "${main_script_args[@]}" &>/dev/null & # setsid + nohup was required to fix bug related to duplicacy exported env vars when user ran script with --view-logs, then closed that running log view
  echo "Archiver main script started in the background."

  # Optionally view logs
  if [ "${logs}" = true ]; then
    "${LOGS_SCRIPT}" --start-time "${START_TIME}"
  fi
fi

exit 0
