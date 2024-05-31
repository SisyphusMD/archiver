#!/bin/bash

# Exit if not run as root
if [ "$(id -u)" -ne 0 ]; then
 echo "This script must be run as root. Please use sudo or log in as the root user."
 exit 1
fi

LOCKFILE="/var/lock/archiver.lock"

# Function to clean up lock file
cleanup_lockfile() {
  if [ -e "${LOCKFILE}" ]; then
    rm -f "${LOCKFILE}"
    echo "Lock file ${LOCKFILE} removed."
  fi
}

# Check if the lock file exists and contains a valid PID
if [ -e "${LOCKFILE}" ]; then
  LOCK_PID=$(cat "${LOCKFILE}")
  if [ -n "${LOCK_PID}" ] && kill -0 "${LOCK_PID}" 2>/dev/null; then
    echo "Stopping Archiver process with PID ${LOCK_PID} and its child processes."
    
    # Terminate the process and its children
    pkill -TERM -P "${LOCK_PID}"
    kill "${LOCK_PID}"
    
    # Wait for the process to terminate
    wait "${LOCK_PID}" 2>/dev/null

    echo "Archiver process and its child processes stopped."
    cleanup_lockfile
    exit 0
  else
    echo "Stale lock file detected. No running Archiver process found with PID ${LOCK_PID}."
    # Clean up stale lock file
    cleanup_lockfile
    exit 1
  fi
else
  echo "No lock file found. Archiver is not running."
  exit 1
fi
