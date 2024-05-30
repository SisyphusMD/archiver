#!/bin/bash

# Exit if not run as root
if [ "$(id -u)" -ne 0 ]; then
 echo "This script must be run as root. Please use sudo or log in as the root user."
 exit 1
fi

LOCKFILE="/var/lock/archiver.lock"

# Check if the lock file exists and contains a valid PID
if [ -e "${LOCKFILE}" ]; then
  LOCK_PID=$(cat "${LOCKFILE}")
  if [ -n "${LOCK_PID}" ] && kill -0 "${LOCK_PID}" 2>/dev/null; then
    echo "Stopping Archiver process with PID ${LOCK_PID}."
    kill "${LOCK_PID}"
    # Optionally, wait for the process to terminate
    wait "${LOCK_PID}" 2>/dev/null
    echo "Archiver process stopped."
    exit 0
  else
    echo "Stale lock file detected. No running Archiver process found with PID ${LOCK_PID}."
    # Clean up stale lock file
    rm -f "${LOCKFILE}"
    echo "Stale lock file removed."
    exit 1
  fi
else
  echo "No lock file found. Archiver is not running."
  exit 1
fi
