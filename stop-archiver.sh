#!/bin/bash

escalate_privileges() {
  echo "This script must be run as root. Attempting to restart with sudo..."
  exec sudo "$0" "$@"
}

# Check if the script is run with sudo
if [ "$(id -u)" -ne 0 ]; then
  escalate_privileges "$@"
fi

# Define unique identifier for the main script (e.g., main script's full path)
SCRIPT_PATH="$(realpath "$0")"
ARCHIVER_DIR="$(dirname "${SCRIPT_PATH}")"
MAIN_SCRIPT_PATH="${ARCHIVER_DIR}/main.sh"
LOCKFILE="/var/lock/archiver-$(echo "${MAIN_SCRIPT_PATH}" | md5sum | cut -d' ' -f1).lock"

# Function to clean up lock file
cleanup_lockfile() {
  if [ -e "${LOCKFILE}" ]; then
    rm -f "${LOCKFILE}"
    echo "Lock file ${LOCKFILE} removed."
  fi
}

# Check if the lock file exists and contains a valid PID
if [ -e "${LOCKFILE}" ]; then
  LOCK_INFO="$(cat "${LOCKFILE}")"
  LOCK_PID="$(echo "${LOCK_INFO}" | cut -d' ' -f1)"
  LOCK_SCRIPT="$(echo "${LOCK_INFO}" | cut -d' ' -f2)"

  if [ -n "${LOCK_PID}" ] && [ "${LOCK_SCRIPT}" = "${MAIN_SCRIPT_PATH}" ] && kill -0 "${LOCK_PID}" 2>/dev/null; then
    echo "Stopping Archiver process with PID ${LOCK_PID} and its child processes."
    
    # Terminate the process and its children
    pkill -TERM -P "${LOCK_PID}"
    kill "${LOCK_PID}"
    
    # Wait for the process to terminate
    wait "${LOCK_PID}" 2>/dev/null

    echo "Archiver process and its child processes stopped."
    cleanup_lockfile
  else
    echo "Stale lock file detected. No running Archiver process found with PID ${LOCK_PID}."
    # Clean up stale lock file
    cleanup_lockfile
  fi
else
  echo "No lock file found."
fi

# Check for running instances using pgrep
pgrep_output=$(pgrep -f "${MAIN_SCRIPT_PATH}")
if [ -n "${pgrep_output}" ]; then
  echo "An instance of ${MAIN_SCRIPT_PATH} is still running, even with no LOCKFILE present. Stopping the process."

  # Kill the running instance(s) and their child processes
  while read -r pid; do
    # Terminate the process and its children
    pkill -TERM -P "${pid}"
    if kill -0 "${pid}" 2>/dev/null; then
      kill "${pid}"
      echo "Killed running instance of ${MAIN_SCRIPT_PATH} with PID: ${pid}"
    fi
  done <<< "${pgrep_output}"
else
  echo "No orphan instances found."
fi
