#!/bin/bash

# Define the path to the Archiver main script
ARCHIVER_SCRIPT="$(readlink -f "${0}" 2>/dev/null)"
ARCHIVER_DIR="$(cd "$(dirname "${ARCHIVER_SCRIPT}")" && pwd)"
MAIN_SCRIPT="${ARCHIVER_DIR}/main.sh"

# Check if Archiver is already running
if pgrep -f "${MAIN_SCRIPT}" > /dev/null; then
  echo "Archiver is already running."
else
  # Start Archiver in the background using nohup and pass all arguments
  nohup "${MAIN_SCRIPT}" "$@" &>/dev/null &
  echo "Archiver started in the background."
fi

exit 0
